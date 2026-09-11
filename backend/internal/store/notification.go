package store

import (
	"sort"
	"time"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// ListNotifications 返回用户通知 feed。通知是 user 维度数据（发给谁的就归谁），
// 不按租户共享、不因租户上下文改变归属；收 Scope 参数只为调用侧统一。
func (s *Store) ListNotifications(sc Scope, filter string, limit int) ([]model.Notification, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.userNotifications[sc.UserID]
	items := make([]model.Notification, 0)

	for i := len(ids) - 1; i >= 0; i-- {
		n, ok := s.notifications[ids[i]]
		if !ok {
			continue
		}
		switch filter {
		case "unread":
			if n.IsRead {
				continue
			}
		case "recent":
			if time.Since(n.CreatedAt) > 24*time.Hour {
				continue
			}
		}
		items = append(items, *n)
		if limit > 0 && len(items) >= limit {
			break
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) UnreadNotificationCount(sc Scope) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, id := range s.userNotifications[sc.UserID] {
		if n, ok := s.notifications[id]; ok && !n.IsRead {
			count++
		}
	}
	return count
}

func (s *Store) MarkNotificationRead(sc Scope, notificationID string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.notifications[notificationID]
	// 已读态是「个人维度数据」（见 scope.go ownedByUser 注释）：即便 Notification
	// 带有 OrgID，也不能走 visibleToScope 的 org 共享语义，否则同租户成员可以
	// 互相把对方的通知标记为已读。这里统一到 scope.go 的 ownedByUser 裁决。
	if !ok || !ownedByUser(sc, n.UserID) {
		return transport.NotFound("notification not found")
	}
	if n.IsRead {
		return nil
	}
	now := time.Now().UTC()
	n.IsRead = true
	n.ReadAt = &now
	s.persistNotificationUnsafe(n)
	s.publishUserEventUnsafe(sc.UserID, "notification.read", map[string]any{
		"notification_id": notificationID,
		"read_at":         now,
		"unread_count":    unreadNotificationCountUnsafe(s.notifications, s.userNotifications[sc.UserID]),
	}, now)
	return nil
}

func (s *Store) MarkAllNotificationsRead(sc Scope) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	now := time.Now().UTC()
	markedIDs := make([]string, 0)
	for _, id := range s.userNotifications[sc.UserID] {
		n, ok := s.notifications[id]
		if !ok || n.IsRead {
			continue
		}
		n.IsRead = true
		n.ReadAt = &now
		s.persistNotificationUnsafe(n)
		markedIDs = append(markedIDs, id)
		count++
	}
	if count > 0 {
		s.publishUserEventUnsafe(sc.UserID, "notifications.all_read", map[string]any{
			"notification_ids": markedIDs,
			"read_at":          now,
			"unread_count":     0,
		}, now)
	}
	return count
}

func unreadNotificationCountUnsafe(notifications map[string]*model.Notification, ids []string) int {
	count := 0
	for _, id := range ids {
		if n, ok := notifications[id]; ok && !n.IsRead {
			count++
		}
	}
	return count
}
