package ai

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"app/internal/features/post"
	"app/internal/response"
)

// Handler adalah controller POST /api/posts/ai-generate.
type Handler struct {
	svc *Service
}

// NewHandler membuat Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Generate: POST /api/posts/ai-generate
func (h *Handler) Generate(c *gin.Context) {
	var req GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	result, err := h.svc.Generate(c.Request.Context(), req)
	if err == nil {
		response.Success(c, http.StatusOK, "Article draft generated", result)
		return
	}
	if errors.Is(err, post.ErrInvalidCategory) {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	message := ErrInvalidAIResponse.Error()
	switch {
	case errors.Is(err, ErrSearchFailed):
		message = "Search engine request failed"
	case errors.Is(err, ErrAIFailed):
		message = "AI request failed"
	}
	response.Error(c, http.StatusBadGateway, message)
}
