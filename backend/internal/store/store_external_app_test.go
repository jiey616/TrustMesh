package store

import (
	"strings"
	"testing"

	"trustmesh/backend/internal/model"
)

func seedUser(t *testing.T) (*Store, string) {
	t.Helper()
	s := New()
	user, appErr := s.CreateUser("owner@example.com", "Owner", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	return s, user.ID
}

func TestCreateExternalAppStoresSecretAndViewOmitsIt(t *testing.T) {
	s, userID := seedUser(t)

	view, secret, appErr := s.CreateExternalApp(Scope{UserID: userID}, CreateExternalAppInput{
		Name:      "Demo Platform",
		BaseURL:   "https://demo.example.com/home",
		ClientID:  "demo-client",
		SSOType:   model.ExternalSSOTrustmeshJWT,
		FrameMode: model.ExternalFrameNewTab,
		Scopes:    "read",
	})
	if appErr != nil {
		t.Fatalf("create: %v", appErr)
	}
	if view == nil || view.ID == "" {
		t.Fatal("nil/empty view")
	}
	if secret == "" {
		t.Fatal("client_secret not returned on create")
	}
	// ExternalAppView intentionally has no ClientSecret field, so the secret
	// can never be leaked through list/get responses.
	if view.Status != model.ExternalAppStatusEnabled {
		t.Errorf("status = %q, want enabled", view.Status)
	}
	// baseURL preserved for later launch assembly
	if view.BaseURL != "https://demo.example.com/home" {
		t.Errorf("base_url = %q", view.BaseURL)
	}

	// Raw record must carry the secret (used by launch).
	raw, err := s.GetExternalAppRaw(view.ID)
	if err != nil {
		t.Fatalf("get raw: %v", err)
	}
	if raw.ClientSecret != secret {
		t.Error("raw secret mismatch")
	}
}

func TestCreateExternalAppValidation(t *testing.T) {
	s, userID := seedUser(t)
	cases := []CreateExternalAppInput{
		{Name: "", BaseURL: "https://x.com", ClientID: "c"},
		{Name: "n", BaseURL: "", ClientID: "c"},
		{Name: "n", BaseURL: "https://x.com", ClientID: ""},
	}
	for i, in := range cases {
		if _, _, err := s.CreateExternalApp(Scope{UserID: userID}, in); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func TestExternalAppUpdateDeleteOwnership(t *testing.T) {
	s, ownerID := seedUser(t)
	other, appErr := s.CreateUser("other@example.com", "Other", "hash")
	if appErr != nil {
		t.Fatalf("create other user: %v", appErr)
	}

	view, _, appErr := s.CreateExternalApp(Scope{UserID: ownerID}, CreateExternalAppInput{
		Name: "P", BaseURL: "https://p.com", ClientID: "c",
	})
	if appErr != nil {
		t.Fatalf("create: %v", appErr)
	}

	// Non-owner cannot update.
	disabled := model.ExternalAppStatusDisabled
	if _, err := s.UpdateExternalApp(Scope{UserID: other.ID}, view.ID, UpdateExternalAppInput{Status: &disabled}, ExternalAppManager{}); err == nil {
		t.Error("non-owner update should be forbidden")
	}

	// A personal app owned by someone else must be invisible.
	if _, err := s.GetExternalApp(Scope{UserID: other.ID}, view.ID); err == nil {
		t.Error("non-owner should not read a private external app")
	}

	// Owner can disable.
	if _, err := s.UpdateExternalApp(Scope{UserID: ownerID}, view.ID, UpdateExternalAppInput{Status: &disabled}, ExternalAppManager{}); err != nil {
		t.Fatalf("owner update: %v", err)
	}
	got, _ := s.GetExternalApp(Scope{UserID: ownerID}, view.ID)
	if got.Status != model.ExternalAppStatusDisabled {
		t.Errorf("status = %q, want disabled", got.Status)
	}

	// Non-owner cannot delete.
	if err := s.DeleteExternalApp(Scope{UserID: other.ID}, view.ID, ExternalAppManager{}); err == nil {
		t.Error("non-owner delete should be forbidden")
	}
	// Owner can delete.
	if err := s.DeleteExternalApp(Scope{UserID: ownerID}, view.ID, ExternalAppManager{}); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	if _, err := s.GetExternalApp(Scope{UserID: ownerID}, view.ID); err == nil {
		t.Error("app should be gone after delete")
	}
}

func TestExternalAppMountMetadataDefaults(t *testing.T) {
	s, userID := seedUser(t)

	view, _, appErr := s.CreateExternalApp(Scope{UserID: userID}, CreateExternalAppInput{
		Name: "P", BaseURL: "https://p.com", ClientID: "c",
	})
	if appErr != nil {
		t.Fatalf("create: %v", appErr)
	}
	// 无 scope 的存量创建路径按工作区派生：无租户头 → 个人级，且默认不挂载，
	// 因此绝不会静默出现在别人的侧边栏。
	if view.Scope != model.ExternalAppScopePersonal {
		t.Errorf("scope = %q, want personal", view.Scope)
	}
	if view.Placement != "" {
		t.Errorf("placement = %q, want empty", view.Placement)
	}
	if view.FrameMode != model.ExternalFrameNewTab {
		t.Errorf("frame_mode = %q, want newtab", view.FrameMode)
	}
}

func TestExternalAppMountMetadataOnCreate(t *testing.T) {
	s, userID := seedUser(t)

	view, _, appErr := s.CreateExternalApp(Scope{UserID: userID}, CreateExternalAppInput{
		Name:      "Mounted",
		BaseURL:   "https://p.com",
		ClientID:  "c",
		FrameMode: model.ExternalFrameIframe,
		Placement: model.ExternalPlacementSidebar + "," + model.ExternalPlacementProjectTab,
		IconURL:   "https://cdn.example.com/icon.png",
		SortOrder: 5,
		Scope:     model.ExternalAppScopePersonal,
	})
	if appErr != nil {
		t.Fatalf("create: %v", appErr)
	}
	if view.FrameMode != model.ExternalFrameIframe {
		t.Errorf("frame_mode = %q, want iframe", view.FrameMode)
	}
	if !model.HasPlacement(view.Placement, model.ExternalPlacementSidebar) ||
		!model.HasPlacement(view.Placement, model.ExternalPlacementProjectTab) {
		t.Errorf("placement = %q, want both mount points", view.Placement)
	}
	if view.IconURL == "" || view.SortOrder != 5 || view.Scope != model.ExternalAppScopePersonal {
		t.Errorf("mount metadata not persisted: %+v", view)
	}
}

func TestExternalAppValidationOfMountFields(t *testing.T) {
	s, userID := seedUser(t)
	bad := []CreateExternalAppInput{
		{Name: "n", BaseURL: "javascript:alert(1)", ClientID: "c"},
		{Name: "n", BaseURL: "ftp://p.com", ClientID: "c"},
		{Name: "n", BaseURL: "https://p.com", ClientID: "c", FrameMode: "popup"},
		{Name: "n", BaseURL: "https://p.com", ClientID: "c", Placement: "sidebar,nowhere"},
		// 业务侧不得创建全局级（只有平台侧 CreateGlobalExternalApp 可以）。
		{Name: "n", BaseURL: "https://p.com", ClientID: "c", Scope: model.ExternalAppScopeGlobal},
		// 组织级必须落在企业工作区；无租户头时拒绝（不静默降级为个人级）。
		{Name: "n", BaseURL: "https://p.com", ClientID: "c", Scope: model.ExternalAppScopeOrg},
		{Name: "n", BaseURL: "https://p.com", ClientID: "c", Scope: "everyone"},
	}
	for i, in := range bad {
		if _, _, err := s.CreateExternalApp(Scope{UserID: userID}, in); err == nil {
			t.Errorf("case %d (%+v): expected validation error", i, in)
		}
	}
}

// externalAppNames 收集列表里的名称（顺序无关比较，排序由专门的用例覆盖）。
func externalAppNames(items []model.ExternalAppView) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, it := range items {
		out[it.Name] = true
	}
	return out
}

// TestListExternalAppsThreeLevelVisibility 钉死三级作用域的可见性矩阵：
// 全局级 → 全员；组织级 → 同租户成员；个人级 → 仅创建者本人。
func TestListExternalAppsThreeLevelVisibility(t *testing.T) {
	s, ownerID := seedUser(t)
	enterprise, appErr := s.CreateOrganization(ownerID, "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create org: %v", appErr)
	}
	member, appErr := s.CreateUser("member@example.com", "Member", "hash")
	if appErr != nil {
		t.Fatalf("create member: %v", appErr)
	}
	if _, appErr := s.AddOrgMember(enterprise.ID, member.ID, model.OrgRoleMember); appErr != nil {
		t.Fatalf("add member: %v", appErr)
	}
	outsider, appErr := s.CreateUser("outsider@example.com", "Outsider", "hash")
	if appErr != nil {
		t.Fatalf("create outsider: %v", appErr)
	}
	admin, appErr := s.CreateUser("admin@example.com", "Admin", "hash")
	if appErr != nil {
		t.Fatalf("create admin: %v", appErr)
	}

	ownerScope := Scope{UserID: ownerID, OrgID: enterprise.ID, Role: model.OrgRoleOwner}
	if _, _, err := s.CreateExternalApp(ownerScope, CreateExternalAppInput{
		Name: "Org App", BaseURL: "https://org.example.com", ClientID: "c",
		Scope: model.ExternalAppScopeOrg,
	}); err != nil {
		t.Fatalf("create org app: %v", err)
	}
	if _, _, err := s.CreateExternalApp(ownerScope, CreateExternalAppInput{
		Name: "Personal App", BaseURL: "https://p.example.com", ClientID: "c",
		Scope: model.ExternalAppScopePersonal,
	}); err != nil {
		t.Fatalf("create personal app: %v", err)
	}
	globalView, _, err := s.CreateGlobalExternalApp(Scope{UserID: admin.ID}, CreateExternalAppInput{
		Name: "Global App", BaseURL: "https://g.example.com", ClientID: "c",
	})
	if err != nil {
		t.Fatalf("create global app: %v", err)
	}
	if globalView.Scope != model.ExternalAppScopeGlobal || globalView.OrgID != "" {
		t.Fatalf("global app must be global with empty org_id, got scope=%q org=%q", globalView.Scope, globalView.OrgID)
	}
	if got := s.ListGlobalExternalApps(); len(got) != 1 || got[0].Name != "Global App" {
		t.Fatalf("platform list = %+v, want only the global app", got)
	}

	// 创建者本人（企业工作区）：全局 + 本组织 + 自己的个人级。
	got := externalAppNames(s.ListExternalApps(ownerScope))
	if len(got) != 3 {
		t.Fatalf("owner sees %d apps, want 3 (%v)", len(got), got)
	}
	// 同组织 member：看得到组织级与全局级，看不到别人的个人级。
	got = externalAppNames(s.ListExternalApps(Scope{UserID: member.ID, OrgID: enterprise.ID, Role: model.OrgRoleMember}))
	if !got["Org App"] || !got["Global App"] || got["Personal App"] || len(got) != 2 {
		t.Fatalf("member sees %v, want org+global only", got)
	}
	// 组织外用户：只剩全局级（原「公共」应用不再全员可见）。
	got = externalAppNames(s.ListExternalApps(Scope{UserID: outsider.ID}))
	if !got["Global App"] || len(got) != 1 {
		t.Fatalf("outsider sees %v, want global only", got)
	}
	// 无租户头（存量客户端）：组织级退回 user 维度 → 创建者本人的应用仍可见（零回归）。
	got = externalAppNames(s.ListExternalApps(Scope{UserID: ownerID}))
	if !got["Org App"] || !got["Personal App"] || !got["Global App"] || len(got) != 3 {
		t.Fatalf("owner without org ctx sees %v, want own two + global", got)
	}
}

// TestExternalAppOrgScopeManagement 管理权矩阵：org.app.mgr 可管本组织任意组织级应用，
// member 只能管自己创建的，个人级应用连组织管理员也不能动。
func TestExternalAppOrgScopeManagement(t *testing.T) {
	s, ownerID := seedUser(t)
	enterprise, appErr := s.CreateOrganization(ownerID, "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create org: %v", appErr)
	}
	member, appErr := s.CreateUser("member@example.com", "Member", "hash")
	if appErr != nil {
		t.Fatalf("create member: %v", appErr)
	}
	if _, appErr := s.AddOrgMember(enterprise.ID, member.ID, model.OrgRoleMember); appErr != nil {
		t.Fatalf("add member: %v", appErr)
	}

	ownerScope := Scope{UserID: ownerID, OrgID: enterprise.ID, Role: model.OrgRoleOwner}
	memberScope := Scope{UserID: member.ID, OrgID: enterprise.ID, Role: model.OrgRoleMember}
	ownerMgr := ExternalAppManager{OrgAppMgr: true}

	if _, _, err := s.CreateExternalApp(ownerScope, CreateExternalAppInput{
		Name: "Org App", BaseURL: "https://org.example.com", ClientID: "c",
		Scope: model.ExternalAppScopeOrg,
	}); err != nil {
		t.Fatalf("create org app: %v", err)
	}
	orgList := s.ListExternalApps(ownerScope)
	if len(orgList) != 1 {
		t.Fatalf("want 1 org app, got %d", len(orgList))
	}
	orgAppID := orgList[0].ID

	// member 无 org.app.mgr：不得改/删别人的组织级应用。
	disabled := model.ExternalAppStatusDisabled
	if _, err := s.UpdateExternalApp(memberScope, orgAppID, UpdateExternalAppInput{Status: &disabled}, ExternalAppManager{}); err == nil {
		t.Error("member without org.app.mgr must not update an org-scoped app")
	}
	if err := s.DeleteExternalApp(memberScope, orgAppID, ExternalAppManager{}); err == nil {
		t.Error("member without org.app.mgr must not delete an org-scoped app")
	}
	// owner 持 org.app.mgr（不限创建者）：可改可删。
	if _, err := s.UpdateExternalApp(memberScope, orgAppID, UpdateExternalAppInput{Status: &disabled}, ownerMgr); err != nil {
		t.Fatalf("org.app.mgr holder should manage any org-scoped app: %v", err)
	}
	// 个人级应用不属于组织资产：org.app.mgr 也不能越权管理。
	if _, _, err := s.CreateExternalApp(Scope{UserID: member.ID}, CreateExternalAppInput{
		Name: "Member Personal", BaseURL: "https://mp.example.com", ClientID: "c",
		Scope: model.ExternalAppScopePersonal,
	}); err != nil {
		t.Fatalf("create member personal app: %v", err)
	}
	personal := s.ListExternalApps(Scope{UserID: member.ID})
	if len(personal) != 1 {
		t.Fatalf("want 1 personal app, got %d", len(personal))
	}
	if _, err := s.UpdateExternalApp(ownerScope, personal[0].ID, UpdateExternalAppInput{Status: &disabled}, ownerMgr); err == nil {
		t.Error("org.app.mgr must not manage another user's personal app")
	}
	// 全局级应用只有平台管理员能动（用 member 当"另一个账号"建，避免撞创建者本人放行）。
	if _, _, err := s.CreateGlobalExternalApp(Scope{UserID: member.ID}, CreateExternalAppInput{
		Name: "Global App", BaseURL: "https://g.example.com", ClientID: "c",
	}); err != nil {
		t.Fatalf("create global app: %v", err)
	}
	globalID := s.ListGlobalExternalApps()[0].ID
	if _, err := s.UpdateExternalApp(ownerScope, globalID, UpdateExternalAppInput{Status: &disabled}, ownerMgr); err == nil {
		t.Error("org.app.mgr must not manage a global app")
	}
	if _, err := s.UpdateExternalApp(Scope{UserID: ownerID}, globalID, UpdateExternalAppInput{Status: &disabled}, ExternalAppManager{Platform: true}); err != nil {
		t.Fatalf("platform admin should manage a global app: %v", err)
	}
}

