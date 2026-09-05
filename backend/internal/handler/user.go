package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type UserHandler struct {
	store *store.Store
}

func NewUserHandler(s *store.Store) *UserHandler {
	return &UserHandler{store: s}
}

// Me 返回当前登录用户的最新资料（改名后前端用其刷新本地缓存）。
func (h *UserHandler) Me(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	user, exists := h.store.FindUserByID(userID)
	if !exists {
		transport.WriteError(c, transport.NotFound("user not found"))
		return
	}
	transport.WriteData(c, http.StatusOK, gin.H{"user": user})
}

type updateProfileRequest struct {
	Name string `json:"name"`
}

// UpdateProfile 修改当前登录用户的资料（目前仅支持显示名）。
// 账号是 user 维度资源，不参与租户裁决：改名只影响账号自身与其个人工作区，
// 企业租户名称独立于用户名，不受影响。
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 64 {
		transport.WriteError(c, transport.Validation("invalid update payload", map[string]any{
			"name": "required, at most 64 chars",
		}))
		return
	}

	user, appErr := h.store.UpdateUserName(userID, req.Name)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, gin.H{"user": user})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword 修改当前登录用户的密码。
// 必须先验证旧密码：否则被盗会话可直接改密接管账号。
func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "invalid json body"))
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		transport.WriteError(c, transport.Validation("invalid update payload", map[string]any{
			"old_password": "required",
			"new_password": "required",
		}))
		return
	}
	if len(req.NewPassword) < 8 {
		transport.WriteError(c, transport.Validation("invalid update payload", map[string]any{
			"new_password": "must be at least 8 chars",
		}))
		return
	}
	if req.OldPassword == req.NewPassword {
		transport.WriteError(c, transport.Validation("invalid update payload", map[string]any{
			"new_password": "must differ from old password",
		}))
		return
	}

	if !h.store.VerifyUserPassword(userID, req.OldPassword) {
		transport.WriteError(c, &transport.AppError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_CREDENTIALS",
			Message: "old password is incorrect",
			Details: map[string]any{},
		})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		transport.WriteError(c, &transport.AppError{Status: 500, Code: "INTERNAL_ERROR", Message: "failed to hash password", Details: map[string]any{}})
		return
	}

	if appErr := h.store.UpdateUserPassword(userID, string(hash)); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// 不签发新 token：JWT 内只有 userID，改密不影响当前会话的合法性。
	// 已知限制：无状态的 JWT 无法主动失效，已签发的旧 token 在过期前仍可用。
	transport.WriteData(c, http.StatusOK, gin.H{"ok": true})
}
