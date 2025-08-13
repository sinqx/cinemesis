package main

import (
	"cinemesis/internal/data"
	"cinemesis/internal/filters"
	"cinemesis/internal/validator"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// @Summary      Create a new movie
// @Description  Creates a movie and stores it in the database
// @Tags         Movies
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        movie  body      data.MovieInput  true  "Movie JSON"
// @Success      201    {object}  data.Movie
// @Failure      400    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /v1/movies [post]
func (app *application) createMovieHandler(w http.ResponseWriter, r *http.Request) {
	var input data.MovieInput

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	genresNames := input.GenreNames
	v := validator.New()
	if data.ValidateGenre(v, &genresNames); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	movie := &data.Movie{
		Title:   input.Title,
		Year:    input.Year,
		Runtime: input.Runtime,
	}

	if data.ValidateMovie(v, movie); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	tx, err := app.models.Movies.DB.BeginTx(ctx, nil)
	if err != nil {
		app.serverErrorResponse(w, r, fmt.Errorf("failed to begin transaction: %w", err))
		return
	}
	defer tx.Rollback()

	genres, err := app.models.Genres.UpsertBatch(ctx, tx, genresNames)
	if err != nil {
		app.serverErrorResponse(w, r, fmt.Errorf("failed to upsert genres: %w", err))
		return
	}

	err = app.models.Movies.Insert(ctx, tx, movie)
	if err != nil {
		app.serverErrorResponse(w, r, fmt.Errorf("failed to create movie: %w", err))
		return
	}

	err = app.models.Genres.AttachGenresToMovie(ctx, tx, movie.ID, genres)
	if err != nil {
		app.serverErrorResponse(w, r, fmt.Errorf("failed to attach genres: %w", err))
		return
	}

	if err := tx.Commit(); err != nil {
		app.serverErrorResponse(w, r, fmt.Errorf("failed to commit transaction: %w", err))
		return
	}

	movie.Genres = genres

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/movies/%d", movie.ID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"movie": movie}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, fmt.Errorf("failed to write response: %w", err))
	}
}

// @Summary      Get a movie by ID
// @Description  Returns the movie with the specified ID
// @Tags         Movies
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "Movie ID"
// @Success      200  {object}  data.Movie
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /v1/movies/{id} [get]
func (app *application) showMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	movie, err := app.models.Movies.Get(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	genres, err := app.models.Genres.GetGenresByMovieID(ctx, movie.ID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	movie.Genres = genres

	reviews, err := app.models.Reviews.GetTopMovieReviews(ctx, id, 5)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"movie": movie, "reviews": reviews}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      List movies
// @Description  Returns a filtered list of movies with optional sorting and pagination
// @Tags         Movies
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        title      query     string   false  "Filter by movie title"
// @Param        genres     query     []string false  "Comma-separated list of genre names (e.g. genres=Action,Drama)"
// @Param        page       query     int      false  "Page number (default is 1)"
// @Param        page_size  query     int      false  "Page size (default is 20)"
// @Param        sort       query     string   false  "Sort by field (id, title, year, runtime), use '-' for descending (e.g. -title)"
// @Success      200        {object}  map[string]interface{}  "movies: []Movie, metadata: Metadata"
// @Failure      400        {object}  ErrorResponse
// @Failure      404        {object}  ErrorResponse
// @Failure      500        {object}  ErrorResponse
// @Router       /v1/movies [get]
func (app *application) listMoviesHandler(w http.ResponseWriter, r *http.Request) {
	v := validator.New()
	filters := filters.ParseMovieFiltersFromQuery(r.URL.Query(), v)

	filters.ValidateMovieFilters(v, filters)
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	genreIDs, err := app.models.Genres.GetIDsByNames(ctx, filters.Genres)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	movies, total_records, err := app.models.Movies.GetFiltered(ctx, genreIDs, filters)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.models.Genres.LoadGenresForMovies(ctx, movies)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	metadata := calculateMetadata(total_records, filters.Page, filters.PageSize)

	err = app.writeJSON(w, http.StatusOK, envelope{"movies": movies, "metadata": metadata}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Update a movie
// @Description  Updates the movie with the specified ID
// @Tags         Movies
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id     path      int        true  "Movie ID"
// @Param        movie  body      data.MovieInput true  "Updated movie data"
// @Success      200    {object}  data.Movie
// @Failure      400    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /v1/movies/{id} [patch]
func (app *application) updateMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	movie, err := app.models.Movies.Get(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	var input struct {
		Title      *string       `json:"title"`
		Year       *int32        `json:"year"`
		Runtime    *data.Runtime `json:"runtime"`
		GenreNames *[]string      `json:"genres,omitempty"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.Title != nil {
		movie.Title = *input.Title
	}
	if input.Year != nil {
		movie.Year = *input.Year
	}
	if input.Runtime != nil {
		movie.Runtime = *input.Runtime
	}

	v := validator.New()
	if data.ValidateMovie(v, movie); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	tx, err := app.models.Movies.DB.BeginTx(ctx, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	defer tx.Rollback()

	if input.GenreNames != nil {
		genresNames := *input.GenreNames
		v := validator.New()
		if data.ValidateGenre(v, &genresNames); !v.Valid() {
			app.failedValidationResponse(w, r, v.Errors)
			return
		}

		genres, err := app.models.Genres.UpsertBatch(ctx, tx, genresNames)
		if err != nil {
			app.serverErrorResponse(w, r, err)
			return
		}

		movie.Genres = genres
	}

	err = app.models.Movies.Update(ctx, tx, movie)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if err := tx.Commit(); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"movie": movie}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Delete a movie
// @Description  Deletes the movie with the specified ID
// @Tags         Movies
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "Movie ID"
// @Success      200  {object}  map[string]string
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /v1/movies/{id} [delete]
func (app *application) deleteMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = app.models.Movies.Delete(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}
	err = app.writeJSON(w, http.StatusOK, envelope{"message": "movie successfully deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
