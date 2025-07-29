package data

import (
	"cinemesis/internal/validator"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
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

type MovieModel struct {
	DB              *sql.DB
	GenreRepository GenreRepository
}

type GenreRepository interface {
	GetIDsByNames(ctx context.Context, names []string) ([]int64, error)
	GetGenresByMovieID(ctx context.Context, movieID int64) ([]Genre, error)
	AttachGenres(ctx context.Context, movies []*Movie) error
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

func (m MovieModel) Insert(movie *Movie) error {
	query := `
        INSERT INTO movies (title, year, runtime, genres) 
        VALUES ($1, $2, $3, $4)
        RETURNING id, created_at, updated_at, version`

	args := []any{movie.Title, movie.Year, movie.Runtime, pq.Array(movie.Genres)}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.DB.QueryRowContext(ctx, query, args...).Scan(&movie.ID, &movie.CreatedAt, &movie.Version)
}
func (m MovieModel) Get(id int64) (*Movie, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var movie Movie
	query := `
		SELECT id, created_at, updated_at, title, year, runtime, version
		FROM movies
		WHERE id = $1`

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

	genres, err := m.GenreRepository.GetGenresByMovieID(ctx, id)
	if err != nil {
		return nil, err
	}
	movie.Genres = genres

	return &movie, nil
}

func (m MovieModel) GetAll(title string, genreNames []string, filters Filters) ([]*Movie, Metadata, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var genreIDs []int64
	if len(genreNames) > 0 {
		ids, err := m.GenreRepository.GetIDsByNames(ctx, genreNames)
		if err != nil {
			return nil, Metadata{}, err
		}
		if len(ids) == 0 {
			return []*Movie{}, Metadata{}, nil
		}
		genreIDs = ids
	}

	movies, totalRecords, err := m.getMoviesFiltered(ctx, title, genreIDs, filters)
	if err != nil {
		return nil, Metadata{}, err
	}

	if len(movies) > 0 {
		err = m.GenreRepository.AttachGenres(ctx, movies)
		if err != nil {
			return nil, Metadata{}, err
		}
	}

	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return movies, metadata, nil
}

func (m MovieModel) getMoviesFiltered(ctx context.Context, title string, genreIDs []int64, filters Filters) ([]*Movie, int, error) {
	conditions := []string{"(to_tsvector('simple', m.title) @@ plainto_tsquery('simple', $1) OR $1 = '')"}
	args := []any{title}

	if len(genreIDs) > 0 {
		conditions = append(conditions, fmt.Sprintf(`m.id IN (
            SELECT movie_id
            FROM movies_genres
            WHERE genre_id = ANY($%d)
            GROUP BY movie_id
            HAVING COUNT(DISTINCT genre_id) = $%d
        )`, len(args)+1, len(args)+2))
		args = append(args, pq.Array(genreIDs))
		args = append(args, len(genreIDs))
	}

	query := fmt.Sprintf(`
        SELECT count(*) OVER(), m.id, m.created_at, m.updated_at, m.title, m.year, m.runtime, m.version
        FROM movies m
        WHERE %s
        ORDER BY %s %s, m.id ASC
        LIMIT $%d OFFSET $%d`,
		strings.Join(conditions, " AND "),
		filters.sortColumn(),
		filters.sortDirection(),
		len(args)+1,
		len(args)+2,
	)
	args = append(args, filters.limit(), filters.offset())

	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, 0, ErrRecordNotFound
		default:
			return nil, 0, err
		}
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

	if err = rows.Err(); err != nil {
		return nil, 0, err
	}

	return movies, totalRecords, nil
}

func (m MovieModel) Update(movie *Movie) error {

	query := `
		UPDATE movies
		SET title = $1, year = $2, runtime = $3, genres = $4, updated_at = NOW(), version = version + 1
		WHERE id = $5 and version = $6
		RETURNING version`

	args := []any{movie.Title, movie.Year, movie.Runtime, pq.Array(movie.Genres), movie.ID, movie.Version}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&movie.Version)
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

func (m MovieModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	query := `
        DELETE FROM movies
        WHERE id = $1`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

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
