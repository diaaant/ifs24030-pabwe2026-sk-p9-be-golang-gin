package post

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"app/internal/database"
)

// Repository adalah akses data artikel. Method pencarian satuan mengembalikan
// (nil, nil) bila data tidak ditemukan.
type Repository interface {
	Create(post *database.Post) error
	Update(post *database.Post) error
	Delete(post *database.Post) error
	SlugExists(slug string) (bool, error)
	FindByID(id uint) (*database.Post, error)
	FindByIDAndUser(id, userID uint) (*database.Post, error)
	FindPublishedBySlug(slug string) (*database.Post, error)
	ListPublished(query, category string) ([]database.Post, error)
	ListByUser(userID uint) ([]database.Post, error)
	ListAll() ([]database.Post, error)
	CountByUser(userID uint) (total, published int64, err error)
	ListUsersWithCounts() ([]UserSummary, error)
	UserExists(id uint) (bool, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository membuat Repository berbasis GORM.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(post *database.Post) error {
	return r.db.Omit(clause.Associations).Create(post).Error
}

func (r *gormRepository) Update(post *database.Post) error {
	return r.db.Omit(clause.Associations).Save(post).Error
}

func (r *gormRepository) Delete(post *database.Post) error {
	return r.db.Delete(post).Error
}

func (r *gormRepository) SlugExists(slug string) (bool, error) {
	var count int64
	err := r.db.Model(&database.Post{}).Where("slug = ?", slug).Count(&count).Error
	return count > 0, err
}

func (r *gormRepository) FindByID(id uint) (*database.Post, error) {
	return findOne(r.db.Where("id = ?", id))
}

func (r *gormRepository) FindByIDAndUser(id, userID uint) (*database.Post, error) {
	return findOne(r.db.Where("id = ? AND user_id = ?", id, userID))
}

func (r *gormRepository) FindPublishedBySlug(slug string) (*database.Post, error) {
	return findOne(r.db.Preload("User").Where("slug = ? AND is_published = ?", slug, true))
}

func (r *gormRepository) ListPublished(query, category string) ([]database.Post, error) {
	tx := r.db.Preload("User").Where("is_published = ?", true)
	if category != "" {
		tx = tx.Where("category = ?", category)
	}
	if query != "" {
		like := "%" + strings.ToLower(query) + "%"
		tx = tx.Where("LOWER(title) LIKE ? OR LOWER(excerpt) LIKE ? OR LOWER(content) LIKE ?", like, like, like)
	}
	var posts []database.Post
	err := tx.Order("published_at DESC, id DESC").Find(&posts).Error
	return posts, err
}

func (r *gormRepository) ListByUser(userID uint) ([]database.Post, error) {
	var posts []database.Post
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC, id DESC").Find(&posts).Error
	return posts, err
}

func (r *gormRepository) ListAll() ([]database.Post, error) {
	var posts []database.Post
	err := r.db.Preload("User").Order("created_at DESC, id DESC").Find(&posts).Error
	return posts, err
}

func (r *gormRepository) CountByUser(userID uint) (int64, int64, error) {
	var row struct {
		Total     int64
		Published int64
	}
	err := r.db.Model(&database.Post{}).
		Select("COUNT(*) AS total, COALESCE(SUM(CASE WHEN is_published THEN 1 ELSE 0 END), 0) AS published").
		Where("user_id = ?", userID).
		Scan(&row).Error
	return row.Total, row.Published, err
}

func (r *gormRepository) ListUsersWithCounts() ([]UserSummary, error) {
	var rows []UserSummary
	err := r.db.Table("users").
		Select("users.id, users.username, users.email, users.role, " +
			"COUNT(posts.id) AS total_posts, " +
			"COALESCE(SUM(CASE WHEN posts.is_published THEN 1 ELSE 0 END), 0) AS published_posts").
		Joins("LEFT JOIN posts ON posts.user_id = users.id").
		Group("users.id, users.username, users.email, users.role").
		Order("users.id").
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) UserExists(id uint) (bool, error) {
	var count int64
	err := r.db.Model(&database.User{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

// findOne memakai Limit(1).Find agar "tidak ditemukan" tidak dicatat sebagai error GORM.
func findOne(tx *gorm.DB) (*database.Post, error) {
	var posts []database.Post
	if err := tx.Limit(1).Find(&posts).Error; err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return nil, nil
	}
	return &posts[0], nil
}