package repository

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
	"github.com/theabdullahishola/mzl-payment-app/prisma/db"
)

// TestMain handles the environment setup for the entire package
func TestMain(m *testing.M) {
	// Load the test environment variables (Port 5433)
	// We use the relative path to find .env.test from the current directory
	_ = godotenv.Load("../../.env.test")

	// Run all tests in the package
	code := m.Run()

	// Exit with the result code
	os.Exit(code)
}

func TestWalletSafetyConstraint(t *testing.T) {
	// 1. Initialize Prisma Client
	client := db.NewClient()
	if err := client.Prisma.Connect(); err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}
	defer client.Prisma.Disconnect()

	ctx := context.Background()
	repo := NewWalletRepository(client)
	user, err := client.User.CreateOne(
		db.User.Email.Set("safety_test@mzl.com"),
		db.User.Password.Set("hashed_pass"),
		db.User.Name.Set("Safety Test"),
	).Exec(ctx)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	defer func() {
		_, _ = client.User.FindUnique(db.User.ID.Equals(user.ID)).Delete().Exec(ctx)
	}()

	wallet, err := client.Wallet.CreateOne(
		db.Wallet.AccountNumber.Set("TEST-ACC-123"),
		db.Wallet.User.Link(db.User.ID.Equals(user.ID)),
	).Exec(ctx)
	if err != nil {
		t.Fatalf("Failed to create test wallet: %v", err)
	}

	err = repo.CreditWallet(ctx, wallet.ID, "USD", 10.0, "SEED_123", "Initial Deposit", db.TransactionTypeDeposit)
	if err != nil {
		t.Fatalf("Failed to seed wallet: %v", err)
	}

	t.Run("Prevent Negative Balance", func(t *testing.T) {
		err := repo.DebitForWithdrawal(ctx, user.ID, "USD", 50.0, "FAIL_REF_001", "CODE_RED", "Illegal Withdrawal Attempt")

		if err == nil {
			t.Errorf("SECURITY BREACH: Transaction allowed balance to go negative!")
			return
		}

		errMsg := strings.ToLower(err.Error())
		isGoHandled := strings.Contains(errMsg, "insufficient funds")
		isDbHandled := strings.Contains(errMsg, "non_negative_balance")

		if !isGoHandled && !isDbHandled {
			t.Errorf("Expected safety error (Go or DB), but got: %v", err)
		}

		if isDbHandled {
			t.Log("Verified: Database SQL Constraint blocked the transaction.")
		} else if isGoHandled {
			t.Log("Verified: Go Repository logic blocked the transaction.")
		}
	})
}