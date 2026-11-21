package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"app/internal/middleware"
	"app/internal/response"
)

// Handler adalah controller endpoint /api/auth/*.
type Handler struct {
	svc *Service
}

// NewHandler membuat Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register godoc: POST /api/auth/register
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}
	result, err := h.svc.Register(req)
	if err != nil {
		if errors.Is(err, ErrUsernameTaken) || errors.Is(err, ErrEmailTaken) {
			response.Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		h.fail(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Registration successful", result)
}

// Login godoc: POST /api/auth/login
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}
	if req.Username == "" && req.Email == "" {
		response.Fail(c, http.StatusBadRequest, "Invalid request: username is required")
		return
	}
	result, err := h.svc.Login(req)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Login successful", result)
}

// Logout godoc: POST /api/auth/logout. JWT bersifat stateless sehingga token
// dibuang oleh klien; endpoint ini mengonfirmasi sesi berakhir.
func (h *Handler) Logout(c *gin.Context) {
	response.Success(c, http.StatusOK, "Logout successful", nil)
}

// Profile godoc: GET /api/auth/profile
func (h *Handler) Profile(c *gin.Context) {
	user, err := h.svc.GetProfile(middleware.CurrentUser(c).ID)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Profile retrieved", user)
}

// UpdateProfile godoc: PUT /api/auth/profile
func (h *Handler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}
	user, err := h.svc.UpdateProfile(middleware.CurrentUser(c).ID, req)
	if err != nil {
		h.fail(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Profile updated", user)
}

func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUsernameTaken), errors.Is(err, ErrEmailTaken):
		response.Fail(c, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidCredentials):
		response.Fail(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, ErrUserNotFound):
		response.Fail(c, http.StatusNotFound, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "Internal server error")
	}
}