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

// @Summary      Create a new genre
// @Description  Creates a genre and stores it in the database
// @Tags         Genres
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        genre  body      string  true  "Genre JSON"
// @Success      201    {object}  data.Genre
// @Failure      400    {object}  ErrorResponse
// @Failure      409    {object}  ErrorResponse "Genre with this name already exists"
// @Failure      500    {object}  ErrorResponse
// @Router       /v1/genres/create [post]
func (app *application) createGenreHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.Name == "" {
		app.failedValidationResponse(w, r, map[string]string{"name": "Name is required"})
		return
	}

	err = app.models.Genres.Insert(input.Name)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	createdGenre := &data.Genre{
		Name: input.Name,
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/genres/create/%d", createdGenre.ID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"genre": createdGenre}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Get genres for a movie
// @Description  Returns a list of genres associated with the movie with the specified ID
// @Tags         Genres
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "Movie ID"
// @Success      200  {object}  []data.Genre
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /v1/genres/movie/{id} [get]
func (app *application) getMovieGenresHandler(w http.ResponseWriter, r *http.Request) {
	movieID, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	genres, err := app.models.Genres.GetGenresByMovieID(ctx, movieID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/genres/movie/%d", movieID))

	err = app.writeJSON(w, http.StatusOK, envelope{"genres": genres}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Attach genres to a movie
// @Description  Attaches the specified genres to the movie with the specified ID
// @Tags         Genres
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id     path      int        true  "Movie ID"
// @Param        genres body      []string   true  "Genres to attach"
// @Success      200    {object}  []data.Genre
// @Failure      400    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /v1/genres/attach/{id} [patch]
func (app *application) addGenresToMovieHandler(w http.ResponseWriter, r *http.Request) {
	movieID, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	var input struct {
		Genres []string `json:"genres"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	v := validator.New()
	if data.ValidateGenre(v, &input.Genres); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	_, err = app.models.Movies.Get(ctx, movieID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	tx, err := app.models.Movies.DB.BeginTx(ctx, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	defer tx.Rollback()

	genres, err := app.models.Genres.UpsertBatch(ctx, tx, input.Genres)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.models.Genres.AttachGenresToMovie(ctx, tx, movieID, genres)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	if err := tx.Commit(); err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/genres/attach/%d", movieID))

	err = app.writeJSON(w, http.StatusAccepted, envelope{"genres": genres}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Replace a movie's genres
// @Description  Replaces the movie with the specified ID's genres with the ones provided in the request body.
// @Tags         Movies
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id     path      int        true  "Movie ID"
// @Param        genres body      []string   true  "The new genres to associate with the movie"
// @Success      202    {object}  data.Movie
// @Failure      400    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /v1/genres/movie/{id}/ [put]
func (app *application) replaceMovieGenresHandler(w http.ResponseWriter, r *http.Request) {
	movieID, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	var input struct {
		Genres []string `json:"genres"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	v := validator.New()
	if data.ValidateGenre(v, &input.Genres); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	movie, err := app.models.Movies.Get(ctx, movieID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	tx, err := app.models.Movies.DB.BeginTx(ctx, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	defer tx.Rollback()

	err = app.models.Genres.DetachGenresFromMovie(ctx, tx, movieID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	if len(input.Genres) > 0 {
		genres, err := app.models.Genres.UpsertBatch(ctx, tx, input.Genres)
		if err != nil {
			app.serverErrorResponse(w, r, err)
			return
		}

		err = app.models.Genres.AttachGenresToMovie(ctx, tx, movieID, genres)
		if err != nil {
			app.serverErrorResponse(w, r, err)
			return
		}
	}

	err = tx.Commit()
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/genres/detach/movie/%d", movieID))

	err = app.writeJSON(w, http.StatusAccepted, envelope{"movie": movie}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Get all genres
// @Description  Returns a list of all genres stored in the database.
// @Tags         Genres
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Success      200  {object}  []data.Genre
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /v1/genres [get]
func (app *application) getAllGenresHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	genres, err := app.models.Genres.GetAll(ctx)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"genres": genres}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Get a genre by ID
// @Description  Returns the genre with the specified ID
// @Tags         Genres
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path      int     true  "Genre ID"
// @Success      200  {object}  data.Genre
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /v1/genres/{id} [get]
func (app *application) showGenreHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	genre, err := app.models.Genres.Get(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"genre": genre}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Update a genre
// @Description  Updates the genre with the specified ID
// @Tags         Genres
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path      int     true  "Genre ID"
// @Param        genre body      data.Genre true  "Updated genre JSON"
// @Success      200  {object}  data.Genre
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /v1/genres/{id} [patch]
func (app *application) updateGenreHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	var input struct {
		Name *string `json:"name"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.Name == nil {
		app.badRequestResponse(w, r, errors.New("name is required"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	err = app.models.Genres.Update(ctx, id, *input.Name)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		case errors.Is(err, data.ErrEditConflict):
			app.editConflictResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	updatedGenre, err := app.models.Genres.Get(ctx, id)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"genre": updatedGenre}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Delete a genre
// @Description  Deletes the genre with the specified ID
// @Tags         Genres
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path      int     true  "Genre ID"
// @Success      204  {object}  nil
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /v1/genres/{id} [delete]
func (app *application) deleteGenreHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	err = app.models.Genres.Delete(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
