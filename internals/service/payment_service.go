package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/theabdullahishola/mzl-payment-app/internals/repository"
	"github.com/theabdullahishola/mzl-payment-app/prisma/db"
)


type PaystackInitResponse struct {
    Status  bool   `json:"status"`
    Message string `json:"message"`
    Data    struct {
        AuthorizationURL string `json:"authorization_url"`
        AccessCode       string `json:"access_code"`
        Reference        string `json:"reference"`
    } `json:"data"`
}

type PaystackRecipientResponse struct {
    Status  bool   `json:"status"`
    Message string `json:"message"`
    Data    struct {
        RecipientCode string `json:"recipient_code"`
    } `json:"data"`
}

type PaystackTransferResponse struct {
    Status  bool   `json:"status"`
    Message string `json:"message"`
    Data    struct {
        TransferCode string `json:"transfer_code"`
        Reference    string `json:"reference"`
        Status       string `json:"status"` 
    } `json:"data"`
}

type PaymentService interface {
    InitializeTransaction(email, currency string, amount float64) (*PaystackInitResponse, error)
	VerifyPaystackSignature(payload []byte, signature string) bool
	ProcessPaystackEvent(ctx context.Context, payload []byte) error
	GetBankList(ctx context.Context,currency string) ([]map[string]interface{}, error) 
    ResolveBankAccount(accountNumber, bankCode string) (string, error)
	CreateTransferRecipient(name, accountNum, bankCode, currency string) (string, error)
    InitiateTransfer(amount float64, recipientCode, reference, reason string) (*PaystackTransferResponse, error)
    VerifyAndCredit(ctx context.Context, reference string) (*db.TransactionModel, error)
}

type paymentService struct {
    repo repository.WalletRepository
    paystackClient Client 
    userRepo repository.UserRepository
    redis   QueueService
}

func NewPaymentService(repo repository.WalletRepository, client Client, redis QueueService, userRepo repository.UserRepository) PaymentService {
    return &paymentService{repo: repo, paystackClient: client, redis:redis,userRepo: userRepo,}
}

func (s *paymentService) InitializeTransaction(email, currency string, amount float64) (*PaystackInitResponse, error) {
    secret := os.Getenv("PAYSTACK_SECRET_KEY")
    if secret == "" {
        return nil, fmt.Errorf("paystack secret key is missing")
    }
    

    callbackURL := os.Getenv("PAYSTACK_CALLBACK_URL")
    if callbackURL == "" {
        callbackURL = "http://localhost:8080/webhooks/paystack" // Default fallback
    }

    amountInSubunits := amount * 100 
    
    var channels []string
    switch currency {
    case "NGN":
        channels = []string{"card", "bank_transfer", "ussd", "bank", "qr", "mobile_money", "apple_pay"}
    case "GHS":
        channels = []string{"card", "mobile_money"}
    case "ZAR":
        channels = []string{"card", "eft"}
    case "USD":
        channels = []string{"card", "apple_pay"}
    default:
        channels = []string{"card"}
    }

    payload := map[string]interface{}{
        "email":        email,
        "amount":       amountInSubunits,
        "currency":     currency,
        "callback_url": callbackURL, 
        "channels":     channels,
    }

    jsonPayload, _ := json.Marshal(payload)

    req, err := http.NewRequest("POST", "https://api.paystack.co/transaction/initialize", bytes.NewBuffer(jsonPayload))
    if err != nil { return nil, err }

    req.Header.Set("Authorization", "Bearer "+secret)
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()

    var result PaystackInitResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    if !result.Status {
        return nil, fmt.Errorf("paystack error: %s", result.Message)
    }

    return &result, nil
}

func (s *paymentService) VerifyPaystackSignature(payload []byte, signature string) bool {
	secret := os.Getenv("PAYSTACK_SECRET_KEY")
	if secret == "" {
		return false
	}
	
	h := hmac.New(sha512.New, []byte(secret))
	h.Write(payload)
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	return expectedSignature == signature
}


