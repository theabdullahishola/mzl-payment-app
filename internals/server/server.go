package server

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/theabdullahishola/mzl-payment-app/internals/config"
	"github.com/theabdullahishola/mzl-payment-app/internals/middlewares"
	"github.com/theabdullahishola/mzl-payment-app/internals/pkg"
	"github.com/theabdullahishola/mzl-payment-app/internals/repository"
	"github.com/theabdullahishola/mzl-payment-app/internals/service"
	"github.com/theabdullahishola/mzl-payment-app/prisma/db"
)

type Server struct {
	Logger         *slog.Logger
	Router         *chi.Mux
	Config         *config.Config
	DB             *db.PrismaClient
	AuthService    service.AuthService
	WalletService  service.WalletService
	AuthMiddleware middlewares.AuthMiddleware
	PaymentService service.PaymentService
	RedisSvc       service.QueueService
}

func New(cfg *config.Config, dbClient *db.PrismaClient) *Server {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.AllowContentType("application/json"))
	r.Use(middleware.Timeout(60 * time.Second))
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	userRepo := repository.NewUserRepository(dbClient)
	walletrepo := repository.NewWalletRepository(dbClient)

	paystack := service.NewClient(config.Load().PAYSTACK_SECRET_KEY)
	authmid := middlewares.NewAuthMiddleware(cfg)
	redisSvc, err := pkg.NewRedisQueue(config.Load().REDIS_URL, config.Load().REDIS_KEY,logger)
	if err != nil {
		return nil
	}

	authSvc := service.NewAuthService(userRepo, cfg, redisSvc)
	paymentsvc := service.NewPaymentService(walletrepo, *paystack, redisSvc, userRepo)
	redisSvc.SetPaymentService(paymentsvc)
	walletsvc := service.NewWalletService(walletrepo, paymentsvc, userRepo, redisSvc)

	s := &Server{
		Logger:         logger,
		Router:         r,
		Config:         cfg,
		DB:             dbClient,
		AuthService:    authSvc,
		WalletService:  walletsvc,
		AuthMiddleware: *authmid,
		PaymentService: paymentsvc,
		RedisSvc:       redisSvc,
	}
	go redisSvc.StartWorker(context.Background(), "payment_webhooks")
	s.registerRoutes()

	return s
}
func (s *Server) Start() {
	s.Logger.Info("server started", "port", s.Config.Port)
	handlerWithCORS := s.CORSMiddleware(s.Router)
	srv := &http.Server{
		Addr:         ":" + s.Config.Port,
		Handler:      handlerWithCORS,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.gracefulShutdown(srv)
}

func (s *Server) gracefulShutdown(srv *http.Server) {

	go func() {
		log.Printf("Server starting on port %s", s.Config.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server startup failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	s.closeDB()

	log.Println("Server exited properly")
}

func (s *Server) closeDB() {
	log.Println("Closing database connection...")
	if err := s.DB.Prisma.Disconnect(); err != nil {
		log.Printf("Error closing DB: %v", err)
	}
}
func (s *Server) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")

		w.Header().Set("Access-Control-Allow-Credentials", "true")

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Idempotency-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			
			s.AuthMiddleware.RateLimitHandler(next, &pkg.RedisQueue{}, limit, window).ServeHTTP(w, r)
		})
	}
}