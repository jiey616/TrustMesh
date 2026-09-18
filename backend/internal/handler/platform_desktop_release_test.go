package handler

import (
	"strings"
	"testing"
	"time"

	"trustmesh/backend/internal/store"
)

// RenderLatestYML 是更新链路的**唯一协议面**：electron-builder / electron-updater
// 按字段名与缩进解析它，任何一处拼写或缩进出错，客户端只会表现为「没有更新」
// —— 没有报错、没有日志线索。因此这里逐字符钉住整份输出。
func TestRenderLatestYML(t *testing.T) {
	feed := &store.DesktopReleaseFeed{
		Version:      "0.3.0",
		FileName:     "TrustMesh-Setup-0.3.0.exe",
		Size:         91234567,
		Sha512:       "U29tZVNlZW1pbmdseUxvbmcrYmFzZTY0K3NoYTUxMitkaWdlc3Q=",
		BlockMapSize: 12345,
		ReleaseDate:  time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
	}
	got := RenderLatestYML(feed)
	want := strings.Join([]string{
		`version: "0.3.0"`,
		`files:`,
		`  - url: "TrustMesh-Setup-0.3.0.exe"`,
		`    sha512: "U29tZVNlZW1pbmdseUxvbmcrYmFzZTY0K3NoYTUxMitkaWdlc3Q="`,
		`    size: 91234567`,
		`    blockMapSize: 12345`,
		`path: "TrustMesh-Setup-0.3.0.exe"`,
		`sha512: "U29tZVNlZW1pbmdseUxvbmcrYmFzZTY0K3NoYTUxMitkaWdlc3Q="`,
		`releaseDate: "2026-09-18T12:00:00.000Z"`,
		"",
	}, "\n")
	if got != want {
		t.Fatalf("latest.yml mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// 显式的字段名清单：即使上面的整体比对被人「顺手更新」掉，漏字段仍会在这里变红。
	for _, key := range []string{"version:", "files:", "url:", "sha512:", "size:", "blockMapSize:", "path:", "releaseDate:"} {
		if !strings.Contains(got, key) {
			t.Errorf("latest.yml is missing required key %q\n%s", key, got)
		}
	}
}

// 没有 .blockmap 时必须**整行省略**，而不是写 blockMapSize: 0
// （写 0 会让更新端认为差分下载可用，然后去拉一个不存在的 blockmap）。
func TestRenderLatestYMLOmitsBlockMapSizeWhenAbsent(t *testing.T) {
	got := RenderLatestYML(&store.DesktopReleaseFeed{
		Version:     "0.3.0",
		FileName:    "TrustMesh-Setup-0.3.0.exe",
		Size:        1,
		Sha512:      "AAA=",
		ReleaseDate: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
	})
	if strings.Contains(got, "blockMapSize") {
		t.Fatalf("blockMapSize must be omitted when there is no blockmap\n%s", got)
	}
	if !strings.Contains(got, "size: 1") {
		t.Fatalf("size must still be present\n%s", got)
	}
}

func TestRenderLatestYMLNilFeed(t *testing.T) {
	if got := RenderLatestYML(nil); got != "" {
		t.Fatalf("nil feed must render empty, got %q", got)
	}
}

// yamlQuote 的安全前提是「全部值加双引号」，所以引号与反斜杠必须转义。
// 上游（store.validateDesktopReleaseFileName）已经拒绝含引号的文件名，
// 这里是第二道 —— 本函数不得依赖调用方守规矩。
func TestYAMLQuoteEscapes(t *testing.T) {
	cases := map[string]string{
		"plain":     `"plain"`,
		`a"b`:       `"a\"b"`,
		`a\b`:       `"a\\b"`,
		`a"b\c`:     `"a\"b\\c"`,
		"":          `""`,
		"0.3.0-rc1": `"0.3.0-rc1"`,
	}
	for in, want := range cases {
		if got := yamlQuote(in); got != want {
			t.Errorf("yamlQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
