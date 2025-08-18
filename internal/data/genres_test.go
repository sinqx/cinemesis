package data

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenreModel_Insert(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		genreName := "Action"
		expectedGenre := Genre{ID: 1, Name: genreName}

		mock.ExpectQuery(`INSERT INTO genres \(name\).*`).
			WithArgs(genreName).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, genreName))

		genre, err := model.Insert(genreName)
		require.NoError(t, err)
		assert.Equal(t, expectedGenre, genre)
	})

	t.Run("Duplicate", func(t *testing.T) {
		genreName := "Action"

		mock.ExpectQuery(`INSERT INTO genres \(name\).*`).
			WithArgs(genreName).
			WillReturnError(sql.ErrNoRows)

		_, err := model.Insert(genreName)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate genre name")
	})

	t.Run("DatabaseError", func(t *testing.T) {
		genreName := "Action"
		expectedErr := errors.New("database error")

		mock.ExpectQuery(`INSERT INTO genres \(name\).*`).
			WithArgs(genreName).
			WillReturnError(expectedErr)

		_, err := model.Insert(genreName)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestGenreModel_UpsertBatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		genreNames := []string{"Action", "Comedy"}
		expectedGenres := []Genre{
			{ID: 1, Name: "Action"},
			{ID: 2, Name: "Comedy"},
		}

		mock.ExpectExec(`INSERT INTO genres \(name\).*`).
			WithArgs(pq.Array(genreNames)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectQuery(`SELECT id, name FROM genres WHERE name = ANY\(\$1\)`).
			WithArgs(pq.Array(genreNames)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
				AddRow(1, "Action").
				AddRow(2, "Comedy"))

		genres, err := model.UpsertBatch(ctx, tx, genreNames)
		require.NoError(t, err)
		assert.Equal(t, expectedGenres, genres)
	})

	mock.ExpectCommit()
	tx.Commit()

}

