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

// @Summary      Create a new review
// @Description  Creates a new review and stores it in the database
// @Tags         Reviews
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        review  body      data.ReviewInput  true  "Review JSON"
// @Success      201    {object}  data.ReviewInput
// @Failure      400    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /v1/reviews [post]
func (app *application) createReviewHandler(w http.ResponseWriter, r *http.Request) {
	var reviewInput data.ReviewInput

	err := app.readJSON(w, r, &reviewInput)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	v := validator.New()
	if data.ValidateReview(v, &reviewInput); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Reviews.Insert(&reviewInput)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/review/%d", reviewInput.ID))

	err = app.writeJSON(w, http.StatusCreated, envelope{"review": reviewInput}, headers)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Show a review
// @Description  Retrieves a single review by id
// @Tags         Reviews
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "The id of the review to retrieve"
// @Success      200  {object}  data.Review
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /v1/reviews/{id} [get]
func (app *application) showReviewHandler(w http.ResponseWriter, r *http.Request) {
	reviewID, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var userID *int64
	if user := app.contextGetUser(r); user != nil {
		userID = &user.ID
	}
	review, err := app.models.Reviews.Get(ctx, reviewID, userID)

	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"review": review}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      List reviews for a specific movie
// @Description  Returns a filtered list of reviews with optional sorting and pagination
// @Tags         Reviews
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id         path      int     true   "Movie ID"
// @Param        rating     query     string  false  "Sort by rating (presence of parameter enables sorting)"
// @Param        upvotes    query     string  false  "Sort by upvotes (presence of parameter enables sorting)"
// @Param        date       query     string  false  "Sort by date (presence of parameter enables sorting)"
// @Param        desc       query     string  false  "Sort in descending order (presence of parameter enables DESC)"
// @Param        page       query     int     false  "Page number (default is 1)"
// @Param        page_size  query     int     false  "Page size (default is 20)"
// @Success      200        {object}  map[string]interface{}  "reviews: []ReviewResponse, metadata: Metadata"
// @Failure      400        {object}  ErrorResponse
// @Failure      404        {object}  ErrorResponse
// @Failure      500        {object}  ErrorResponse
// @Router       /v1/movies/{id}/reviews [get]
func (app *application) listMovieReviewsHandler(w http.ResponseWriter, r *http.Request) {
	movieID, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	v := validator.New()
	reviewFilters := filters.ParseReviewFiltersFromQuery(r.URL.Query(), v)
	reviewFilters.MovieID = movieID

	reviewFilters.ValidateReviewFilters(v, reviewFilters)
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var currentUserID int64
	if user := app.contextGetUser(r); user != nil {
		currentUserID = user.ID
	}

	reviews, totalRecords, err := app.models.Reviews.GetFiltered(ctx, currentUserID, reviewFilters)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	metadata := calculateMetadata(totalRecords, reviewFilters.Page, reviewFilters.PageSize)

	err = app.writeJSON(w, http.StatusOK, envelope{"reviews": reviews, "metadata": metadata}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      List reviews by a specific user
// @Description  Returns a filtered list of reviews by a user with optional sorting and pagination
// @Tags         Reviews
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id         path      int     true   "User ID"
// @Param        rating     query     string  false  "Sort by rating"
// @Param        upvotes    query     string  false  "Sort by upvotes"
// @Param        date       query     string  false  "Sort by date"
// @Param        desc       query     string  false  "Sort in descending order"
// @Param        page       query     int     false  "Page number (default is 1)"
// @Param        page_size  query     int     false  "Page size (default is 20)"
// @Success      200        {object}  map[string]interface{}  "reviews: []ReviewResponse, metadata: Metadata"
// @Failure      400        {object}  ErrorResponse
// @Failure      404        {object}  ErrorResponse
// @Failure      500        {object}  ErrorResponse
// @Router       /v1/users/{id}/reviews [get]
func (app *application) listUserReviewsHandler(w http.ResponseWriter, r *http.Request) {
	userReviewsID, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	v := validator.New()
	reviewFilters := filters.ParseReviewFiltersFromQuery(r.URL.Query(), v)
	reviewFilters.UserID = userReviewsID

	reviewFilters.ValidateReviewFilters(v, reviewFilters)
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var currentUserID int64
	if user := app.contextGetUser(r); user != nil {
		currentUserID = user.ID
	}

	reviews, totalRecords, err := app.models.Reviews.GetFiltered(ctx, currentUserID, reviewFilters)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	metadata := calculateMetadata(totalRecords, reviewFilters.Page, reviewFilters.PageSize)

	err = app.writeJSON(w, http.StatusOK, envelope{"reviews": reviews, "metadata": metadata}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// @Summary      Vote for a review
// @Description  Casts a vote for a review
// @Tags         Reviews
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path      int  true  "Review ID"
// @Param        vote  body      data.VoteType  true  "Vote type (upvote or downvote)"
// @Success      200  {object}  envelope
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /v1/reviews/{id}/vote [post]
func (app *application) voteForReview(w http.ResponseWriter, r *http.Request) {
	reviewID, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	var voteType data.VoteType
	err = app.readJSON(w, r, &voteType)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if voteType > 1 || voteType < -1 {
		app.badRequestResponse(w, r, errors.New("invalid vote type"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var userID int64
	if user := app.contextGetUser(r); user != nil {
		userID = user.ID
	}

	err = app.models.Reviews.VoteReview(ctx, reviewID, userID, voteType)

	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "vote successful"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) listTopFiveMovieReviewsHandler(w http.ResponseWriter, r *http.Request) {
	v := validator.New()
	filters := filters.ParseReviewFiltersFromQuery(r.URL.Query(), v)

	filters.ValidateReviewFilters(v, filters)
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var userID int64
	if user := app.contextGetUser(r); user != nil {
		userID = user.ID
	}

	reviews, total_records, err := app.models.Reviews.GetFiltered(ctx, userID, filters)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	metadata := calculateMetadata(total_records, filters.Page, filters.PageSize)

	err = app.writeJSON(w, http.StatusOK, envelope{"reviews": reviews, "metadata": metadata}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
