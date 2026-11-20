package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func run(t *testing.T, handler gin.HandlerFunc) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := gin.New()
	r.GET("/", handler)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w, body
}

func TestSuccess_WithData(t *testing.T) {
	w, body := run(t, func(c *gin.Context) {
		Success(c, http.StatusCreated, "created", gin.H{"id": 1})
	})

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "success", body["status"])
	assert.Equal(t, "created", body["message"])
	assert.Equal(t, map[string]any{"id": float64(1)}, body["data"])
}

func TestSuccess_WithoutDataOmitsField(t *testing.T) {
	w, body := run(t, func(c *gin.Context) { Success(c, http.StatusOK, "ok", nil) })

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, body, "data")
}

func TestFail(t *testing.T) {
	w, body := run(t, func(c *gin.Context) { Fail(c, http.StatusBadRequest, "bad") })

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "fail", body["status"])
	assert.Equal(t, "bad", body["message"])
}

func TestError(t *testing.T) {
	w, body := run(t, func(c *gin.Context) { Error(c, http.StatusInternalServerError, "boom") })

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "error", body["status"])
}

func TestAbortFail_StopsChain(t *testing.T) {
	called := false
	r := gin.New()
	r.GET("/", func(c *gin.Context) { AbortFail(c, http.StatusUnauthorized, "nope") },
		func(c *gin.Context) { called = true })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, called)
	assert.Contains(t, w.Body.String(), `"status":"fail"`)
}

func TestAbortError_StopsChain(t *testing.T) {
	called := false
	r := gin.New()
	r.GET("/", func(c *gin.Context) { AbortError(c, http.StatusInternalServerError, "x") },
		func(c *gin.Context) { called = true })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.False(t, called)
	assert.Contains(t, w.Body.String(), `"status":"error"`)
}
