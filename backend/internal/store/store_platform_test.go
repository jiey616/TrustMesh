package store

import (
	"fmt"
	"strings"
	"testing"

	"trustmesh/backend/internal/model"
)

// SetPlatformGlobalConfig：hidden_menus 复用 org.menu_overrides 的规范化
// （去空白、去重、空项丢弃），并原样回读。
func TestSetPlatformGlobalConfigNormalizesHiddenMenus(t *testing.T) {
	s := New()

	cfg, appErr := s.SetPlatformGlobalConfig("u1", model.PlatformGlobalConfig{
		Quota:       model.DefaultOrgQuota(),
		HiddenMenus: []string{" /knowledge ", "/knowledge", "", "   ", "/ops"},
	})
	if appErr != nil {
		t.Fatalf("set platform global config: %v", appErr)
	}
	want := []string{"/knowledge", "/ops"}
	if len(cfg.HiddenMenus) != len(want) {
		t.Fatalf("HiddenMenus = %v, want %v", cfg.HiddenMenus, want)
	}
	for i, key := range want {
		if cfg.HiddenMenus[i] != key {
			t.Fatalf("HiddenMenus[%d] = %q, want %q（应去空白去重）", i, cfg.HiddenMenus[i], key)
		}
	}

	got := s.PlatformHiddenMenus()
	if len(got) != len(want) {
		t.Fatalf("PlatformHiddenMenus = %v, want %v", got, want)
	}
	for i, key := range want {
		if got[i] != key {
			t.Fatalf("PlatformHiddenMenus[%d] = %q, want %q", i, got[i], key)
		}
	}
}

// SetPlatformGlobalConfig：hidden_menus 校验失败不得落库（沿用全局配置校验语义）。
func TestSetPlatformGlobalConfigRejectsInvalidHiddenMenus(t *testing.T) {
	s := New()

	// 单键 > 64 字符。
	if _, appErr := s.SetPlatformGlobalConfig("u1", model.PlatformGlobalConfig{
		Quota:       model.DefaultOrgQuota(),
		HiddenMenus: []string{strings.Repeat("x", 65)},
	}); appErr == nil {
		t.Fatal("超长菜单键必须被拒绝")
	}
	// 超过 50 项。
	many := make([]string, 0, 51)
	for i := 0; i < 51; i++ {
		many = append(many, fmt.Sprintf("/menu-%d", i))
	}
	if _, appErr := s.SetPlatformGlobalConfig("u1", model.PlatformGlobalConfig{
		Quota:       model.DefaultOrgQuota(),
		HiddenMenus: many,
	}); appErr == nil {
		t.Fatal("超过 50 项菜单键必须被拒绝")
	}
	if s.PlatformHiddenMenus() != nil {
		t.Fatal("校验失败不得落库")
	}
}

// 深拷贝语义：SetPlatformGlobalConfig / PlatformHiddenMenus / GetPlatformGlobalConfig
// 三个出入口的切片都必须是副本，任一方向的改动都不得回流 store。
func TestPlatformHiddenMenusCopySemantics(t *testing.T) {
	s := New()

	in := []string{"/knowledge", "/ops"}
	cfg, appErr := s.SetPlatformGlobalConfig("u1", model.PlatformGlobalConfig{
		Quota:       model.DefaultOrgQuota(),
		HiddenMenus: in,
	})
	if appErr != nil {
		t.Fatalf("set platform global config: %v", appErr)
	}

	// 入参方向：调用方改自己传进去的切片，不得影响 store。
	in[0] = "/hacked"
	if got := s.PlatformHiddenMenus(); got[0] != "/knowledge" {
		t.Fatalf("入参切片未深拷贝，store 内部被改写: %v", got)
	}
	// 返回值方向（SetPlatformGlobalConfig）。
	cfg.HiddenMenus[1] = "/hacked"
	if got := s.PlatformHiddenMenus(); got[1] != "/ops" {
		t.Fatalf("SetPlatformGlobalConfig 返回值未深拷贝，store 内部被改写: %v", got)
	}
	// 返回值方向（PlatformHiddenMenus）。
	out := s.PlatformHiddenMenus()
	out[0] = "/hacked"
	if got := s.PlatformHiddenMenus(); got[0] != "/knowledge" {
		t.Fatalf("PlatformHiddenMenus 返回值未深拷贝，store 内部被改写: %v", got)
	}
	// 返回值方向（GetPlatformGlobalConfig）。
	outCfg := s.GetPlatformGlobalConfig()
	outCfg.HiddenMenus[0] = "/hacked"
	if got := s.PlatformHiddenMenus(); got[0] != "/knowledge" {
		t.Fatalf("GetPlatformGlobalConfig 返回值未深拷贝，store 内部被改写: %v", got)
	}
}

// 未落库 / 清空 hidden_menus 时 PlatformHiddenMenus 返回 nil（前端按「无基线」处理）。
func TestPlatformHiddenMenusEmptyWhenUnset(t *testing.T) {
	s := New()
	if got := s.PlatformHiddenMenus(); got != nil {
		t.Fatalf("未落库时应返回 nil，实际 %v", got)
	}

	if _, appErr := s.SetPlatformGlobalConfig("u1", model.PlatformGlobalConfig{
		Quota:       model.DefaultOrgQuota(),
		HiddenMenus: []string{"/knowledge"},
	}); appErr != nil {
		t.Fatalf("set platform global config: %v", appErr)
	}
	if got := s.PlatformHiddenMenus(); len(got) != 1 {
		t.Fatalf("落库后应有 1 项，实际 %v", got)
	}

	// 再次保存时清空（前端取消全部勾选）应回到 nil，而不是保留旧值。
	if _, appErr := s.SetPlatformGlobalConfig("u1", model.PlatformGlobalConfig{
		Quota: model.DefaultOrgQuota(),
	}); appErr != nil {
		t.Fatalf("set platform global config (clear): %v", appErr)
	}
	if got := s.PlatformHiddenMenus(); got != nil {
		t.Fatalf("清空后应返回 nil，实际 %v", got)
	}
}
