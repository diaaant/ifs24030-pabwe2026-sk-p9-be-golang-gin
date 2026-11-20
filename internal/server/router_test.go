package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"app/internal/config"
	"app/internal/database"
)

func init() { gin.SetMode(gin.TestMode) }

type app struct {
	router http.Handler
	t      *testing.T
}

func newApp(t *testing.T) *app {
	t.Helper()

	openAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"title\":\"Artikel AI\",\"category\":\"Teknologi\",\"excerpt\":\"e\",\"content\":\"## c\"}"}}]}`))
	}))
	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"S","url":"https://s","snippet":"x"}]}`))
	}))
	t.Cleanup(openAI.Close)
	t.Cleanup(search.Close)

	db, err := database.ConnectSQLite()
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	cfg := &config.Config{
		AppKey: "router-secret", AdminUsername: "admin", AdminPassword: "admin",
		OpenAIEndpoint: openAI.URL, OpenAIKey: "k", OpenAIModel: "m", SearchEngineEndpoint: search.URL,
	}
	require.NoError(t, database.Seed(db, cfg))

	return &app{router: NewRouter(db, cfg), t: t}
}

func (a *app) do(method, path string, body any, token string, headers ...string) (*httptest.ResponseRecorder, map[string]any) {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, req)

	var parsed map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	return w, parsed
}

func (a *app) login(username, password string) string {
	w, body := a.do(http.MethodPost, "/api/auth/login", gin.H{"username": username, "password": password}, "")
	require.Equal(a.t, http.StatusOK, w.Code)
	return body["data"].(map[string]any)["token"].(string)
}

func TestRouter_FullAuthorFlow(t *testing.T) {
	a := newApp(t)

	w, _ := a.do(http.MethodPost, "/api/auth/register", gin.H{"username": "writer", "email": "w@x.com", "password": "secret1"}, "")
	require.Equal(t, http.StatusCreated, w.Code)
	token := a.login("writer", "secret1")

	w, _ = a.do(http.MethodGet, "/api/auth/profile", nil, token)
	assert.Equal(t, http.StatusOK, w.Code)

	w, body := a.do(http.MethodPost, "/api/posts", gin.H{"title": "Hello World", "content": "isi", "category": "Teknologi"}, token)
	require.Equal(t, http.StatusCreated, w.Code)
	id := int(body["data"].(map[string]any)["id"].(float64))

	w, _ = a.do(http.MethodPatch, "/api/posts/1/toggle-publish", nil, token)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, id)

	_, body = a.do(http.MethodGet, "/api/public/posts", nil, "")
	assert.Len(t, body["data"], 1)

	w, _ = a.do(http.MethodGet, "/api/public/posts/hello-world", nil, "")
	assert.Equal(t, http.StatusOK, w.Code)

	_, body = a.do(http.MethodGet, "/api/posts/stats", nil, token)
	assert.EqualValues(t, 1, body["data"].(map[string]any)["total_terbit"])

	w, _ = a.do(http.MethodPost, "/api/auth/logout", nil, token)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_AIGenerateEndToEnd(t *testing.T) {
	a := newApp(t)
	token := a.login("member", "member")

	w, body := a.do(http.MethodPost, "/api/posts/ai-generate",
		gin.H{"topic": "Golang", "include_web_search": true}, token)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := body["data"].(map[string]any)
	assert.Equal(t, "Artikel AI", data["title"])
	assert.Len(t, data["sources"], 1)
}

func TestRouter_AdminAccessControl(t *testing.T) {
	a := newApp(t)

	w, _ := a.do(http.MethodGet, "/api/admin/users", nil, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w, _ = a.do(http.MethodGet, "/api/admin/users", nil, a.login("member", "member"))
	assert.Equal(t, http.StatusForbidden, w.Code)

	adminToken := a.login("admin", "admin")
	w, body := a.do(http.MethodGet, "/api/admin/users", nil, adminToken)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, body["data"], 2)

	w, _ = a.do(http.MethodGet, "/api/admin/posts", nil, adminToken)
	assert.Equal(t, http.StatusOK, w.Code)
	w, _ = a.do(http.MethodGet, "/api/admin/users/1/posts", nil, adminToken)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRouter_NoRouteAndCORS(t *testing.T) {
	a := newApp(t)

	w, body := a.do(http.MethodGet, "/api/unknown", nil, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "Endpoint not found", body["message"])

	w, _ = a.do(http.MethodOptions, "/api/posts", nil, "")
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestRouter_OpenAPIJSON(t *testing.T) {
	a := newApp(t)

	w, body := a.do(http.MethodGet, "/api.json", nil, "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "example.com", body["host"])
	assert.Equal(t, []any{"http"}, body["schemes"])

	_, body = a.do(http.MethodGet, "/api.json", nil, "", "X-Forwarded-Proto", "https")
	assert.Equal(t, []any{"https"}, body["schemes"])

	req := httptest.NewRequest(http.MethodGet, "https://api.test/api.json", nil)
	req.TLS = &tlsState
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	assert.Contains(t, rec.Body.String(), `"https"`)
}

func TestRouter_OpenAPIJSONError(t *testing.T) {
	original := swaggerJSON
	t.Cleanup(func() { swaggerJSON = original })
	swaggerJSON = func(string, string) ([]byte, error) { return nil, errors.New("boom") }
	a := newApp(t)

	w, body := a.do(http.MethodGet, "/api.json", nil, "")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "error", body["status"])
}

func TestRouter_SwaggerUI(t *testing.T) {
	a := newApp(t)

	w, _ := a.do(http.MethodGet, "/swagger/index.html", nil, "")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "swagger")
}

var tlsState = tls.ConnectionState{}
