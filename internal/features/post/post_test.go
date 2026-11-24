package post

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"app/internal/database"
	"app/internal/middleware"
)

const secret = "test-secret"

var errBoom = errors.New("boom")

func init() { gin.SetMode(gin.TestMode) }

// ---------- helpers ----------

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.ConnectSQLite()
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	return db
}

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func makeUser(t *testing.T, db *gorm.DB, username, role string) database.User {
	t.Helper()
	u := database.User{Username: username, Email: username + "@x.com", Password: "h", Role: role}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func tokenFor(t *testing.T, u database.User) string {
	t.Helper()
	claims := middleware.Claims{
		UserID: u.ID, Username: u.Username, Role: u.Role,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)
	return token
}

// failingRepo membungkus Repository nyata dan memaksa error per nama method.
type failingRepo struct {
	Repository
	fail map[string]error
}

func (f *failingRepo) Create(p *database.Post) error {
	if err := f.fail["Create"]; err != nil {
		return err
	}
	return f.Repository.Create(p)
}

func (f *failingRepo) Update(p *database.Post) error {
	if err := f.fail["Update"]; err != nil {
		return err
	}
	return f.Repository.Update(p)
}

func (f *failingRepo) Delete(p *database.Post) error {
	if err := f.fail["Delete"]; err != nil {
		return err
	}
	return f.Repository.Delete(p)
}

func (f *failingRepo) SlugExists(s string) (bool, error) {
	if err := f.fail["SlugExists"]; err != nil {
		return false, err
	}
	return f.Repository.SlugExists(s)
}

func (f *failingRepo) FindByIDAndUser(id, uid uint) (*database.Post, error) {
	if err := f.fail["FindByIDAndUser"]; err != nil {
		return nil, err
	}
	return f.Repository.FindByIDAndUser(id, uid)
}

func (f *failingRepo) ListByUser(uid uint) ([]database.Post, error) {
	if err := f.fail["ListByUser"]; err != nil {
		return nil, err
	}
	return f.Repository.ListByUser(uid)
}

func (f *failingRepo) UserExists(id uint) (bool, error) {
	if err := f.fail["UserExists"]; err != nil {
		return false, err
	}
	return f.Repository.UserExists(id)
}

// ---------- fungsi murni ----------

func TestSlugify(t *testing.T) {
	assert.Equal(t, "halo-dunia", Slugify("Halo Dunia!"))
	assert.Equal(t, "go-1-22-rocks", Slugify("  Go 1.22   Rocks "))
	assert.Equal(t, "caf", Slugify("Café"))
	assert.Equal(t, "post", Slugify("  ---  "))
	assert.Equal(t, "post", Slugify(""))
}

func TestReadingTime(t *testing.T) {
	assert.Equal(t, 1, ReadingTime(""))
	assert.Equal(t, 1, ReadingTime(strings.Repeat("kata ", 200)))
	assert.Equal(t, 2, ReadingTime(strings.Repeat("kata ", 201)))
	assert.Equal(t, 3, ReadingTime(strings.Repeat("kata ", 450)))
}

func TestNormalizeCategory(t *testing.T) {
	got, err := NormalizeCategory("")
	assert.NoError(t, err)
	assert.Equal(t, "Umum", got)

	got, err = NormalizeCategory("  teknologi ")
	assert.NoError(t, err)
	assert.Equal(t, "Teknologi", got)

	_, err = NormalizeCategory("Olahraga")
	assert.ErrorIs(t, err, ErrInvalidCategory)
}

func TestToResponse_ContentAndAuthor(t *testing.T) {
	p := &database.Post{ID: 1, Content: "isi", User: &database.User{ID: 9, Username: "ann"}}

	assert.Empty(t, toResponse(p, false).Content)
	full := toResponse(p, true)
	assert.Equal(t, "isi", full.Content)
	require.NotNil(t, full.Author)
	assert.Equal(t, "ann", full.Author.Username)
	assert.Nil(t, toResponse(&database.Post{}, true).Author)
}

// ---------- repository ----------

func TestRepository_DatabaseErrors(t *testing.T) {
	db := newDB(t)
	repo := NewRepository(db)
	closeDB(t, db)
	p := &database.Post{ID: 1}

	assert.Error(t, repo.Create(p))
	assert.Error(t, repo.Update(p))
	assert.Error(t, repo.Delete(p))
	_, err := repo.SlugExists("x")
	assert.Error(t, err)
	_, err = repo.FindByIDAndUser(1, 1)
	assert.Error(t, err)
	_, err = repo.FindPublishedBySlug("x")
	assert.Error(t, err)
	_, err = repo.ListPublished("", "")
	assert.Error(t, err)
	_, err = repo.ListByUser(1)
	assert.Error(t, err)
	_, err = repo.ListAll()
	assert.Error(t, err)
	_, _, err = repo.CountByUser(1)
	assert.Error(t, err)
	_, err = repo.ListUsersWithCounts()
	assert.Error(t, err)
	_, err = repo.UserExists(1)
	assert.Error(t, err)
}

// ---------- service ----------

func TestService_CreateAndSlugUniqueness(t *testing.T) {
	db := newDB(t)
	author := makeUser(t, db, "alice", database.RoleMember)
	svc := NewService(NewRepository(db))
	req := PostRequest{Title: "Belajar Go", Content: "isi artikel", IsPublished: true}

	first, err := svc.Create(author.ID, req)
	require.NoError(t, err)
	second, err := svc.Create(author.ID, req)
	require.NoError(t, err)
	third, err := svc.Create(author.ID, req)
	require.NoError(t, err)

	assert.Equal(t, "belajar-go", first.Slug)
	assert.Equal(t, "belajar-go-2", second.Slug)
	assert.Equal(t, "belajar-go-3", third.Slug)
	assert.Equal(t, "Umum", first.Category)
	assert.NotNil(t, first.PublishedAt)

	draft, err := svc.Create(author.ID, PostRequest{Title: "Draf", Content: "x"})
	require.NoError(t, err)
	assert.Nil(t, draft.PublishedAt)
}

func TestService_CreateErrors(t *testing.T) {
	db := newDB(t)
	author := makeUser(t, db, "alice", database.RoleMember)
	repo := NewRepository(db)
	req := PostRequest{Title: "T", Content: "c"}

	_, err := NewService(repo).Create(author.ID, PostRequest{Title: "T", Content: "c", Category: "Salah"})
	assert.ErrorIs(t, err, ErrInvalidCategory)

	_, err = NewService(&failingRepo{repo, map[string]error{"SlugExists": errBoom}}).Create(author.ID, req)
	assert.ErrorIs(t, err, errBoom)

	_, err = NewService(&failingRepo{repo, map[string]error{"Create": errBoom}}).Create(author.ID, req)
	assert.ErrorIs(t, err, errBoom)
}

func TestService_UpdateToggleDelete(t *testing.T) {
	db := newDB(t)
	author := makeUser(t, db, "alice", database.RoleMember)
	other := makeUser(t, db, "bob", database.RoleMember)
	repo := NewRepository(db)
	svc := NewService(repo)

	created, err := svc.Create(author.ID, PostRequest{Title: "Awal", Content: "isi"})
	require.NoError(t, err)

	updated, err := svc.Update(author.ID, created.ID, PostRequest{Title: " Baru ", Category: "bisnis", Content: "isi baru"})
	require.NoError(t, err)
	assert.Equal(t, "Baru", updated.Title)
	assert.Equal(t, "Bisnis", updated.Category)
	assert.Equal(t, created.Slug, updated.Slug) // slug stabil

	published, err := svc.TogglePublish(author.ID, created.ID)
	require.NoError(t, err)
	assert.True(t, published.IsPublished)
	assert.NotNil(t, published.PublishedAt)

	draft, err := svc.TogglePublish(author.ID, created.ID)
	require.NoError(t, err)
	assert.False(t, draft.IsPublished)
	assert.Nil(t, draft.PublishedAt)

	// artikel milik orang lain -> forbidden
	_, err = svc.GetMine(other.ID, created.ID)
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = svc.Update(other.ID, created.ID, PostRequest{Title: "x", Content: "c"})
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = svc.TogglePublish(other.ID, created.ID)
	assert.ErrorIs(t, err, ErrForbidden)
	assert.ErrorIs(t, svc.Delete(other.ID, created.ID), ErrForbidden)

	// artikel yang memang tidak ada -> not found
	_, err = svc.GetMine(author.ID, 9999)
	assert.ErrorIs(t, err, ErrPostNotFound)

	require.NoError(t, svc.Delete(author.ID, created.ID))
	_, err = svc.GetMine(author.ID, created.ID)
	assert.ErrorIs(t, err, ErrPostNotFound)
}

// failingRepo tidak punya FindByID, jadi service memakai jalur cadangan FindByIDAndUser.
func TestService_MineFallbackNotFound(t *testing.T) {
	db := newDB(t)
	repo := NewRepository(db)
	svc := NewService(&failingRepo{repo, map[string]error{}})

	_, err := svc.GetMine(1, 9999)
	assert.ErrorIs(t, err, ErrPostNotFound)
}

func TestService_MutationErrors(t *testing.T) {
	db := newDB(t)
	author := makeUser(t, db, "alice", database.RoleMember)
	repo := NewRepository(db)
	created, err := NewService(repo).Create(author.ID, PostRequest{Title: "T", Content: "c"})
	require.NoError(t, err)
	req := PostRequest{Title: "T2", Content: "c2"}

	failing := func(method string) *Service {
		return NewService(&failingRepo{repo, map[string]error{method: errBoom}})
	}

	_, err = failing("FindByIDAndUser").Update(author.ID, created.ID, req)
	assert.ErrorIs(t, err, errBoom)
	_, err = NewService(repo).Update(author.ID, created.ID, PostRequest{Title: "T", Content: "c", Category: "Salah"})
	assert.ErrorIs(t, err, ErrInvalidCategory)
	_, err = failing("Update").Update(author.ID, created.ID, req)
	assert.ErrorIs(t, err, errBoom)

	_, err = failing("FindByIDAndUser").TogglePublish(author.ID, created.ID)
	assert.ErrorIs(t, err, errBoom)
	_, err = failing("Update").TogglePublish(author.ID, created.ID)
	assert.ErrorIs(t, err, errBoom)

	assert.ErrorIs(t, failing("FindByIDAndUser").Delete(author.ID, created.ID), errBoom)
	assert.ErrorIs(t, failing("Delete").Delete(author.ID, created.ID), errBoom)
}

func TestService_ReadErrors(t *testing.T) {
	db := newDB(t)
	repo := NewRepository(db)
	closeDB(t, db)
	svc := NewService(repo)

	_, err := svc.ListPublic("q", "Umum")
	assert.Error(t, err)
	_, err = svc.GetPublic("slug")
	assert.Error(t, err)
	_, err = svc.ListMine(1)
	assert.Error(t, err)
	_, err = svc.Stats(1)
	assert.Error(t, err)
	_, err = svc.GetMine(1, 1)
	assert.Error(t, err)
	_, err = svc.AdminUsers()
	assert.Error(t, err)
	_, err = svc.AdminPosts()
	assert.Error(t, err)
	_, err = svc.AdminUserPosts(1)
	assert.Error(t, err)
}

func TestService_AdminUserPostsErrors(t *testing.T) {
	db := newDB(t)
	repo := NewRepository(db)
	user := makeUser(t, db, "alice", database.RoleMember)

	_, err := NewService(repo).AdminUserPosts(999)
	assert.ErrorIs(t, err, ErrUserNotFound)

	_, err = NewService(&failingRepo{repo, map[string]error{"ListByUser": errBoom}}).AdminUserPosts(user.ID)
	assert.ErrorIs(t, err, errBoom)

	_, err = NewService(&failingRepo{repo, map[string]error{"UserExists": errBoom}}).AdminUserPosts(user.ID)
	assert.ErrorIs(t, err, errBoom)
}

// ---------- handler ----------

type env struct {
	router *gin.Engine
	db     *gorm.DB
	alice  database.User
	bob    database.User
	admin  database.User
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := newDB(t)
	h := NewHandler(NewService(NewRepository(db)))

	r := gin.New()
	r.GET("/public/categories", h.Categories)
	r.GET("/public/posts", h.ListPublic)
	r.GET("/public/posts/:slug", h.GetPublic)

	authed := r.Group("/", middleware.Auth(secret))
	authed.GET("/posts", h.ListMine)
	authed.POST("/posts", h.Create)
	authed.GET("/posts/stats", h.Stats)
	authed.GET("/posts/:id", h.GetMine)
	authed.PUT("/posts/:id", h.Update)
	authed.PATCH("/posts/:id/toggle-publish", h.TogglePublish)
	authed.DELETE("/posts/:id", h.Delete)

	admin := authed.Group("/admin", middleware.RequireRole(database.RoleAdmin))
	admin.GET("/users", h.AdminUsers)
	admin.GET("/posts", h.AdminPosts)
	admin.GET("/users/:id/posts", h.AdminUserPosts)

	return &env{
		router: r, db: db,
		alice: makeUser(t, db, "alice", database.RoleMember),
		bob:   makeUser(t, db, "bob", database.RoleMember),
		admin: makeUser(t, db, "root", database.RoleAdmin),
	}
}

func (e *env) call(t *testing.T, method, path string, body any, u *database.User) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if s, ok := body.(string); ok {
		reader = bytes.NewReader([]byte(s))
	} else {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if u != nil {
		req.Header.Set("Authorization", "Bearer "+tokenFor(t, *u))
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)

	var parsed map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	return w, parsed
}

func (e *env) createPost(t *testing.T, u *database.User, title, category string, publish bool) map[string]any {
	t.Helper()
	w, body := e.call(t, http.MethodPost, "/posts", gin.H{
		"title": title, "category": category, "excerpt": "ringkas " + title,
		"content": "isi lengkap " + title, "is_published": publish,
	}, u)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	return body["data"].(map[string]any)
}

func TestHandler_PublicEndpoints(t *testing.T) {
	e := newEnv(t)
	e.createPost(t, &e.alice, "Belajar Golang", "Teknologi", true)
	e.createPost(t, &e.alice, "Resep Nasi Goreng", "Gaya Hidup", true)
	e.createPost(t, &e.alice, "Draf Rahasia", "Opini", false)

	w, body := e.call(t, http.MethodGet, "/public/categories", nil, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, body["data"], 6)

	_, body = e.call(t, http.MethodGet, "/public/posts", nil, nil)
	list := body["data"].([]any)
	assert.Len(t, list, 2) // draf tidak tampil
	first := list[0].(map[string]any)
	assert.NotContains(t, first, "content") // list tanpa konten
	assert.Equal(t, "alice", first["author"].(map[string]any)["username"])

	_, body = e.call(t, http.MethodGet, "/public/posts?q=golang", nil, nil)
	assert.Len(t, body["data"], 1)

	_, body = e.call(t, http.MethodGet, "/public/posts?category=Gaya+Hidup", nil, nil)
	assert.Len(t, body["data"], 1)

	_, body = e.call(t, http.MethodGet, "/public/posts?q=zzz", nil, nil)
	assert.Empty(t, body["data"]) // hasil kosong

	w, body = e.call(t, http.MethodGet, "/public/posts/belajar-golang", nil, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	data := body["data"].(map[string]any)
	assert.Equal(t, "isi lengkap Belajar Golang", data["content"])
	assert.EqualValues(t, 1, data["reading_time"])

	w, _ = e.call(t, http.MethodGet, "/public/posts/draf-rahasia", nil, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_AuthorCRUDFlow(t *testing.T) {
	e := newEnv(t)
	created := e.createPost(t, &e.alice, "Artikel Satu", "", false)
	id := "/posts/" + jsonID(created["id"])

	assert.Equal(t, "Umum", created["category"])

	w, body := e.call(t, http.MethodGet, "/posts", nil, &e.alice)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, body["data"], 1)

	_, body = e.call(t, http.MethodGet, "/posts", nil, &e.bob)
	assert.Empty(t, body["data"])

	w, body = e.call(t, http.MethodGet, id, nil, &e.alice)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "isi lengkap Artikel Satu", body["data"].(map[string]any)["content"])

	// artikel milik orang lain -> 403
	w, _ = e.call(t, http.MethodGet, id, nil, &e.bob)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, body = e.call(t, http.MethodPut, id, gin.H{"title": "Judul Baru", "category": "Opini", "content": "baru"}, &e.alice)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Judul Baru", body["data"].(map[string]any)["title"])

	w, _ = e.call(t, http.MethodPut, id, gin.H{"title": "x", "category": "Salah", "content": "c"}, &e.alice)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w, _ = e.call(t, http.MethodPut, id, gin.H{"title": ""}, &e.alice)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w, body = e.call(t, http.MethodPatch, id+"/toggle-publish", nil, &e.alice)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, body["data"].(map[string]any)["is_published"])

	_, body = e.call(t, http.MethodGet, "/posts/stats", nil, &e.alice)
	assert.Equal(t, map[string]any{
		"total":          float64(1),
		"total_terbit":   float64(1),
		"total_draf":     float64(0),
		"total_articles": float64(1),
		"total_selesai":  float64(1),
		"belum_selesai":  float64(0),
	}, body["data"])

	// bob mencoba mengubah / menghapus artikel alice -> 403
	w, _ = e.call(t, http.MethodPut, id, gin.H{"title": "x", "content": "c"}, &e.bob)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = e.call(t, http.MethodPatch, id+"/toggle-publish", nil, &e.bob)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = e.call(t, http.MethodDelete, id, nil, &e.bob)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, _ = e.call(t, http.MethodDelete, id, nil, &e.alice)
	assert.Equal(t, http.StatusOK, w.Code)
	w, _ = e.call(t, http.MethodGet, id, nil, &e.alice)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_CreateValidation(t *testing.T) {
	e := newEnv(t)

	w, body := e.call(t, http.MethodPost, "/posts", gin.H{"title": "", "content": ""}, &e.alice)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "fail", body["status"])

	w, _ = e.call(t, http.MethodPost, "/posts", gin.H{"title": "T", "content": "c", "category": "Salah"}, &e.alice)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w, _ = e.call(t, http.MethodPost, "/posts", gin.H{"title": "T", "content": "c"}, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_InvalidIDs(t *testing.T) {
	e := newEnv(t)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/posts/abc"},
		{http.MethodPut, "/posts/abc"},
		{http.MethodPatch, "/posts/abc/toggle-publish"},
		{http.MethodDelete, "/posts/abc"},
		{http.MethodGet, "/admin/users/abc/posts"},
	}
	for _, rt := range routes {
		w, _ := e.call(t, rt.method, rt.path, gin.H{"title": "t", "content": "c"}, &e.admin)
		assert.Equal(t, http.StatusBadRequest, w.Code, rt.method+" "+rt.path)
	}
}

func TestHandler_NotFoundForMissingPost(t *testing.T) {
	e := newEnv(t)

	w, _ := e.call(t, http.MethodPut, "/posts/999", gin.H{"title": "t", "content": "c"}, &e.alice)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w, _ = e.call(t, http.MethodPatch, "/posts/999/toggle-publish", nil, &e.alice)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_AdminEndpoints(t *testing.T) {
	e := newEnv(t)
	e.createPost(t, &e.alice, "Satu", "Umum", true)
	e.createPost(t, &e.alice, "Dua", "Umum", false)
	e.createPost(t, &e.bob, "Tiga", "Bisnis", true)

	w, _ := e.call(t, http.MethodGet, "/admin/users", nil, &e.alice)
	assert.Equal(t, http.StatusForbidden, w.Code)

	w, body := e.call(t, http.MethodGet, "/admin/users", nil, &e.admin)
	assert.Equal(t, http.StatusOK, w.Code)
	users := body["data"].([]any)
	require.Len(t, users, 3)
	aliceRow := users[0].(map[string]any)
	assert.Equal(t, "alice", aliceRow["username"])
	assert.EqualValues(t, 2, aliceRow["total_posts"])
	assert.EqualValues(t, 1, aliceRow["published_posts"])
	assert.EqualValues(t, 0, users[2].(map[string]any)["total_posts"])

	_, body = e.call(t, http.MethodGet, "/admin/posts", nil, &e.admin)
	assert.Len(t, body["data"], 3)

	_, body = e.call(t, http.MethodGet, "/admin/users/"+jsonID(float64(e.bob.ID))+"/posts", nil, &e.admin)
	assert.Len(t, body["data"], 1)

	w, _ = e.call(t, http.MethodGet, "/admin/users/999/posts", nil, &e.admin)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_InternalServerError(t *testing.T) {
	e := newEnv(t)
	closeDB(t, e.db)

	w, body := e.call(t, http.MethodGet, "/public/posts", nil, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "error", body["status"])
}

func jsonID(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
