// Package database mengelola koneksi (PostgreSQL & SQLite in-memory untuk test),
// model GORM, AutoMigrate, dan Seeder akun bawaan.
package database

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"app/internal/config"
)

// Nilai role yang valid.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// DefaultCategory adalah kategori bawaan artikel.
const DefaultCategory = "Umum"

// User adalah akun pengguna / penulis (tabel users).
type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"size:255;not null;uniqueIndex" json:"username"`
	Email     string    `gorm:"size:255;not null;uniqueIndex" json:"email"`
	Password  string    `gorm:"type:text;not null" json:"-"`
	Role      string    `gorm:"size:50;not null;default:member" json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Post adalah artikel blog (tabel posts).
type Post struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	UserID         uint       `gorm:"not null;index" json:"user_id"`
	User           *User      `gorm:"constraint:OnDelete:CASCADE" json:"user,omitempty"`
	Title          string     `gorm:"size:255;not null" json:"title"`
	Slug           string     `gorm:"size:255;not null;uniqueIndex" json:"slug"`
	Category       string     `gorm:"size:100;not null;default:Umum" json:"category"`
	Excerpt        string     `gorm:"type:text" json:"excerpt"`
	Content        string     `gorm:"type:text" json:"content"`
	CoverImagePath string     `gorm:"type:text" json:"cover_image_path"`
	IsPublished    bool       `gorm:"not null;default:false" json:"is_published"`
	PublishedAt    *time.Time `json:"published_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Hook pembuka koneksi. Dibuat variabel agar dapat diganti saat pengujian
// sehingga jalur sukses Connect() bisa diuji tanpa server PostgreSQL.
var (
	openDialector = func(dsn string) gorm.Dialector {
		return postgres.Open(dsn)
	}
	openSQLite = func(cfg *gorm.Config) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open(":memory:"), cfg)
	}
)

// Connect membuka koneksi ke PostgreSQL berdasarkan konfigurasi.
func Connect(cfg *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(openDialector(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("gagal terhubung ke PostgreSQL: %w", err)
	}
	return db, nil
}

// ConnectSQLite membuka database SQLite in-memory yang terisolasi.
// Dipakai khusus untuk unit testing di seluruh paket.
func ConnectSQLite() (*gorm.DB, error) {
	db, err := openSQLite(&gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("gagal membuka SQLite in-memory: %w", err)
	}

	// Setiap koneksi ":memory:" adalah database terpisah, jadi pool dibatasi
	// satu koneksi. Error db.DB() tidak mungkin terjadi setelah Open sukses.
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	// SQLite tidak menegakkan foreign key secara default.
	db.Exec("PRAGMA foreign_keys = ON")

	return db, nil
}

// Migrate menjalankan AutoMigrate untuk seluruh model.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&User{}, &Post{})
}

// Seed membuat akun bawaan (idempoten): admin (dari ADMIN_USERNAME/ADMIN_PASSWORD,
// default admin/admin) dan member/member.
func Seed(db *gorm.DB, cfg *config.Config) error {
	if err := seedUser(db, cfg.AdminUsername, cfg.AdminPassword, RoleAdmin); err != nil {
		return err
	}
	return seedUser(db, "member", "member", RoleMember)
}

func seedUser(db *gorm.DB, username, password, role string) error {
	var count int64
	if err := db.Model(&User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return db.Create(&User{
		Username: username,
		Email:    username + "@delcom.local",
		Password: string(hash),
		Role:     role,
	}).Error
}
