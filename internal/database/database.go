// Package database mengelola koneksi (PostgreSQL & SQLite in-memory untuk test),
// model GORM, AutoMigrate, dan Seeder akun bawaan.
package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
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
	// PreferSimpleProtocol dibutuhkan agar kompatibel dengan PgBouncer (mode
	// transaction pooling) yang dipakai platform deploy: tanpa prepared statement.
	openDialector = func(dsn string) gorm.Dialector {
		return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
	}
	openMySQL = func(dsn string) gorm.Dialector {
		return mysql.Open(dsn)
	}
	openSQLite = func(cfg *gorm.Config) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open(":memory:"), cfg)
	}
)

// mysqlDSN membentuk DSN MySQL/MariaDB (utf8mb4, parseTime) dengan escaping aman.
func mysqlDSN(cfg *config.Config) string {
	return (&mysqldriver.Config{
		User:                 cfg.DBUsername,
		Passwd:               cfg.DBPassword,
		Net:                  "tcp",
		Addr:                 cfg.DBHost + ":" + cfg.DBPort,
		DBName:               cfg.DBDatabase,
		ParseTime:            true,
		Loc:                  time.Local,
		AllowNativePasswords: true,
		Params:               map[string]string{"charset": "utf8mb4"},
	}).FormatDSN()
}

// Connect membuka koneksi ke PostgreSQL (default) atau MySQL/MariaDB sesuai DB_CONNECTION.
func Connect(cfg *config.Config) (*gorm.DB, error) {
	engine := "PostgreSQL"
	dialector := openDialector(cfg.DSN())
	if cfg.UsesMySQL() {
		engine = "MySQL"
		dialector = openMySQL(mysqlDSN(cfg))
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("gagal terhubung ke %s: %w", engine, err)
	}
	return db, nil
}

// EnsureDatabase membuat database target bila belum ada, dengan terhubung ke
// database pemeliharaan "postgres" memakai kredensial yang sama. AutoMigrate
// hanya membuat tabel, bukan database, sehingga langkah ini mencegah
// 'database does not exist' saat deploy ke server baru.
func EnsureDatabase(cfg *config.Config) error {
	maintenance := *cfg
	maintenance.DBDatabase = "postgres"

	db, err := gorm.Open(openDialector(maintenance.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("gagal terhubung ke database pemeliharaan: %w", err)
	}
	// Error db.DB() tidak mungkin terjadi setelah Open sukses.
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM pg_database WHERE datname = ?", cfg.DBDatabase).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	quoted := `"` + strings.ReplaceAll(cfg.DBDatabase, `"`, `""`) + `"`
	return db.Exec("CREATE DATABASE " + quoted).Error
}

// ConnectOrCreate terhubung ke database target. Hanya bila gagal, mencoba membuat
// database tersebut (EnsureDatabase) lalu terhubung ulang. Pada database terkelola
// (mis. PgBouncer milik platform) koneksi pertama langsung berhasil sehingga
// database pemeliharaan "postgres" tidak pernah disentuh.
func ConnectOrCreate(cfg *config.Config) (*gorm.DB, error) {
	db, err := Connect(cfg)
	// MySQL/MariaDB terkelola selalu menyediakan databasenya; pembuatan otomatis
	// hanya relevan untuk PostgreSQL.
	if err == nil || cfg.UsesMySQL() {
		return db, err
	}
	if ensureErr := EnsureDatabase(cfg); ensureErr != nil {
		return nil, fmt.Errorf("%w (pembuatan database otomatis juga gagal: %v)", err, ensureErr)
	}
	return Connect(cfg)
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

// seedUser membuat akun bila belum ada. Bila akun sudah ada, role-nya dipastikan
// sesuai, dan untuk admin password-nya disamakan dengan konfigurasi.
func seedUser(db *gorm.DB, username, password, role string) error {
	var existing User
	res := db.Where("username = ?", username).Limit(1).Find(&existing)
	if res.Error != nil {
		return res.Error
	}

	if res.RowsAffected > 0 {
		updates := map[string]any{}
		if existing.Role != role {
			updates["role"] = role
		}
		if role == RoleAdmin && bcrypt.CompareHashAndPassword([]byte(existing.Password), []byte(password)) != nil {
			hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			updates["password"] = string(hash)
		}
		if len(updates) == 0 {
			return nil
		}
		return db.Model(&existing).Updates(updates).Error
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
