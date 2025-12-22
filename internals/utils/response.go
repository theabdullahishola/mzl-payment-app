package utils

import (
	"net/http"

	"github.com/go-chi/render"
)

func JSON(w http.ResponseWriter, r *http.Request, status int, payload interface{}) {
	render.Status(r, status)
	render.JSON(w, r, payload)
}

func ErrorJSON(w http.ResponseWriter, r *http.Request, status int, err error) {
	JSON(w, r, status, map[string]string{
		"error":  err.Error(),
		"status": http.StatusText(status),
	})
}
