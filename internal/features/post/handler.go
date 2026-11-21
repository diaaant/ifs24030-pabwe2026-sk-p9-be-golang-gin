package post

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"app/internal/middleware"
	"app/internal/response"
)

// Handler adalah controller endpoint /api/public/*, /api/posts/*, dan /api/admin/*.
type Handler struct {
	svc *Service
}

// NewHandler membuat Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ---------- publik ----------

// Categories: GET /api/public/categories
func (h *Handler) Categories(c *gin.Context) {
	response.Success(c, http.StatusOK, "Categories retrieved", Categories)
}

// ListPublic: GET /api/public/posts?q=&category=
func (h *Handler) ListPublic(c *gin.Context) {
	data, err := h.svc.ListPublic(c.Query("q"), c.Query("category"))
	h.reply(c, http.StatusOK, "Posts retrieved", data, err)
}

// GetPublic: GET /api/public/posts/:slug
func (h *Handler) GetPublic(c *gin.Context) {
	data, err := h.svc.GetPublic(c.Param("slug"))
	h.reply(c, http.StatusOK, "Post retrieved", data, err)
}

// ---------- penulis ----------

// ListMine: GET /api/posts
func (h *Handler) ListMine(c *gin.Context) {
	data, err := h.svc.ListMine(middleware.CurrentUser(c).ID)
	h.reply(c, http.StatusOK, "Posts retrieved", data, err)
}

// Stats: GET /api/posts/stats
func (h *Handler) Stats(c *gin.Context) {
	data, err := h.svc.Stats(middleware.CurrentUser(c).ID)
	h.reply(c, http.StatusOK, "Stats retrieved", data, err)
}

// GetMine: GET /api/posts/:id
func (h *Handler) GetMine(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	data, err := h.svc.GetMine(middleware.CurrentUser(c).ID, id)
	h.reply(c, http.StatusOK, "Post retrieved", data, err)
}

// Create: POST /api/posts
func (h *Handler) Create(c *gin.Context) {
	var req PostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}
	data, err := h.svc.Create(middleware.CurrentUser(c).ID, req)
	h.reply(c, http.StatusCreated, "Post created", data, err)
}

// Update: PUT/PATCH /api/posts/:id. Kepemilikan artikel diperiksa lebih dulu
// (404 / 403) sebelum isi body divalidasi (400).
func (h *Handler) Update(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	userID := middleware.CurrentUser(c).ID
	if _, err := h.svc.GetMine(userID, id); err != nil {
		h.reply(c, http.StatusOK, "", nil, err)
		return
	}
	var req PostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}
	data, err := h.svc.Update(userID, id, req)
	h.reply(c, http.StatusOK, "Post updated", data, err)
}

// TogglePublish: PATCH /api/posts/:id/toggle-publish
func (h *Handler) TogglePublish(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	data, err := h.svc.TogglePublish(middleware.CurrentUser(c).ID, id)
	h.reply(c, http.StatusOK, "Publish status updated", data, err)
}

// Delete: DELETE /api/posts/:id
func (h *Handler) Delete(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	h.reply(c, http.StatusOK, "Post deleted", nil, h.svc.Delete(middleware.CurrentUser(c).ID, id))
}

// ---------- admin ----------

// AdminUsers: GET /api/admin/users
func (h *Handler) AdminUsers(c *gin.Context) {
	data, err := h.svc.AdminUsers()
	h.reply(c, http.StatusOK, "Users retrieved", data, err)
}

// AdminPosts: GET /api/admin/posts
func (h *Handler) AdminPosts(c *gin.Context) {
	data, err := h.svc.AdminPosts()
	h.reply(c, http.StatusOK, "Posts retrieved", data, err)
}

// AdminUserPosts: GET /api/admin/users/:id/posts
func (h *Handler) AdminUserPosts(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	data, err := h.svc.AdminUserPosts(id)
	h.reply(c, http.StatusOK, "Posts retrieved", data, err)
}

// ---------- helper ----------

func idParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "Invalid ID")
		return 0, false
	}
	return uint(id), true
}

// reply mengirim respons sukses, atau memetakan error domain ke status HTTP.
func (h *Handler) reply(c *gin.Context, status int, message string, data any, err error) {
	switch {
	case err == nil:
		response.Success(c, status, message, data)
	case errors.Is(err, ErrPostNotFound), errors.Is(err, ErrUserNotFound):
		response.Fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrForbidden):
		response.Fail(c, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidCategory):
		response.Fail(c, http.StatusBadRequest, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "Internal server error")
	}
}