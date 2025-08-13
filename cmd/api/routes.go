package main

import (
	_ "cinemesis/docs"
	"expvar"
	"net/http"

	"github.com/julienschmidt/httprouter"
	httpSwagger "github.com/swaggo/http-swagger"
)

func (app *application) routes() http.Handler {
	router := httprouter.New()
	router.NotFound = http.HandlerFunc(app.notFoundResponse)
	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	router.HandlerFunc(http.MethodGet, "/docs/*filepath", httpSwagger.WrapHandler)
	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthcheckHandler)

	router.HandlerFunc(http.MethodPost, "/v1/genres/create", app.requirePermission("genres:write", app.createGenreHandler))
	router.HandlerFunc(http.MethodGet, "/v1/genres/movie/:id", app.requirePermission("genres:read", app.getMovieGenresHandler))
	router.HandlerFunc(http.MethodPut, "/v1/genres/update/movie/:id", app.requirePermission("genres:write", app.replaceMovieGenresHandler))
	router.HandlerFunc(http.MethodPatch, "/v1/genres/attach/:id", app.requirePermission("genres:write", app.addGenresToMovieHandler))
	router.HandlerFunc(http.MethodGet, "/v1/genres", app.requirePermission("genres:read", app.getAllGenresHandler))
	router.HandlerFunc(http.MethodGet, "/v1/genres/get/:id", app.requirePermission("genres:read", app.showGenreHandler))
	router.HandlerFunc(http.MethodPatch, "/v1/genres/update/:id", app.requirePermission("genres:write", app.updateGenreHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/genres/delete/:id", app.requirePermission("genres:write", app.deleteGenreHandler))

	router.HandlerFunc(http.MethodPost, "/v1/movies", app.requirePermission("movies:write", app.createMovieHandler))
	router.HandlerFunc(http.MethodGet, "/v1/movies", app.requirePermission("movies:read", app.listMoviesHandler))
	router.HandlerFunc(http.MethodGet, "/v1/movies/:id", app.requirePermission("movies:read", app.showMovieHandler))
	router.HandlerFunc(http.MethodPatch, "/v1/movies/:id", app.requirePermission("movies:write", app.updateMovieHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/movies/:id", app.requirePermission("movies:write", app.deleteMovieHandler))

	router.HandlerFunc(http.MethodPost, "/v1/reviews", app.requirePermission("reviews:write", app.createReviewHandler))
	router.HandlerFunc(http.MethodGet, "/v1/reviews/:id", app.requirePermission("reviews:read", app.showReviewHandler))
	router.HandlerFunc(http.MethodPost, "/v1/reviews/:id/vote", app.requirePermission("reviews:write", app.voteForReview))
	router.HandlerFunc(http.MethodGet, "/v1/movies/:id/reviews", app.requirePermission("reviews:read", app.listMovieReviewsHandler))

	router.HandlerFunc(http.MethodPost, "/v1/users", app.registerUserHandler)
	router.HandlerFunc(http.MethodPut, "/v1/users/activated", app.activateUserHandler)
	router.HandlerFunc(http.MethodPost, "/v1/tokens/update", app.updateUserPasswordHandler)
	router.HandlerFunc(http.MethodPost, "/v1/tokens/reset", app.createPasswordResetTokenHandler)
	router.HandlerFunc(http.MethodPost, "/v1/tokens/authentication", app.createAuthTokenHandler)
	router.HandlerFunc(http.MethodPost, "/v1/tokens/activation", app.createActivationTokenHandler)

	router.Handler(http.MethodGet, "/debug/vars", expvar.Handler())

	return app.metrics(app.recoverPanic(app.enableCORS(app.rateLimit(app.authenticate(router)))))
}
