package store

import (
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

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
	// A1：首个注册用户自动成为平台管理员（可管平台级 LLM 配置）。
	firstUser := len(s.users) == 0
	u := &model.User{
		ID:           id,
		Email:        normalized,
		Name:         strings.TrimSpace(name),
		PasswordHash: passwordHash,
		IsAdmin:      firstUser,
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

// UpdateUserName 更新用户显示名，并同步其个人租户名称。
// 个人租户即用户本人的工作区，不同步会导致改名后侧边栏「个人空间」仍显示旧名。
// 企业租户名称独立于用户名，不受影响。
func (s *Store) UpdateUserName(userID, name string) (*model.User, *transport.AppError) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, transport.Validation("invalid update payload", map[string]any{"name": "required"})
	}
	if len([]rune(name)) > 64 {
		return nil, transport.Validation("invalid update payload", map[string]any{"name": "must be at most 64 chars"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	u, ok := s.users[userID]
	if !ok {
		return nil, transport.NotFound("user not found")
	}
	u.Name = name
	u.UpdatedAt = time.Now().UTC()
	if err := s.persistUserUnsafe(u); err != nil {
		return nil, mongoWriteError(err)
	}

	if orgID := s.personalOrgOfUnsafe(userID); orgID != "" {
		if org, ok := s.organizations[orgID]; ok && org.Kind == model.OrgKindPersonal {
			org.Name = name
			org.UpdatedAt = u.UpdatedAt
			if err := s.persistOrganizationUnsafe(org); err != nil && s.log != nil {
				s.log.Warn("sync personal org name failed", zap.String("user_id", userID), zap.Error(err))
			}
		}
	}

	return copyUser(u), nil
}

// VerifyUserPassword 校验用户当前密码。用于改密前的身份确认。
// bcrypt 比对耗时较长，先拷出哈希再释放锁，避免长时间持锁。
func (s *Store) VerifyUserPassword(userID, plainPassword string) bool {
	s.mu.RLock()
	u, ok := s.users[userID]
	var hash string
	if ok {
		hash = u.PasswordHash
	}
	s.mu.RUnlock()

	if !ok || hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plainPassword)) == nil
}

// UpdateUserPassword 写入新密码哈希。调用方须先用 VerifyUserPassword 确认旧密码。
func (s *Store) UpdateUserPassword(userID, newPasswordHash string) *transport.AppError {
	if newPasswordHash == "" {
		return transport.Validation("invalid update payload", map[string]any{"password": "required"})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	u, ok := s.users[userID]
	if !ok {
		return transport.NotFound("user not found")
	}
	u.PasswordHash = newPasswordHash
	u.UpdatedAt = time.Now().UTC()
	if err := s.persistUserUnsafe(u); err != nil {
		return mongoWriteError(err)
	}
	return nil
}
