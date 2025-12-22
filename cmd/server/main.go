package main

import (
	"log"

	"github.com/theabdullahishola/mzl-payment-app/internals/config"
	"github.com/theabdullahishola/mzl-payment-app/internals/server"
	"github.com/theabdullahishola/mzl-payment-app/prisma/db"
)

func main() {

	cfg := config.Load()

	client := db.NewClient()
	if err := client.Prisma.Connect(); err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}

	srv := server.New(cfg, client)

	srv.Start()
}
