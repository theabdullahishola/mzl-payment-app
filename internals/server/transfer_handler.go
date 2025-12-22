package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/theabdullahishola/mzl-payment-app/internals/middlewares"
	"github.com/theabdullahishola/mzl-payment-app/internals/service"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
)

// --- TRANSFER HANDLER ---

type TransferRequest struct {
	AccountNumber string  `json:"account_number"`
	Currency      string  `json:"currency"`
	Amount        float64 `json:"amount"`
	Pin           string  `json:"pin"`
	Description   string  `json:"description"`
}

func (s *Server) TransferFundsHandlerV1(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("missing Idempotency-Key header"))
		return
	}

	// 1. Contextual Logger: Every log for this request will track the ID
	logger := s.Logger.With("idemp_key", idempotencyKey)

	var req TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}

	// Validation
	if req.Amount <= 0 {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("amount must be greater than zero"))
		return
	}
	if req.AccountNumber == "" || req.Currency == "" {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("account number and currency are required"))
		return
	}

	val := r.Context().Value(middlewares.UserIDKey)
	userID, ok := val.(string)
	if !ok {
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	logger = logger.With("user_id", userID)

	if req.Pin == "" {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("transaction pin is required"))
		return
	}

	// Pin Verification
	err := s.AuthService.VerifyTransactionPin(r.Context(), userID, req.Pin)
	if err != nil {
		logger.Warn("incorrect pin attempt")
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("incorrect transaction pin"))
		return
	}

	// Logic Execution
	receiverName, err := s.WalletService.TransferFunds(r.Context(), userID, req.AccountNumber, req.Currency, req.Amount, req.Description, idempotencyKey)
	if err != nil {
		// Handle Idempotency Duplicate
		if errors.Is(err, service.ErrTransactionAlreadyProcessed) {
			logger.Info("idempotent transfer request detected")
			utils.JSON(w, r, http.StatusOK, map[string]interface{}{
				"status":  "success",
				"message": "transfer already processed",
				"data": map[string]interface{}{
					"recipient": req.AccountNumber,
					"amount":    req.Amount,
					"currency":  req.Currency,
				},
			})
			return
		}

		// Handle Business Logic Errors
		if err.Error() == "insufficient balance" || err.Error() == "recipient account number not found" {
			logger.Warn("transfer blocked", "reason", err.Error())
			utils.ErrorJSON(w, r, http.StatusBadRequest, err)
			return
		}

		logger.Error("transfer critical failure", "error", err)
		utils.ErrorJSON(w, r, http.StatusInternalServerError, errors.New("transfer processing failed"))
		return
	}

	// Final Success
	logger.Info("transfer successful", "amount", req.Amount, "to", req.AccountNumber)
	utils.JSON(w, r, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "transfer successful",
		"data": map[string]interface{}{
			"recipient":       req.AccountNumber,
			"amount":          req.Amount,
			"currency":        req.Currency,
			"receipientName":  receiverName,
		},
	})
}

