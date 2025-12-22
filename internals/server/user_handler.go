package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/theabdullahishola/mzl-payment-app/internals/middlewares"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
	"github.com/theabdullahishola/mzl-payment-app/prisma/db"
)

type UserProfileResponse struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	HasPin bool   `json:"has_pin"`
}

func (s *Server) GetUserProfileHandler(w http.ResponseWriter, r *http.Request) {

	val := r.Context().Value(middlewares.UserIDKey)
	userID, ok := val.(string)
	if !ok {
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	user, err := s.AuthService.GetUserByID(r.Context(), userID)
	if err != nil {
		s.Logger.Error("failed to fetch profile", "error", err)
		utils.ErrorJSON(w, r, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}

	_, pinExists := user.TransactionPin()

	response := UserProfileResponse{
		ID:     user.ID,
		Email:  user.Email,
		Name:   user.Name,
		HasPin: pinExists,
	}

	utils.JSON(w, r, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "user profile retrieved",
		"data":    response,
	})
}

func (s *Server) LookupUserHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("query parameter 'q' is required"))
		return
	}

	user, err := s.WalletService.LookupUser(r.Context(), query)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			utils.ErrorJSON(w, r, http.StatusNotFound, errors.New("user not found"))
			return
		}
		s.Logger.Error("failed to lookup user", "error", err)
		fmt.Println(err)
		utils.ErrorJSON(w, r, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}

	utils.JSON(w, r, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "user found",
		"data": map[string]string{
			"name":           user.Name,
			"account_number": user.AccountNumber,
		},
	})
}
