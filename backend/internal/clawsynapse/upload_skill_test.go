package clawsynapse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// daemon 未实现技能上传端点（404）：应返回明确错误，而不是尝试解析纯文本导致二次失败。
func TestUploadSkillFileDaemon404ReturnsClearError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/peers/n1-404/capabilities" && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"","data":{"product":"hermes","available":true,"skills":[],"models":[],"jobs":[],"reason":""},"ts":1}`))
			return
		}
		if r.URL.Path == "/v1/peers/n1-404/skills" && r.Method == http.MethodPost {
			// daemon 未实现：返回纯文本 404（旧版行为）
			http.Error(w, "404 page not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"ok":false,"code":"NOT_FOUND","message":"not found","data":{},"ts":1}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, 0)
	_, err := c.UploadSkillFile(context.Background(), "n1-404", "skill.zip", strings.NewReader("zip-bytes"))
	if err == nil {
		t.Fatal("expected error for 404 upload, got nil")
	}
	// 错误应包含明确的 status 提示（而不是 decode 报错）
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status 404, got: %v", err)
	}
}

// 正常上传：daemon 返回 fileId，客户端透传。
func TestUploadSkillFileSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/peers/n1-ok/skills" && r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"ok":true,"code":"skill.uploaded","message":"","data":{"fileId":"fid-123"},"ts":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":false,"code":"NOT_FOUND","message":"not found","data":{},"ts":1}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, 0)
	fileID, err := c.UploadSkillFile(context.Background(), "n1-ok", "skill.zip", strings.NewReader("zip-bytes"))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if fileID != "fid-123" {
		t.Errorf("fileId = %q, want fid-123", fileID)
	}
}
