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

type Genre struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func ValidateGenre(v *validator.Validator, genres *[]string) {
	v.Check(genres != nil, "genres", "must be provided")
	v.Check(len(*genres) >= 1, "genres", "must contain at least 1 genre")
	v.Check(len(*genres) <= 5, "genres", "must not contain more than 5 genres")
	v.Check(validator.Unique(*genres), "genres", "must not contain duplicate values")
	for _, genre := range *genres {
		v.Check(len(strings.TrimSpace(genre)) > 0, "genres", "must not contain empty values")
		v.Check(len(genre) <= 100, "genres", "must not be more than 100 bytes long")
	}
}

type GenreModel struct {
	DB *sql.DB
}

func (g GenreModel) Insert(genreName string) error {
	query := `
	INSERT INTO genres (name)
	VALUES ($1)
	RETURNING id, name`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return g.DB.QueryRowContext(ctx, query, genreName).Scan(genreName)
}

// UpsertBatch creates or updates a list of genres in the database.
// It returns the ID and name for each genre provided.
func (g GenreModel) UpsertBatch(ctx context.Context, tx *sql.Tx, genreNames []string) ([]Genre, error) {
	if len(genreNames) == 0 {
		return []Genre{}, nil
	}

	insertQuery := `
        INSERT INTO genres (name)
        SELECT UNNEST($1::text[])
        ON CONFLICT (name) DO NOTHING`

	_, err := tx.ExecContext(ctx, insertQuery, pq.Array(genreNames))
	if err != nil {
		return nil, err
	}

	selectQuery := `SELECT id, name FROM genres WHERE name = ANY($1)`
	rows, err := tx.QueryContext(ctx, selectQuery, pq.Array(genreNames))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var genres []Genre
	for rows.Next() {
		var genre Genre
		if err := rows.Scan(&genre.ID, &genre.Name); err != nil {
			return nil, err
		}
		genres = append(genres, genre)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return genres, nil
}

func (g GenreModel) AttachGenresToMovie(ctx context.Context, tx *sql.Tx, movieID int64, genres []Genre) error {
	if len(genres) == 0 {
		return nil
	}

	query := `INSERT INTO movies_genres (movie_id, genre_id) VALUES `
	args := []any{}
	values := []string{}

	for i, genre := range genres {
		queryPart := fmt.Sprintf("($%d, $%d)", i*2+1, i*2+2)
		values = append(values, queryPart)
		args = append(args, movieID, genre.ID)
	}

	query += strings.Join(values, ", ")

	return tx.QueryRowContext(ctx, query, args...).Scan(&movieID, &genres[0].ID)
}

func (g GenreModel) LoadGenresForMovies(ctx context.Context, movies []*Movie) error {
	if len(movies) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(movies))
	movieMap := make(map[int64]*Movie)
	for _, movie := range movies {
		ids = append(ids, movie.ID)
		movie.Genres = []Genre{}
		movieMap[movie.ID] = movie
	}

	query := `
		SELECT mg.movie_id, g.id, g.name
		FROM movies_genres mg
		INNER JOIN genres g ON g.id = mg.genre_id
		WHERE mg.movie_id = ANY($1)
		ORDER BY g.name
	`

	rows, err := g.DB.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var movieID int64
		var genre Genre

		if err := rows.Scan(&movieID, &genre.ID, &genre.Name); err != nil {
			return err
		}

		if movie, ok := movieMap[movieID]; ok {
			movie.Genres = append(movie.Genres, genre)
		}
	}

	return rows.Err()
}

func (g GenreModel) Get(ctx context.Context, id int64) (*Genre, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
		SELECT id, name
		FROM genres
		WHERE id = $1`

	var genre Genre

	err := g.DB.QueryRowContext(ctx, query, id).Scan(
		&genre.ID,
		&genre.Name,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &genre, nil
}

func (g GenreModel) GetIDsByNames(ctx context.Context, genreNames []string) ([]int64, error) {
	const query = "SELECT id FROM genres WHERE name = ANY($1)"
	rows, err := g.DB.QueryContext(ctx, query, pq.Array(genreNames))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}

func (g GenreModel) GetGenresByMovieID(ctx context.Context, movieID int64) ([]Genre, error) {
	query := `
		SELECT g.id, g.name
		FROM genres g
		INNER JOIN movies_genres mg ON mg.genre_id = g.id
		WHERE mg.movie_id = $1
		ORDER BY g.name`

	rows, err := g.DB.QueryContext(ctx, query, movieID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var genres []Genre
	for rows.Next() {
		var g Genre
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		genres = append(genres, g)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return genres, nil
}

func (g GenreModel) Delete(ctx context.Context, id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	query := `DELETE FROM genres WHERE id = $1`

	result, err := g.DB.ExecContext(ctx, query, id)
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
