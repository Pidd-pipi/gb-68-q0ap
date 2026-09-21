package services

import (
	"errors"

	"irrigation/internal/middleware"
	"irrigation/internal/models"
	"irrigation/internal/utils"
	"irrigation/pkg/database"
)

type AuthService struct{}

func NewAuthService() *AuthService {
	return &AuthService{}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token string       `json:"token"`
	User  models.User `json:"user"`
}

func (s *AuthService) Login(req *LoginRequest) (*LoginResponse, error) {
	var user models.User
	if err := database.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		return nil, errors.New("invalid credentials")
	}

	if !utils.CheckPasswordHash(req.Password, user.PasswordHash) {
		return nil, errors.New("invalid credentials")
	}

	token, err := middleware.GenerateToken(user.ID, user.Username)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		Token: token,
		User:  user,
	}, nil
}

func (s *AuthService) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	if err := database.DB.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
