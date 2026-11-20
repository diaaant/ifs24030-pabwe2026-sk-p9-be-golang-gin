// Command server menjalankan REST API Delcom Blog.
package main

import (
	"log"

	"app/internal/config"
	"app/internal/database"
	"app/internal/server"
)

func main() {
	cfg := config.LoadConfig()

	// Terhubung ke database; bila gagal, coba buat database lalu terhubung ulang.
	db, err := database.ConnectOrCreate(cfg)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := database.Seed(db, cfg); err != nil {
		log.Fatalf("seed: %v", err)
	}

	router := server.NewRouter(db, cfg)
	if err := router.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("server: %v", err)
	}
}