func TestGenreModel_AttachGenresToMovie(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		movieID := int64(1)
		genres := []Genre{
			{ID: 1, Name: "Action"},
			{ID: 2, Name: "Comedy"},
		}

		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO movies_genres (movie_id, genre_id) VALUES ($1, $2), ($3, $4)`)).
			WithArgs(movieID, genres[0].ID, movieID, genres[1].ID).
			WillReturnResult(sqlmock.NewResult(0, 2))

		err := model.AttachGenresToMovie(ctx, tx, movieID, genres)
		require.NoError(t, err)
	})

	mock.ExpectCommit()
	tx.Commit()
}

func TestGenreModel_DetachGenresFromMovie(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)

	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		movieID := int64(1)

		mock.ExpectExec(`DELETE FROM movies_genres WHERE movie_id = \$1`).
			WithArgs(movieID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := model.DetachGenresFromMovie(ctx, tx, movieID)
		require.NoError(t, err)
	})

	mock.ExpectCommit()
	tx.Commit()
}

func TestGenreModel_LoadGenresForMovies(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		movies := []*Movie{
			{ID: 1},
			{ID: 2},
		}

		rows := sqlmock.NewRows([]string{"movie_id", "id", "name"}).
			AddRow(1, 1, "Action").
			AddRow(1, 2, "Comedy").
			AddRow(2, 3, "Drama")

		mock.ExpectQuery(`SELECT mg.movie_id, g.id, g.name.*`).
			WithArgs(pq.Array([]int64{1, 2})).
			WillReturnRows(rows)

		err := model.LoadGenresForMovies(ctx, movies)
		require.NoError(t, err)
		assert.Len(t, movies[0].Genres, 2)
		assert.Len(t, movies[1].Genres, 1)
	})

	t.Run("EmptyMovies", func(t *testing.T) {
		err := model.LoadGenresForMovies(ctx, []*Movie{})
		require.NoError(t, err)
	})

	t.Run("QueryError", func(t *testing.T) {
		movies := []*Movie{{ID: 1}}
		expectedErr := errors.New("query error")

		mock.ExpectQuery(`SELECT mg.movie_id, g.id, g.name.*`).
			WithArgs(pq.Array([]int64{1})).
			WillReturnError(expectedErr)

		err := model.LoadGenresForMovies(ctx, movies)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("ScanError", func(t *testing.T) {
		movies := []*Movie{{ID: 1}}

		rows := sqlmock.NewRows([]string{"movie_id", "id", "name"}).
			AddRow("invalid", 1, "Action") // Неправильный тип для movie_id

		mock.ExpectQuery(`SELECT mg.movie_id, g.id, g.name.*`).
			WithArgs(pq.Array([]int64{1})).
			WillReturnRows(rows)

		err := model.LoadGenresForMovies(ctx, movies)
		assert.Error(t, err)
	})
}

func TestGenreModel_Get(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		genreID := int64(1)
		expectedGenre := &Genre{ID: genreID, Name: "Action"}

		mock.ExpectQuery(`SELECT id, name FROM genres WHERE id = \$1`).
			WithArgs(genreID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
				AddRow(genreID, "Action"))

		genre, err := model.Get(ctx, genreID)
		require.NoError(t, err)
		assert.Equal(t, expectedGenre, genre)
	})

	t.Run("NotFound", func(t *testing.T) {
		genreID := int64(999)

		mock.ExpectQuery(`SELECT id, name FROM genres WHERE id = \$1`).
			WithArgs(genreID).
			WillReturnError(sql.ErrNoRows)

		_, err := model.Get(ctx, genreID)
		assert.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("InvalidID", func(t *testing.T) {
		_, err := model.Get(ctx, 0)
		assert.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		genreID := int64(1)
		expectedErr := errors.New("database error")

		mock.ExpectQuery(`SELECT id, name FROM genres WHERE id = \$1`).
			WithArgs(genreID).
			WillReturnError(expectedErr)

		_, err := model.Get(ctx, genreID)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestGenreModel_GetIDsByNames(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		genreNames := []string{"Action", "Comedy"}
		expectedIDs := []int64{1, 2}

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM genres WHERE name = ANY($1)`)).
			WithArgs(pq.Array(genreNames)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).
				AddRow(1).
				AddRow(2))

		ids, err := model.GetIDsByNames(ctx, genreNames)
		require.NoError(t, err)
		assert.Equal(t, expectedIDs, ids)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyNames", func(t *testing.T) {
		ids, err := model.GetIDsByNames(ctx, []string{})
		require.NoError(t, err)
		assert.Empty(t, ids)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		genreNames := []string{"Action"}
		expectedErr := errors.New("database error")

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM genres WHERE name = ANY($1)`)).
			WithArgs(pq.Array(genreNames)).
			WillReturnError(expectedErr)

		_, err := model.GetIDsByNames(ctx, genreNames)
		assert.ErrorIs(t, err, expectedErr)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGenreModel_GetGenresByMovieID(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		movieID := int64(1)
		expectedGenres := []Genre{
			{ID: 1, Name: "Action"},
			{ID: 2, Name: "Comedy"},
		}

		rows := sqlmock.NewRows([]string{"id", "name"}).
			AddRow(1, "Action").
			AddRow(2, "Comedy")

		mock.ExpectQuery(`SELECT g.id, g.name.*`).
			WithArgs(movieID).
			WillReturnRows(rows)

		genres, err := model.GetGenresByMovieID(ctx, movieID)
		require.NoError(t, err)
		assert.Equal(t, expectedGenres, genres)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		movieID := int64(1)
		expectedErr := errors.New("database error")

		mock.ExpectQuery(`SELECT g.id, g.name.*`).
			WithArgs(movieID).
			WillReturnError(expectedErr)

		_, err := model.GetGenresByMovieID(ctx, movieID)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestGenreModel_GetAll(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		expectedGenres := []Genre{
			{ID: 1, Name: "Action"},
			{ID: 2, Name: "Comedy"},
		}

		rows := sqlmock.NewRows([]string{"id", "name"}).
			AddRow(1, "Action").
			AddRow(2, "Comedy")

		mock.ExpectQuery(`SELECT id, name FROM genres ORDER BY name`).
			WillReturnRows(rows)

		genres, err := model.GetAll(ctx)
		require.NoError(t, err)
		assert.Equal(t, expectedGenres, genres)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		expectedErr := errors.New("database error")

		mock.ExpectQuery(`SELECT id, name FROM genres ORDER BY name`).
			WillReturnError(expectedErr)

		_, err := model.GetAll(ctx)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestGenreModel_Update(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		genreID := int64(1)
		newName := "New Action"

		mock.ExpectQuery(`UPDATE genres SET name = \$1 WHERE id = \$2 RETURNING id`).
			WithArgs(newName, genreID).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(genreID))

		err := model.Update(ctx, genreID, newName)
		require.NoError(t, err)
	})

	t.Run("NotFound", func(t *testing.T) {
		genreID := int64(999)
		newName := "New Action"

		mock.ExpectQuery(`UPDATE genres SET name = \$1 WHERE id = \$2 RETURNING id`).
			WithArgs(newName, genreID).
			WillReturnError(sql.ErrNoRows)

		err := model.Update(ctx, genreID, newName)
		assert.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		genreID := int64(1)
		newName := "New Action"
		expectedErr := errors.New("database error")

		mock.ExpectQuery(`UPDATE genres SET name = \$1 WHERE id = \$2 RETURNING id`).
			WithArgs(newName, genreID).
			WillReturnError(expectedErr)

		err := model.Update(ctx, genreID, newName)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestGenreModel_Delete(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	model := GenreModel{DB: db}

	t.Run("Success", func(t *testing.T) {
		genreID := int64(1)

		mock.ExpectExec(`DELETE FROM genres WHERE id = \$1`).
			WithArgs(genreID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := model.Delete(ctx, genreID)
		require.NoError(t, err)
	})

	t.Run("NotFound", func(t *testing.T) {
		genreID := int64(999)

		mock.ExpectExec(`DELETE FROM genres WHERE id = \$1`).
			WithArgs(genreID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := model.Delete(ctx, genreID)
		assert.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("InvalidID", func(t *testing.T) {
		err := model.Delete(ctx, 0)
		assert.ErrorIs(t, err, ErrRecordNotFound)
	})

	t.Run("DatabaseError", func(t *testing.T) {
		genreID := int64(1)
		expectedErr := errors.New("database error")

		mock.ExpectExec(`DELETE FROM genres WHERE id = \$1`).
			WithArgs(genreID).
			WillReturnError(expectedErr)

		err := model.Delete(ctx, genreID)
		assert.ErrorIs(t, err, expectedErr)
	})
}
