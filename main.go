// Command server menjalankan REST API Delcom Blog.
package main

import (
	"log"
	"os"

	"app/internal/config"
	"app/internal/database"
	"app/internal/server"
)

func main() {
	cfg := config.LoadConfig()

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := database.Seed(db, cfg); err != nil {
		log.Fatalf("seed: %v", err)
	}

	port := cfg.AppPort
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	router := server.NewRouter(db, cfg)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("server: %v", err)
	}
}
