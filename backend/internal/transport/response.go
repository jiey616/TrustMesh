package transport

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ListMeta struct {
	Count int `json:"count"`
}

type AppError struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
}

func (e *AppError) Error() string {
	return e.Code + ": " + e.Message
}

func NewError(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message, Details: map[string]any{}}
}

func WriteData(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data})
}

func WriteList(c *gin.Context, items any, count int) {
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{"items": items},
		"meta": ListMeta{Count: count},
	})
}

func WriteError(c *gin.Context, err *AppError) {
	details := err.Details
	if details == nil {
		details = map[string]any{}
	}
	c.JSON(err.Status, ErrorResponse{Error: ErrorBody{
		Code:    err.Code,
		Message: err.Message,
		Details: details,
	}})
}

func BadRequest(code, message string) *AppError {
	return NewError(http.StatusBadRequest, code, message)
}

func Unauthorized(message string) *AppError {
	return NewError(http.StatusUnauthorized, "UNAUTHORIZED", message)
}

// OrgScopeDenied 用于「已认证，但当前用户不属于请求头 X-Org-Id 指定的租户」这一拒绝场景。
//
// 语义上更贴近 403 Forbidden，但本批刻意维持 401，以最小化对既有消费方的破坏
// （改状态码会牵动未知的既有断言/客户端分支，不在本批范围）。关键点是它携带
// **独立 code**（"NOT_A_MEMBER"）——使客户端能靠 code 天然区分
// 「token 过期/无效」（UNAUTHORIZED）与「租户越权」（NOT_A_MEMBER），
// 无需依赖 message 文案（文案耦合的失效方向是静默的）。
func OrgScopeDenied(message string) *AppError {
	return NewError(http.StatusUnauthorized, "NOT_A_MEMBER", message)
}

func Forbidden(message string) *AppError {
	return NewError(http.StatusForbidden, "FORBIDDEN", message)
}

func NotFound(message string) *AppError {
	return NewError(http.StatusNotFound, "NOT_FOUND", message)
}

func Conflict(code, message string) *AppError {
	return NewError(http.StatusConflict, code, message)
}

func Validation(message string, details map[string]any) *AppError {
	err := NewError(http.StatusUnprocessableEntity, "VALIDATION_ERROR", message)
	err.Details = details
	return err
}
