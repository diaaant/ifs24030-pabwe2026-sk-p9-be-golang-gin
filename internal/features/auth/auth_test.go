package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"app/internal/database"
	"app/internal/middleware"
)

const secret = "test-secret"

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

// fakeRepo adalah Repository in-memory dengan injeksi error per method.
type fakeRepo struct {
	users                                                      map[uint]*database.User
	nextID                                                     uint
	errCreate, errUpdate, errFindID, errFindName, errFindEmail error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{users: map[uint]*database.User{}, nextID: 1}
}

func (f *fakeRepo) Create(u *database.User) error {
	if f.errCreate != nil {
		return f.errCreate
	}
	u.ID = f.nextID
	f.nextID++
	f.users[u.ID] = u
	return nil
}

func (f *fakeRepo) Update(u *database.User) error {
	if f.errUpdate != nil {
		return f.errUpdate
	}
	f.users[u.ID] = u
	return nil
}

func (f *fakeRepo) find(match func(*database.User) bool) *database.User {
	for _, u := range f.users {
		if match(u) {
			return u
		}
	}
	return nil
}

func (f *fakeRepo) FindByID(id uint) (*database.User, error) {
	return f.users[id], f.errFindID
}

func (f *fakeRepo) FindByUsername(name string) (*database.User, error) {
	if f.errFindName != nil {
		return nil, f.errFindName
	}
	return f.find(func(u *database.User) bool { return u.Username == name }), nil
}

func (f *fakeRepo) FindByEmail(email string) (*database.User, error) {
	if f.errFindEmail != nil {
		return nil, f.errFindEmail
	}
	return f.find(func(u *database.User) bool { return u.Email == email }), nil
}

func seedFake(t *testing.T, f *fakeRepo, username, email, password string) *database.User {
	t.Helper()
	svc := NewService(f, secret)
	resp, err := svc.Register(RegisterRequest{Username: username, Email: email, Password: password})
	require.NoError(t, err)
	return f.users[resp.ID]
}

var errBoom = errors.New("boom")

// ---------- repository (SQLite) ----------

func TestRepository_CRUD(t *testing.T) {
	repo := NewRepository(newDB(t))
	user := &database.User{Username: "alice", Email: "alice@x.com", Password: "h", Role: database.RoleMember}

	require.NoError(t, repo.Create(user))
	assert.NotZero(t, user.ID)

	byID, err := repo.FindByID(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "alice", byID.Username)

	byName, err := repo.FindByUsername("alice")
	require.NoError(t, err)
	assert.Equal(t, user.ID, byName.ID)

	byEmail, err := repo.FindByEmail("alice@x.com")
	require.NoError(t, err)
	assert.Equal(t, user.ID, byEmail.ID)

	user.Username = "alice2"
	require.NoError(t, repo.Update(user))
	updated, err := repo.FindByID(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "alice2", updated.Username)
}

func TestRepository_NotFoundReturnsNil(t *testing.T) {
	repo := NewRepository(newDB(t))

	user, err := repo.FindByID(999)

	assert.NoError(t, err)
	assert.Nil(t, user)
}

func TestRepository_DatabaseErrors(t *testing.T) {
	db := newDB(t)
	repo := NewRepository(db)
	closeDB(t, db)

	_, err := repo.FindByID(1)
	assert.Error(t, err)
	assert.Error(t, repo.Create(&database.User{Username: "a", Email: "a@x.com"}))
	assert.Error(t, repo.Update(&database.User{ID: 1}))
}

// ---------- service (fake repo) ----------

