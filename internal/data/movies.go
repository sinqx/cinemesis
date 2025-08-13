package data

import (
	"cinemesis/internal/filters"
	"cinemesis/internal/validator"
	"context"
	"database/sql"
	"errors"
	"time"
)

type Movie struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
	Title     string    `json:"title"`
	Year      int32     `json:"year,omitzero"`
	Runtime   Runtime   `json:"runtime,omitzero"`
	Genres    []Genre   `json:"genres,omitempty"`
	Version   int32     `json:"version"`
}

type MovieInput struct {
	Title      string   `json:"title"`
	Year       int32    `json:"year"`
	Runtime    Runtime  `json:"runtime"`
	GenreNames []string `json:"genres,omitempty"`
}

type MovieModel struct {
	DB *sql.DB
}

func ValidateMovie(v *validator.Validator, movie *Movie) {
	v.Check(movie.Title != "", "title", "must be provided")
	v.Check(len(movie.Title) <= 500, "title", "must not be more than 500 bytes long")
	v.Check(movie.Year != 0, "year", "must be provided")
	v.Check(movie.Year >= 1888, "year", "must be greater than 1888")
	v.Check(movie.Year <= int32(time.Now().Year()), "year", "must not be in the future")
	v.Check(movie.Runtime != 0, "runtime", "must be provided")
	v.Check(movie.Runtime > 0, "runtime", "must be a positive integer")
}

func (m MovieModel) Insert(ctx context.Context, tx *sql.Tx, movie *Movie) error {
	query := `
        INSERT INTO movies (title, year, runtime)
        VALUES ($1, $2, $3)
        RETURNING id, created_at, updated_at, version`

	args := []any{movie.Title, movie.Year, movie.Runtime}

	return tx.QueryRowContext(ctx, query, args...).Scan(&movie.ID, &movie.CreatedAt, &movie.UpdatedAt, &movie.Version)
}

func (m MovieModel) Get(ctx context.Context, id int64) (*Movie, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
	SELECT id, created_at, updated_at, title, year, runtime, version
	FROM movies
	WHERE id = $1`

	var movie Movie
	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&movie.ID,
		&movie.CreatedAt,
		&movie.UpdatedAt,
		&movie.Title,
		&movie.Year,
		&movie.Runtime,
		&movie.Version,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	return &movie, nil
}

func (m MovieModel) GetFiltered(ctx context.Context, genreIDs []int64, mf filters.MovieFilters) ([]*Movie, int, error) {

	query, args := filters.NewMovieQueryBuilder().
		WithTitle(mf.Title).
		WithGenres(genreIDs).
		WithYearRange(mf.MinYear, mf.MaxYear).
		WithRuntimeRange(mf.MinRuntime, mf.MaxRuntime).
		Build(mf)

	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var movies []*Movie
	var totalRecords int

	for rows.Next() {
		var movie Movie

		err := rows.Scan(
			&totalRecords,
			&movie.ID,
			&movie.CreatedAt,
			&movie.UpdatedAt,
			&movie.Title,
			&movie.Year,
			&movie.Runtime,
			&movie.Version,
		)
		if err != nil {
			return nil, 0, ErrRecordNotFound
		}

		movie.Genres = []Genre{}
		movies = append(movies, &movie)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return movies, totalRecords, nil
}

func (m MovieModel) Update(ctx context.Context, tx *sql.Tx, movie *Movie) error {

	query := `
		UPDATE movies
		SET title = $1, year = $2, runtime = $3, updated_at = NOW(), version = version + 1
		WHERE id = $4 and version = $5
		RETURNING version`

	args := []any{movie.Title, movie.Year, movie.Runtime, movie.ID, movie.Version}

	err := tx.QueryRowContext(ctx, query, args...).Scan(&movie.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}

	return nil
}

func (m MovieModel) Delete(ctx context.Context, id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	query := `
        DELETE FROM movies
        WHERE id = $1`

	result, err := m.DB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err

	}

	if rowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}
