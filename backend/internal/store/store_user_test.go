package store

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// 改名：账号名更新，且个人租户名称同步（个人空间展示跟随用户名）。
func TestUpdateUserNameSyncsPersonalOrg(t *testing.T) {
	s := New()

	u, appErr := s.CreateUser("u1@example.com", "Old Name", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	personalOrgID := s.personalOrgOfUnsafe(u.ID)
	if personalOrgID == "" {
		t.Fatal("personal org should exist after CreateUser")
	}

	updated, appErr := s.UpdateUserName(u.ID, "  New Name  ")
	if appErr != nil {
		t.Fatalf("update name: %v", appErr)
	}
	if updated.Name != "New Name" {
		t.Fatalf("user.Name = %q, want %q", updated.Name, "New Name")
	}
	if org, ok := s.organizations[personalOrgID]; !ok || org.Name != "New Name" {
		t.Fatalf("personal org name not synced: %+v", org)
	}

	// 空白名 / 超长名应被拒绝
	if _, err := s.UpdateUserName(u.ID, "   "); err == nil {
		t.Fatal("blank name must be rejected")
	}
	if _, err := s.UpdateUserName(u.ID, strings.Repeat("x", 65)); err == nil {
		t.Fatal("name longer than 64 chars must be rejected")
	}
	// 用户不存在
	if _, err := s.UpdateUserName("ghost", "Someone"); err == nil {
		t.Fatal("unknown user must be rejected")
	}
}

// 改密：旧密码校验 + 新哈希生效。
func TestChangePasswordVerification(t *testing.T) {
	s := New()

	originalHash, err := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u, appErr := s.CreateUser("u2@example.com", "u2", string(originalHash))
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}

	if !s.VerifyUserPassword(u.ID, "oldpass123") {
		t.Fatal("original password should verify")
	}
	if s.VerifyUserPassword(u.ID, "wrongpass") {
		t.Fatal("wrong password must not verify")
	}
	if s.VerifyUserPassword("ghost", "oldpass123") {
		t.Fatal("unknown user must not verify")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte("newpass456"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if appErr := s.UpdateUserPassword(u.ID, string(newHash)); appErr != nil {
		t.Fatalf("update password: %v", appErr)
	}

	if s.VerifyUserPassword(u.ID, "oldpass123") {
		t.Fatal("old password must stop working after change")
	}
	if !s.VerifyUserPassword(u.ID, "newpass456") {
		t.Fatal("new password should verify after change")
	}
	if appErr := s.UpdateUserPassword("ghost", "whatever"); appErr == nil {
		t.Fatal("unknown user password update must fail")
	}
}
