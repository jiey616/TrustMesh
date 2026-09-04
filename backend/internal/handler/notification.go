package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type NotificationHandler struct {
	store *store.Store
}

func NewNotificationHandler(s *store.Store) *NotificationHandler {
	return &NotificationHandler{store: s}
}

func (h *NotificationHandler) List(c *gin.Context) {
	sc := middleware.Scope(c)
	filter := c.DefaultQuery("filter", "recent")
	limit := queryInt(c, "limit", 50)
	items, appErr := h.store.ListNotifications(sc, filter, limit)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteList(c, items, len(items))
}

func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	sc := middleware.Scope(c)
	count := h.store.UnreadNotificationCount(sc)
	transport.WriteData(c, http.StatusOK, map[string]int{"count": count})
}

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	sc := middleware.Scope(c)
	appErr := h.store.MarkNotificationRead(sc, c.Param("id"))
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	transport.WriteData(c, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	sc := middleware.Scope(c)
	count := h.store.MarkAllNotificationsRead(sc)
	transport.WriteData(c, http.StatusOK, map[string]int{"marked": count})
}