func (s *paymentService) ProcessPaystackEvent(ctx context.Context, payload []byte) error {

    var event struct {
        Event string `json:"event"`
        Data  struct {
            Reference    string  `json:"reference"`
            TransferCode string  `json:"transfer_code"`
            Amount       float64 `json:"amount"`
            Currency     string  `json:"currency"`
            Status       string  `json:"status"`
            Customer     struct {
                Email string `json:"email"`
            } `json:"customer"`
        } `json:"data"`
    }

    if err := json.Unmarshal(payload, &event); err != nil {
        return fmt.Errorf("failed to parse webhook body: %w", err)
    }

    if event.Event == "charge.success" && event.Data.Status == "success" {
        user, err := s.userRepo.FindUserByEmail(ctx, event.Data.Customer.Email)
        if err == nil && user != nil {
            cacheKey := fmt.Sprintf("wallet:%s", user.ID)
            _ = s.redis.Delete(ctx, cacheKey)
            _ = s.redis.Delete(ctx, fmt.Sprintf("tx_history:%s", user.ID))
        }
        
        actualAmount := event.Data.Amount / 100.0
        description := fmt.Sprintf("Deposit via Paystack (%s)", event.Data.Currency)

        return s.repo.CreditWalletByEmail(
            ctx,
            event.Data.Customer.Email,
            event.Data.Currency,
            actualAmount,
            event.Data.Reference,
            description,
            "PAYSTACK",
        )
    }

    if event.Event == "transfer.success" {
        return s.repo.UpdateTransactionStatus(
            ctx,
            event.Data.TransferCode,
            db.TransactionStatusSuccess,
        )
    }

    if event.Event == "transfer.failed" || event.Event == "transfer.reversed" {
        return s.repo.RefundWithdrawal(ctx, event.Data.TransferCode)
    }

    return nil
}
func (s *paymentService) GetBankList(ctx context.Context,currency string) ([]map[string]interface{}, error) {
    secret := os.Getenv("PAYSTACK_SECRET_KEY")
    
    if currency == "" {
        currency = "NGN"
    }
    cacheKey := fmt.Sprintf("banks:%s", currency)
    var banks []map[string]interface{}
    if err := s.redis.Get(ctx, cacheKey, &banks); err == nil {
        return banks, nil
    }
   
    url := fmt.Sprintf("https://api.paystack.co/bank?currency=%s", currency)
    
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("Authorization", "Bearer "+secret)

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()

    var result struct {
        Status  bool `json:"status"`
        Message string `json:"message"`
        Data    []map[string]interface{} `json:"data"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    if !result.Status {
        return nil, fmt.Errorf("paystack error: %s", result.Message)
    }
    _ = s.redis.Set(ctx, cacheKey, result.Data, 24*time.Hour)
    return result.Data, nil
}

func (s *paymentService) ResolveBankAccount(accountNumber, bankCode string) (string, error) {
    secret := os.Getenv("PAYSTACK_SECRET_KEY")
  
    url := fmt.Sprintf("https://api.paystack.co/bank/resolve?account_number=%s&bank_code=%s", accountNumber, bankCode)
    
    req, err:= http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
    req.Header.Set("Authorization", "Bearer "+secret)

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()

    var result struct {
        Status  bool `json:"status"`
        Message string `json:"message"`
        Data    struct {
            AccountName string `json:"account_name"`
            AccountNumber string `json:"account_number"`
        } `json:"data"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }

    if !result.Status {
        return "", fmt.Errorf("could not resolve account: %s", result.Message)
    }

    return result.Data.AccountName, nil
}

func (s *paymentService) CreateTransferRecipient(name, accountNum, bankCode, currency string) (string, error) {
    secret := os.Getenv("PAYSTACK_SECRET_KEY")

    var recipientType string

    switch currency {
    case "NGN":
        recipientType = "nuban" // Nigeria Bank Account
    case "GHS":
        recipientType = "mobile_money" // Ghana Mobile Money
    case "ZAR":
        recipientType = "basa" // South Africa Bank Account
    default:
        recipientType = "nuban" // Default fallback
    }

 
    payload := map[string]interface{}{
        "type":           recipientType, 
        "name":           name,
        "account_number": accountNum,
        "bank_code":      bankCode,
        "currency":       currency,
    }

    jsonPayload, _ := json.Marshal(payload)

    req, err:= http.NewRequest("POST", "https://api.paystack.co/transferrecipient", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", err
	}
    req.Header.Set("Authorization", "Bearer "+secret)
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()

    var result PaystackRecipientResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }

    if !result.Status {
        return "", fmt.Errorf("paystack recipient error: %s", result.Message)
    }

    return result.Data.RecipientCode, nil
}

func (s *paymentService) InitiateTransfer(amount float64, recipientCode, reference, reason string) (*PaystackTransferResponse, error) {
    secret := os.Getenv("PAYSTACK_SECRET_KEY")
    
    amountInSubunits := amount * 100

    payload := map[string]interface{}{
        "source":    "balance", 
        "amount":    amountInSubunits,
        "recipient": recipientCode,
        "reference": reference,
        "reason":    reason,
    }

    jsonPayload, _ := json.Marshal(payload)

    req, err:= http.NewRequest("POST", "https://api.paystack.co/transfer", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}
    req.Header.Set("Authorization", "Bearer "+secret)
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()

    var result PaystackTransferResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }

    if !result.Status {
        return nil, fmt.Errorf("transfer error: %s", result.Message)
    }

    return &result, nil
}

func (s *paymentService) VerifyAndCredit(ctx context.Context, reference string) (*db.TransactionModel, error) {
	
	paystackData, err := s.paystackClient.VerifyTransaction(reference)
	if err != nil {
		return nil, fmt.Errorf("paystack verification failed: %w", err)
	}

	if paystackData.Data.Status != "success" {
		return nil, fmt.Errorf("payment failed or abandoned")
	}

	actualAmount := paystackData.Data.Amount / 100.0
	currency := paystackData.Data.Currency
	email := paystackData.Data.Customer.Email
	description := fmt.Sprintf("Deposit via Paystack (%s)", currency)

	err = s.repo.CreditWalletByEmail(
		ctx,
		email,
		currency,
		actualAmount,
		reference,
		description,
		"PAYSTACK",
	)
	if err != nil {
		return nil, err
	}
    user, err := s.userRepo.FindUserByEmail(ctx, email)
    if err == nil && user != nil {
         _ = s.redis.Delete(ctx, fmt.Sprintf("wallet:%s", user.ID))
    }
	return s.repo.GetTransactionByReference(ctx, reference)
}
