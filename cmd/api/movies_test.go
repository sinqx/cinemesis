package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"cinemesis/internal/data"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateMovieHandler(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	app := &application{
		config: config{
			env:  "testing",
			cors: struct{ trustedOrigins []string }{trustedOrigins: []string{"http://localhost"}},
		},
		logger: logger,
		models: data.Models{
			Movies:  data.MovieModel{DB: db},
			Genres:  data.GenreModel{DB: db},
			Reviews: data.ReviewModel{DB: db},
		},
	}

	t.Run("Success", func(t *testing.T) {
		input := data.MovieInput{
			Title:      "Test Movie",
			Year:       2020,
			Runtime:    data.Runtime(120),
			GenreNames: []string{"Action", "Adventure"},
		}
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/v1/movies", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token") // Mock authentication
		w := httptest.NewRecorder()

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO genres (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id, name`)).
			WithArgs("Action").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "Action"))
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO genres (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id, name`)).
			WithArgs("Adventure").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(2, "Adventure"))
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO movies (title, year, runtime) VALUES ($1, $2, $3) RETURNING id, created_at, version`)).
			WithArgs(input.Title, input.Year, int32(input.Runtime)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "version"}).AddRow(1, time.Now(), 1))
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO movie_genres (movie_id, genre_id) VALUES ($1, $2)`)).
			WithArgs(int64(1), int64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO movie_genres (movie_id, genre_id) VALUES ($1, $2)`)).
			WithArgs(int64(1), int64(2)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		app.createMovieHandler(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		movie, ok := resp["movie"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(1), movie["id"])
		assert.Equal(t, input.Title, movie["title"])
		assert.Equal(t, float64(input.Year), movie["year"])
		assert.Equal(t, float64(input.Runtime), movie["runtime"])
		genres, ok := movie["genres"].([]any)
		assert.True(t, ok)
		assert.Equal(t, []interface{}{"Action", "Adventure"}, genres)
		assert.Equal(t, "/v1/movies/1", w.Header().Get("Location"))

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/movies", strings.NewReader("{invalid json}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		app.createMovieHandler(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Contains(t, resp["error"], "invalid character")
	})

	t.Run("Validation failure", func(t *testing.T) {
		input := data.MovieInput{
			Title:      "",
			Year:       2020,
			Runtime:    data.Runtime(120),
			GenreNames: []string{""},
		}
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/v1/movies", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		app.createMovieHandler(w, req)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Contains(t, resp["error"], map[string]string{
			"title":  "must be provided",
			"genres": "must not be empty",
		})
	})

	t.Run("Database error", func(t *testing.T) {
		input := data.MovieInput{
			Title:      "Test Movie",
			Year:       2020,
			Runtime:    data.Runtime(120),
			GenreNames: []string{"Action"},
		}
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/v1/movies", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO genres (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id, name`)).
			WithArgs("Action").
			WillReturnError(&pq.Error{Code: "23505"})

		app.createMovieHandler(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Contains(t, resp["error"], "failed to upsert genres")

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Missing Bearer Token", func(t *testing.T) {
		input := data.MovieInput{
			Title:      "Test Movie",
			Year:       2020,
			Runtime:    data.Runtime(120),
			GenreNames: []string{"Action"},
		}
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/v1/movies", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		// No Authorization header
		w := httptest.NewRecorder()

		// Mock middleware behavior (assuming it exists)
		app.createMovieHandler(w, req)

		// If middleware enforces BearerAuth, this will likely fail.
		// Adjust based on actual middleware response.
		assert.Equal(t, http.StatusUnauthorized, w.Code) // Adjust based on your auth middleware
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Contains(t, resp["error"], "unauthorized") // Adjust based on your middleware
	})
}

