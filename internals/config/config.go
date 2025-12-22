package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                string
	DatabaseURL         string
	JWTSecret           string
	PAYSTACK_SECRET_KEY string
	REDIS_URL string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("Info: No .env file found, relying on system environment")
	}

	return &Config{
		Port:                getEnv("PORT", "8080"),
		DatabaseURL:         getEnv("DATABASE_URL", ""),
		JWTSecret:           getEnv("JWT_ACCESS_SECRET", "super-secret"),
		PAYSTACK_SECRET_KEY: getEnv("PAYSTACK_SECRET_KEY", ""),
		REDIS_URL:getEnv("REDIS_URL",""),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
