package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/theabdullahishola/mzl-payment-app/internals/repository"
	"github.com/theabdullahishola/mzl-payment-app/prisma/db"
)

var (
	ErrTransactionAlreadyProcessed = errors.New("transaction with this reference already exists")
	ErrWalletNotFound              = errors.New("wallet not found for this currency")
)

type WalletService interface {
	GetWalletWithAssets(ctx context.Context, userID string) (*db.WalletModel, error)
	GetAssetByCurrency(ctx context.Context, userID string, currency string) (*db.WalletAssetModel, error)
	FundWallet(ctx context.Context, userID, currency string, amount float64, reference, description string) error
	WithdrawFunds(ctx context.Context, userID string, req WithdrawalRequest) error
	LookupUser(ctx context.Context, query string) (*UserLookupResult, error)
	SwapFunds(ctx context.Context, userID, fromCurrency, toCurrency string, amount float64, reference string) (*map[string]interface{}, error)
	GetTransactionHistory(ctx context.Context, userID string) ([]db.TransactionModel, error)
	TransferFunds(ctx context.Context, userID, toAccount, currency string, amount float64, description string, reference string) (string, error)
}

type WithdrawalRequest struct {
    Amount        float64 `json:"amount"`
    AccountNumber string  `json:"account_number"`
    AccountName   string  `json:"account_name"`
    BankCode      string  `json:"bank_code"`
    Currency      string  `json:"currency"`
    Pin           string  `json:"pin"`
    Reason        string  `json:"reason"`
}

type walletService struct {
	repo           repository.WalletRepository
	paymentService PaymentService
	userRepo       repository.UserRepository
	redis          QueueService // This is your Redis wrapper
}

type ExchangeRateResponse struct {
	Rates map[string]float64 `json:"rates"`
}

func NewWalletService(repo repository.WalletRepository, paymentService PaymentService, userRepo repository.UserRepository, redis QueueService) WalletService {
	return &walletService{repo: repo, paymentService: paymentService, userRepo: userRepo, redis: redis}
}

func (s *walletService) GetWalletWithAssets(ctx context.Context, userID string) (*db.WalletModel, error) {
	cacheKey := fmt.Sprintf("wallet:%s", userID)

	var cachedWallet db.WalletModel
	err := s.redis.Get(ctx, cacheKey, &cachedWallet)
	if err == nil {
		return &cachedWallet, nil
	}

	wallet, err := s.repo.GetWalletWithAssets(ctx, userID)
	if err != nil {
		return nil, err
	}
	if wallet == nil {
		return nil, errors.New("wallet does not exist")
	}

	_ = s.redis.Set(ctx, cacheKey, wallet, 5*time.Minute)

	return wallet, nil
}

func (s *walletService) GetAssetByCurrency(ctx context.Context, userID string, currency string) (*db.WalletAssetModel, error) {
	asset, err := s.repo.GetAssetByCurrency(ctx, userID, currency)
	if err != nil {
		return nil, err
	}
	if asset == nil {
		return nil, ErrWalletNotFound
	}
	return asset, nil
}

func (s *walletService) FundWallet(ctx context.Context, userID, currency string, amount float64, reference, description string) error {
    locked, _ := s.redis.TryLockIdempotencyKey(ctx, reference, 5*time.Minute)
    if !locked {
        return ErrTransactionAlreadyProcessed
    }

    asset, err := s.repo.GetAssetByCurrency(ctx, userID, currency)
    if err != nil {
        _ = s.redis.Delete(ctx, "idemp:"+reference)
        return err
    }

    err = s.repo.CreditWallet(ctx, asset.WalletID, currency, amount, reference, description, db.TransactionTypeDeposit)
    if err != nil {
        if _, ok := db.IsErrUniqueConstraint(err); ok {
            return ErrTransactionAlreadyProcessed
        }
        _ = s.redis.Delete(ctx, "idemp:"+reference)
        return err
    }

    _ = s.redis.Set(ctx, "idemp:"+reference, "completed", 24*time.Hour)
    _ = s.redis.Delete(ctx, fmt.Sprintf("wallet:%s", userID))
    return nil
}

func (s *walletService) fetchExchangeRate(fromCurrency, toCurrency string) (float64, error) {
	if fromCurrency == toCurrency {
		return 1.0, nil
	}

	url := fmt.Sprintf("https://open.er-api.com/v6/latest/%s", fromCurrency)

	resp, err := http.Get(url)
	if err != nil {
		return 0, errors.New("failed to fetch exchange rates")
	}
	defer resp.Body.Close()

	var result ExchangeRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, errors.New("failed to parse exchange rates")
	}

	rate, exists := result.Rates[toCurrency]
	if !exists {
		return 0, errors.New("currency pair not supported")
	}

	return rate, nil
}

func (s *walletService) SwapFunds(ctx context.Context, userID, fromCurrency, toCurrency string, amountIn float64, reference string) (*map[string]interface{}, error) {
	// Idempotency lock
	locked, _ := s.redis.TryLockIdempotencyKey(ctx, reference, 5*time.Minute)
	if !locked {
		return nil, ErrTransactionAlreadyProcessed
	}

	rate, err := s.fetchExchangeRate(fromCurrency, toCurrency)
	if err != nil {
		_ = s.redis.Delete(ctx, "idemp:"+reference)
		return nil, err
	}
	amountOut := amountIn * rate

	description := fmt.Sprintf("Swap %s to %s @ %f", fromCurrency, toCurrency, rate)

	err = s.repo.SwapFunds(ctx, userID, fromCurrency, toCurrency, amountIn, amountOut, reference, description)
	if err != nil {
		if _, ok := db.IsErrUniqueConstraint(err); ok {
			return nil, ErrTransactionAlreadyProcessed
		}
		_ = s.redis.Delete(ctx, "idemp:"+reference)
		return nil, err
	}

	_ = s.redis.Set(ctx, "idemp:"+reference, "completed", 24*time.Hour)
	_ = s.redis.Delete(ctx, fmt.Sprintf("wallet:%s", userID))
	_ = s.redis.Delete(ctx, fmt.Sprintf("tx_history:%s", userID))

	return &map[string]interface{}{
		"source_currency": fromCurrency,
		"source_amount":   amountIn,
		"dest_currency":   toCurrency,
		"dest_amount":     amountOut,
		"rate":            rate,
	}, nil
}

