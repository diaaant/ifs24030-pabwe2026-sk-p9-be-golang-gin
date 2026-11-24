package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"app/internal/database"
	"app/internal/features/post"
)

const systemPrompt = `Anda adalah penulis blog profesional berbahasa Indonesia.
Tulis artikel yang informatif, runtut, dan menarik dalam format Markdown (gunakan heading ##, daftar, dan paragraf).
Balas HANYA dengan satu objek JSON valid tanpa teks lain, dengan kunci:
"title" (string), "category" (salah satu: Teknologi, Pendidikan, Gaya Hidup, Bisnis, Opini, Umum),
"excerpt" (ringkasan pemikat 1-2 kalimat), dan "content" (isi artikel Markdown).`

// Service merangkai Search Engine dan OpenAI menjadi generator draf artikel.
type Service struct {
	chat     ChatClient
	searcher Searcher
}

// NewService membuat Service.
func NewService(chat ChatClient, searcher Searcher) *Service {
	return &Service{chat: chat, searcher: searcher}
}

type generated struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Excerpt  string `json:"excerpt"`
	Content  string `json:"content"`
}

// Generate menghasilkan draf artikel utuh untuk sebuah topik.
func (s *Service) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	requested := ""
	if req.Category != "" {
		category, err := post.NormalizeCategory(req.Category)
		if err != nil {
			return nil, err
		}
		requested = category
	}

	sources := []SearchSource{}
	if req.IncludeWebSearch {
		found, err := s.searcher.Search(ctx, req.Topic)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSearchFailed, err)
		}
		sources = found
	}

	raw, err := s.chat.Complete(ctx, systemPrompt, buildUserPrompt(req.Topic, requested, sources))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAIFailed, err)
	}

	article, err := parseArticle(raw)
	if err != nil {
		return nil, err
	}

	category := requested
	if category == "" {
		normalized, err := post.NormalizeCategory(article.Category)
		if err != nil {
			normalized = database.DefaultCategory
		}
		category = normalized
	}

	return &GenerateResponse{
		Title:    article.Title,
		Category: category,
		Excerpt:  article.Excerpt,
		Content:  article.Content,
		Sources:  sources,
	}, nil
}

func buildUserPrompt(topic, category string, sources []SearchSource) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Topik artikel: %s\n", topic)
	if category != "" {
		fmt.Fprintf(&b, "Kategori: %s\n", category)
	}
	if len(sources) > 0 {
		b.WriteString("\nGunakan fakta terkini dari hasil penelusuran berikut dan sitasi dengan format [n] di dalam teks, ")
		b.WriteString("lalu tutup artikel dengan bagian \"## Referensi\":\n")
		for i, src := range sources {
			fmt.Fprintf(&b, "[%d] %s (%s): %s\n", i+1, src.Title, src.URL, src.Snippet)
		}
	}
	return b.String()
}

// parseArticle mengekstrak objek JSON dari jawaban model (toleran terhadap
// pagar kode ```json dan teks pembuka/penutup).
func parseArticle(raw string) (*generated, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("%w: no JSON object found", ErrInvalidAIResponse)
	}

	var article generated
	if err := json.Unmarshal([]byte(raw[start:end+1]), &article); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAIResponse, err)
	}
	if strings.TrimSpace(article.Title) == "" || strings.TrimSpace(article.Content) == "" {
		return nil, fmt.Errorf("%w: title or content is empty", ErrInvalidAIResponse)
	}
	return &article, nil
}