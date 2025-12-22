package server

import (
	"errors"
	"net/http"

	"github.com/theabdullahishola/mzl-payment-app/internals/middlewares"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
)

func (s *Server) GetTransactionHistoryV1(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value(middlewares.UserIDKey)
	userID, ok := val.(string)
	if !ok {
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	transactions, err := s.WalletService.GetTransactionHistory(r.Context(), userID)
	if err != nil {
		s.Logger.Error("failed to get history", "error", err)
		utils.ErrorJSON(w, r, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}

	utils.JSON(w,r, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "transaction history retrieved",
		"data":    transactions,
	})
}