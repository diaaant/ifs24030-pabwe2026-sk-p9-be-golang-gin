// Package config memuat konfigurasi aplikasi dari environment / berkas .env.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config menampung seluruh konfigurasi runtime aplikasi.
type Config struct {
	AppEnv  string
	AppPort string
	AppKey  string

	AdminUsername string
	AdminPassword string

	DBConnection string
	DBHost       string
	DBPort       string
	DBDatabase   string
	DBUsername   string
	DBPassword   string
	DBSSLMode    string

	SearchEngineEndpoint string

	OpenAIEndpoint string
	OpenAIKey      string
	OpenAIModel    string
}

// LoadConfig membaca berkas .env (jika ada) lalu variabel lingkungan,
// dengan nilai default sebagai fallback. Variabel OS selalu diprioritaskan
// di atas isi berkas .env.
func LoadConfig() *Config {
	// Berkas .env bersifat opsional (mis. di CI/produksi tidak ada).
	_ = godotenv.Load()

	return &Config{
		AppEnv:  getEnv("APP_ENV", "development"),
		AppPort: firstEnv("8080", "APP_PORT", "PORT"),
		AppKey:  firstEnv("delcom_blog_secret_key_super_secure_2026_random_string_32chars", "JWT_SECRET", "APP_KEY"),

		AdminUsername: getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin"),

		DBConnection: getEnv("DB_CONNECTION", "pgsql"),
		DBHost:       getEnv("DB_HOST", "127.0.0.1"),
		DBPort:       getEnv("DB_PORT", "5432"),
		DBDatabase:   getEnv("DB_DATABASE", "pabwe_p9_blog"),
		DBUsername:   getEnv("DB_USERNAME", "postgres"),
		DBPassword:   getEnv("DB_PASSWORD", "postgres"),
		DBSSLMode:    getEnv("DB_SSLMODE", "disable"),

		SearchEngineEndpoint: getEnv("SEARCH_ENGINE_ENDPOINT", "https://delcom.org/api/v1/search"),

		OpenAIEndpoint: getEnv("OPENAI_ENDPOINT", "https://delcom.org/api/v1"),
		OpenAIKey:      getEnv("OPENAI_KEY", "key"),
		OpenAIModel:    getEnv("OPENAI_MODEL", "nama-model"),
	}
}

// DSN membentuk connection string PostgreSQL (format key=value).
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Jakarta",
		c.DBHost, c.DBUsername, c.DBPassword, c.DBDatabase, c.DBPort, c.DBSSLMode,
	)
}

// UsesMySQL true bila DB_CONNECTION bernilai mysql atau mariadb (tidak peka huruf besar/kecil).
func (c *Config) UsesMySQL() bool {
	driver := strings.ToLower(strings.TrimSpace(c.DBConnection))
	return driver == "mysql" || driver == "mariadb"
}

// getEnv mengembalikan nilai variabel lingkungan, atau fallback jika kosong/tidak ada.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

// firstEnv mengembalikan nilai variabel pertama yang terisi dari daftar keys
// (mis. APP_PORT lalu PORT), atau fallback bila semuanya kosong. Berguna agar
// aplikasi mengikuti variabel lingkungan yang disediakan platform deploy.
func firstEnv(fallback string, keys ...string) string {
	for _, key := range keys {
		if value := getEnv(key, ""); value != "" {
			return value
		}
	}
	return fallback
}
