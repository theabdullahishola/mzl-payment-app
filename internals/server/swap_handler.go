package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/theabdullahishola/mzl-payment-app/internals/middlewares"
	"github.com/theabdullahishola/mzl-payment-app/internals/service"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
)



type SwapRequest struct {
	FromCurrency string  `json:"from_currency"`
	ToCurrency   string  `json:"to_currency"`
	Amount       float64 `json:"amount"`
	Pin          string  `json:"pin"`
}

func (s *Server) SwapHandlerV1(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("missing Idempotency-Key header"))
		return
	}

	logger := s.Logger.With("idemp_key", idempotencyKey)

	var req SwapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}

	if req.Amount <= 0 {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("amount must be greater than zero"))
		return
	}

	val := r.Context().Value(middlewares.UserIDKey)
	userID, ok := val.(string)
	if !ok {
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	logger = logger.With("user_id", userID)

	// Pin Verification
	err := s.AuthService.VerifyTransactionPin(r.Context(), userID, req.Pin)
	if err != nil {
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("incorrect transaction pin"))
		return
	}

	result, err := s.WalletService.SwapFunds(r.Context(), userID, req.FromCurrency, req.ToCurrency, req.Amount, idempotencyKey)
	if err != nil {
		if errors.Is(err, service.ErrTransactionAlreadyProcessed) {
			logger.Info("idempotent swap request detected")
			utils.JSON(w, r, http.StatusOK, map[string]interface{}{
				"status":  "success",
				"message": "swap already processed (idempotent)",
			})
			return
		}

		logger.Error("swap failed", "error", err)
		utils.ErrorJSON(w, r, http.StatusBadRequest, err)
		return
	}

	logger.Info("swap successful", "from", req.FromCurrency, "to", req.ToCurrency)
	utils.JSON(w, r, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "swap successful",
		"data":    result,
	})
}