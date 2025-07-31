package main

import (
	"net/http"
)

// @Summary      Health check
// @Description  Returns server status and system information
// @Tags         Debug
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /v1/healthcheck [get]
func (app *application) healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	env := envelope{
		"status": "available",
		"system_info": map[string]string{
			"environment": app.config.env,
			"version":     version,
		},
	}

	err := app.writeJSON(w, http.StatusOK, env, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
