package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const maxSources = 5

// Searcher mencari fakta terkini untuk sebuah topik.
type Searcher interface {
	Search(ctx context.Context, query string) ([]SearchSource, error)
}

// SearchClient adalah client HTTP untuk Search Engine (GET endpoint?q=...).
type SearchClient struct {
	endpoint string
	apiKey   string
	http     *http.Client
}

// NewSearchClient membuat SearchClient.
func NewSearchClient(endpoint, apiKey string) *SearchClient {
	return &SearchClient{endpoint: endpoint, apiKey: apiKey, http: &http.Client{Timeout: 30 * time.Second}}
}

type searchItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Link        string `json:"link"`
	Snippet     string `json:"snippet"`
	Content     string `json:"content"`
	Description string `json:"description"`
}

// searchPayload menerima bentuk {"results": [...]} maupun {"data": {"results": [...]}}.
type searchPayload struct {
	Results []searchItem `json:"results"`
	Data    struct {
		Results []searchItem `json:"results"`
	} `json:"data"`
}

// Search mengirim query dan menormalkan hasilnya (maksimal 5 sumber).
func (s *SearchClient) Search(ctx context.Context, query string) ([]SearchSource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("q", query)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("search engine returned status %d", resp.StatusCode)
	}

	var payload searchPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	items := payload.Results
	if len(items) == 0 {
		items = payload.Data.Results
	}

	sources := make([]SearchSource, 0, maxSources)
	for _, item := range items {
		if len(sources) == maxSources {
			break
		}
		sources = append(sources, SearchSource{
			Title:   item.Title,
			URL:     firstNonEmpty(item.URL, item.Link),
			Snippet: firstNonEmpty(item.Snippet, item.Content, item.Description),
		})
	}
	return sources, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
