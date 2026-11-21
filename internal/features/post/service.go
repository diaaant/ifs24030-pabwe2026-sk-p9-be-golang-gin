package post

import (
	"fmt"
	"strings"
	"time"

	"app/internal/database"
)

// Service berisi logika bisnis artikel.
type Service struct {
	repo Repository
}

// NewService membuat Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ---------- publik ----------

// ListPublic mengambil artikel terbit dengan pencarian kata kunci & filter kategori.
func (s *Service) ListPublic(query, category string) ([]PostResponse, error) {
	posts, err := s.repo.ListPublished(strings.TrimSpace(query), strings.TrimSpace(category))
	if err != nil {
		return nil, err
	}
	return toResponses(posts), nil
}

// GetPublic mengambil satu artikel terbit berdasarkan slug.
func (s *Service) GetPublic(slug string) (*PostResponse, error) {
	post, err := s.repo.FindPublishedBySlug(slug)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, ErrPostNotFound
	}
	resp := toResponse(post, true)
	return &resp, nil
}

// ---------- penulis ----------

// ListMine mengambil seluruh artikel milik penulis (terbit maupun draf).
func (s *Service) ListMine(userID uint) ([]PostResponse, error) {
	posts, err := s.repo.ListByUser(userID)
	if err != nil {
		return nil, err
	}
	return toResponses(posts), nil
}

// Stats menghitung ringkasan 3 kartu statistik penulis.
func (s *Service) Stats(userID uint) (*StatsResponse, error) {
	total, published, err := s.repo.CountByUser(userID)
	if err != nil {
		return nil, err
	}
	return &StatsResponse{Total: total, TotalTerbit: published, TotalDraf: total - published}, nil
}

// GetMine mengambil satu artikel milik penulis (lengkap dengan konten).
func (s *Service) GetMine(userID, id uint) (*PostResponse, error) {
	post, err := s.mine(userID, id)
	if err != nil {
		return nil, err
	}
	resp := toResponse(post, true)
	return &resp, nil
}

// Create membuat artikel baru dengan slug unik.
func (s *Service) Create(userID uint, req PostRequest) (*PostResponse, error) {
	category, err := NormalizeCategory(req.Category)
	if err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(req.Title)
	if err != nil {
		return nil, err
	}

	post := &database.Post{
		UserID:         userID,
		Title:          strings.TrimSpace(req.Title),
		Slug:           slug,
		Category:       category,
		Excerpt:        req.Excerpt,
		Content:        req.Content,
		CoverImagePath: req.CoverImagePath,
		IsPublished:    req.IsPublished,
	}
	if post.IsPublished {
		now := time.Now()
		post.PublishedAt = &now
	}
	if err := s.repo.Create(post); err != nil {
		return nil, err
	}

	resp := toResponse(post, true)
	return &resp, nil
}

// Update memperbarui artikel milik penulis. Slug tidak berubah agar tautan tetap valid.
func (s *Service) Update(userID, id uint, req PostRequest) (*PostResponse, error) {
	post, err := s.mine(userID, id)
	if err != nil {
		return nil, err
	}
	category, err := NormalizeCategory(req.Category)
	if err != nil {
		return nil, err
	}

	post.Title = strings.TrimSpace(req.Title)
	post.Category = category
	post.Excerpt = req.Excerpt
	post.Content = req.Content
	post.CoverImagePath = req.CoverImagePath
	if err := s.repo.Update(post); err != nil {
		return nil, err
	}

	resp := toResponse(post, true)
	return &resp, nil
}

// TogglePublish membalik status Draf <-> Terbit beserta published_at.
func (s *Service) TogglePublish(userID, id uint) (*PostResponse, error) {
	post, err := s.mine(userID, id)
	if err != nil {
		return nil, err
	}

	post.IsPublished = !post.IsPublished
	if post.IsPublished {
		now := time.Now()
		post.PublishedAt = &now
	} else {
		post.PublishedAt = nil
	}
	if err := s.repo.Update(post); err != nil {
		return nil, err
	}

	resp := toResponse(post, true)
	return &resp, nil
}

// Delete menghapus artikel milik penulis.
func (s *Service) Delete(userID, id uint) error {
	post, err := s.mine(userID, id)
	if err != nil {
		return err
	}
	return s.repo.Delete(post)
}

// ---------- admin ----------

// AdminUsers mengambil seluruh user beserta jumlah artikelnya.
func (s *Service) AdminUsers() ([]UserSummary, error) {
	return s.repo.ListUsersWithCounts()
}

// AdminPosts mengambil arsip seluruh artikel dari semua penulis.
func (s *Service) AdminPosts() ([]PostResponse, error) {
	posts, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}
	return toResponses(posts), nil
}

// AdminUserPosts mengambil seluruh artikel milik satu user.
func (s *Service) AdminUserPosts(userID uint) ([]PostResponse, error) {
	exists, err := s.repo.UserExists(userID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrUserNotFound
	}
	return s.ListMine(userID)
}

// ---------- helper ----------

func (s *Service) mine(userID, id uint) (*database.Post, error) {
	post, err := s.repo.FindByIDAndUser(id, userID)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, ErrPostNotFound
	}
	return post, nil
}

// uniqueSlug membuat slug dari judul; bila sudah dipakai ditambah akhiran -2, -3, dst.
func (s *Service) uniqueSlug(title string) (string, error) {
	base := Slugify(title)
	slug := base
	for i := 2; ; i++ {
		exists, err := s.repo.SlugExists(slug)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}
