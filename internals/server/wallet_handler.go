package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/theabdullahishola/mzl-payment-app/internals/middlewares"
	"github.com/theabdullahishola/mzl-payment-app/internals/service"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
	"github.com/theabdullahishola/mzl-payment-app/prisma/db"
)


type FundRequest struct {
	Currency    string  `json:"currency" binding:"required"`
	Amount      float64 `json:"amount" binding:"required"`
	Description string  `json:"description"` // Optional
}

// HandleGetWallet retrieves the user's wallet details.
func (s *Server) GetWalletHandlerV1(w http.ResponseWriter, r *http.Request) {
	//  Get UserID from Context
	val := r.Context().Value(middlewares.UserIDKey)
	userID, ok := val.(string)
	if !ok {
		s.Logger.Error("failed to get user_id from context")
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	// Check for Query Parameter "?currency=..."
	currencyParam := r.URL.Query().Get("currency")

	if currencyParam != "" {
		asset, err := s.WalletService.GetAssetByCurrency(r.Context(), userID, currencyParam)
		if err != nil {
			if errors.Is(err, service.ErrWalletNotFound) || errors.Is(err, db.ErrNotFound) {
				utils.ErrorJSON(w, r, http.StatusNotFound, errors.New("wallet asset not found for this currency"))
				return
			}
			s.Logger.Error("failed to get specific asset", "error", err)
			utils.ErrorJSON(w, r, http.StatusInternalServerError, errors.New("internal server error"))
			return
		}

		utils.JSON(w,r, http.StatusOK, map[string]interface{}{
			"status":  "success",
			"message": "asset retrieved successfully",
			"data":    asset,
		})
		return
	}

	
	wallet, err := s.WalletService.GetWalletWithAssets(r.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			utils.ErrorJSON(w, r, http.StatusNotFound, errors.New("wallet not found"))
			return
		}
		s.Logger.Error("failed to get wallet", "error", err)
		utils.ErrorJSON(w, r, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}

	utils.JSON(w,r, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "wallet retrieved successfully",
		"data": map[string]interface{}{
			"id":             wallet.ID,
			"account_number": wallet.AccountNumber,
			"assets":         wallet.Assets(), // Returns the list of NGN, USD, etc.
		},
	})
}


func (s *Server) FundWalletHandlerV1(w http.ResponseWriter, r *http.Request) {
	//  Idempotency Check 
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("missing Idempotency-Key header"))
		return
	}

	var req FundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}

	if req.Currency == "" || req.Amount <= 0 {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("valid currency and amount greater than 0 are required"))
		return
	}

	// Get UserID
	val := r.Context().Value(middlewares.UserIDKey)
	userID, ok := val.(string)
	if !ok {
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	
	err := s.WalletService.FundWallet(r.Context(), userID, req.Currency, req.Amount, idempotencyKey, req.Description)
	if err != nil {
		if errors.Is(err, service.ErrTransactionAlreadyProcessed) {
			utils.JSON(w, r, http.StatusOK, map[string]interface{}{
				"status":  "success",
				"message": "transaction already processed (idempotent)",
			})
			return
		}

		// Handle "Wallet Not Found"  //CHECK-HERE
		if errors.Is(err, service.ErrWalletNotFound) {
			utils.ErrorJSON(w, r, http.StatusNotFound, errors.New("wallet currency not supported "))
			return
		}

		s.Logger.Error("funding failed", "error", err)
		utils.ErrorJSON(w, r, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}


	utils.JSON(w, r, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "wallet funded successfully",
		"data": map[string]interface{}{
			"amount":      req.Amount,
			"currency":    req.Currency,
			"reference":   idempotencyKey,
		},
	})
}