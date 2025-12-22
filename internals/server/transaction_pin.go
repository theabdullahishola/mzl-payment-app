package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/theabdullahishola/mzl-payment-app/internals/middlewares"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
)



type SetPinRequest struct {
    Pin string `json:"pin"`
}

func (s *Server) SetPinHandlerV1(w http.ResponseWriter, r *http.Request) {
    var req SetPinRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("invalid request"))
        return
    }

    val := r.Context().Value(middlewares.UserIDKey)
    userID, ok := val.(string)
    if !ok {
        utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("unauthorized"))
        return
    }

    err := s.AuthService.SetTransactionPin(r.Context(), userID, req.Pin)
    if err != nil {
        utils.ErrorJSON(w, r, http.StatusBadRequest, err)
        return
    }

    utils.JSON(w, r,http.StatusOK, map[string]string{
        "status": "success",
        "message": "transaction pin set successfully",
    })
}