package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"trustmesh/backend/internal/auth"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// disabledAccountError 是「账号已被平台管理员禁用」的统一响应。
// 用独立 code（USER_DISABLED）而非笼统 FORBIDDEN，使客户端能靠 code 区分
// 「被禁用」与「token 过期/租户越权」，不会误触发 refresh 重放。
func disabledAccountError() *transport.AppError {
	return transport.NewError(http.StatusForbidden, "USER_DISABLED", "account has been disabled by platform admin")
}

type AuthHandler struct {
	store *store.Store
	jwt   *auth.JWTManager
}

func NewAuthHandler(s *store.Store, jwt *auth.JWTManager) *AuthHandler {
	return &AuthHandler{store: s, jwt: jwt}
}

type registerRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Name = strings.TrimSpace(req.Name)
	if req.Email == "" || req.Name == "" || len(req.Password) < 8 {
		transport.WriteError(c, transport.Validation("invalid register payload", map[string]any{
			"email":    "required",
			"name":     "required",
			"password": "must be at least 8 chars",
		}))
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		transport.WriteError(c, &transport.AppError{Status: 500, Code: "INTERNAL_ERROR", Message: "failed to hash password", Details: map[string]any{}})
		return
	}

	user, appErr := h.store.CreateUser(req.Email, req.Name, string(hash))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	pair, err := h.jwt.IssueTokenPair(user.ID)
	if err != nil {
		transport.WriteError(c, &transport.AppError{Status: 500, Code: "INTERNAL_ERROR", Message: "failed to issue token", Details: map[string]any{}})
		return
	}

	transport.WriteData(c, 201, gin.H{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
		"user":          user,
	})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	// 权威读：多实例下进程内存可能落后（别的实例刚重置过密码/禁用过账号），
	// 因此登录以 Mongo 文档为准（Mongo 不可用/无该文档时回落内存），见
	// store.FindUserByEmailAuthoritative 的说明。
	user, ok := h.store.FindUserByEmailAuthoritative(req.Email)
	if !ok {
		transport.WriteError(c, &transport.AppError{Status: 401, Code: "INVALID_CREDENTIALS", Message: "invalid email or password", Details: map[string]any{}})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		transport.WriteError(c, &transport.AppError{Status: 401, Code: "INVALID_CREDENTIALS", Message: "invalid email or password", Details: map[string]any{}})
		return
	}
	// 平台管理员禁用（平台用户管理）：口令校验**之后**才报禁用态 ——
	// 反过来会把「账号是否被禁用」变成账号存在性的探针。
	if user.Disabled {
		transport.WriteError(c, disabledAccountError())
		return
	}
	// 平台管理员登录落审计（设计文档 §6.4）：这条链路上 RequireAuth 还没跑，
	// 因此直接构造审计记录（不经过 recordAudit 的中间件取 actor 逻辑）。
	if h.store.UserIsPlatformAdmin(user.ID) {
		h.store.RecordAudit(model.AuditLog{
			ActorUserID: user.ID,
			ActorEmail:  user.Email,
			Scope:       model.AuditScopePlatform,
			Action:      model.AuditActionPlatformAdminLogin,
			TargetType:  model.AuditTargetPlatformLogin,
			TargetID:    user.ID,
			IP:          c.ClientIP(),
		})
	}

	pair, err := h.jwt.IssueTokenPair(user.ID)
	if err != nil {
		transport.WriteError(c, &transport.AppError{Status: 500, Code: "INTERNAL_ERROR", Message: "failed to issue token", Details: map[string]any{}})
		return
	}
	transport.WriteData(c, 200, gin.H{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
		"user":          user,
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "refresh_token is required"))
		return
	}

	claims, err := h.jwt.ParseToken(req.RefreshToken)
	if err != nil {
		transport.WriteError(c, transport.Unauthorized("invalid or expired refresh token"))
		return
	}
	if claims.TokenType != auth.TokenTypeRefresh {
		transport.WriteError(c, transport.Unauthorized("invalid token type"))
		return
	}

	// Verify user still exists（权威读：禁用与改密都以 Mongo 文档为准，见
	// store.FindUserByIDAuthoritative；否则「换台实例就能靠 refresh 续命」）。
	user, ok := h.store.FindUserByIDAuthoritative(claims.UserID)
	if !ok {
		transport.WriteError(c, transport.Unauthorized("user not found"))
		return
	}
	// 被禁用的账号不得靠 refresh token 续命（否则禁用形同虚设）。
	if user.Disabled {
		transport.WriteError(c, disabledAccountError())
		return
	}

	pair, err := h.jwt.IssueTokenPair(claims.UserID)
	if err != nil {
		transport.WriteError(c, &transport.AppError{Status: 500, Code: "INTERNAL_ERROR", Message: "failed to issue token", Details: map[string]any{}})
		return
	}

	transport.WriteData(c, 200, gin.H{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
	})
}
