package clawsynapse

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"trustmesh/backend/internal/metrics"
	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
)

// T1.10 产出物质量门禁测试（客观可判定缺陷；默认只标记，gate 开启才硬拒）。

func withStrictDeliverableQualityGate(t *testing.T, on bool) {
	t.Helper()
	orig := strictDeliverableQualityGate
	strictDeliverableQualityGate = on
	t.Cleanup(func() { strictDeliverableQualityGate = orig })
}

func TestJudgeDeliverableQuality(t *testing.T) {
	cases := []struct {
		name string
		size int64
		want int
	}{
		{"正常大小_通过", 16780, 0},
		{"0字节_判为缺陷", 0, 1},
		{"负值_判为缺陷", -5, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := judgeDeliverableQuality("storyboard-01.png", tc.size, "image/png")
			if len(got) != tc.want {
				t.Fatalf("violations = %d (%+v), want %d", len(got), got, tc.want)
			}
			if tc.want > 0 {
				if got[0].RuleID != deliverableRuleZeroByte || got[0].Severity != "reject" {
					t.Fatalf("violation = %+v, want rule %q severity reject", got[0], deliverableRuleZeroByte)
				}
			}
		})
	}
}

// 未知扩展名不得被判为缺陷：store.extToMime 是窄白名单（缺 .srt/.mov/.wav 等
// 影视常用格式），用它判定会误伤合法字幕/音视频，故本门禁刻意不编码该规则。
func TestJudgeDeliverableQualityIgnoresUnknownType(t *testing.T) {
	if v := judgeDeliverableQuality("subtitle.srt", 2048, ""); len(v) != 0 {
		t.Fatalf("unknown extension must not be a violation, got %+v", v)
	}
}

func postZeroByteTransfer(t *testing.T, h *WebhookHandler, taskID, devNode string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"transferId": "tid-zero-byte-000001",
		"fileName":   "storyboard-01.png",
		"fileSize":   0,
		"localPath":  "/var/lib/trustmesh-transfers/tid-zero-byte-000001-storyboard-01.png",
		"mimeType":   "image/png",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhook/clawsynapse", nil)
	h.handleTransferReceived(c, protocol.WebhookPayload{
		NodeID:   devNode,
		Type:     "transfer.received",
		From:     devNode,
		Message:  string(body),
		Metadata: map[string]any{"taskId": taskID, "todoId": "TD_01"},
	})
	return w
}

// gate 默认 OFF：0 字节文件只被标记，仍入库，并在任务时间线留痕。
func TestDeliverableQualityGateMarksByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictDeliverableQualityGate(t, false)

	s, userID, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postZeroByteTransfer(t, h, taskID, devNode)
	if w.Code != http.StatusOK {
		t.Fatalf("observe mode must let a 0-byte upload through: status=%d body=%s", w.Code, w.Body.String())
	}
	if n := metrics.Count(metrics.DeliverableQualityWarnedTotal); n != 1 {
		t.Fatalf("DeliverableQualityWarnedTotal = %d, want 1", n)
	}
	if n := metrics.Count(metrics.DeliverableQualityRejectedTotal); n != 0 {
		t.Fatalf("DeliverableQualityRejectedTotal = %d, want 0", n)
	}
	if len(s.GetArtifactsByTaskID(taskID)) != 1 {
		t.Fatalf("observe mode must still file the artifact")
	}

	comments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
	if appErr != nil {
		t.Fatalf("list comments: %v", appErr)
	}
	found := false
	for _, cm := range comments {
		if strings.Contains(cm.Content, "质量门禁") && strings.Contains(cm.Content, "0 字节") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a quality-gate system comment, got %d comment(s)", len(comments))
	}
}

// gate ON：0 字节文件硬拒，且不产生任何 artifact（可追溯的拦截）。
func TestDeliverableQualityGateBlocksWhenOn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictDeliverableQualityGate(t, true)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postZeroByteTransfer(t, h, taskID, devNode)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "DELIVERABLE_QUALITY_REJECTED") {
		t.Fatalf("body missing code DELIVERABLE_QUALITY_REJECTED: %s", w.Body.String())
	}
	if n := metrics.Count(metrics.DeliverableQualityRejectedTotal); n != 1 {
		t.Fatalf("DeliverableQualityRejectedTotal = %d, want 1", n)
	}
	if len(s.GetArtifactsByTaskID(taskID)) != 0 {
		t.Fatalf("rejected upload must not create an artifact")
	}
}

// gate ON 也不得误伤正常文件。
func TestDeliverableQualityGatePassesHealthyFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics.Reset()
	t.Cleanup(metrics.Reset)
	withStrictDeliverableQualityGate(t, true)

	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	body, err := json.Marshal(map[string]any{
		"transferId": "tid-healthy-00000001",
		"fileName":   "script-v1.md",
		"fileSize":   16780,
		"localPath":  "/var/lib/trustmesh-transfers/tid-healthy-00000001-script-v1.md",
		"mimeType":   "text/markdown",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhook/clawsynapse", nil)
	h.handleTransferReceived(c, protocol.WebhookPayload{
		NodeID:   devNode,
		Type:     "transfer.received",
		From:     devNode,
		Message:  string(body),
		Metadata: map[string]any{"taskId": taskID, "todoId": "TD_01"},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("healthy file must pass: status=%d body=%s", w.Code, w.Body.String())
	}
	if n := metrics.Count(metrics.DeliverableQualityRejectedTotal); n != 0 {
		t.Fatalf("DeliverableQualityRejectedTotal = %d, want 0", n)
	}
	if len(s.GetArtifactsByTaskID(taskID)) != 1 {
		t.Fatalf("healthy upload must be filed")
	}
}
