package store

import (
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
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
	// 平台管理员打标（权限体系 §6.3）：
	//   - 配了 PLATFORM_ADMIN_EMAILS（种子模式）→ 只认 env 白名单（不需要重启即生效）；
	//   - 未配 env → 保持历史行为：首个注册用户自动成为平台管理员（可管平台级配置）。
	firstUser := len(s.users) == 0
	seedMode := len(s.platformAdminEmails) > 0
	u := &model.User{
		ID:           id,
		Email:        normalized,
		Name:         strings.TrimSpace(name),
		PasswordHash: passwordHash,
		IsAdmin:      s.platformAdminEmails[normalized] || (!seedMode && firstUser),
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

// ─── 权威用户读（多实例下登录/刷新的读侧兜底）────────────────────────────────
//
// 背景：FindUserByEmail / FindUserByID 只读进程内存，而用户写路径是「内存 + Mongo 整文档
// ReplaceOne」。双实例共享同一 Mongo 时会出现两种失真（2026-09-16 生产实测）：
//  1. 实例 A 重置密码 / 禁用账号后，实例 B 的内存仍是旧值 → 命中 B 的登录用旧哈希校验，
//     表现为「同一密码 10 次登录 6 次成功、4 次 INVALID_CREDENTIALS」，禁用账号在 B 上照样
//     拿到 access token（禁用形同虚设）；
//  2. 更糟的是 B 之后的任意一次用户写会把 A 刚写的字段整文档覆盖回旧值（实测 Mongo 里的
//     disabled=true 被后一次重置密码覆盖没了）。
//
// 处理：只让「登录」与「刷新」这两条安全敏感路径改走 **Mongo 优先读**，其它读路径保持内存读
// （读侧陈旧窗口是既定取舍，见设计文档）。Mongo 未启用或文档缺失时回落内存，单机与降级行为
// 完全不变。这里刻意**不回写内存**：避免再引入一条「用可能陈旧的副本覆盖内存」的整文档写路径。
//
// 性能：仅登录 / 刷新调用（非热路径，刷新为每客户端 15 分钟一次），一次点查 + 不持锁，
// 不会把 Mongo 往返塞进 s.mu 临界区。

// FindUserByEmailAuthoritative 按邮箱读取权威用户记录（Mongo 优先，回落内存）。
func (s *Store) FindUserByEmailAuthoritative(email string) (*model.User, bool) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if u, ok := s.findUserInMongo(bson.M{"email": normalized}); ok {
		return u, true
	}
	return s.FindUserByEmail(normalized)
}

// FindUserByIDAuthoritative 按 id 读取权威用户记录（Mongo 优先，回落内存）。
func (s *Store) FindUserByIDAuthoritative(userID string) (*model.User, bool) {
	if u, ok := s.findUserInMongo(bson.M{"_id": userID}); ok {
		return u, true
	}
	return s.FindUserByID(userID)
}

// findUserInMongo 直读 users 集合。不持 s.mu：Mongo 句柄在服务启动后只读，
// 且把网络往返放进锁临界区会拖慢全部用户读写（同 refresh_on_conflict.go 的取舍）。
func (s *Store) findUserInMongo(filter bson.M) (*model.User, bool) {
	collection := s.mongoUsers
	if !s.mongoEnabled || collection == nil {
		return nil, false
	}
	ctx, cancel := s.mongoContext()
	defer cancel()
	var u model.User
	if err := collection.FindOne(ctx, filter).Decode(&u); err != nil {
		return nil, false
	}
	return &u, true
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
	// 字段级落库：只落 password_hash / updated_at，避免用本实例可能落后的内存快照
	// 整文档覆盖掉其它实例刚写的字段（例如平台侧的 disabled）。
	if err := s.persistUserFieldsUnsafe(u.ID, bson.M{
		"password_hash": newPasswordHash,
		"updated_at":    u.UpdatedAt,
	}, nil); err != nil {
		return mongoWriteError(err)
	}
	return nil
}
