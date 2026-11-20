// Package post menangani artikel blog: CRUD, slug unik, estimasi waktu baca,
// statistik penulis, publikasi publik, dan monitoring admin.
package post

import (
	"errors"
	"strings"
	"time"

	"app/internal/database"
)

// Categories adalah daftar kategori artikel yang valid.
var Categories = []string{"Teknologi", "Pendidikan", "Gaya Hidup", "Bisnis", "Opini", "Umum"}

// Error domain yang dipetakan handler ke status HTTP.
var (
	ErrPostNotFound    = errors.New("post not found")
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidCategory = errors.New("invalid category, must be one of: " + strings.Join(Categories, ", "))
)

// PostRequest adalah payload membuat / memperbarui artikel.
type PostRequest struct {
	Title          string `json:"title" binding:"required,max=255"`
	Category       string `json:"category" binding:"omitempty,max=100"`
	Excerpt        string `json:"excerpt"`
	Content        string `json:"content" binding:"required"`
	CoverImagePath string `json:"cover_image_path"`
	IsPublished    bool   `json:"is_published"`
}

// AuthorResponse adalah ringkasan penulis artikel.
type AuthorResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

// PostResponse adalah representasi artikel untuk klien. Content hanya
// disertakan pada respons detail.
type PostResponse struct {
	ID             uint            `json:"id"`
	UserID         uint            `json:"user_id"`
	Title          string          `json:"title"`
	Slug           string          `json:"slug"`
	Category       string          `json:"category"`
	Excerpt        string          `json:"excerpt"`
	Content        string          `json:"content,omitempty"`
	CoverImagePath string          `json:"cover_image_path"`
	IsPublished    bool            `json:"is_published"`
	PublishedAt    *time.Time      `json:"published_at"`
	ReadingTime    int             `json:"reading_time"`
	Author         *AuthorResponse `json:"author,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// StatsResponse adalah ringkasan 3 kartu statistik penulis.
type StatsResponse struct {
	Total       int64 `json:"total"`
	TotalTerbit int64 `json:"total_terbit"`
	TotalDraf   int64 `json:"total_draf"`
}

// UserSummary adalah ringkasan user untuk monitoring admin.
type UserSummary struct {
	ID             uint   `json:"id"`
	Username       string `json:"username"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	TotalPosts     int64  `json:"total_posts"`
	PublishedPosts int64  `json:"published_posts"`
}

// NormalizeCategory memvalidasi kategori (tanpa membedakan huruf besar/kecil).
// Kategori kosong menghasilkan kategori bawaan "Umum".
func NormalizeCategory(category string) (string, error) {
	category = strings.TrimSpace(category)
	if category == "" {
		return database.DefaultCategory, nil
	}
	for _, valid := range Categories {
		if strings.EqualFold(category, valid) {
			return valid, nil
		}
	}
	return "", ErrInvalidCategory
}

// ReadingTime menghitung estimasi menit baca (200 kata per menit, minimal 1).
func ReadingTime(content string) int {
	words := len(strings.Fields(content))
	minutes := (words + 199) / 200
	if minutes < 1 {
		return 1
	}
	return minutes
}

// Slugify mengubah judul menjadi slug URL (a-z, 0-9, dipisah tanda hubung).
func Slugify(title string) string {
	var b strings.Builder
	lastDash := true // mencegah tanda hubung di awal
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.TrimSuffix(b.String(), "-")
	if slug == "" {
		return "post"
	}
	return slug
}

func toResponse(p *database.Post, withContent bool) PostResponse {
	resp := PostResponse{
		ID:             p.ID,
		UserID:         p.UserID,
		Title:          p.Title,
		Slug:           p.Slug,
		Category:       p.Category,
		Excerpt:        p.Excerpt,
		CoverImagePath: p.CoverImagePath,
		IsPublished:    p.IsPublished,
		PublishedAt:    p.PublishedAt,
		ReadingTime:    ReadingTime(p.Content),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
	if withContent {
		resp.Content = p.Content
	}
	if p.User != nil {
		resp.Author = &AuthorResponse{ID: p.User.ID, Username: p.User.Username}
	}
	return resp
}

func toResponses(posts []database.Post) []PostResponse {
	out := make([]PostResponse, 0, len(posts))
	for i := range posts {
		out = append(out, toResponse(&posts[i], false))
	}
	return out
}