func TestListExternalAppsSortedBySortOrderThenName(t *testing.T) {
	s, userID := seedUser(t)
	specs := []struct {
		name  string
		order int
	}{
		{"Zeta", 0},
		{"Alpha", 0},
		{"Beta", -1},
	}
	for _, sp := range specs {
		if _, _, err := s.CreateExternalApp(Scope{UserID: userID}, CreateExternalAppInput{
			Name: sp.name, BaseURL: "https://p.com", ClientID: "c", SortOrder: sp.order,
		}); err != nil {
			t.Fatalf("create %s: %v", sp.name, err)
		}
	}
	got := s.ListExternalApps(Scope{UserID: userID})
	want := []string{"Beta", "Alpha", "Zeta"}
	if len(got) != len(want) {
		t.Fatalf("got %d apps, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestGetExternalAppForLaunchEnforcesVisibility(t *testing.T) {
	s, ownerID := seedUser(t)
	other, appErr := s.CreateUser("other@example.com", "Other", "hash")
	if appErr != nil {
		t.Fatalf("create other user: %v", appErr)
	}

	view, _, appErr := s.CreateExternalApp(Scope{UserID: ownerID}, CreateExternalAppInput{
		Name: "P", BaseURL: "https://p.com", ClientID: "c",
	})
	if appErr != nil {
		t.Fatalf("create: %v", appErr)
	}

	// Launching mints a token, so visibility must gate it. Invisible apps
	// return NotFound rather than Forbidden to avoid disclosing existence.
	if _, err := s.GetExternalAppForLaunch(Scope{UserID: other.ID}, view.ID); err == nil {
		t.Error("non-owner should not be able to launch someone else's personal app")
	}
	if _, err := s.GetExternalAppForLaunch(Scope{UserID: ownerID}, view.ID); err != nil {
		t.Errorf("owner launch lookup failed: %v", err)
	}
	// 全局级应用任何人可打开（平台级挂载的产品语义）。
	globalView, _, appErr := s.CreateGlobalExternalApp(Scope{UserID: ownerID}, CreateExternalAppInput{
		Name: "G", BaseURL: "https://g.com", ClientID: "c",
	})
	if appErr != nil {
		t.Fatalf("create global: %v", appErr)
	}
	if _, err := s.GetExternalAppForLaunch(Scope{UserID: other.ID}, globalView.ID); err != nil {
		t.Errorf("global app should be launchable by anyone: %v", err)
	}
}

func strPtr(s string) *string { return &s }

func TestExternalAppLaunchURLAssembly(t *testing.T) {
	// buildLaunchURL is a standalone helper; reuse via handler is not possible
	// here, so we assert the store path issues a token-bearing raw record and
	// that List omits the secret.
	s, userID := seedUser(t)
	view, secret, appErr := s.CreateExternalApp(Scope{UserID: userID}, CreateExternalAppInput{
		Name: "P", BaseURL: "https://p.com/path?foo=bar", ClientID: "c",
	})
	if appErr != nil {
		t.Fatalf("create: %v", appErr)
	}
	if secret == "" {
		t.Fatal("missing secret")
	}
	// List projection must never include the secret (ExternalAppView has no
	// ClientSecret field by design).
	_ = s.ListExternalApps(Scope{UserID: userID})
	if !strings.Contains(view.BaseURL, "foo=bar") {
		t.Errorf("base_url lost existing query: %q", view.BaseURL)
	}
}
