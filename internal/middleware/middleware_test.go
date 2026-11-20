package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "test-secret"

func init() { gin.SetMode(gin.TestMode) }

func signToken(t *testing.T, method jwt.SigningMethod, key string, expiresIn time.Duration) string {
	t.Helper()
	claims := Claims{
		UserID: 7, Username: "alice", Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiresIn))},
	}
	token, err := jwt.NewWithClaims(method, claims).SignedString([]byte(key))
	require.NoError(t, err)
	return token
}

func do(r http.Handler, method, path, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ---------- CORS ----------

func TestCORS_SetsHeaders(t *testing.T) {
	r := gin.New()
	r.Use(CORS())
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	w := do(r, http.MethodGet, "/ping", "")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "PATCH")
}

func TestCORS_PreflightReturnsNoContent(t *testing.T) {
	r := gin.New()
	r.Use(CORS())
	r.OPTIONS("/ping", func(c *gin.Context) { c.String(http.StatusOK, "should not run") })

	w := do(r, http.MethodOptions, "/ping", "")

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
}

// ---------- Auth ----------

func newAuthRouter() *gin.Engine {
	r := gin.New()
	r.GET("/me", Auth(secret), func(c *gin.Context) {
		u := CurrentUser(c)
		c.JSON(http.StatusOK, gin.H{"id": u.ID, "username": u.Username, "role": u.Role})
	})
	return r
}

func TestAuth_Success(t *testing.T) {
	token := signToken(t, jwt.SigningMethodHS256, secret, time.Hour)

	w := do(newAuthRouter(), http.MethodGet, "/me", "Bearer "+token)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"id":7,"username":"alice","role":"admin"}`, w.Body.String())
}

func TestAuth_Rejections(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"missing header", ""},
		{"wrong scheme", "Basic abc"},
		{"empty token", "Bearer "},
		{"garbage token", "Bearer not-a-jwt"},
		{"wrong secret", "Bearer " + signToken(t, jwt.SigningMethodHS256, "other", time.Hour)},
		{"expired token", "Bearer " + signToken(t, jwt.SigningMethodHS256, secret, -time.Hour)},
		{"unexpected algorithm", "Bearer " + signToken(t, jwt.SigningMethodHS512, secret, time.Hour)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(newAuthRouter(), http.MethodGet, "/me", tc.header)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Contains(t, w.Body.String(), `"status":"fail"`)
		})
	}
}

func TestCurrentUser_WithoutAuthReturnsZeroValue(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	assert.Equal(t, AuthUser{}, CurrentUser(c))
}

// ---------- RequireRole ----------

func newRoleRouter() *gin.Engine {
	r := gin.New()
	r.GET("/admin", Auth(secret), RequireRole("admin"), func(c *gin.Context) {
		c.String(http.StatusOK, "welcome")
	})
	return r
}

func TestRequireRole_AllowsMatchingRole(t *testing.T) {
	token := signToken(t, jwt.SigningMethodHS256, secret, time.Hour) // role admin

	w := do(newRoleRouter(), http.MethodGet, "/admin", "Bearer "+token)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "welcome", w.Body.String())
}

func TestRequireRole_ForbidsOtherRole(t *testing.T) {
	claims := Claims{UserID: 1, Username: "bob", Role: "member"}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)

	w := do(newRoleRouter(), http.MethodGet, "/admin", "Bearer "+token)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"fail"`)
}

// ---------- RateLimiter ----------

func TestRateLimiter_DefaultAllowsNormalTraffic(t *testing.T) {
	r := gin.New()
	r.Use(RateLimiter())
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/", "").Code)
}

func TestNewRateLimiter_UsesRealClock(t *testing.T) {
	r := gin.New()
	r.Use(NewRateLimiter(1, time.Hour))
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/", "").Code)
	assert.Equal(t, http.StatusTooManyRequests, do(r, http.MethodGet, "/", "").Code)
}

func TestRateLimiter_BlocksAfterLimitAndResetsAfterWindow(t *testing.T) {
	current := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := gin.New()
	r.Use(newRateLimiter(2, time.Minute, func() time.Time { return current }))
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/", "").Code)
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/", "").Code)

	w := do(r, http.MethodGet, "/", "")
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"fail"`)

	current = current.Add(time.Minute) // jendela baru
	assert.Equal(t, http.StatusOK, do(r, http.MethodGet, "/", "").Code)
}

func TestRateLimiter_TracksEachIPSeparately(t *testing.T) {
	r := gin.New()
	r.Use(newRateLimiter(1, time.Hour, time.Now))
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	reqFrom := func(addr string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = addr
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	assert.Equal(t, http.StatusOK, reqFrom("10.0.0.1:1000"))
	assert.Equal(t, http.StatusTooManyRequests, reqFrom("10.0.0.1:1001"))
	assert.Equal(t, http.StatusOK, reqFrom("10.0.0.2:1000"))
}
