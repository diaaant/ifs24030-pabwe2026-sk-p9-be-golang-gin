// Package auth menangani registrasi, login JWT, dan profil pengguna.
package auth

import (
	"time"

	"app/internal/database"
)

// RegisterRequest adalah payload registrasi akun baru.
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=255"`
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required,min=6,max=72"`
}

// LoginRequest adalah payload login.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// UpdateProfileRequest adalah payload pembaruan profil. Password opsional.
type UpdateProfileRequest struct {
	Username string `json:"username" binding:"required,min=3,max=255"`
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"omitempty,min=6,max=72"`
}

// UserResponse adalah representasi user yang aman dikirim ke klien.
type UserResponse struct {
	ID        uint      `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// LoginResponse berisi token JWT dan data user.
type LoginResponse struct {
	Token string       `json:"token"`
	User  UserResponse `json:"user"`
}

func toUserResponse(u *database.User) UserResponse {
	return UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
	}
}
