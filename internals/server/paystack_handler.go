package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/theabdullahishola/mzl-payment-app/internals/middlewares"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
)

type InitiatePaymentRequest struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

func (s *Server) InitiatePaymentHandler(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value(middlewares.UserIDKey)
	userID, _ := val.(string)

	user, err := s.AuthService.GetUserByID(r.Context(), userID)
	if err != nil {
		utils.ErrorJSON(w, r, http.StatusInternalServerError, err)
		return
	}

	var req InitiatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("invalid request"))
		return
	}

	data, err := s.PaymentService.InitializeTransaction(user.Email, req.Currency, req.Amount)
	if err != nil {
		utils.ErrorJSON(w, r, http.StatusBadRequest, err)
		return
	}

	utils.JSON(w, r, http.StatusOK, map[string]interface{}{
		"status": "success",
		"data": map[string]string{
			"authorization_url": data.Data.AuthorizationURL,
			"reference":         data.Data.Reference,
		},
	})
}

func (s *Server) PaystackWebhookHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.Logger.Error("failed to read webhook body", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	signature := r.Header.Get("x-paystack-signature")
	if signature == "" {
		s.Logger.Warn("webhook missing signature header")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !s.PaymentService.VerifyPaystackSignature(body, signature) {
		s.Logger.Warn("invalid paystack signature attempt")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var event struct {
		Data struct {
			Reference string `json:"reference"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err == nil && event.Data.Reference != "" {
		locked, _ := s.RedisSvc.TryLockIdempotencyKey(r.Context(), event.Data.Reference, 24*time.Hour)
		if !locked {
			s.Logger.Info("ignoring duplicate webhook", "ref", event.Data.Reference)
			w.WriteHeader(http.StatusOK) 
			return
		}
	}

	err = s.RedisSvc.Enqueue(r.Context(), "payment_webhooks", body)
	if err != nil {
		if event.Data.Reference != "" {
			_ = s.RedisSvc.Delete(r.Context(), "idemp:"+event.Data.Reference)
		}
		s.Logger.Error("failed to queue webhook", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
