package controllers

import (
	"github.com/gin-gonic/gin"

	"irrigation/internal/services"
	"irrigation/pkg/response"
)

type AuthController struct {
	authService *services.AuthService
}

func NewAuthController() *AuthController {
	return &AuthController{
		authService: services.NewAuthService(),
	}
}

// Login godoc
// @Summary 用户登录
// @Description 用户登录获取JWT Token
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body services.LoginRequest true "登录信息"
// @Success 200 {object} services.LoginResponse
// @Failure 401 {object} response.Response
// @Router /api/auth/login [post]
func (c *AuthController) Login(ctx *gin.Context) {
	var req services.LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "Invalid request body")
		return
	}

	result, err := c.authService.Login(&req)
	if err != nil {
		response.Unauthorized(ctx, err.Error())
		return
	}

	response.Success(ctx, result)
}

// GetProfile godoc
// @Summary 获取当前用户信息
// @Description 获取当前登录用户的信息
// @Tags 认证
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} models.User
// @Failure 401 {object} response.Response
// @Router /api/auth/profile [get]
func (c *AuthController) GetProfile(ctx *gin.Context) {
	userID := ctx.GetUint("user_id")
	user, err := c.authService.GetUserByID(userID)
	if err != nil {
		response.NotFound(ctx, "User not found")
		return
	}

	response.Success(ctx, user)
}
