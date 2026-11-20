package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var envKeys = []string{
	"APP_ENV", "APP_PORT", "PORT", "APP_KEY", "JWT_SECRET",
	"ADMIN_USERNAME", "ADMIN_PASSWORD",
	"DB_CONNECTION", "DB_HOST", "DB_PORT", "DB_DATABASE",
	"DB_USERNAME", "DB_PASSWORD", "DB_SSLMODE",
	"SEARCH_ENGINE_ENDPOINT",
	"OPENAI_ENDPOINT", "OPENAI_KEY", "OPENAI_MODEL",
}

// clearEnv menghapus seluruh variabel yang dipakai Config dan memulihkannya
// kembali (ada/tidak ada) saat test selesai, sehingga test saling terisolasi.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range envKeys {
		original, existed := os.LookupEnv(key)
		require.NoError(t, os.Unsetenv(key))
		k, v, e := key, original, existed
		t.Cleanup(func() {
			if e {
				_ = os.Setenv(k, v)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	clearEnv(t)
	t.Chdir(t.TempDir()) // tidak ada berkas .env

	cfg := LoadConfig()

	assert.Equal(t, "development", cfg.AppEnv)
	assert.Equal(t, "8080", cfg.AppPort)
	assert.Equal(t, "delcom_blog_secret_key_super_secure_2026_random_string_32chars", cfg.AppKey)
	assert.Equal(t, "admin", cfg.AdminUsername)
	assert.Equal(t, "admin", cfg.AdminPassword)
	assert.Equal(t, "pgsql", cfg.DBConnection)
	assert.Equal(t, "127.0.0.1", cfg.DBHost)
	assert.Equal(t, "5432", cfg.DBPort)
	assert.Equal(t, "pabwe_p9_blog", cfg.DBDatabase)
	assert.Equal(t, "postgres", cfg.DBUsername)
	assert.Equal(t, "postgres", cfg.DBPassword)
	assert.Equal(t, "disable", cfg.DBSSLMode)
	assert.Equal(t, "https://delcom.org/api/v1/search", cfg.SearchEngineEndpoint)
	assert.Equal(t, "https://delcom.org/api/v1", cfg.OpenAIEndpoint)
	assert.Equal(t, "key", cfg.OpenAIKey)
	assert.Equal(t, "nama-model", cfg.OpenAIModel)
}

func TestLoadConfig_FromEnvironment(t *testing.T) {
	clearEnv(t)
	t.Chdir(t.TempDir())

	values := map[string]string{
		"APP_ENV":                "production",
		"APP_PORT":               "9090",
		"APP_KEY":                "secret",
		"ADMIN_USERNAME":         "root",
		"ADMIN_PASSWORD":         "toor",
		"DB_CONNECTION":          "pgsql",
		"DB_HOST":                "db.local",
		"DB_PORT":                "5433",
		"DB_DATABASE":            "mydb",
		"DB_USERNAME":            "user",
		"DB_PASSWORD":            "pass",
		"DB_SSLMODE":             "require",
		"SEARCH_ENGINE_ENDPOINT": "https://search.example.com",
		"OPENAI_ENDPOINT":        "https://example.com/v1",
		"OPENAI_KEY":             "abc",
		"OPENAI_MODEL":           "gpt-x",
	}
	for k, v := range values {
		require.NoError(t, os.Setenv(k, v))
	}

	cfg := LoadConfig()

	assert.Equal(t, "production", cfg.AppEnv)
	assert.Equal(t, "9090", cfg.AppPort)
	assert.Equal(t, "secret", cfg.AppKey)
	assert.Equal(t, "root", cfg.AdminUsername)
	assert.Equal(t, "toor", cfg.AdminPassword)
	assert.Equal(t, "pgsql", cfg.DBConnection)
	assert.Equal(t, "db.local", cfg.DBHost)
	assert.Equal(t, "5433", cfg.DBPort)
	assert.Equal(t, "mydb", cfg.DBDatabase)
	assert.Equal(t, "user", cfg.DBUsername)
	assert.Equal(t, "pass", cfg.DBPassword)
	assert.Equal(t, "require", cfg.DBSSLMode)
	assert.Equal(t, "https://search.example.com", cfg.SearchEngineEndpoint)
	assert.Equal(t, "https://example.com/v1", cfg.OpenAIEndpoint)
	assert.Equal(t, "abc", cfg.OpenAIKey)
	assert.Equal(t, "gpt-x", cfg.OpenAIModel)
}

func TestLoadConfig_FromDotEnvFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	t.Chdir(dir)

	content := "APP_PORT=7777\nDB_HOST=from-dotenv\nSEARCH_ENGINE_ENDPOINT=https://dotenv.test/search\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600))

	cfg := LoadConfig()

	assert.Equal(t, "7777", cfg.AppPort)
	assert.Equal(t, "from-dotenv", cfg.DBHost)
	assert.Equal(t, "https://dotenv.test/search", cfg.SearchEngineEndpoint)
	assert.Equal(t, "development", cfg.AppEnv) // tidak ada di .env -> default
}

func TestLoadConfig_EmptyValueUsesFallback(t *testing.T) {
	clearEnv(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.Setenv("APP_PORT", ""))

	cfg := LoadConfig()

	assert.Equal(t, "8080", cfg.AppPort)
}

func TestConfig_DSN(t *testing.T) {
	cfg := &Config{
		DBHost:     "localhost",
		DBUsername: "postgres",
		DBPassword: "secret",
		DBDatabase: "blog",
		DBPort:     "5432",
		DBSSLMode:  "disable",
	}

	assert.Equal(t,
		// PERUBAHAN ADA DI BARIS DI BAWAH INI (default_query_exec_mode=simple_protocol)
		"host=localhost user=postgres password=secret dbname=blog port=5432 sslmode=disable TimeZone=Asia/Jakarta default_query_exec_mode=simple_protocol",
		cfg.DSN(),
	)
}

func TestGetEnv(t *testing.T) {
	clearEnv(t)

	assert.Equal(t, "fallback", getEnv("APP_ENV", "fallback"))

	require.NoError(t, os.Setenv("APP_ENV", "value"))
	assert.Equal(t, "value", getEnv("APP_ENV", "fallback"))
}

func TestLoadConfig_PlatformVariables(t *testing.T) {
	clearEnv(t)
	t.Chdir(t.TempDir())

	// Hanya PORT yang tersedia.
	require.NoError(t, os.Setenv("PORT", "7202"))
	assert.Equal(t, "7202", LoadConfig().AppPort)

	// APP_PORT diprioritaskan di atas PORT.
	require.NoError(t, os.Setenv("APP_PORT", "9000"))
	assert.Equal(t, "9000", LoadConfig().AppPort)

	// JWT_SECRET diprioritaskan di atas APP_KEY.
	require.NoError(t, os.Setenv("APP_KEY", "base64:abc="))
	assert.Equal(t, "base64:abc=", LoadConfig().AppKey)
	require.NoError(t, os.Setenv("JWT_SECRET", "jwt-secret"))
	assert.Equal(t, "jwt-secret", LoadConfig().AppKey)
}

func TestConfig_UsesMySQL(t *testing.T) {
	for driver, want := range map[string]bool{
		"mysql": true, "MySQL": true, " mariadb ": true, "pgsql": false, "": false,
	} {
		assert.Equal(t, want, (&Config{DBConnection: driver}).UsesMySQL(), driver)
	}
}