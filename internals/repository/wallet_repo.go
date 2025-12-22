package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/theabdullahishola/mzl-payment-app/prisma/db"
)

type WalletRepository interface {
	GetWalletWithAssets(ctx context.Context, userID string) (*db.WalletModel, error)
	UpdateTransactionStatus(ctx context.Context, reference string, status db.TransactionStatus) error
	RefundWithdrawal(ctx context.Context, reference string) error
	GetAssetByCurrency(ctx context.Context, userID string, currency string) (*db.WalletAssetModel, error)
	DebitForWithdrawal(ctx context.Context, userID, currency string, amount float64, reference, transferCode, description string) error
	CreditWallet(ctx context.Context, walletID, currency string, amount float64, reference, description string, txType db.TransactionType) error
	CreditWalletByEmail(ctx context.Context, email, currency string, amount float64, reference, description, provider string) error
	SwapFunds(ctx context.Context, userID, fromCurrency, toCurrency string, sourceAmount, amountOut float64, reference, description string) error
	GetTransactions(ctx context.Context, userID string) ([]db.TransactionModel, error)
	GetUserByID(ctx context.Context, userID string) (*db.UserModel, error)
	GetWalletByAccountNumber(ctx context.Context, accountNumber string) (*db.WalletModel, error)
	GetTransactionByReference(ctx context.Context, reference string) (*db.TransactionModel, error)
	TransferFunds(ctx context.Context, fromUserID, toAccountNumber, currency string, amount float64, reference, descSender, descReceiver string) error
}

type walletRepository struct {
	client *db.PrismaClient
}

func NewWalletRepository(client *db.PrismaClient) WalletRepository {
	return &walletRepository{client: client}
}

func (r *walletRepository) GetWalletWithAssets(ctx context.Context, userID string) (*db.WalletModel, error) {
	return r.client.Wallet.FindUnique(
		db.Wallet.UserID.Equals(userID),
	).With(
		db.Wallet.Assets.Fetch(),
	).Exec(ctx)
}

func (r *walletRepository) GetAssetByCurrency(ctx context.Context, userID string, currency string) (*db.WalletAssetModel, error) {
	return r.client.WalletAsset.FindFirst(
		db.WalletAsset.Currency.Equals(currency),
		db.WalletAsset.Wallet.Where(db.Wallet.UserID.Equals(userID)),
	).Exec(ctx)
}

func (r *walletRepository) GetUserByID(ctx context.Context, userID string) (*db.UserModel, error) {
	return r.client.User.FindUnique(db.User.ID.Equals(userID)).Exec(ctx)
}

func (r *walletRepository) GetWalletByAccountNumber(ctx context.Context, accountNumber string) (*db.WalletModel, error) {
	return r.client.Wallet.FindUnique(
		db.Wallet.AccountNumber.Equals(accountNumber),
	).With(
		db.Wallet.User.Fetch(),
	).Exec(ctx)
}

func (r *walletRepository) GetTransactions(ctx context.Context, userID string) ([]db.TransactionModel, error) {
	return r.client.Transaction.FindMany(
		db.Transaction.Wallet.Where(db.Wallet.UserID.Equals(userID)),
	).OrderBy(
		db.Transaction.CreatedAt.Order(db.SortOrderDesc),
	).Exec(ctx)
}

func (r *walletRepository) GetTransactionByReference(ctx context.Context, reference string) (*db.TransactionModel, error) {
	return r.client.Transaction.FindUnique(
		db.Transaction.Reference.Equals(reference),
	).With(
		db.Transaction.Wallet.Fetch(),
	).Exec(ctx)
}



func (r *walletRepository) CreditWallet(ctx context.Context, walletID, currency string, amount float64, reference, description string, txType db.TransactionType) error {
	opTx := r.client.Transaction.CreateOne(
		db.Transaction.Wallet.Link(db.Wallet.ID.Equals(walletID)),
		db.Transaction.Amount.Set(amount),
		db.Transaction.Currency.Set(currency),
		db.Transaction.Type.Set(txType),
		db.Transaction.Reference.Set(reference),
		db.Transaction.Status.Set(db.TransactionStatusSuccess),
		db.Transaction.Description.Set(description),
	).Tx()

	opAsset := r.client.WalletAsset.UpsertOne(
		db.WalletAsset.WalletIDCurrency(
			db.WalletAsset.WalletID.Equals(walletID),
			db.WalletAsset.Currency.Equals(currency),
		),
	).Create(
		db.WalletAsset.Wallet.Link(db.Wallet.ID.Equals(walletID)),
		db.WalletAsset.Currency.Set(currency),
		db.WalletAsset.Balance.Set(amount),
	).Update(
		db.WalletAsset.Balance.Increment(amount),
	).Tx()

	return r.client.Prisma.Transaction(opTx, opAsset).Exec(ctx)
}

