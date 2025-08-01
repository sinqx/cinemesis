package main

import (
	"cinemesis/internal/data"
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
// @Param        movie  body      data.Movie  true  "Movie JSON"
// @Success      201    {object}  data.Movie
// @Failure      400    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /v1/movies [post]
func (app *application) createMovieHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title      string       `json:"title"`
		Year       int32        `json:"year"`
		Runtime    data.Runtime `json:"runtime"`
		GenreNames []string     `json:"genres,omitempty"`
	}

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
		app.serverErrorResponse(w, r, err)
		return
	}
	defer tx.Rollback()

	genres, err := app.models.Genres.UpsertBatch(ctx, tx, genresNames)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.models.Movies.Insert(ctx, tx, movie)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.models.Genres.AttachGenresToMovie(ctx, tx, movie.ID, genres)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	if err := tx.Commit(); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/movies/%d", movie.ID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"movie": movie}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Get a movie by ID
// @Description  Returns the movie with the specified ID
// @Tags         Movies
// @Security     BearerAuth
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

	err = app.writeJSON(w, http.StatusOK, envelope{"movie": movie}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      List movies
// @Description  Returns a list of all movies (with optional filtering/pagination)
// @Tags         Movies
// @Security     BearerAuth
// @Produce      json
// @Param        title   query     string  false  "Filter by title"
// @Param        genres  query     string  false  "Filter by comma-separated genres"
// @Param        page    query     int     false  "Page number"
// @Param        limit   query     int     false  "Items per page"
// @Success      200     {array}   data.Movie
// @Failure      500     {object}  ErrorResponse
// @Router       /v1/movies [get]
func (app *application) listMoviesHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title  string
		Genres []string
		data.Filters
	}
	v := validator.New()
	qs := r.URL.Query()
	input.Title = app.readString(qs, "title", "")
	input.Genres = app.readCSV(qs, "genres", []string{})
	input.Filters.Page = app.readInt(qs, "page", 1, v)
	input.Filters.PageSize = app.readInt(qs, "page_size", 20, v)
	input.Filters.Sort = app.readString(qs, "sort", "id")
	input.Filters.SortSafelist = []string{"id", "title", "year", "runtime", "-id", "-title", "-year", "-runtime"}

	if data.ValidateFilters(v, input.Filters); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	genreIDs, err := app.models.Genres.GetIDsByNames(ctx, input.Genres)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	movies, metadata, err := app.models.Movies.GetFiltered(ctx, input.Title, genreIDs, input.Filters)
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
// @Param        movie  body      data.Movie true  "Updated movie data"
// @Success      200    {object}  data.Movie
// @Failure      400    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /v1/movies/{id} [put]
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
		Title   *string       `json:"title"`
		Year    *int32        `json:"year"`
		Runtime *data.Runtime `json:"runtime"`
		Genres  []data.Genre  `json:"genres"`
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

	err = app.models.Movies.Update(ctx, movie)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
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