func TestService_Register(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		f := newFakeRepo()
		resp, err := NewService(f, secret).Register(RegisterRequest{Username: "bob", Email: "bob@x.com", Password: "secret1"})

		require.NoError(t, err)
		assert.Equal(t, "bob", resp.Username)
		assert.Equal(t, database.RoleMember, resp.Role)
		assert.NotEqual(t, "secret1", f.users[resp.ID].Password)
	})

	t.Run("username taken", func(t *testing.T) {
		f := newFakeRepo()
		seedFake(t, f, "bob", "bob@x.com", "secret1")
		_, err := NewService(f, secret).Register(RegisterRequest{Username: "bob", Email: "other@x.com", Password: "secret1"})
		assert.ErrorIs(t, err, ErrUsernameTaken)
	})

	t.Run("email taken", func(t *testing.T) {
		f := newFakeRepo()
		seedFake(t, f, "bob", "bob@x.com", "secret1")
		_, err := NewService(f, secret).Register(RegisterRequest{Username: "carl", Email: "bob@x.com", Password: "secret1"})
		assert.ErrorIs(t, err, ErrEmailTaken)
	})

	t.Run("username lookup error", func(t *testing.T) {
		f := newFakeRepo()
		f.errFindName = errBoom
		_, err := NewService(f, secret).Register(RegisterRequest{Username: "a", Email: "a@x.com", Password: "secret1"})
		assert.ErrorIs(t, err, errBoom)
	})

	t.Run("email lookup error", func(t *testing.T) {
		f := newFakeRepo()
		f.errFindEmail = errBoom
		_, err := NewService(f, secret).Register(RegisterRequest{Username: "a", Email: "a@x.com", Password: "secret1"})
		assert.ErrorIs(t, err, errBoom)
	})

	t.Run("password too long for bcrypt", func(t *testing.T) {
		_, err := NewService(newFakeRepo(), secret).Register(RegisterRequest{
			Username: "a", Email: "a@x.com", Password: strings.Repeat("x", 73),
		})
		assert.Error(t, err)
	})

	t.Run("create error", func(t *testing.T) {
		f := newFakeRepo()
		f.errCreate = errBoom
		_, err := NewService(f, secret).Register(RegisterRequest{Username: "a", Email: "a@x.com", Password: "secret1"})
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestService_Login(t *testing.T) {
	t.Run("success issues parsable token", func(t *testing.T) {
		f := newFakeRepo()
		seedFake(t, f, "bob", "bob@x.com", "secret1")

		resp, err := NewService(f, secret).Login(LoginRequest{Username: "bob", Password: "secret1"})

		require.NoError(t, err)
		assert.Equal(t, "bob", resp.User.Username)
		claims := &middleware.Claims{}
		_, err = jwt.ParseWithClaims(resp.Token, claims, func(*jwt.Token) (any, error) { return []byte(secret), nil })
		require.NoError(t, err)
		assert.Equal(t, resp.User.ID, claims.UserID)
		assert.Equal(t, database.RoleMember, claims.Role)
	})

	t.Run("lookup error", func(t *testing.T) {
		f := newFakeRepo()
		f.errFindName = errBoom
		_, err := NewService(f, secret).Login(LoginRequest{Username: "bob", Password: "x"})
		assert.ErrorIs(t, err, errBoom)
	})

	t.Run("unknown user", func(t *testing.T) {
		_, err := NewService(newFakeRepo(), secret).Login(LoginRequest{Username: "ghost", Password: "x"})
		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("wrong password", func(t *testing.T) {
		f := newFakeRepo()
		seedFake(t, f, "bob", "bob@x.com", "secret1")
		_, err := NewService(f, secret).Login(LoginRequest{Username: "bob", Password: "wrong"})
		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})
}

func TestService_GetProfile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		f := newFakeRepo()
		u := seedFake(t, f, "bob", "bob@x.com", "secret1")
		resp, err := NewService(f, secret).GetProfile(u.ID)
		require.NoError(t, err)
		assert.Equal(t, "bob@x.com", resp.Email)
	})

	t.Run("lookup error", func(t *testing.T) {
		f := newFakeRepo()
		f.errFindID = errBoom
		_, err := NewService(f, secret).GetProfile(1)
		assert.ErrorIs(t, err, errBoom)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := NewService(newFakeRepo(), secret).GetProfile(42)
		assert.ErrorIs(t, err, ErrUserNotFound)
	})
}

func TestService_UpdateProfile(t *testing.T) {
	req := UpdateProfileRequest{Username: "bob2", Email: "bob2@x.com"}

	t.Run("success without password keeps hash", func(t *testing.T) {
		f := newFakeRepo()
		u := seedFake(t, f, "bob", "bob@x.com", "secret1")
		oldHash := u.Password

		resp, err := NewService(f, secret).UpdateProfile(u.ID, req)

		require.NoError(t, err)
		assert.Equal(t, "bob2", resp.Username)
		assert.Equal(t, oldHash, f.users[u.ID].Password)
	})

	t.Run("success with new password and own username", func(t *testing.T) {
		f := newFakeRepo()
		u := seedFake(t, f, "bob", "bob@x.com", "secret1")
		oldHash := u.Password

		_, err := NewService(f, secret).UpdateProfile(u.ID, UpdateProfileRequest{
			Username: "bob", Email: "bob@x.com", Password: "newsecret",
		})

		require.NoError(t, err)
		assert.NotEqual(t, oldHash, f.users[u.ID].Password)
	})

	t.Run("user lookup error", func(t *testing.T) {
		f := newFakeRepo()
		f.errFindID = errBoom
		_, err := NewService(f, secret).UpdateProfile(1, req)
		assert.ErrorIs(t, err, errBoom)
	})

	t.Run("user not found", func(t *testing.T) {
		_, err := NewService(newFakeRepo(), secret).UpdateProfile(5, req)
		assert.ErrorIs(t, err, ErrUserNotFound)
	})

	t.Run("username used by another user", func(t *testing.T) {
		f := newFakeRepo()
		u := seedFake(t, f, "bob", "bob@x.com", "secret1")
		seedFake(t, f, "bob2", "other@x.com", "secret1")
		_, err := NewService(f, secret).UpdateProfile(u.ID, req)
		assert.ErrorIs(t, err, ErrUsernameTaken)
	})

	t.Run("email used by another user", func(t *testing.T) {
		f := newFakeRepo()
		u := seedFake(t, f, "bob", "bob@x.com", "secret1")
		seedFake(t, f, "carl", "bob2@x.com", "secret1")
		_, err := NewService(f, secret).UpdateProfile(u.ID, req)
		assert.ErrorIs(t, err, ErrEmailTaken)
	})

	t.Run("uniqueness lookup error", func(t *testing.T) {
		f := newFakeRepo()
		u := seedFake(t, f, "bob", "bob@x.com", "secret1")
		f.errFindEmail = errBoom
		_, err := NewService(f, secret).UpdateProfile(u.ID, req)
		assert.ErrorIs(t, err, errBoom)
	})

	t.Run("password too long for bcrypt", func(t *testing.T) {
		f := newFakeRepo()
		u := seedFake(t, f, "bob", "bob@x.com", "secret1")
		_, err := NewService(f, secret).UpdateProfile(u.ID, UpdateProfileRequest{
			Username: "bob", Email: "bob@x.com", Password: strings.Repeat("x", 73),
		})
		assert.Error(t, err)
	})

	t.Run("update error", func(t *testing.T) {
		f := newFakeRepo()
		u := seedFake(t, f, "bob", "bob@x.com", "secret1")
		f.errUpdate = errBoom
		_, err := NewService(f, secret).UpdateProfile(u.ID, req)
		assert.ErrorIs(t, err, errBoom)
	})
}

// ---------- handler (SQLite + router) ----------

type env struct {
	router *gin.Engine
	db     *gorm.DB
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := newDB(t)
	h := NewHandler(NewService(NewRepository(db), secret))

	r := gin.New()
	r.POST("/register", h.Register)
	r.POST("/login", h.Login)
	authed := r.Group("/", middleware.Auth(secret))
	authed.POST("/logout", h.Logout)
	authed.GET("/profile", h.Profile)
	authed.PUT("/profile", h.UpdateProfile)
	return &env{router: r, db: db}
}

func (e *env) call(method, path string, body any, token string) (*httptest.ResponseRecorder, map[string]any) {
	var reader *bytes.Reader
	switch v := body.(type) {
	case string:
		reader = bytes.NewReader([]byte(v))
	case nil:
		reader = bytes.NewReader(nil)
	default:
		raw, _ := json.Marshal(v)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)

	var parsed map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	return w, parsed
}

func (e *env) registerAndLogin(t *testing.T, username string) string {
	t.Helper()
	w, _ := e.call(http.MethodPost, "/register", gin.H{
		"username": username, "email": username + "@x.com", "password": "secret1",
	}, "")
	require.Equal(t, http.StatusCreated, w.Code)

	w, body := e.call(http.MethodPost, "/login", gin.H{"username": username, "password": "secret1"}, "")
	require.Equal(t, http.StatusOK, w.Code)
	return body["data"].(map[string]any)["token"].(string)
}

func TestHandler_Register(t *testing.T) {
	e := newEnv(t)
	valid := gin.H{"username": "alice", "email": "alice@x.com", "password": "secret1"}

	w, body := e.call(http.MethodPost, "/register", valid, "")
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "success", body["status"])
	data := body["data"].(map[string]any)
	assert.Equal(t, "member", data["role"])
	assert.NotContains(t, data, "password")

	w, _ = e.call(http.MethodPost, "/register", valid, "")
	assert.Equal(t, http.StatusConflict, w.Code)

	w, body = e.call(http.MethodPost, "/register", gin.H{"username": "x", "email": "bad", "password": "1"}, "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "fail", body["status"])

	closeDB(t, e.db)
	w, body = e.call(http.MethodPost, "/register", valid, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "error", body["status"])
}

func TestHandler_Login(t *testing.T) {
	e := newEnv(t)
	token := e.registerAndLogin(t, "alice")
	assert.NotEmpty(t, token)

	w, _ := e.call(http.MethodPost, "/login", gin.H{"username": "alice", "password": "wrong"}, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w, _ = e.call(http.MethodPost, "/login", "{not json", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Logout(t *testing.T) {
	e := newEnv(t)
	token := e.registerAndLogin(t, "alice")

	w, body := e.call(http.MethodPost, "/logout", nil, token)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "success", body["status"])

	w, _ = e.call(http.MethodPost, "/logout", nil, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandler_Profile(t *testing.T) {
	e := newEnv(t)
	token := e.registerAndLogin(t, "alice")

	w, body := e.call(http.MethodGet, "/profile", nil, token)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "alice", body["data"].(map[string]any)["username"])

	// Token valid tetapi user sudah terhapus.
	require.NoError(t, e.db.Exec("DELETE FROM users").Error)
	w, _ = e.call(http.MethodGet, "/profile", nil, token)
	assert.Equal(t, http.StatusNotFound, w.Code)

	closeDB(t, e.db)
	w, _ = e.call(http.MethodGet, "/profile", nil, token)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandler_UpdateProfile(t *testing.T) {
	e := newEnv(t)
	token := e.registerAndLogin(t, "alice")
	e.registerAndLogin(t, "bob")

	w, body := e.call(http.MethodPut, "/profile", gin.H{"username": "alice2", "email": "alice2@x.com"}, token)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "alice2", body["data"].(map[string]any)["username"])

	w, _ = e.call(http.MethodPut, "/profile", gin.H{"username": "bob", "email": "alice2@x.com"}, token)
	assert.Equal(t, http.StatusConflict, w.Code)

	w, _ = e.call(http.MethodPut, "/profile", gin.H{"username": "", "email": "nope"}, token)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