func TestShowMovieHandler(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	app := &application{
		config: config{
			env:  "testing",
			cors: struct{ trustedOrigins []string }{trustedOrigins: []string{"http://localhost"}},
		},
		logger: logger,
		models: data.Models{
			Movies:  data.MovieModel{DB: db},
			Genres:  data.GenreModel{DB: db},
			Reviews: data.ReviewModel{DB: db},
		},
	}

	t.Run("Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/movies/1", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, created_at, title, year, runtime, version FROM movies WHERE id = $1`)).
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "title", "year", "runtime", "version"}).
				AddRow(1, time.Now(), "Test Movie", 2020, 120, 1))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name FROM genres WHERE id IN (SELECT genre_id FROM movie_genres WHERE movie_id = $1)`)).
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "Action"))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT r.id, r.user_id, r.movie_id, LEFT(r.text, 300) AS text, r.rating, r.upvotes, r.downvotes, r.created_at, r.edited, u.name AS user_name, (r.upvotes - r.downvotes) AS total_votes FROM reviews r JOIN users u ON r.user_id = u.id WHERE r.movie_id = $1 AND r.upvotes > 0 ORDER BY r.upvotes DESC LIMIT $2`)).
			WithArgs(int64(1), 5).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "movie_id", "text", "rating", "upvotes", "downvotes", "created_at", "edited", "user_name", "total_votes"}).
				AddRow(1, 1, 1, "Great!", 8, 10, 2, time.Now(), false, "Test User", 8))

		app.showMovieHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		movie, ok := resp["movie"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(1), movie["id"])
		assert.Equal(t, "Test Movie", movie["title"])
		reviews, ok := resp["reviews"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, reviews, 1)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Invalid ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/movies/invalid", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		app.showMovieHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "the requested resource could not be found", resp["error"])
	})

	t.Run("Not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/movies/999", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, created_at, title, year, runtime, version FROM movies WHERE id = $1`)).
			WithArgs(int64(999)).
			WillReturnError(data.ErrRecordNotFound)

		app.showMovieHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "the requested resource could not be found", resp["error"])

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestListMoviesHandler(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	app := &application{
		config: config{
			env:  "testing",
			cors: struct{ trustedOrigins []string }{trustedOrigins: []string{"http://localhost"}},
		},
		logger: logger,
		models: data.Models{
			Movies:  data.MovieModel{DB: db},
			Genres:  data.GenreModel{DB: db},
			Reviews: data.ReviewModel{DB: db},
		},
	}

	t.Run("Success", func(t *testing.T) {
		queryParams := url.Values{}
		queryParams.Add("title", "Test")
		queryParams.Add("genres", "Action")
		queryParams.Add("page", "1")
		queryParams.Add("page_size", "20")
		queryParams.Add("sort", "title")
		req := httptest.NewRequest(http.MethodGet, "/v1/movies?"+queryParams.Encode(), nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM genres WHERE name = ANY($1)`)).
			WithArgs(pq.Array([]string{"Action"})).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) OVER(), id, created_at, title, year, runtime, version FROM movies WHERE (title ILIKE $1 OR $1 = '') AND ($2 = 0 OR $2 = ANY((SELECT array_agg(genre_id) FROM movie_genres WHERE movie_id = movies.id))) ORDER BY title ASC LIMIT $3 OFFSET $4`)).
			WithArgs("%Test%", int64(1), 20, 0).
			WillReturnRows(sqlmock.NewRows([]string{"total_records", "id", "created_at", "title", "year", "runtime", "version"}).
				AddRow(1, 1, time.Now(), "Test Movie", 2020, 120, 1))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, name FROM genres WHERE id IN (SELECT genre_id FROM movie_genres WHERE movie_id = ANY($1))`)).
			WithArgs(pq.Array([]int64{1})).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "Action"))

		app.listMoviesHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		movies, ok := resp["movies"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, movies, 1)
		movie := movies[0].(map[string]interface{})
		assert.Equal(t, "Test Movie", movie["title"])
		assert.Equal(t, []interface{}{"Action"}, movie["genres"])
		metadata, ok := resp["metadata"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(1), metadata["total_records"])

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Validation failure", func(t *testing.T) {
		queryParams := url.Values{}
		queryParams.Add("page", "0")
		req := httptest.NewRequest(http.MethodGet, "/v1/movies?"+queryParams.Encode(), nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		app.listMoviesHandler(w, req)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Contains(t, resp["error"], map[string]string{"page": "must be greater than zero"})
	})

	t.Run("Not found", func(t *testing.T) {
		queryParams := url.Values{}
		queryParams.Add("title", "Nonexistent")
		req := httptest.NewRequest(http.MethodGet, "/v1/movies?"+queryParams.Encode(), nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM genres WHERE name = ANY($1)`)).
			WithArgs(pq.Array([]string{})).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) OVER(), id, created_at, title, year, runtime, version FROM movies WHERE (title ILIKE $1 OR $1 = '') AND ($2 = 0 OR $2 = ANY((SELECT array_agg(genre_id) FROM movie_genres WHERE movie_id = movies.id))) ORDER BY id ASC LIMIT $3 OFFSET $4`)).
			WithArgs("%Nonexistent%", int64(0), 20, 0).
			WillReturnError(data.ErrRecordNotFound)

		app.listMoviesHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "the requested resource could not be found", resp["error"])

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestUpdateMovieHandler(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	app := &application{
		config: config{
			env:  "testing",
			cors: struct{ trustedOrigins []string }{trustedOrigins: []string{"http://localhost"}},
		},
		logger: logger,
		models: data.Models{
			Movies:  data.MovieModel{DB: db},
			Genres:  data.GenreModel{DB: db},
			Reviews: data.ReviewModel{DB: db},
		},
	}

	t.Run("Success", func(t *testing.T) {
		input := struct {
			Title      *string       `json:"title"`
			Year       *int32        `json:"year"`
			Runtime    *data.Runtime `json:"runtime"`
			GenreNames *[]string     `json:"genres"`
		}{
			Title:      ptr("Updated Movie"),
			Year:       ptr(int32(2021)),
			Runtime:    ptr(data.Runtime(130)),
			GenreNames: ptr([]string{"Drama"}),
		}
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPatch, "/v1/movies/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, created_at, title, year, runtime, version FROM movies WHERE id = $1`)).
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "title", "year", "runtime", "version"}).
				AddRow(1, time.Now(), "Test Movie", 2020, 120, 1))
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO genres (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id, name`)).
			WithArgs("Drama").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(3, "Drama"))
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE movies SET title = $1, year = $2, runtime = $3, version = version + 1 WHERE id = $4 AND version = $5 RETURNING version`)).
			WithArgs("Updated Movie", int32(2021), int32(130), int64(1), 1).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))
		mock.ExpectCommit()

		app.updateMovieHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		movie, ok := resp["movie"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, float64(1), movie["id"])
		assert.Equal(t, "Updated Movie", movie["title"])
		assert.Equal(t, float64(2021), movie["year"])
		assert.Equal(t, float64(130), movie["runtime"])
		assert.Equal(t, []interface{}{"Drama"}, movie["genres"])

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Invalid ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/v1/movies/invalid", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		app.updateMovieHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "the requested resource could not be found", resp["error"])
	})

	t.Run("Edit conflict", func(t *testing.T) {
		input := struct {
			Title *string `json:"title"`
		}{Title: ptr("Updated Movie")}
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPatch, "/v1/movies/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, created_at, title, year, runtime, version FROM movies WHERE id = $1`)).
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "title", "year", "runtime", "version"}).
				AddRow(1, time.Now(), "Test Movie", 2020, 120, 1))
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE movies SET title = $1, year = $2, runtime = $3, version = version + 1 WHERE id = $4 AND version = $5 RETURNING version`)).
			WithArgs("Updated Movie", int32(2020), int32(120), int64(1), 1).
			WillReturnError(data.ErrEditConflict)

		app.updateMovieHandler(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "unable to update the record due to an edit conflict, please try again", resp["error"])

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestDeleteMovieHandler(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	app := &application{
		config: config{
			env:  "testing",
			cors: struct{ trustedOrigins []string }{trustedOrigins: []string{"http://localhost"}},
		},
		logger: logger,
		models: data.Models{
			Movies:  data.MovieModel{DB: db},
			Genres:  data.GenreModel{DB: db},
			Reviews: data.ReviewModel{DB: db},
		},
	}

	t.Run("Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/movies/1", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM movies WHERE id = $1`)).
			WithArgs(int64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		app.deleteMovieHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "movie successfully deleted", resp["message"])

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/movies/999", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM movies WHERE id = $1`)).
			WithArgs(int64(999)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		app.deleteMovieHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "the requested resource could not be found", resp["error"])

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Invalid ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/movies/invalid", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		app.deleteMovieHandler(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "the requested resource could not be found", resp["error"])
	})
}

func TestRateLimitExceededResponse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	app := &application{
		config: config{
			env:  "testing",
			cors: struct{ trustedOrigins []string }{trustedOrigins: []string{"http://localhost"}},
		},
		logger: logger,
	}

	t.Run("Rate Limit Exceeded", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/movies", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		w := httptest.NewRecorder()

		app.rateLimitExceededResponse(w, req)

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		var resp envelope
		err := json.NewDecoder(w.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "rate limit exceeded", resp["error"])
	})
}

// Helper function to create a pointer to a value
func ptr[T any](v T) *T {
	return &v
}
