// Package ai menghasilkan draf artikel utuh dengan OpenAI Chat Completions,
// diperkaya konteks hasil penelusuran Search Engine (opsional).
package ai

import "errors"

// Error domain yang dipetakan handler ke status HTTP.
var (
	ErrSearchFailed      = errors.New("search engine request failed")
	ErrAIFailed          = errors.New("ai request failed")
	ErrInvalidAIResponse = errors.New("ai returned an invalid response")
)

// GenerateRequest adalah payload POST /api/posts/ai-generate.
type GenerateRequest struct {
	Topic            string `json:"topic" binding:"required,max=500"`
	Category         string `json:"category" binding:"omitempty,max=100"`
	IncludeWebSearch bool   `json:"include_web_search"`
}

// SearchSource adalah satu sumber rujukan dari Search Engine.
type SearchSource struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// GenerateResponse adalah draf artikel hasil AI beserta sumber rujukannya.
type GenerateResponse struct {
	Title    string         `json:"title"`
	Category string         `json:"category"`
	Excerpt  string         `json:"excerpt"`
	Content  string         `json:"content"`
	Sources  []SearchSource `json:"sources"`
}
