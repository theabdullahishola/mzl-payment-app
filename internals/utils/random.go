package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

func GenerateAccountNumber() (int64, error) {

	min := big.NewInt(1000000000)
	max := big.NewInt(9999999999)

	r := new(big.Int).Sub(max, min)
	r.Add(r, big.NewInt(1))

	n, err := rand.Int(rand.Reader, r)
	if err != nil {
		return 0, fmt.Errorf("failed to generate secure random number: %w", err)
	}
	accountNumber := new(big.Int).Add(n, min)

	return accountNumber.Int64(), nil
}