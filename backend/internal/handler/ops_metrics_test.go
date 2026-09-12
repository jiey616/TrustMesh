package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/middleware"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

// T0.11 acceptance: the observability trio must be queryable through an
// endpoint, and the alert rules must be part of the payload (not just folklore
// in a doc).
//
// The metrics themselves are process-wide aggregates, so the endpoint is gated
// to org owner/admin. This test pins both halves: the gate rejects a request
// without an org context and a plain member, and the owner gets counters,
// histograms and the rule set.
func TestOpsMetricsRequiresOrgAdminAndExposesTrio(t *testing.T) {
	metrics.Reset()
	t.Cleanup(metrics.Reset)

	gin.SetMode(gin.TestMode)

	st := store.New()
	owner, appErr := st.CreateUser("owner@example.com", "Owner", "hash")
	if appErr != nil {
		t.Fatalf("create owner: %v", appErr)
	}
	member, appErr := st.CreateUser("member@example.com", "Member", "hash")
	if appErr != nil {
		t.Fatalf("create member: %v", appErr)
	}
	org, appErr := st.CreateOrganization(owner.ID, "Acme", "acme", model.OrgKindEnterprise)
	if appErr != nil {
		t.Fatalf("create org: %v", appErr)
	}
	if _, appErr := st.AddOrgMember(org.ID, member.ID, model.OrgRoleMember); appErr != nil {
		t.Fatalf("add member: %v", appErr)
	}

	newRouter := func(userID string) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("user_id", userID) })
		r.Use(middleware.OrgScope(st))
		r.GET("/ops/metrics", NewOpsHandler(st).Metrics)
		return r
	}

	do := func(r *gin.Engine, orgID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/ops/metrics", nil)
		if orgID != "" {
			req.Header.Set("X-Org-Id", orgID)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// 1. No tenant context (personal space) → forbidden.
	if w := do(newRouter(owner.ID), ""); w.Code != http.StatusForbidden {
		t.Fatalf("no org context: status = %d, want 403", w.Code)
	}

	// 2. Authenticated org member without an admin role → forbidden.
	if w := do(newRouter(member.ID), org.ID); w.Code != http.StatusForbidden {
		t.Fatalf("plain member: status = %d, want 403", w.Code)
	}

	// 3. Org owner → 200 with the trio.
	metrics.Inc(metrics.TodoAutoAdvancedTotal)
	w := do(newRouter(owner.ID), org.ID)
	if w.Code != http.StatusOK {
		t.Fatalf("owner: status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}

	var body struct {
		Data struct {
			Counters   map[string]int64          `json:"counters"`
			Histograms []metrics.HistogramReport `json:"histograms"`
			Alerts     []metrics.AlertRule       `json:"alerts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, w.Body.String())
	}

	if got := body.Data.Counters[metrics.TodoAutoAdvancedTotal]; got != 1 {
		t.Fatalf("counters[%s] = %d, want 1", metrics.TodoAutoAdvancedTotal, got)
	}
	if len(body.Data.Histograms) != len(metrics.KnownCounterNames()) {
		t.Fatalf("histograms = %d, want %d (one per declared metric)", len(body.Data.Histograms), len(metrics.KnownCounterNames()))
	}
	if len(body.Data.Alerts) == 0 {
		t.Fatalf("alerts must be part of the payload; got none")
	}
	if len(body.Data.Alerts) != len(metrics.AlertRules) {
		t.Fatalf("alerts = %d, want %d", len(body.Data.Alerts), len(metrics.AlertRules))
	}

	// The three T0.11 signals must be present and alertable.
	declared := make(map[string]bool, len(metrics.KnownCounterNames()))
	for _, h := range body.Data.Histograms {
		declared[h.Name] = true
	}
	for _, name := range []string{metrics.StepAdvanceDuration, metrics.TodoStalledDuration, metrics.AgentStepStalledTotal} {
		if !declared[name] {
			t.Fatalf("observability trio member %q missing from the endpoint payload", name)
		}
	}
}
