package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/theabdullahishola/mzl-payment-app/internals/middlewares"
	"github.com/theabdullahishola/mzl-payment-app/internals/service"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
)

func (s *Server) GetBanksDetailHandler(w http.ResponseWriter, r *http.Request) {
  
    currency := r.URL.Query().Get("currency")
    
    banks, err := s.PaymentService.GetBankList(r.Context(),currency)
    if err != nil {
        utils.ErrorJSON(w, r, http.StatusBadGateway, err)
        return
    }
 
    utils.JSON(w,r, http.StatusOK, map[string]interface{}{
        "status": "success",
        "data":   banks,
    })
}



type ResolveAccountRequest struct {
    AccountNumber string `json:"account_number"`
    BankCode      string `json:"bank_code"`
}

func (s *Server) ResolveAccountDetailsHandler(w http.ResponseWriter, r *http.Request) {
    var req ResolveAccountRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("invalid request"))
        return
    }

    accountName, err := s.PaymentService.ResolveBankAccount(req.AccountNumber, req.BankCode)
    if err != nil {
        utils.ErrorJSON(w, r, http.StatusBadRequest, err)
        return
    }

    utils.JSON(w,r ,http.StatusOK, map[string]interface{}{
        "status": "success",
        "data": map[string]string{
            "account_name":   accountName,
            "account_number": req.AccountNumber,
        },
    })
}

type WithdrawalPayload service.WithdrawalRequest

func (s *Server) WithdrawalHandler(w http.ResponseWriter, r *http.Request) {
   
    var req WithdrawalPayload
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("invalid request"))
        return
    }


    userID := r.Context().Value(middlewares.UserIDKey).(string)

  
    if err := s.AuthService.VerifyTransactionPin(r.Context(), userID, req.Pin); err != nil {
        utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("incorrect transaction pin"))
        return
    }
    recipientCode, err := s.PaymentService.CreateTransferRecipient(req.AccountName, req.AccountNumber, req.BankCode, req.Currency)
    if err != nil {
        utils.ErrorJSON(w, r, http.StatusBadGateway, fmt.Errorf("recipient creation failed: %w", err))
        return
    }

 
    reference := fmt.Sprintf("WDR-%d", time.Now().UnixNano())
    
    paystackResp, err := s.PaymentService.InitiateTransfer(req.Amount, recipientCode, reference, req.Reason)
    if err != nil {
        utils.ErrorJSON(w, r, http.StatusBadGateway, fmt.Errorf("transfer failed: %w", err))
        return
    }

    err = s.WalletService.WithdrawFunds(
        r.Context(), 
        userID, 
        service.WithdrawalRequest(req),
    )

    if err != nil {
        s.Logger.Error("CRITICAL: Paystack transfer success but DB debit failed", "ref", reference, "error", err)
        utils.ErrorJSON(w, r, http.StatusInternalServerError, errors.New("system error processing withdrawal"))
        return
    }


    utils.JSON(w, r,http.StatusOK, map[string]interface{}{
        "status": "success",
        "message": "Withdrawal processing",
        "data": paystackResp.Data,
    })
}

func (s *Server) VerifyPaymentHandler(w http.ResponseWriter, r *http.Request) {
	reference := r.URL.Query().Get("reference")
	if reference == "" {
		utils.ErrorJSON(w, r, http.StatusBadRequest, fmt.Errorf("reference is required"))
		return
	}

	result, err := s.PaymentService.VerifyAndCredit(r.Context(), reference)
	if err != nil {
		s.Logger.Error("verification failed", "ref", reference, "error", err)
		utils.ErrorJSON(w, r, http.StatusBadRequest, err)
		return
	}


	utils.JSON(w, r, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Payment verified successfully",
		"data":    result,
	})
}