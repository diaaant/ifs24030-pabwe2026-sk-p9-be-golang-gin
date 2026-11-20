package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"app/internal/database"
	"app/internal/middleware"
)

// Error domain yang dipetakan handler ke status HTTP.
var (
	ErrUsernameTaken      = errors.New("username is already taken")
	ErrEmailTaken         = errors.New("email is already registered")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserNotFound       = errors.New("user not found")
)

// Service berisi logika bisnis autentikasi.
type Service struct {
	repo   Repository
	secret string
	ttl    time.Duration
}

// NewService membuat Service dengan masa berlaku token 24 jam.
func NewService(repo Repository, secret string) *Service {
	return &Service{repo: repo, secret: secret, ttl: 24 * time.Hour}
}

// Register membuat akun baru dengan role member.
func (s *Service) Register(req RegisterRequest) (*UserResponse, error) {
	if err := s.ensureUnique(req.Username, req.Email, 0); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &database.User{
		Username: req.Username,
		Email:    req.Email,
		Password: string(hash),
		Role:     database.RoleMember,
	}
	if err := s.repo.Create(user); err != nil {
		return nil, err
	}

	resp := toUserResponse(user)
	return &resp, nil
}

// Login memverifikasi kredensial dan menerbitkan token JWT.
func (s *Service) Login(req LoginRequest) (*LoginResponse, error) {
	user, err := s.repo.FindByUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		return nil, ErrInvalidCredentials
	}

	return &LoginResponse{Token: s.generateToken(user), User: toUserResponse(user)}, nil
}

// GetProfile mengambil profil user berdasarkan ID.
func (s *Service) GetProfile(id uint) (*UserResponse, error) {
	user, err := s.findByID(id)
	if err != nil {
		return nil, err
	}
	resp := toUserResponse(user)
	return &resp, nil
}

// UpdateProfile memperbarui username, email, dan (opsional) password.
func (s *Service) UpdateProfile(id uint, req UpdateProfileRequest) (*UserResponse, error) {
	user, err := s.findByID(id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureUnique(req.Username, req.Email, id); err != nil {
		return nil, err
	}

	user.Username = req.Username
	user.Email = req.Email
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		user.Password = string(hash)
	}
	if err := s.repo.Update(user); err != nil {
		return nil, err
	}

	resp := toUserResponse(user)
	return &resp, nil
}

func (s *Service) findByID(id uint) (*database.User, error) {
	user, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// ensureUnique memastikan username & email tidak dipakai user lain (selain exceptID).
func (s *Service) ensureUnique(username, email string, exceptID uint) error {
	byName, err := s.repo.FindByUsername(username)
	if err != nil {
		return err
	}
	if byName != nil && byName.ID != exceptID {
		return ErrUsernameTaken
	}

	byEmail, err := s.repo.FindByEmail(email)
	if err != nil {
		return err
	}
	if byEmail != nil && byEmail.ID != exceptID {
		return ErrEmailTaken
	}
	return nil
}

func (s *Service) generateToken(user *database.User) string {
	now := time.Now()
	claims := middleware.Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
	}
	// SignedString dengan kunci []byte untuk HS256 tidak pernah gagal.
	signed, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.secret))
	return signed
}
