package store

import (
	"net/http"
	"testing"
	"time"
)

// newStoreWithSeedAdmin 让 store 进入 env 种子模式：未配置种子时 CreateUser 会把
// 「首个注册用户」自动提升为平台管理员（历史兜底），而平台管理员账号不可禁用，
// 会干扰以下用例。种子里放一个不参与用例的邮箱即可关掉该兜底。
func newStoreWithSeedAdmin() *Store {
	s := New()
	s.SeedPlatformAdmins([]string{"seed-admin@example.com"})
	return s
}

// ListAllUsers：按注册时间升序返回，且返回的是副本（改动不回流 store 内部状态）。
func TestListAllUsersSortedAndCopied(t *testing.T) {
	s := New()

	u1, appErr := s.CreateUser("a@example.com", "A", "hash")
	if appErr != nil {
		t.Fatalf("create user a: %v", appErr)
	}
	u2, appErr := s.CreateUser("b@example.com", "B", "hash")
	if appErr != nil {
		t.Fatalf("create user b: %v", appErr)
	}
	u3, appErr := s.CreateUser("c@example.com", "C", "hash")
	if appErr != nil {
		t.Fatalf("create user c: %v", appErr)
	}

	// CreateUser 用 time.Now() 生成 CreatedAt，同进程内可能撞同一纳秒；
	// 直接写死注册时间，让排序断言不依赖时钟精度。
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.mu.Lock()
	s.users[u1.ID].CreatedAt = base.Add(2 * time.Second)
	s.users[u2.ID].CreatedAt = base
	s.users[u3.ID].CreatedAt = base.Add(1 * time.Second)
	s.mu.Unlock()

	all := s.ListAllUsers()
	if len(all) != 3 {
		t.Fatalf("ListAllUsers len = %d, want 3", len(all))
	}
	wantOrder := []string{u2.ID, u3.ID, u1.ID}
	for i, want := range wantOrder {
		if all[i].ID != want {
			t.Fatalf("ListAllUsers[%d] = %s, want %s（应按注册时间升序）", i, all[i].ID, want)
		}
	}

	// 副本语义：改返回值不得影响 store 内部状态。
	all[0].Name = "Hacked"
	all[0].IsAdmin = true
	all[0].Disabled = true
	if u := s.users[u2.ID]; u.Name != "B" || u.IsAdmin || u.Disabled {
		t.Fatalf("ListAllUsers 返回值不是副本，store 内部被改写: %+v", u)
	}
}

