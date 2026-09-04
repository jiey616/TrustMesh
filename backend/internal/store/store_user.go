package store

import (
	"strings"
	"time"

	"go.uber.org/zap"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

func (s *Store) CreateUser(email, name, passwordHash string) (*model.User, *transport.AppError) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" || strings.TrimSpace(name) == "" || passwordHash == "" {
		return nil, transport.Validation("invalid register payload", map[string]any{"email": "required", "name": "required", "password": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.usersByMail[normalized]; exists {
		return nil, transport.Conflict("EMAIL_EXISTS", "email already exists")
	}
	now := time.Now().UTC()
	id := newID()
	u := &model.User{
		ID:           id,
		Email:        normalized,
		Name:         strings.TrimSpace(name),
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.users[id] = u
	s.usersByMail[normalized] = id
	if err := s.persistUserUnsafe(u); err != nil {
		return nil, mongoWriteError(err)
	}

	// 多租户阶段 1：新用户同步开通个人租户，保证增量数据始终有归属。
	// 个人租户是兜底设施，失败只告警、不阻断注册（可事后补偿）。
	if _, appErr := s.ensurePersonalOrgUnsafe(u.ID, u.Name); appErr != nil && s.log != nil {
		s.log.Warn("ensure personal org failed", zap.String("user_id", u.ID), zap.String("code", appErr.Code), zap.String("message", appErr.Message))
	}

	return copyUser(u), nil
}

func (s *Store) FindUserByEmail(email string) (*model.User, bool) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.usersByMail[normalized]
	if !ok {
		return nil, false
	}
	u, ok := s.users[id]
	if !ok {
		return nil, false
	}
	return copyUser(u), true
}

func (s *Store) FindUserByID(userID string) (*model.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[userID]
	if !ok {
		return nil, false
	}
	return copyUser(u), true
}
