// Package middleware berisi CORS, autentikasi JWT, pembatasan role, dan rate limiter.
package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"app/internal/response"
)

// UserContextKey adalah kunci gin.Context tempat data user disimpan oleh Auth.
const UserContextKey = "user"

// Claims adalah isi (payload) token JWT aplikasi.
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// AuthUser adalah identitas user yang disuntikkan ke gin.Context.
type AuthUser struct {
	ID       uint
	Username string
	Role     string
}

// CurrentUser mengambil user yang diautentikasi dari context.
// Mengembalikan nilai kosong jika Auth belum dijalankan.
func CurrentUser(c *gin.Context) AuthUser {
	value, _ := c.Get(UserContextKey)
	user, _ := value.(AuthUser)
	return user
}

// CORS mengizinkan akses lintas-domain bagi frontend dan menjawab preflight.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Origin")
		h.Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// Auth memverifikasi Bearer token JWT (HS256) dan menyimpan AuthUser di context.
func Auth(secretKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, found := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !found || tokenString == "" {
			response.AbortFail(c, http.StatusUnauthorized, "Missing or invalid authorization header")
			return
		}

		claims := &Claims{}
		_, err := jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (any, error) {
			return []byte(secretKey), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		if err != nil {
			response.AbortFail(c, http.StatusUnauthorized, "Invalid or expired token")
			return
		}

		c.Set(UserContextKey, AuthUser{ID: claims.UserID, Username: claims.Username, Role: claims.Role})
		c.Next()
	}
}

// RequireRole hanya mengizinkan user dengan salah satu role yang diberikan.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := CurrentUser(c).Role
		for _, allowed := range roles {
			if strings.EqualFold(role, allowed) {
				c.Next()
				return
			}
		}
		response.AbortFail(c, http.StatusForbidden, "You do not have permission to access this resource")
	}
}

type visitor struct {
	count   int
	resetAt time.Time
}

// RateLimiter membatasi 120 request per menit per alamat IP.
func RateLimiter() gin.HandlerFunc {
	return NewRateLimiter(120, time.Minute)
}

// NewRateLimiter membuat rate limiter fixed-window per IP.
func NewRateLimiter(limit int, window time.Duration) gin.HandlerFunc {
	return newRateLimiter(limit, window, time.Now)
}

func newRateLimiter(limit int, window time.Duration, now func() time.Time) gin.HandlerFunc {
	var mu sync.Mutex
	visitors := map[string]*visitor{}

	return func(c *gin.Context) {
		ip := c.ClientIP()
		current := now()

		mu.Lock()
		v, ok := visitors[ip]
		if !ok || !current.Before(v.resetAt) {
			v = &visitor{resetAt: current.Add(window)}
			visitors[ip] = v
		}
		v.count++
		allowed := v.count <= limit
		mu.Unlock()

		if !allowed {
			response.AbortFail(c, http.StatusTooManyRequests, "Too many requests, please try again later")
			return
		}
		c.Next()
	}
}