// SetUserDisabled：禁用置 DisabledAt、启用清空；幂等调用不写库（凭时间戳不变判定）。
func TestSetUserDisabledLifecycle(t *testing.T) {
	s := newStoreWithSeedAdmin()

	u, appErr := s.CreateUser("d@example.com", "D", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	before := s.users[u.ID].UpdatedAt

	disabled, appErr := s.SetUserDisabled(u.ID, true)
	if appErr != nil {
		t.Fatalf("disable user: %v", appErr)
	}
	if !disabled.Disabled || disabled.DisabledAt == nil {
		t.Fatalf("禁用后返回值应带禁用态: %+v", disabled)
	}
	stored := s.users[u.ID]
	if !stored.Disabled || stored.DisabledAt == nil {
		t.Fatalf("store 内部未落禁用态: %+v", stored)
	}
	if !stored.UpdatedAt.After(before) {
		t.Fatal("禁用应更新 UpdatedAt")
	}
	firstDisabledAt := *stored.DisabledAt
	firstUpdatedAt := stored.UpdatedAt

	// 幂等：重复禁用不得刷新时间戳（handler 据此判断「无变化不落审计」）。
	again, appErr := s.SetUserDisabled(u.ID, true)
	if appErr != nil {
		t.Fatalf("disable user again: %v", appErr)
	}
	if !again.Disabled {
		t.Fatal("第二次禁用的返回值 Disabled = false")
	}
	stored = s.users[u.ID]
	if !stored.UpdatedAt.Equal(firstUpdatedAt) || !stored.DisabledAt.Equal(firstDisabledAt) {
		t.Fatal("重复禁用被当作变更写了库（时间戳被刷新）")
	}

	// 副本语义：返回值的 DisabledAt 是指针，改动不得回流 store。
	*again.DisabledAt = time.Time{}
	if s.users[u.ID].DisabledAt.IsZero() {
		t.Fatal("SetUserDisabled 返回值的 DisabledAt 不是副本")
	}

	enabled, appErr := s.SetUserDisabled(u.ID, false)
	if appErr != nil {
		t.Fatalf("enable user: %v", appErr)
	}
	if enabled.Disabled || enabled.DisabledAt != nil {
		t.Fatalf("启用后应清空禁用态: %+v", enabled)
	}
	if stored = s.users[u.ID]; stored.Disabled || stored.DisabledAt != nil {
		t.Fatalf("store 内部未恢复启用态: %+v", stored)
	}

	// 重复启用同样幂等。
	reEnabled, appErr := s.SetUserDisabled(u.ID, false)
	if appErr != nil {
		t.Fatalf("enable user again: %v", appErr)
	}
	if reEnabled.Disabled || reEnabled.DisabledAt != nil {
		t.Fatalf("重复启用的返回值应保持启用态: %+v", reEnabled)
	}
}

// SetUserDisabled：未知用户 404；平台管理员账号拒绝（env 种子为唯一权威）。
func TestSetUserDisabledRejections(t *testing.T) {
	s := newStoreWithSeedAdmin()

	_, appErr := s.SetUserDisabled("ghost", true)
	if appErr == nil {
		t.Fatal("未知用户必须报错")
	}
	if appErr.Status != http.StatusNotFound || appErr.Code != "NOT_FOUND" {
		t.Fatalf("未知用户错误 = %d/%s, want 404/NOT_FOUND", appErr.Status, appErr.Code)
	}

	admin, appErr := s.CreateUser("admin@example.com", "Admin", "hash")
	if appErr != nil {
		t.Fatalf("create admin: %v", appErr)
	}
	s.mu.Lock()
	s.users[admin.ID].IsAdmin = true
	s.mu.Unlock()

	if _, appErr := s.SetUserDisabled(admin.ID, true); appErr == nil {
		t.Fatal("平台管理员账号必须拒绝禁用")
	} else if appErr.Status != http.StatusForbidden {
		t.Fatalf("平台管理员禁用错误状态 = %d, want 403", appErr.Status)
	}
	if s.users[admin.ID].Disabled {
		t.Fatal("平台管理员账号被写入了禁用态")
	}
	// 启用方向同样拒绝，避免管理员的启用请求被当成合法运维动作。
	if _, appErr := s.SetUserDisabled(admin.ID, false); appErr == nil {
		t.Fatal("平台管理员账号必须拒绝启用操作")
	}
}

// ListAllUsers 与 SetUserDisabled 组合：禁用态应出现在列表视图里（平台用户页数据源）。
func TestListAllUsersCarriesDisabledState(t *testing.T) {
	s := newStoreWithSeedAdmin()

	u, appErr := s.CreateUser("e@example.com", "E", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	now := time.Now().UTC()
	if _, appErr := s.SetUserDisabled(u.ID, true); appErr != nil {
		t.Fatalf("disable user: %v", appErr)
	}

	all := s.ListAllUsers()
	if len(all) != 1 {
		t.Fatalf("ListAllUsers len = %d, want 1", len(all))
	}
	if !all[0].Disabled || all[0].DisabledAt == nil {
		t.Fatalf("列表未带出禁用态: %+v", all[0])
	}
	if all[0].DisabledAt.Before(now.Add(-time.Minute)) {
		t.Fatalf("DisabledAt 疑似零值或未更新: %v", all[0].DisabledAt)
	}
}
