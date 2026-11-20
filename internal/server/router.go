// Package server merakit seluruh rute API, Swagger UI, dan handler 404.
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	"app/docs"
	"app/internal/config"
	"app/internal/database"
	"app/internal/features/ai"
	"app/internal/features/auth"
	"app/internal/features/post"
	"app/internal/middleware"
	"app/internal/response"
)

// swaggerJSON dapat diganti pada pengujian untuk memicu jalur error.
var swaggerJSON = docs.GetSwaggerJSON

// NewRouter membuat Gin engine lengkap dengan seluruh rute aplikasi.
func NewRouter(db *gorm.DB, cfg *config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), middleware.CORS(), middleware.RateLimiter())

	// Dependency wiring
	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(db), cfg.AppKey))
	postHandler := post.NewHandler(post.NewService(post.NewRepository(db)))
	aiHandler := ai.NewHandler(ai.NewService(
		ai.NewOpenAIClient(cfg.OpenAIEndpoint, cfg.OpenAIKey, cfg.OpenAIModel),
		ai.NewSearchClient(cfg.SearchEngineEndpoint, cfg.OpenAIKey),
	))

	// Dokumentasi
	r.GET("/api.json", func(c *gin.Context) {
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		spec, err := swaggerJSON(c.Request.Host, scheme)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "Failed to load API specification")
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", spec)
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/api.json")))

	api := r.Group("/api")

	// Publik
	api.POST("/auth/register", authHandler.Register)
	api.POST("/auth/login", authHandler.Login)
	api.GET("/public/categories", postHandler.Categories)
	api.GET("/public/posts", postHandler.ListPublic)
	api.GET("/public/posts/:slug", postHandler.GetPublic)

	// Terautentikasi (penulis & admin)
	authed := api.Group("", middleware.Auth(cfg.AppKey))
	authed.POST("/auth/logout", authHandler.Logout)
	authed.GET("/auth/profile", authHandler.Profile)
	authed.PUT("/auth/profile", authHandler.UpdateProfile)

	authed.GET("/posts", postHandler.ListMine)
	authed.POST("/posts", postHandler.Create)
	authed.GET("/posts/stats", postHandler.Stats)
	authed.POST("/posts/ai-generate", aiHandler.Generate)
	authed.GET("/posts/:id", postHandler.GetMine)
	authed.PUT("/posts/:id", postHandler.Update)
	authed.PATCH("/posts/:id/toggle-publish", postHandler.TogglePublish)
	authed.DELETE("/posts/:id", postHandler.Delete)

	// Khusus admin
	admin := authed.Group("/admin", middleware.RequireRole(database.RoleAdmin))
	admin.GET("/users", postHandler.AdminUsers)
	admin.GET("/posts", postHandler.AdminPosts)
	admin.GET("/users/:id/posts", postHandler.AdminUserPosts)

	r.NoRoute(func(c *gin.Context) {
		response.Fail(c, http.StatusNotFound, "Endpoint not found")
	})

	return r
}
