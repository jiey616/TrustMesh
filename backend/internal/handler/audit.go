package handler

import (
	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

// 审计切面（设计文档 §6.4）：企业敏感操作与平台操作各落一条 audit_logs。
//
// 调用约定：**业务成功之后**调用（失败的操作不落审计），且不参与错误处理 ——
// 审计写失败只由 store 记告警，绝不能让审计缺失变成业务失败。
func recordAudit(c *gin.Context, s *store.Store, scope, action, targetType, targetID string, detail map[string]any) {
	if s == nil || c == nil || action == "" {
		return
	}
	userID := middleware.UserID(c)
	email := ""
	if u, ok := s.FindUserByID(userID); ok {
		email = u.Email
	}
	s.RecordAudit(model.AuditLog{
		ActorUserID: userID,
		ActorEmail:  email,
		Scope:       scope,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		Detail:      detail,
		IP:          c.ClientIP(),
	})
}
