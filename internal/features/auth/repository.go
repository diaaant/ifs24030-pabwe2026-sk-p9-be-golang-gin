package auth

import (
	"gorm.io/gorm"

	"app/internal/database"
)

// Repository adalah akses data user. Method pencarian mengembalikan (nil, nil)
// bila data tidak ditemukan.
type Repository interface {
	Create(user *database.User) error
	Update(user *database.User) error
	FindByID(id uint) (*database.User, error)
	FindByUsername(username string) (*database.User, error)
	FindByEmail(email string) (*database.User, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository membuat Repository berbasis GORM.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(user *database.User) error {
	return r.db.Create(user).Error
}

func (r *gormRepository) Update(user *database.User) error {
	return r.db.Save(user).Error
}

func (r *gormRepository) FindByID(id uint) (*database.User, error) {
	return r.first("id = ?", id)
}

func (r *gormRepository) FindByUsername(username string) (*database.User, error) {
	return r.first("username = ?", username)
}

func (r *gormRepository) FindByEmail(email string) (*database.User, error) {
	return r.first("email = ?", email)
}

// first memakai Limit(1).Find agar "tidak ditemukan" tidak dicatat sebagai error GORM.
func (r *gormRepository) first(query string, args ...any) (*database.User, error) {
	var users []database.User
	if err := r.db.Where(query, args...).Limit(1).Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &users[0], nil
}
