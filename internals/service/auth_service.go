package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/theabdullahishola/mzl-payment-app/internals/config"
	"github.com/theabdullahishola/mzl-payment-app/internals/repository"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
	"github.com/theabdullahishola/mzl-payment-app/prisma/db"
	"golang.org/x/crypto/bcrypt"
)

type AuthService interface {
	Register(ctx context.Context, email, password, fullName string) (*db.UserModel, error)
	Login(ctx context.Context, email, password string) (*utils.TokenPair, error)
	RotateRefreshToken(ctx context.Context, oldToken string) (*utils.TokenPair, error)
	GetUserByID(ctx context.Context, userID string) (*db.UserModel, error)
	VerifyTransactionPin(ctx context.Context, userID, plainPin string) error
	SetTransactionPin(ctx context.Context, userID, plainPin string) error
	RevokeRefreshToken(ctx context.Context, oldRefreshToken string) error
}

var (
	ErrUserAlreadyExists  = errors.New("email already in use")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

type authService struct {
	userRepo repository.UserRepository
	config   *config.Config
	redis    QueueService
}

type UserCacheDTO struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	FullName       string    `json:"name"` // Ensure this matches the Prisma field name (usually 'name')
	CreatedAt      time.Time `json:"createdAt"`
	TransactionPin string    `json:"transaction_pin"`
}

func toUserCache(u *db.UserModel) UserCacheDTO {
	pin, _ := u.TransactionPin()
	return UserCacheDTO{
		ID:             u.ID,
		FullName:       u.Name,
		Email:          u.Email,
		TransactionPin: pin,
	}
}

func fromUserCache(c UserCacheDTO) *db.UserModel {
	data, _ := json.Marshal(c)

	var user db.UserModel
	_ = json.Unmarshal(data, &user)

	return &user
}
func NewAuthService(userRepo repository.UserRepository, cfg *config.Config, redis QueueService) AuthService {
	return &authService{userRepo: userRepo, config: cfg, redis: redis}
}

func (s *authService) Register(ctx context.Context, email, password, fullName string) (*db.UserModel, error) {

	existingUser, _ := s.userRepo.FindUserByEmail(ctx, email)
	if existingUser != nil {
		return nil, ErrUserAlreadyExists
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.CreateUserWithWallet(ctx, email, string(hashedBytes), fullName)

	if err != nil {
		if _, ok := db.IsErrUniqueConstraint(err); ok {
			return nil, ErrUserAlreadyExists
		}
		return nil, err
	}

	return user, nil
}

func (s *authService) Login(ctx context.Context, email, password string) (*utils.TokenPair, error) {

	user, err := s.userRepo.FindUserByEmail(ctx, email)
	if err != nil {
		return &utils.TokenPair{}, errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return &utils.TokenPair{}, errors.New("invalid credentials")
	}

	tokens, err := utils.GenerateTokenPair(user.ID)
	if err != nil {
		return &utils.TokenPair{}, err
	}

	err = s.userRepo.SaveRefreshToken(ctx, user.ID, tokens.RefreshToken, time.Now().Add(time.Hour*24*7))
	if err != nil {
		return &utils.TokenPair{}, err
	}

	return tokens, nil
}
func (s *authService) RotateRefreshToken(ctx context.Context, oldRefreshToken string) (*utils.TokenPair, error) {

	token, err := jwt.Parse(oldRefreshToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_REFRESH_SECRET")), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}
	userID := claims["sub"].(string)

	isValid, err := s.userRepo.ValidateRefreshToken(ctx, oldRefreshToken)
	if err != nil || !isValid {
		if err := s.userRepo.RevokeRefreshToken(ctx, oldRefreshToken); err != nil {
			return nil, err
		}
		return nil, errors.New("token has been revoked or used")
	}

	newTokens, err := utils.GenerateTokenPair(userID)
	if err != nil {
		return nil, err
	}
	if err := s.userRepo.RevokeRefreshToken(ctx, oldRefreshToken); err != nil {
		return nil, err
	}

	err = s.userRepo.SaveRefreshToken(ctx, userID, newTokens.RefreshToken, time.Now().Add(time.Hour*24*7))
	if err != nil {
		return nil, err
	}

	return newTokens, nil
}
func (s *authService) RevokeRefreshToken(ctx context.Context, oldRefreshToken string) error {
	return s.userRepo.RevokeRefreshToken(ctx, oldRefreshToken)
}
func (s *authService) GetUserByID(ctx context.Context, userID string) (*db.UserModel, error) {
	cacheKey := fmt.Sprintf("user_profile:%s", userID)
	var cachedDTO UserCacheDTO
	err := s.redis.Get(ctx, cacheKey, &cachedDTO)
	if err == nil {
		return fromUserCache(cachedDTO), nil
	}

	user, err := s.userRepo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	dto := toUserCache(user)
	_ = s.redis.Set(ctx, cacheKey, dto, 30*time.Minute)
	return user, nil

}

func (s *authService) VerifyTransactionPin(ctx context.Context, userID, plainPin string) error {

	hashedPin, err := s.userRepo.GetTransactionPin(ctx, userID)
	if err != nil {
		return err
	}
	if hashedPin == "" {
		return errors.New("transaction pin not set")
	}

	err = bcrypt.CompareHashAndPassword([]byte(hashedPin), []byte(plainPin))
	if err != nil {
		return errors.New("invalid transaction pin")
	}

	return nil
}

func (s *authService) SetTransactionPin(ctx context.Context, userID, plainPin string) error {
	if len(plainPin) != 4 {
		return errors.New("pin must be 4 digits")
	}

	hashedPin, err := bcrypt.GenerateFromPassword([]byte(plainPin), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	cacheKey := fmt.Sprintf("user_profile:%s", userID)
	s.redis.Delete(ctx, cacheKey)
	return s.userRepo.UpdateTransactionPin(ctx, userID, string(hashedPin))
}
