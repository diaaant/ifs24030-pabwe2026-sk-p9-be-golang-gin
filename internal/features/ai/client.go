package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatClient mengirim prompt ke model bahasa dan mengembalikan teks jawabannya.
type ChatClient interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// OpenAIClient adalah client HTTP untuk OpenAI Chat Completions.
type OpenAIClient struct {
	endpoint string
	apiKey   string
	model    string
	http     *http.Client
}

// NewOpenAIClient membuat OpenAIClient. Endpoint berupa base URL (mis. .../v1).
func NewOpenAIClient(endpoint, apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		apiKey:   apiKey,
		model:    model,
		http:     &http.Client{Timeout: 120 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Complete memanggil POST {endpoint}/chat/completions.
func (c *OpenAIClient) Complete(ctx context.Context, system, user string) (string, error) {
	// Marshal struct sederhana berisi string tidak pernah gagal.
	body, _ := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.7,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return "", fmt.Errorf("openai returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("openai returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
