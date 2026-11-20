package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

var errBoom = errors.New("boom")

// ---------- fakes ----------

type fakeChat struct {
	reply   string
	err     error
	gotUser string
}

func (f *fakeChat) Complete(_ context.Context, _, user string) (string, error) {
	f.gotUser = user
	return f.reply, f.err
}

type fakeSearcher struct {
	sources []SearchSource
	err     error
	called  bool
}

func (f *fakeSearcher) Search(context.Context, string) ([]SearchSource, error) {
	f.called = true
	return f.sources, f.err
}

const validJSON = `{"title":"Judul AI","category":"Teknologi","excerpt":"Ringkas.","content":"## Isi\n\nTeks."}`

// ---------- SearchClient ----------

func TestSearchClient_Success(t *testing.T) {
	var gotQuery, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"results":[
			{"title":"A","url":"https://a","snippet":"sa"},
			{"title":"B","link":"https://b","content":"cb"},
			{"title":"C","url":"https://c","description":"dc"},
			{"title":"D"},{"title":"E"},{"title":"F"}]}`))
	}))
	defer srv.Close()

	sources, err := NewSearchClient(srv.URL, "k").Search(context.Background(), "go lang")

	require.NoError(t, err)
	assert.Equal(t, "go lang", gotQuery)
	assert.Equal(t, "Bearer k", gotAuth)
	require.Len(t, sources, 5) // dibatasi 5
	assert.Equal(t, SearchSource{Title: "A", URL: "https://a", Snippet: "sa"}, sources[0])
	assert.Equal(t, SearchSource{Title: "B", URL: "https://b", Snippet: "cb"}, sources[1])
	assert.Equal(t, "dc", sources[2].Snippet)
}

func TestSearchClient_NestedDataResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"results":[{"title":"X","url":"https://x","snippet":"sx"}]}}`))
	}))
	defer srv.Close()

	sources, err := NewSearchClient(srv.URL, "k").Search(context.Background(), "q")

	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, "X", sources[0].Title)
}

func TestSearchClient_Errors(t *testing.T) {
	t.Run("non-2xx status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		_, err := NewSearchClient(srv.URL, "k").Search(context.Background(), "q")
		assert.ErrorContains(t, err, "status 500")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer srv.Close()
		_, err := NewSearchClient(srv.URL, "k").Search(context.Background(), "q")
		assert.Error(t, err)
	})

	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		_, err := NewSearchClient(url, "k").Search(context.Background(), "q")
		assert.Error(t, err)
	})

	t.Run("invalid endpoint", func(t *testing.T) {
		_, err := NewSearchClient("://bad", "k").Search(context.Background(), "q")
		assert.Error(t, err)
	})
}

// ---------- OpenAIClient ----------

