package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
	"github.com/theabdullahishola/mzl-payment-app/prisma/db"
)


type UserRepository interface {
	CreateUserWithWallet(ctx context.Context, email, password, name string) (*db.UserModel, error)
	FindUserByEmail(ctx context.Context, email string) (*db.UserModel, error)
	UpdateTransactionPin(ctx context.Context, userID, hashedPin string) error
    GetTransactionPin(ctx context.Context, userID string) (string, error)
	FindUserByID(ctx context.Context, userID string) (*db.UserModel, error)
	SaveRefreshToken(ctx context.Context, userID, token string, expiresAt time.Time) error
	ValidateRefreshToken(ctx context.Context, token string) (bool, error)
	RevokeRefreshToken(ctx context.Context, token string) error
	FindUserByEmailOrAccount(ctx context.Context, query string) (*db.UserModel, error)
}


type userRepo struct {
	client *db.PrismaClient
}


func NewUserRepository(client *db.PrismaClient) UserRepository {
	return &userRepo{
		client: client,
	}
}

func (r *userRepo) CreateUserWithWallet(ctx context.Context, email, password, name string) (*db.UserModel, error) {
	// Create User
	user, err := r.client.User.CreateOne(
		db.User.Email.Set(email),
		db.User.Password.Set(password),
		db.User.Name.Set(name),
	).Exec(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	accountNumber, err :=utils.GenerateAccountNumber()
	if err != nil {
		return nil, errors.New("failed to create account number")
	}
	accNum:= strconv.Itoa(int(accountNumber))
	wallet, err := r.client.Wallet.CreateOne(
		db.Wallet.AccountNumber.Set(accNum),
		db.Wallet.User.Link(db.User.ID.Equals(user.ID)),
	).Exec(ctx)

	if err != nil {
		
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	// Create Default Asset 
	_, err = r.client.WalletAsset.CreateOne(
		db.WalletAsset.Wallet.Link(db.Wallet.ID.Equals(wallet.ID)),
		db.WalletAsset.Currency.Set("NGN"),
	).Exec(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to create default asset: %w", err)
	}

	return user, nil
}

func (r *userRepo) FindUserByEmail(ctx context.Context, email string) (*db.UserModel, error) {
	return r.client.User.FindUnique(
		db.User.Email.Equals(email),
	).Exec(ctx)
}
func (r *userRepo) FindUserByID(ctx context.Context, userID string) (*db.UserModel, error) {
	return r.client.User.FindUnique(
		db.User.ID.Equals(userID),
	).Exec(ctx)
}

func (r *userRepo) UpdateTransactionPin(ctx context.Context, userID, hashedPin string) error {
    _, err := r.client.User.FindUnique(
        db.User.ID.Equals(userID),
    ).Update(
        db.User.TransactionPin.Set(hashedPin),
    ).Exec(ctx)
    return err
}

func (r *userRepo) GetTransactionPin(ctx context.Context, userID string) (string, error) {
    user, err := r.client.User.FindUnique(
        db.User.ID.Equals(userID),
    ).Exec(ctx)
    
    if err != nil {
        return "", err
    }


    pin, ok := user.TransactionPin()
    
    if !ok {
        return "", nil
    }
    return pin, nil 
}

func (r *userRepo) SaveRefreshToken(ctx context.Context, userID, token string, expiresAt time.Time) error {
    _, err := r.client.RefreshToken.CreateOne(
        db.RefreshToken.Token.Set(token),
        db.RefreshToken.User.Link(db.User.ID.Equals(userID)),
        db.RefreshToken.ExpiresAt.Set(expiresAt),
    ).Exec(ctx)
    return err
}

func (r *userRepo) ValidateRefreshToken(ctx context.Context, token string) (bool, error) {
    found, err := r.client.RefreshToken.FindFirst(
        db.RefreshToken.Token.Equals(token),
        db.RefreshToken.Revoked.Equals(false), 
        db.RefreshToken.ExpiresAt.After(time.Now()), 
    ).Exec(ctx)
    if err != nil { return false, err }
    return found != nil, nil
}

func (r *userRepo) RevokeRefreshToken(ctx context.Context, token string) error {
    _, err := r.client.RefreshToken.FindUnique(
        db.RefreshToken.Token.Equals(token),
    ).Update(
        db.RefreshToken.Revoked.Set(true),
    ).Exec(ctx)
    return err
}

func (r *userRepo) FindUserByEmailOrAccount(ctx context.Context, query string) (*db.UserModel, error) {
   
    return r.client.User.FindFirst(
        db.User.Or(
            db.User.Email.Equals(query),
            db.User.Wallet.Where(
                db.Wallet.AccountNumber.Equals(query),
            ),
        ),
    ).With(
        db.User.Wallet.Fetch(),
    ).Exec(ctx)
}