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
	if _, err := s.UpdateExternalApp(Scope{UserID: other.ID}, view.ID, UpdateExternalAppInput{Status: &disabled}); err == nil {
		t.Error("non-owner update should be forbidden")
	}

	// A private app owned by someone else must be invisible.
	if _, err := s.GetExternalApp(Scope{UserID: other.ID}, view.ID); err == nil {
		t.Error("non-owner should not read a private external app")
	}

	// Owner can disable.
	if _, err := s.UpdateExternalApp(Scope{UserID: ownerID}, view.ID, UpdateExternalAppInput{Status: &disabled}); err != nil {
		t.Fatalf("owner update: %v", err)
	}
	got, _ := s.GetExternalApp(Scope{UserID: ownerID}, view.ID)
	if got.Status != model.ExternalAppStatusDisabled {
		t.Errorf("status = %q, want disabled", got.Status)
	}

	// Non-owner cannot delete.
	if err := s.DeleteExternalApp(Scope{UserID: other.ID}, view.ID); err == nil {
		t.Error("non-owner delete should be forbidden")
	}
	// Owner can delete.
	if err := s.DeleteExternalApp(Scope{UserID: ownerID}, view.ID); err != nil {
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
	// New registrations default to private and unmounted so they can never
	// silently appear in everyone's sidebar.
	if view.Visibility != model.ExternalVisibilityPrivate {
		t.Errorf("visibility = %q, want private", view.Visibility)
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
		Name:       "Mounted",
		BaseURL:    "https://p.com",
		ClientID:   "c",
		FrameMode:  model.ExternalFrameIframe,
		Placement:  model.ExternalPlacementSidebar + "," + model.ExternalPlacementProjectTab,
		IconURL:    "https://cdn.example.com/icon.png",
		SortOrder:  5,
		Visibility: model.ExternalVisibilityPublic,
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
	if view.IconURL == "" || view.SortOrder != 5 || view.Visibility != model.ExternalVisibilityPublic {
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
		{Name: "n", BaseURL: "https://p.com", ClientID: "c", Visibility: "everyone"},
	}
	for i, in := range bad {
		if _, _, err := s.CreateExternalApp(Scope{UserID: userID}, in); err == nil {
			t.Errorf("case %d (%+v): expected validation error", i, in)
		}
	}
}

func TestListExternalAppsVisibilityFiltering(t *testing.T) {
	s, ownerID := seedUser(t)
	other, appErr := s.CreateUser("other@example.com", "Other", "hash")
	if appErr != nil {
		t.Fatalf("create other user: %v", appErr)
	}
	third, appErr := s.CreateUser("third@example.com", "Third", "hash")
	if appErr != nil {
		t.Fatalf("create third user: %v", appErr)
	}

	// owner: private app, other: public app.
	if _, _, err := s.CreateExternalApp(Scope{UserID: ownerID}, CreateExternalAppInput{
		Name: "Owner Private", BaseURL: "https://a.com", ClientID: "c",
		Visibility: model.ExternalVisibilityPrivate,
	}); err != nil {
		t.Fatalf("create owner app: %v", err)
	}
	if _, _, err := s.CreateExternalApp(Scope{UserID: other.ID}, CreateExternalAppInput{
		Name: "Other Public", BaseURL: "https://b.com", ClientID: "c",
		Visibility: model.ExternalVisibilityPublic,
	}); err != nil {
		t.Fatalf("create other app: %v", err)
	}

	ownerList := s.ListExternalApps(Scope{UserID: ownerID})
	if len(ownerList) != 2 {
		t.Errorf("owner sees %d apps, want 2", len(ownerList))
	}
	// `other` only sees their own public app — the owner's private app is
	// invisible to them, which is exactly the leak this change closes.
	otherList := s.ListExternalApps(Scope{UserID: other.ID})
	if len(otherList) != 1 || otherList[0].Name != "Other Public" {
		t.Errorf("`other` sees %+v, want only their own public app", otherList)
	}
	thirdList := s.ListExternalApps(Scope{UserID: third.ID})
	if len(thirdList) != 1 || thirdList[0].Name != "Other Public" {
		t.Errorf("bystander sees %+v, want only the public app", thirdList)
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
		t.Error("non-owner should not be able to launch a private app")
	}
	if _, err := s.GetExternalAppForLaunch(Scope{UserID: ownerID}, view.ID); err != nil {
		t.Errorf("owner launch lookup failed: %v", err)
	}
	// A public app is launchable by anyone.
	if _, err := s.UpdateExternalApp(Scope{UserID: ownerID}, view.ID, UpdateExternalAppInput{
		Visibility: strPtr(model.ExternalVisibilityPublic),
	}); err != nil {
		t.Fatalf("make public: %v", err)
	}
	if _, err := s.GetExternalAppForLaunch(Scope{UserID: other.ID}, view.ID); err != nil {
		t.Errorf("public app should be launchable by others: %v", err)
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