func (r *walletRepository) SwapFunds(ctx context.Context, userID, fromCurrency, toCurrency string, sourceAmount, amountOut float64, reference, description string) error {
	sourceAsset, err := r.GetAssetByCurrency(ctx, userID, fromCurrency)
	if err != nil || sourceAsset == nil {
		return fmt.Errorf("insufficient funds: source wallet not found")
	}

	if sourceAsset.Balance < sourceAmount {
		return fmt.Errorf("insufficient balance")
	}

	opDebit := r.client.WalletAsset.FindUnique(db.WalletAsset.ID.Equals(sourceAsset.ID)).
		Update(db.WalletAsset.Balance.Decrement(sourceAmount)).Tx()

	opCredit := r.client.WalletAsset.UpsertOne(
		db.WalletAsset.WalletIDCurrency(
			db.WalletAsset.WalletID.Equals(sourceAsset.WalletID),
			db.WalletAsset.Currency.Equals(toCurrency),
		),
	).Create(
		db.WalletAsset.Wallet.Link(db.Wallet.ID.Equals(sourceAsset.WalletID)),
		db.WalletAsset.Currency.Set(toCurrency),
		db.WalletAsset.Balance.Set(amountOut),
	).Update(db.WalletAsset.Balance.Increment(amountOut)).Tx()

	opLogOut := r.client.Transaction.CreateOne(
		db.Transaction.Wallet.Link(db.Wallet.ID.Equals(sourceAsset.WalletID)),
		db.Transaction.Amount.Set(sourceAmount),
		db.Transaction.Currency.Set(fromCurrency),
		db.Transaction.Type.Set(db.TransactionTypeSwap),
		db.Transaction.Reference.Set(reference+"-OUT"),
		db.Transaction.Status.Set(db.TransactionStatusSuccess),
		db.Transaction.Description.Set(description),
	).Tx()

	opLogIn := r.client.Transaction.CreateOne(
		db.Transaction.Wallet.Link(db.Wallet.ID.Equals(sourceAsset.WalletID)),
		db.Transaction.Amount.Set(amountOut),
		db.Transaction.Currency.Set(toCurrency),
		db.Transaction.Type.Set(db.TransactionTypeSwap),
		db.Transaction.Reference.Set(reference+"-IN"),
		db.Transaction.Status.Set(db.TransactionStatusSuccess),
		db.Transaction.Description.Set(description),
	).Tx()

	err = r.client.Prisma.Transaction(opDebit, opCredit, opLogOut, opLogIn).Exec(ctx)
	if err != nil && strings.Contains(err.Error(), "non_negative_balance") {
		return fmt.Errorf("insufficient funds (race condition)")
	}
	return err
}

func (r *walletRepository) TransferFunds(ctx context.Context, fromUserID, toAccountNumber, currency string, amount float64, reference, descSender, descReceiver string) error {
	senderAsset, err := r.GetAssetByCurrency(ctx, fromUserID, currency)
	if err != nil || senderAsset == nil {
		return fmt.Errorf("insufficient funds")
	}

	receiverWallet, err := r.GetWalletByAccountNumber(ctx, toAccountNumber)
	if err != nil || receiverWallet == nil {
		return fmt.Errorf("recipient not found")
	}

	if receiverWallet.UserID == fromUserID {
		return fmt.Errorf("cannot transfer to self")
	}

	opDebit := r.client.WalletAsset.FindUnique(db.WalletAsset.ID.Equals(senderAsset.ID)).
		Update(db.WalletAsset.Balance.Decrement(amount)).Tx()

	opCredit := r.client.WalletAsset.UpsertOne(
		db.WalletAsset.WalletIDCurrency(
			db.WalletAsset.WalletID.Equals(receiverWallet.ID),
			db.WalletAsset.Currency.Equals(currency),
		),
	).Create(
		db.WalletAsset.Wallet.Link(db.Wallet.ID.Equals(receiverWallet.ID)),
		db.WalletAsset.Currency.Set(currency),
		db.WalletAsset.Balance.Set(amount),
	).Update(db.WalletAsset.Balance.Increment(amount)).Tx()

	opLogS := r.client.Transaction.CreateOne(
		db.Transaction.Wallet.Link(db.Wallet.ID.Equals(senderAsset.WalletID)),
		db.Transaction.Amount.Set(amount),
		db.Transaction.Currency.Set(currency),
		db.Transaction.Type.Set(db.TransactionTypeTransfer),
		db.Transaction.Reference.Set(reference+"-DEBIT"),
		db.Transaction.Status.Set(db.TransactionStatusSuccess),
		db.Transaction.Description.Set(descSender),
	).Tx()

	opLogR := r.client.Transaction.CreateOne(
		db.Transaction.Wallet.Link(db.Wallet.ID.Equals(receiverWallet.ID)),
		db.Transaction.Amount.Set(amount),
		db.Transaction.Currency.Set(currency),
		db.Transaction.Type.Set(db.TransactionTypeTransfer),
		db.Transaction.Reference.Set(reference+"-CREDIT"),
		db.Transaction.Status.Set(db.TransactionStatusSuccess),
		db.Transaction.Description.Set(descReceiver),
	).Tx()

	err = r.client.Prisma.Transaction(opDebit, opCredit, opLogS, opLogR).Exec(ctx)
	if err != nil && strings.Contains(err.Error(), "non_negative_balance") {
		return fmt.Errorf("insufficient funds (race condition)")
	}
	return err
}