func TestOpenAIClient_Success(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"halo"}}]}`))
	}))
	defer srv.Close()

	out, err := NewOpenAIClient(srv.URL+"/v1/", "secret", "model-x").Complete(context.Background(), "sys", "usr")

	require.NoError(t, err)
	assert.Equal(t, "halo", out)
	assert.Equal(t, "/v1/chat/completions", gotPath)
	assert.Equal(t, "Bearer secret", gotAuth)
	assert.Equal(t, "model-x", gotBody.Model)
	require.Len(t, gotBody.Messages, 2)
	assert.Equal(t, "sys", gotBody.Messages[0].Content)
	assert.Equal(t, "usr", gotBody.Messages[1].Content)
}

func TestOpenAIClient_Errors(t *testing.T) {
	serve := func(status int, body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
	}

	t.Run("non-2xx includes body", func(t *testing.T) {
		srv := serve(http.StatusUnauthorized, "bad key")
		defer srv.Close()
		_, err := NewOpenAIClient(srv.URL, "k", "m").Complete(context.Background(), "s", "u")
		assert.ErrorContains(t, err, "status 401: bad key")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		srv := serve(http.StatusOK, "nope")
		defer srv.Close()
		_, err := NewOpenAIClient(srv.URL, "k", "m").Complete(context.Background(), "s", "u")
		assert.Error(t, err)
	})

	t.Run("no choices", func(t *testing.T) {
		srv := serve(http.StatusOK, `{"choices":[]}`)
		defer srv.Close()
		_, err := NewOpenAIClient(srv.URL, "k", "m").Complete(context.Background(), "s", "u")
		assert.ErrorContains(t, err, "no choices")
	})

	t.Run("connection refused", func(t *testing.T) {
		srv := serve(http.StatusOK, "")
		url := srv.URL
		srv.Close()
		_, err := NewOpenAIClient(url, "k", "m").Complete(context.Background(), "s", "u")
		assert.Error(t, err)
	})

	t.Run("invalid endpoint", func(t *testing.T) {
		_, err := NewOpenAIClient("://bad", "k", "m").Complete(context.Background(), "s", "u")
		assert.Error(t, err)
	})
}

// ---------- Service ----------

func TestService_Generate_WithSearchAndFencedJSON(t *testing.T) {
	chat := &fakeChat{reply: "Berikut hasilnya:\n```json\n" + validJSON + "\n```"}
	searcher := &fakeSearcher{sources: []SearchSource{{Title: "Sumber", URL: "https://s", Snippet: "fakta"}}}

	resp, err := NewService(chat, searcher).Generate(context.Background(),
		GenerateRequest{Topic: "Golang", Category: "bisnis", IncludeWebSearch: true})

	require.NoError(t, err)
	assert.True(t, searcher.called)
	assert.Equal(t, "Judul AI", resp.Title)
	assert.Equal(t, "Bisnis", resp.Category) // kategori permintaan menang
	assert.Equal(t, "Ringkas.", resp.Excerpt)
	assert.Contains(t, resp.Content, "## Isi")
	assert.Equal(t, searcher.sources, resp.Sources)
	assert.Contains(t, chat.gotUser, "[1] Sumber (https://s): fakta")
	assert.Contains(t, chat.gotUser, "Kategori: Bisnis")
}

func TestService_Generate_WithoutSearchUsesAICategory(t *testing.T) {
	chat := &fakeChat{reply: validJSON}
	searcher := &fakeSearcher{}

	resp, err := NewService(chat, searcher).Generate(context.Background(), GenerateRequest{Topic: "Golang"})

	require.NoError(t, err)
	assert.False(t, searcher.called)
	assert.Equal(t, "Teknologi", resp.Category)
	assert.Empty(t, resp.Sources)
	assert.NotNil(t, resp.Sources)
	assert.NotContains(t, chat.gotUser, "Kategori:")
	assert.NotContains(t, chat.gotUser, "penelusuran")
}

func TestService_Generate_UnknownAICategoryFallsBackToUmum(t *testing.T) {
	chat := &fakeChat{reply: `{"title":"T","category":"Olahraga","excerpt":"e","content":"c"}`}

	resp, err := NewService(chat, &fakeSearcher{}).Generate(context.Background(), GenerateRequest{Topic: "x"})

	require.NoError(t, err)
	assert.Equal(t, "Umum", resp.Category)
}

func TestService_Generate_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid requested category", func(t *testing.T) {
		_, err := NewService(&fakeChat{}, &fakeSearcher{}).Generate(ctx, GenerateRequest{Topic: "x", Category: "Salah"})
		assert.Error(t, err)
	})

	t.Run("search failure", func(t *testing.T) {
		_, err := NewService(&fakeChat{reply: validJSON}, &fakeSearcher{err: errBoom}).
			Generate(ctx, GenerateRequest{Topic: "x", IncludeWebSearch: true})
		assert.ErrorIs(t, err, ErrSearchFailed)
	})

	t.Run("chat failure", func(t *testing.T) {
		_, err := NewService(&fakeChat{err: errBoom}, &fakeSearcher{}).Generate(ctx, GenerateRequest{Topic: "x"})
		assert.ErrorIs(t, err, ErrAIFailed)
	})

	for name, reply := range map[string]string{
		"no json":         "maaf, tidak bisa",
		"broken json":     `{"title": }`,
		"empty title":     `{"title":" ","content":"c"}`,
		"empty content":   `{"title":"t","content":""}`,
		"reversed braces": "} {",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewService(&fakeChat{reply: reply}, &fakeSearcher{}).Generate(ctx, GenerateRequest{Topic: "x"})
			assert.ErrorIs(t, err, ErrInvalidAIResponse)
		})
	}
}

// ---------- Handler ----------

func callGenerate(svc *Service, body string) (*httptest.ResponseRecorder, map[string]any) {
	r := gin.New()
	r.POST("/ai-generate", NewHandler(svc).Generate)
	req := httptest.NewRequest(http.MethodPost, "/ai-generate", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var parsed map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	return w, parsed
}

func TestHandler_Generate(t *testing.T) {
	okSvc := NewService(&fakeChat{reply: validJSON}, &fakeSearcher{})

	t.Run("success", func(t *testing.T) {
		w, body := callGenerate(okSvc, `{"topic":"Golang"}`)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "Judul AI", body["data"].(map[string]any)["title"])
	})

	t.Run("validation error", func(t *testing.T) {
		w, body := callGenerate(okSvc, `{"topic":""}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "fail", body["status"])
	})

	t.Run("invalid category", func(t *testing.T) {
		w, _ := callGenerate(okSvc, `{"topic":"x","category":"Salah"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("search failure", func(t *testing.T) {
		svc := NewService(&fakeChat{reply: validJSON}, &fakeSearcher{err: errBoom})
		w, body := callGenerate(svc, `{"topic":"x","include_web_search":true}`)
		assert.Equal(t, http.StatusBadGateway, w.Code)
		assert.Equal(t, "error", body["status"])
		assert.Equal(t, "Search engine request failed", body["message"])
	})

	t.Run("ai failure", func(t *testing.T) {
		w, body := callGenerate(NewService(&fakeChat{err: errBoom}, &fakeSearcher{}), `{"topic":"x"}`)
		assert.Equal(t, http.StatusBadGateway, w.Code)
		assert.Equal(t, "AI request failed", body["message"])
	})

	t.Run("invalid ai response", func(t *testing.T) {
		w, body := callGenerate(NewService(&fakeChat{reply: "kosong"}, &fakeSearcher{}), `{"topic":"x"}`)
		assert.Equal(t, http.StatusBadGateway, w.Code)
		assert.True(t, strings.Contains(body["message"].(string), "invalid response"))
	})
}
