package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/theabdullahishola/mzl-payment-app/internals/service"
	"github.com/theabdullahishola/mzl-payment-app/internals/utils"
)

type RegisterRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	FullName string `json:"full_name" binding:"required"`
}

func (s *Server) RegisterHandlerV1(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.ErrorJSON(w, r, http.StatusBadRequest, err)
		return
	}

	if req.Email == "" || req.Password == "" || req.FullName == "" {
		utils.ErrorJSON(w, r, http.StatusBadRequest, errors.New("all fields are required"))
		return
	}
	user, err := s.AuthService.Register(r.Context(), req.Email, req.Password, req.FullName)
	if err != nil {
		if errors.Is(err, service.ErrUserAlreadyExists) {
			utils.ErrorJSON(w, r, http.StatusConflict, errors.New("this email is already registered"))
			return
		}
		utils.ErrorJSON(w, r, http.StatusInternalServerError, err)
		return
	}

	utils.JSON(w, r, http.StatusCreated, map[string]interface{}{
		"status":  "success",
		"message": "user registered successfully",
		"data": map[string]string{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		},
	})
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (s *Server) LoginHandlerV1(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.ErrorJSON(w, r, http.StatusBadRequest, err)
		return
	}

	token, err := s.AuthService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("invalid email or password"))
		return
	}
	s.setRefreshCookie(w, token.RefreshToken)
	utils.JSON(w, r, http.StatusOK, map[string]string{
		"token": token.AccessToken,
	})
}

func (s *Server) setRefreshCookie(w http.ResponseWriter, refreshToken string) {
	isProd := os.Getenv("APP_ENV") == "production"
	cookie := http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   isProd,
		SameSite: http.SameSiteNoneMode,
	}
	http.SetCookie(w, &cookie)
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {

	cookie, err := r.Cookie("refresh_token")
	if err != nil {

		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("no refresh token found"))
		return
	}

	refreshToken := cookie.Value

	newTokens, err := s.AuthService.RotateRefreshToken(r.Context(), refreshToken)
	if err != nil {
		s.clearRefreshCookie(w)
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("session expired"))
		return
	}

	s.setRefreshCookie(w, newTokens.RefreshToken)

	utils.JSON(w, r, http.StatusOK, map[string]interface{}{
		"status":       "success",
		"access_token": newTokens.AccessToken,
	})
}

func (s *Server) clearRefreshCookie(w http.ResponseWriter) {
	cookie := http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}
	http.SetCookie(w, &cookie)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {

	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		utils.ErrorJSON(w, r, http.StatusUnauthorized, errors.New("no refresh token found"))
		return
	}

	_ = s.AuthService.RevokeRefreshToken(r.Context(), cookie.Value)

	s.clearRefreshCookie(w)

	utils.JSON(w, r, http.StatusOK, map[string]string{"message": "logged out successfully"})
}

func (s *Server) LogoutHandler(w http.ResponseWriter, r *http.Request) {
    cookie, err := r.Cookie("refresh_token")
    if err == nil && cookie.Value != "" {
        _ = s.AuthService.RevokeRefreshToken(r.Context(), cookie.Value)
    }

    s.clearRefreshCookie(w)

    utils.JSON(w, r, http.StatusOK, map[string]string{
        "status":  "success",
        "message": "logged out successfully",
    })
}