func (s *walletService) TransferFunds(ctx context.Context, userID, toAccount, currency string, amount float64, userDesc, reference string) (string, error) {
	// Idempotency lock
	locked, _ := s.redis.TryLockIdempotencyKey(ctx, reference, 5*time.Minute)
	if !locked {
		return "", ErrTransactionAlreadyProcessed
	}

	sender, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		_ = s.redis.Delete(ctx, "idemp:"+reference)
		return "", fmt.Errorf("failed to fetch sender details: %w", err)
	}

	senderName := "Unknown User"
	if sender != nil {
		senderName = sender.Name
	}

	receiverWallet, err := s.repo.GetWalletByAccountNumber(ctx, toAccount)
	if err != nil {
		_ = s.redis.Delete(ctx, "idemp:"+reference)
		return "", err
	}
	if receiverWallet == nil {
		_ = s.redis.Delete(ctx, "idemp:"+reference)
		return "", errors.New("recipient account number not found")
	}

	if receiverWallet.UserID == userID {
		_ = s.redis.Delete(ctx, "idemp:"+reference)
		return "", errors.New("cannot transfer money to yourself")
	}

	receiverUser := receiverWallet.User()
	receiverName := "Unknown"
	if receiverUser != nil {
		receiverName = receiverUser.Name
	}

	descSender := fmt.Sprintf("Transfer to %s", receiverName)
	if userDesc != "" {
		descSender += fmt.Sprintf(" / %s", userDesc)
	}

	descReceiver := fmt.Sprintf("Received from %s", senderName)
	if userDesc != "" {
		descReceiver += fmt.Sprintf(" /DESCRIPTION: %s", userDesc)
	}

	err = s.repo.TransferFunds(ctx, userID, toAccount, currency, amount, reference, descSender, descReceiver)
	if err != nil {
		if _, ok := db.IsErrUniqueConstraint(err); ok {
			return "", ErrTransactionAlreadyProcessed
		}
		_ = s.redis.Delete(ctx, "idemp:"+reference)
		return "", err
	}

	_ = s.redis.Set(ctx, "idemp:"+reference, "completed", 24*time.Hour)
	_ = s.redis.Delete(ctx, fmt.Sprintf("wallet:%s", userID))
	_ = s.redis.Delete(ctx, fmt.Sprintf("wallet:%s", receiverWallet.UserID))
	_ = s.redis.Delete(ctx, fmt.Sprintf("tx_history:%s", userID))

	return receiverName, nil
}

func (s *walletService) GetTransactionHistory(ctx context.Context, userID string) ([]db.TransactionModel, error) {
	cacheKey := fmt.Sprintf("tx_history:%s", userID)

	var cachedBytes []byte
	if err := s.redis.Get(ctx, cacheKey, &cachedBytes); err == nil {
		var transactions []db.TransactionModel
		if err := json.Unmarshal(cachedBytes, &transactions); err == nil {
			return transactions, nil
		}
	}
	transactions, err := s.repo.GetTransactions(ctx, userID)
	if err != nil {
		return nil, err
	}
	_ = s.redis.Set(ctx, cacheKey, transactions, 10*time.Minute)

	return transactions, nil
}

func (s *walletService) WithdrawFunds(ctx context.Context, userID string, req WithdrawalRequest) error {
	reference := fmt.Sprintf("WDR-%d", time.Now().UnixNano())

	recipientCode, err := s.paymentService.CreateTransferRecipient(
		req.AccountName,
		req.AccountNumber,
		req.BankCode,
		req.Currency,
	)
	if err != nil {
		return fmt.Errorf("failed to create paystack recipient: %w", err)
	}

	transferResp, err := s.paymentService.InitiateTransfer(
		req.Amount,
		recipientCode,
		reference,
		req.Reason,
	)
	if err != nil {
		return fmt.Errorf("failed to initiate paystack transfer: %w", err)
	}

	err = s.repo.DebitForWithdrawal(
		ctx,
		userID,
		req.Currency,
		req.Amount,
		reference,
		transferResp.Data.TransferCode,
		req.Reason,
	)
	if err != nil {
		return fmt.Errorf("withdrawal successful but db update failed: %w", err)
	}

	cacheKey := fmt.Sprintf("wallet:%s", userID)
	_ = s.redis.Delete(ctx, cacheKey)
	_ = s.redis.Delete(ctx, fmt.Sprintf("tx_history:%s", userID))
	return nil
}
type UserLookupResult struct {
	Name          string
	AccountNumber string
}

func (s *walletService) LookupUser(ctx context.Context, query string) (*UserLookupResult, error) {
	user, err := s.userRepo.FindUserByEmailOrAccount(ctx, query)
	if err != nil {
		return nil, err
	}

	wallet, ok := user.Wallet()
	if !ok {
		return nil, db.ErrNotFound
	}
	return &UserLookupResult{
		Name:          user.Name,
		AccountNumber: wallet.AccountNumber,
	}, nil
}