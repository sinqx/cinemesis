package data

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"cinemesis/internal/filters"
	"cinemesis/internal/validator"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMovie(t *testing.T) {
	tests := []struct {
		name    string
		movie   *Movie
		wantErr bool
		errors  map[string]string
	}{
		{
			name: "Valid movie",
			movie: &Movie{
				Title:   "Test Movie",
				Year:    2020,
				Runtime: 120,
			},
			wantErr: false,
		},
		{
			name: "Missing title",
			movie: &Movie{
				Title:   "",
				Year:    2020,
				Runtime: 120,
			},
			wantErr: true,
			errors: map[string]string{
				"title": "must be provided",
			},
		},
		{
			name: "Title too long",
			movie: &Movie{
				Title:   string(make([]byte, 501)),
				Year:    2020,
				Runtime: 120,
			},
			wantErr: true,
			errors: map[string]string{
				"title": "must not be more than 500 bytes long",
			},
		},
		{
			name: "Missing year",
			movie: &Movie{
				Title:   "Test Movie",
				Year:    0,
				Runtime: 120,
			},
			wantErr: true,
			errors: map[string]string{
				"year": "must be provided",
			},
		},
		{
			name: "Year too early",
			movie: &Movie{
				Title:   "Test Movie",
				Year:    1887,
				Runtime: 120,
			},
			wantErr: true,
			errors: map[string]string{
				"year": "must be greater than 1888",
			},
		},
		{
			name: "Year in future",
			movie: &Movie{
				Title:   "Test Movie",
				Year:    int32(time.Now().Year() + 1),
				Runtime: 120,
			},
			wantErr: true,
			errors: map[string]string{
				"year": "must not be in the future",
			},
		},
		{
			name: "Missing runtime",
			movie: &Movie{
				Title:   "Test Movie",
				Year:    2020,
				Runtime: 0,
			},
			wantErr: true,
			errors: map[string]string{
				"runtime": "must be provided",
			},
		},
		{
			name: "Negative runtime",
			movie: &Movie{
				Title:   "Test Movie",
				Year:    2020,
				Runtime: -120,
			},
			wantErr: true,
			errors: map[string]string{
				"runtime": "must be a positive integer",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validator.New()
			ValidateMovie(v, tt.movie)
			if tt.wantErr {
				assert.False(t, v.Valid())
				assert.Equal(t, tt.errors, v.Errors)
			} else {
				assert.True(t, v.Valid())
			}
		})
	}
}

func TestMovieModelInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := MovieModel{DB: db}

	movie := &Movie{
		Title:   "Test Movie",
		Year:    2020,
		Runtime: 120,
	}

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	mock.ExpectQuery(`
        INSERT INTO movies \(title, year, runtime\)
        VALUES \(\$1, \$2, \$3\)
        RETURNING id, created_at, updated_at, version`).
		WithArgs(movie.Title, movie.Year, movie.Runtime).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "version"}).
			AddRow(1, time.Now(), time.Now(), 1))

	err = m.Insert(context.Background(), tx, movie)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), movie.ID)
	assert.Equal(t, int32(1), movie.Version)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMovieModelGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := MovieModel{DB: db}

	t.Run("Successful get", func(t *testing.T) {
		mock.ExpectQuery(`
	SELECT id, created_at, updated_at, title, year, runtime, version
	FROM movies
	WHERE id = \$1`).
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title", "year", "runtime", "version"}).
				AddRow(1, time.Now(), time.Now(), "Test Movie", 2020, 120, 1))

		movie, err := m.Get(context.Background(), 1)
		assert.NoError(t, err)
		assert.Equal(t, "Test Movie", movie.Title)
		assert.Equal(t, int32(2020), movie.Year)
		assert.Equal(t, Runtime(120), movie.Runtime)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Not found", func(t *testing.T) {
		mock.ExpectQuery(`
	SELECT id, created_at, updated_at, title, year, runtime, version
	FROM movies
	WHERE id = \$1`).
			WithArgs(int64(999)).
			WillReturnError(sql.ErrNoRows)

		_, err := m.Get(context.Background(), 999)
		assert.Equal(t, ErrRecordNotFound, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestMovieModelGetFiltered(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := MovieModel{DB: db}

	fixedCreatedAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	fixedUpdatedAt := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

	t.Run("Successful with one row and empty filters", func(t *testing.T) {
		genreIDs := []int64{}
		mf := filters.NewMovieFilters()
		mf.Title = ""
		mf.Genres = []string{}
		mf.MinYear = 0
		mf.MaxYear = 0
		mf.MinRuntime = 0
		mf.MaxRuntime = 0

		query, args := filters.NewMovieQueryBuilder().
			WithTitle(mf.Title).
			WithGenres(genreIDs).
			WithYearRange(mf.MinYear, mf.MaxYear).
			WithRuntimeRange(mf.MinRuntime, mf.MaxRuntime).
			Build(mf)

		// Escape query to avoid regex issues
		escapedQuery := regexp.QuoteMeta(query)

		// Convert args to driver.Value
		driverArgs := make([]driver.Value, len(args))
		for i, arg := range args {
			switch v := arg.(type) {
			case string, int64, int32, int, float64, bool, []byte, time.Time, nil:
				driverArgs[i] = v
			default:
				t.Fatalf("unsupported type %T in args at index %d", arg, i)
			}
		}

		mock.ExpectQuery(escapedQuery).
			WithArgs(driverArgs...).
			WillReturnRows(sqlmock.NewRows([]string{"total_records", "id", "created_at", "updated_at", "title", "year", "runtime", "version"}).
				AddRow(1, 1, fixedCreatedAt, fixedUpdatedAt, "Test Movie", 2020, 120, 1))

		movies, total, err := m.GetFiltered(context.Background(), genreIDs, mf)

		assert.NoError(t, err)
		assert.Len(t, movies, 1)
		assert.Equal(t, 1, total)
		assert.Equal(t, "Test Movie", movies[0].Title)
		assert.Equal(t, int32(2020), movies[0].Year)
		assert.Equal(t, Runtime(120), movies[0].Runtime)
		assert.Empty(t, movies[0].Genres)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Successful with title filter", func(t *testing.T) {
		genreIDs := []int64{}
		mf := filters.NewMovieFilters()
		mf.Title = "Test"

		query, args := filters.NewMovieQueryBuilder().
			WithTitle(mf.Title).
			WithGenres(genreIDs).
			WithYearRange(mf.MinYear, mf.MaxYear).
			WithRuntimeRange(mf.MinRuntime, mf.MaxRuntime).
			Build(mf)

		escapedQuery := regexp.QuoteMeta(query)

		driverArgs := make([]driver.Value, len(args))
		for i, arg := range args {
			switch v := arg.(type) {
			case string, int64, int32, int, float64, bool, []byte, time.Time, nil:
				driverArgs[i] = v
			default:
				t.Fatalf("unsupported type %T in args at index %d", arg, i)
			}
		}

		mock.ExpectQuery(escapedQuery).
			WithArgs(driverArgs...).
			WillReturnRows(sqlmock.NewRows([]string{"total_records", "id", "created_at", "updated_at", "title", "year", "runtime", "version"}).
				AddRow(1, 1, fixedCreatedAt, fixedUpdatedAt, "Test Movie", 2020, 120, 1))

		movies, total, err := m.GetFiltered(context.Background(), genreIDs, mf)

		assert.NoError(t, err)
		assert.Len(t, movies, 1)
		assert.Equal(t, 1, total)
		assert.Equal(t, "Test Movie", movies[0].Title)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("No rows", func(t *testing.T) {
		genreIDs := []int64{}
		mf := filters.NewMovieFilters()

		query, args := filters.NewMovieQueryBuilder().
			WithTitle(mf.Title).
			WithGenres(genreIDs).
			WithYearRange(mf.MinYear, mf.MaxYear).
			WithRuntimeRange(mf.MinRuntime, mf.MaxRuntime).
			Build(mf)

		escapedQuery := regexp.QuoteMeta(query)

		driverArgs := make([]driver.Value, len(args))
		for i, arg := range args {
			switch v := arg.(type) {
			case string, int64, int32, int, float64, bool, []byte, time.Time, nil:
				driverArgs[i] = v
			default:
				t.Fatalf("unsupported type %T in args at index %d", arg, i)
			}
		}

		mock.ExpectQuery(escapedQuery).
			WithArgs(driverArgs...).
			WillReturnRows(sqlmock.NewRows([]string{"total_records", "id", "created_at", "updated_at", "title", "year", "runtime", "version"}))

		movies, total, err := m.GetFiltered(context.Background(), genreIDs, mf)

		assert.NoError(t, err)
		assert.Empty(t, movies)
		assert.Equal(t, 0, total)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Query error", func(t *testing.T) {
		genreIDs := []int64{}
		mf := filters.NewMovieFilters()

		query, args := filters.NewMovieQueryBuilder().
			WithTitle(mf.Title).
			WithGenres(genreIDs).
			WithYearRange(mf.MinYear, mf.MaxYear).
			WithRuntimeRange(mf.MinRuntime, mf.MaxRuntime).
			Build(mf)

		escapedQuery := regexp.QuoteMeta(query)

		driverArgs := make([]driver.Value, len(args))
		for i, arg := range args {
			switch v := arg.(type) {
			case string, int64, int32, int, float64, bool, []byte, time.Time, nil:
				driverArgs[i] = v
			default:
				t.Fatalf("unsupported type %T in args at index %d", arg, i)
			}
		}

		mock.ExpectQuery(escapedQuery).
			WithArgs(driverArgs...).
			WillReturnError(errors.New("database error"))

		movies, total, err := m.GetFiltered(context.Background(), genreIDs, mf)

		assert.Error(t, err)
		assert.Equal(t, "database error", err.Error())
		assert.Nil(t, movies)
		assert.Equal(t, 0, total)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Scan error", func(t *testing.T) {
		genreIDs := []int64{}
		mf := filters.NewMovieFilters()

		query, args := filters.NewMovieQueryBuilder().
			WithTitle(mf.Title).
			WithGenres(genreIDs).
			WithYearRange(mf.MinYear, mf.MaxYear).
			WithRuntimeRange(mf.MinRuntime, mf.MaxRuntime).
			Build(mf)

		escapedQuery := regexp.QuoteMeta(query)

		driverArgs := make([]driver.Value, len(args))
		for i, arg := range args {
			switch v := arg.(type) {
			case string, int64, int32, int, float64, bool, []byte, time.Time, nil:
				driverArgs[i] = v
			default:
				t.Fatalf("unsupported type %T in args at index %d", arg, i)
			}
		}

		mock.ExpectQuery(escapedQuery).
			WithArgs(driverArgs...).
			WillReturnRows(sqlmock.NewRows([]string{"total_records", "id", "created_at", "updated_at", "title", "year", "runtime", "version"}).
				AddRow(1, 1, fixedCreatedAt, fixedUpdatedAt, "Test Movie", 2020, 120, "invalid_version"))

		movies, total, err := m.GetFiltered(context.Background(), genreIDs, mf)

		assert.Error(t, err)
		assert.Equal(t, ErrRecordNotFound, err)
		assert.Nil(t, movies)
		assert.Equal(t, 0, total)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
func TestMovieModelUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := MovieModel{DB: db}

	movie := &Movie{
		ID:      1,
		Title:   "Updated Movie",
		Year:    2021,
		Runtime: 130,
		Version: 1,
	}

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	mock.ExpectQuery(`
		UPDATE movies
		SET title = \$1, year = \$2, runtime = \$3, updated_at = NOW\(\), version = version \+ 1
		WHERE id = \$4 and version = \$5
		RETURNING version`).
		WithArgs(movie.Title, movie.Year, movie.Runtime, movie.ID, movie.Version).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))

	err = m.Update(context.Background(), tx, movie)
	assert.NoError(t, err)
	assert.Equal(t, int32(2), movie.Version)

	assert.NoError(t, mock.ExpectationsWereMet())

	t.Run("Edit conflict", func(t *testing.T) {
		mock.ExpectBegin()
		tx, err := db.Begin()
		require.NoError(t, err)

		mock.ExpectQuery(`
		UPDATE movies
		SET title = \$1, year = \$2, runtime = \$3, updated_at = NOW\(\), version = version \+ 1
		WHERE id = \$4 and version = \$5
		RETURNING version`).
			WithArgs(movie.Title, movie.Year, movie.Runtime, movie.ID, movie.Version).
			WillReturnError(sql.ErrNoRows)

		err = m.Update(context.Background(), tx, movie)
		assert.Equal(t, ErrEditConflict, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestMovieModelDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	m := MovieModel{DB: db}

	t.Run("Successful delete", func(t *testing.T) {
		mock.ExpectExec(`
        DELETE FROM movies
        WHERE id = \$1`).
			WithArgs(int64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := m.Delete(context.Background(), 1)
		assert.NoError(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Not found", func(t *testing.T) {
		mock.ExpectExec(`
        DELETE FROM movies
        WHERE id = \$1`).
			WithArgs(int64(999)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := m.Delete(context.Background(), 999)
		assert.Equal(t, ErrRecordNotFound, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
