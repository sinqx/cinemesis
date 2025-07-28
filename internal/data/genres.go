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

func (g GenreModel) UpsertBatch(genreNames []string) ([]Genre, error) {
	placeholders := make([]string, len(genreNames))
	args := make([]any, len(genreNames))
	for i, name := range genreNames {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = name
	}

	selectQuery := fmt.Sprintf(`
		SELECT name, id FROM genres 
		WHERE name IN (%s)`,
		strings.Join(placeholders, ", "))

	existingGenres := make(map[string]int64)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := g.DB.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var id int64
		if err := rows.Scan(&name, &id); err != nil {
			return nil, err
		}
		existingGenres[name] = id
	}

	var newGenres []string
	for _, name := range genreNames {
		if _, exists := existingGenres[name]; !exists {
			newGenres = append(newGenres, name)
		}
	}

	if len(newGenres) > 0 {
		var newPlaceholders []string
		var newArgs []any

		for i, name := range newGenres {
			newPlaceholders = append(newPlaceholders, fmt.Sprintf("($%d)", i+1))
			newArgs = append(newArgs, name)
		}

		insertQuery := fmt.Sprintf(`
			INSERT INTO genres (name) 
			VALUES %s
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id, name`,
			strings.Join(newPlaceholders, ", "))

		insertRows, err := g.DB.QueryContext(ctx, insertQuery, newArgs...)
		if err != nil {
			return nil, err
		}
		defer insertRows.Close()

		for insertRows.Next() {
			var name string
			var id int64
			if err := insertRows.Scan(&id, &name); err != nil {
				return nil, err
			}
			existingGenres[name] = id
		}
	}

	result := make([]Genre, 0, len(genreNames))
	for _, name := range genreNames {
		if id, exists := existingGenres[name]; exists {
			result = append(result, Genre{
				ID:   id,
				Name: name,
			})
		}
	}

	return result, nil
}

func (g GenreModel) GetIDsByNames(ctx context.Context, genreNames []string) (*[]int64, error) {
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

	return &ids, nil
}

func (g GenreModel) AttachGenres(ctx context.Context, movies []*Movie) error {
	idMap := make(map[int64]*Movie, len(movies))
	movieIDs := make([]int64, len(movies))

	for i, movie := range movies {
		idMap[movie.ID] = movie
		movieIDs[i] = movie.ID
	}

	placeholders := make([]string, len(movieIDs))
	args := make([]interface{}, len(movieIDs))
	for i, id := range movieIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
        SELECT mg.movie_id, g.id, g.name
        FROM movies_genres mg
        JOIN genres g ON mg.genre_id = g.id
        WHERE mg.movie_id IN (%s)
        ORDER BY mg.movie_id, g.name`,
		strings.Join(placeholders, ", "),
	)

	rows, err := g.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var movieID int64
		var genre Genre
		err := rows.Scan(&movieID, &genre.ID, &genre.Name)
		if err != nil {
			return err
		}
		if movie, exists := idMap[movieID]; exists {
			movie.Genres = append(movie.Genres, genre)
		}
	}

	return rows.Err()
}

// func (g GenreModel) GetByStrings(genres []string) (*[]Genre, error) {
// 	if len(genreIDs) == 0 {
// 		return &[]Genre{}, nil
// 	}

// 	placeholders := make([]string, len(genreIDs))
// 	args := make([]any, len(genreIDs))
// 	for i, id := range genreIDs {
// 		placeholders[i] = fmt.Sprintf("$%d", i+1)
// 		args[i] = id
// 	}

// 	query := fmt.Sprintf(`
// 		SELECT id, name FROM genres g
// 		WHERE id IN (%s)
// 		ORDER BY name`,
// 		strings.Join(placeholders, ", "))

// 	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
// 	defer cancel()

// 	rows, err := g.DB.QueryContext(ctx, query, args...)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()

// 	var genres []Genre
// 	for rows.Next() {
// 		var genre Genre
// 		err := rows.Scan(&genre.ID, &genre.Name)
// 		if err != nil {
// 			return nil, err
// 		}
// 		genres = append(genres, genre)
// 	}

// 	return &genres, nil
// }

func (g GenreModel) GetByIDs(genreIDs []int64) (*[]Genre, error) {
	if len(genreIDs) == 0 {
		return &[]Genre{}, nil
	}

	placeholders := make([]string, len(genreIDs))
	args := make([]any, len(genreIDs))
	for i, id := range genreIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, name FROM genres g
		WHERE id IN (%s)
		ORDER BY name`,
		strings.Join(placeholders, ", "))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := g.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var genres []Genre
	for rows.Next() {
		var genre Genre
		err := rows.Scan(&genre.ID, &genre.Name)
		if err != nil {
			return nil, err
		}
		genres = append(genres, genre)
	}

	return &genres, nil
}

func (g GenreModel) Get(id int64) (*Genre, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
		SELECT id, name
		FROM genres
		WHERE id = $1`

	var genre Genre

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

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

func (g GenreModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	query := `
        DELETE FROM genres
        WHERE id = $1`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

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