func (r *walletRepository) DebitForWithdrawal(ctx context.Context, userID, currency string, amount float64, reference, transferCode, description string) error {
	asset, err := r.GetAssetByCurrency(ctx, userID, currency)
	if err != nil || asset == nil || asset.Balance < amount {
		return fmt.Errorf("insufficient funds")
	}

	opDebit := r.client.WalletAsset.FindUnique(db.WalletAsset.ID.Equals(asset.ID)).
		Update(db.WalletAsset.Balance.Decrement(amount)).Tx()

	opLog := r.client.Transaction.CreateOne(
		db.Transaction.Wallet.Link(db.Wallet.ID.Equals(asset.WalletID)),
		db.Transaction.Amount.Set(amount),
		db.Transaction.Currency.Set(currency),
		db.Transaction.Type.Set(db.TransactionTypeWithdrawal),
		db.Transaction.Reference.Set(reference),
		db.Transaction.Status.Set(db.TransactionStatusPending),
		db.Transaction.GatewayRef.Set(transferCode),
		db.Transaction.Provider.Set("PAYSTACK"),
		db.Transaction.Description.Set(description),
	).Tx()

	err = r.client.Prisma.Transaction(opDebit, opLog).Exec(ctx)
	if err != nil && strings.Contains(err.Error(), "non_negative_balance") {
		return fmt.Errorf("insufficient funds (race condition)")
	}
	return err
}

func (r *walletRepository) CreditWalletByEmail(ctx context.Context, email, currency string, amount float64, reference, description, provider string) error {
	user, err := r.client.User.FindUnique(db.User.Email.Equals(email)).With(db.User.Wallet.Fetch()).Exec(ctx)
	if err != nil || user == nil {
		return fmt.Errorf("user not found")
	}

	wallet, ok := user.Wallet()
	if !ok {
		return fmt.Errorf("wallet not initialized")
	}

	opAsset := r.client.WalletAsset.UpsertOne(
		db.WalletAsset.WalletIDCurrency(
			db.WalletAsset.WalletID.Equals(wallet.ID),
			db.WalletAsset.Currency.Equals(currency),
		),
	).Create(
		db.WalletAsset.Wallet.Link(db.Wallet.ID.Equals(wallet.ID)),
		db.WalletAsset.Currency.Set(currency),
		db.WalletAsset.Balance.Set(amount),
	).Update(db.WalletAsset.Balance.Increment(amount)).Tx()

	opLog := r.client.Transaction.CreateOne(
		db.Transaction.Wallet.Link(db.Wallet.ID.Equals(wallet.ID)),
		db.Transaction.Amount.Set(amount),
		db.Transaction.Currency.Set(currency),
		db.Transaction.Type.Set(db.TransactionTypeDeposit),
		db.Transaction.Reference.Set(reference),
		db.Transaction.Status.Set(db.TransactionStatusSuccess),
		db.Transaction.Description.Set(description),
		db.Transaction.Provider.Set(provider),
	).Tx()

	err = r.client.Prisma.Transaction(opAsset, opLog).Exec(ctx)
	if isUniqueConstraintError(err) {
		return nil
	}
	return err
}

func (r *walletRepository) RefundWithdrawal(ctx context.Context, transferCode string) error {
	txn, err := r.client.Transaction.FindFirst(
		db.Transaction.GatewayRef.Equals(transferCode),
		db.Transaction.Type.Equals(db.TransactionTypeWithdrawal),
	).Exec(ctx)

	if err != nil || txn == nil || txn.Status == db.TransactionStatusFailed {
		return nil
	}

	opRefund := r.client.WalletAsset.FindUnique(
		db.WalletAsset.WalletIDCurrency(
			db.WalletAsset.WalletID.Equals(txn.WalletID),
			db.WalletAsset.Currency.Equals(txn.Currency),
		),
	).Update(db.WalletAsset.Balance.Increment(txn.Amount)).Tx()

	opStatus := r.client.Transaction.FindUnique(db.Transaction.ID.Equals(txn.ID)).
		Update(db.Transaction.Status.Set(db.TransactionStatusFailed)).Tx()

	return r.client.Prisma.Transaction(opRefund, opStatus).Exec(ctx)
}

func (r *walletRepository) UpdateTransactionStatus(ctx context.Context, reference string, status db.TransactionStatus) error {
	_, err := r.client.Transaction.FindMany(db.Transaction.GatewayRef.Equals(reference)).
		Update(db.Transaction.Status.Set(status)).Exec(ctx)
	return err
}

func isUniqueConstraintError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "Unique constraint failed") || strings.Contains(err.Error(), "P2002"))
}
