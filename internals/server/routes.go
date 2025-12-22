package server

import (
	"net/http"
	"time"
)

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.Handler
}

func (s *Server) registerRoutes() {
	routes := []Route{
		{
			Name:        "Health Check",
			Method:      "GET",
			Pattern:     "/health",
			HandlerFunc: http.HandlerFunc(s.HandleHealth),
		},
		{
			Name:        "Register User",
			Method:      "POST",
			Pattern:     "/api/v1/auth/register",
			HandlerFunc: http.HandlerFunc(s.RegisterHandlerV1),
		},
		{
			Name:        "Login User",
			Method:      "POST",
			Pattern:     "/api/v1/auth/login",
			HandlerFunc: http.HandlerFunc(s.LoginHandlerV1),
		},
		{
			Name:        "Logout User",
			Method:      "POST",
			Pattern:     "/api/v1/auth/logout",
			HandlerFunc: http.HandlerFunc(s.Logout),
		},
		{
			Name:        "Fund Wallet",
			Method:      "POST",
			Pattern:     "/api/v1/wallet/fund",
			HandlerFunc: s.AddMiddlewaresToHandler(http.HandlerFunc(s.FundWalletHandlerV1), s.AuthMiddleware.MiddlewareAuthHandler),
		},
		{
			Name:        "Get Wallet",
			Method:      "GET",
			Pattern:     "/api/v1/wallet",
			HandlerFunc: s.AddMiddlewaresToHandler(http.HandlerFunc(s.GetWalletHandlerV1), s.AuthMiddleware.MiddlewareAuthHandler),
		},
		{
			Name:        "Swap Funds",
			Method:      "POST",
			Pattern:     "/api/v1/swap",
			HandlerFunc: s.AddMiddlewaresToHandler(http.HandlerFunc(s.SwapHandlerV1), s.AuthMiddleware.MiddlewareAuthHandler, s.RateLimit(10, time.Minute)),
		},
		{
			Name:    "Transfer Funds",
			Method:  "POST",
			Pattern: "/api/v1/wallet/transfer",
			HandlerFunc: s.AddMiddlewaresToHandler(
				http.HandlerFunc(s.TransferFundsHandlerV1),
				s.AuthMiddleware.MiddlewareAuthHandler, s.RateLimit(5, time.Minute),
			),
		},
		{
			Name:    "Transaction History",
			Method:  "GET",
			Pattern: "/api/v1/transactions",
			HandlerFunc: s.AddMiddlewaresToHandler(
				http.HandlerFunc(s.GetTransactionHistoryV1),
				s.AuthMiddleware.MiddlewareAuthHandler,
			),
		},
		{
			Name:    "Get Rate",
			Method:  "GET",
			Pattern: "/api/v1/rates",
			HandlerFunc: s.AddMiddlewaresToHandler(
				http.HandlerFunc(s.GetExchangeRatesHandler),
				s.AuthMiddleware.MiddlewareAuthHandler,
			),
		},
		{
			Name:    "Get User Profile",
			Method:  "GET",
			Pattern: "/api/v1/user",
			HandlerFunc: s.AddMiddlewaresToHandler(
				http.HandlerFunc(s.GetUserProfileHandler),
				s.AuthMiddleware.MiddlewareAuthHandler,
			),
		},
		{
			Name:        "Set PIN",
			Method:      "POST",
			Pattern:     "/api/v1/auth/pin",
			HandlerFunc: s.AddMiddlewaresToHandler(http.HandlerFunc(s.SetPinHandlerV1), s.AuthMiddleware.MiddlewareAuthHandler),
		},
		{
			Name:        "Webhook Trigger",
			Method:      "POST",
			Pattern:     "/api/v1/webhooks/paystack",
			HandlerFunc: http.HandlerFunc(s.PaystackWebhookHandler),
		},
		{
			Name:        "Verify Payment",
			Method:      "GET",
			Pattern:     "/api/v1/transaction/verify",
			HandlerFunc: s.AddMiddlewaresToHandler(http.HandlerFunc(s.VerifyPaymentHandler), s.AuthMiddleware.MiddlewareAuthHandler),
		},
		{
			Name:        "initiate paystack",
			Method:      "POST",
			Pattern:     "/api/v1/paystack/initiate",
			HandlerFunc: s.AddMiddlewaresToHandler(http.HandlerFunc(s.InitiatePaymentHandler), s.AuthMiddleware.MiddlewareAuthHandler),
		},
		{
			Name:        "GET Banks",
			Method:      "GET",
			Pattern:     "/api/v1/banks",
			HandlerFunc: s.AddMiddlewaresToHandler(http.HandlerFunc(s.GetBanksDetailHandler), s.AuthMiddleware.MiddlewareAuthHandler),
		},
		{
			Name:        "GET Receipient detail",
			Method:      "POST",
			Pattern:     "/api/v1/banks/resolve",
			HandlerFunc: s.AddMiddlewaresToHandler(http.HandlerFunc(s.ResolveAccountDetailsHandler), s.AuthMiddleware.MiddlewareAuthHandler),
		},
		{
			Name:        "withdraw funds",
			Method:      "POST",
			Pattern:     "/api/v1/withdraw",
			HandlerFunc: s.AddMiddlewaresToHandler(http.HandlerFunc(s.WithdrawalHandler), s.AuthMiddleware.MiddlewareAuthHandler, s.RateLimit(5, time.Minute)),
		},
		{
			Name:        "rotate Refresh token",
			Method:      "POST",
			Pattern:     "/api/v1/auth/refresh",
			HandlerFunc: http.HandlerFunc(s.RefreshTokenHandler),
		},
		{
			Name:        "Look Up user",
			Method:      "GET",
			Pattern:     "/api/v1/users/lookup",
			HandlerFunc: http.HandlerFunc(s.LookupUserHandler),
		},
	}

	for _, route := range routes {
		handler := Logger(s.Logger, route.HandlerFunc, route.Name)
		switch route.Method {
		case "GET":
			s.Router.Get(route.Pattern, handler.ServeHTTP)
		case "POST":
			s.Router.Post(route.Pattern, handler.ServeHTTP)
		case "PUT":
			s.Router.Put(route.Pattern, handler.ServeHTTP)
		case "DELETE":
			s.Router.Delete(route.Pattern, handler.ServeHTTP)
		}
	}
}
