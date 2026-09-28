package middleware

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// 压缩中间件契约测试。
//
// 两条最关键、也是最容易回归的红线：
//  1. SSE 不能被破坏 —— handler/sse.go 对 c.Writer 有 http.Flusher 硬断言，
//     并在 30 分钟上限 / 心跳里逐帧 Flush。一旦中间件的包装器漏实现某个方法，
//     SSE 会**静默失效**（不报错，只是不再实时）——正是本中间件立项要修的问题。
//  2. 二进制下载（http.ServeContent + Range）必须原样透传 —— electron-updater
//     的差分更新依赖 .blockmap + Range，压坏后不报错、只会退化成每次全量 87 MB。

func newGzipTestEngine(route string, h gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Gzip())
	r.Any(route, h)
	return r
}

func doGzipRequest(t *testing.T, r *gin.Engine, method, path string, headers map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Result()
}

func gunzipBody(t *testing.T, res *http.Response) []byte {
	t.Helper()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v (body prefix=%q)", err, string(raw[:min(len(raw), 32)]))
	}
	defer func() { _ = zr.Close() }()
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	return plain
}

func bigJSONPayload(t *testing.T, n int) []byte {
	t.Helper()
	items := make([]map[string]string, n)
	for i := range items {
		items[i] = map[string]string{
			"id":      strings.Repeat("a", 16),
			"content": strings.Repeat("事件内容 ", 8),
		}
	}
	b, err := json.Marshal(gin.H{"items": items})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if len(b) < minGzipBytes {
		t.Fatalf("payload too small for the test: %d bytes", len(b))
	}
	return b
}

// 大 JSON 必须被压缩：响应头带 Content-Encoding、撤掉 Content-Length、
// 带 Vary，且解压后与原文逐字节相等。
func TestGzipCompressesLargeJSON(t *testing.T) {
	payload := bigJSONPayload(t, 200)
	r := newGzipTestEngine("/big", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
	})

	res := doGzipRequest(t, r, http.MethodGet, "/big", map[string]string{"Accept-Encoding": "gzip"})

	if got := res.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := res.Header.Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q, want removed (压缩后长度未知)", got)
	}
	if vary := res.Header.Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Fatalf("Vary = %q, missing Accept-Encoding", vary)
	}
	if plain := gunzipBody(t, res); !bytes.Equal(plain, payload) {
		t.Fatalf("gunzipped body mismatch: got %d bytes, want %d", len(plain), len(payload))
	}
}

// 小响应不压：压了反而更大，还白耗 CPU。
func TestGzipSkipsSmallJSON(t *testing.T) {
	payload := []byte(`{"status":"ok"}`)
	r := newGzipTestEngine("/small", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
	})

	res := doGzipRequest(t, r, http.MethodGet, "/small", map[string]string{"Accept-Encoding": "gzip"})

	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty (小于 %d 字节不应压缩)", got, minGzipBytes)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, payload) {
		t.Fatalf("body = %q, want %q", body, payload)
	}
}

// 🔴 SSE 契约：必须完整复刻 handler/sse.go 的调用序列，证明中间件没有把它弄坏。
//
// sse.go 依次做：设 text/event-stream 头 -> c.Status(200) ->
// NewResponseController(SetWriteDeadline) -> c.Writer.(http.Flusher) 硬断言 ->
// WriteHeaderNow() -> Flush() -> 循环 SSEvent + Flush。
func TestGzipLeavesEventStreamIntact(t *testing.T) {
	var flusherOK, unwrapOK bool
	r := newGzipTestEngine("/api/v1/events/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache, no-transform")
		c.Status(http.StatusOK)

		// sse.go:24 清除写超时 —— 依赖 Unwrap 链能找到底层 writer。
		if _, ok := c.Writer.(interface{ Unwrap() http.ResponseWriter }); !ok {
			t.Error("c.Writer 缺少 Unwrap()，http.NewResponseController 将无法透传 SetWriteDeadline")
		} else {
			unwrapOK = true
		}

		// sse.go:34 的硬断言。
		f, ok := c.Writer.(http.Flusher)
		if !ok {
			t.Error("c.Writer 不再是 http.Flusher：sse.go 会 AbortWithStatus(500)")
			return
		}
		flusherOK = true

		c.Writer.WriteHeaderNow()
		f.Flush()

		c.SSEvent("snapshot", gin.H{"n": 1})
		f.Flush()
	})

	res := doGzipRequest(t, r, http.MethodGet, "/api/v1/events/stream", map[string]string{
		"Accept-Encoding": "gzip",
		"Accept":          "text/event-stream",
	})

	if !flusherOK || !unwrapOK {
		t.Fatalf("SSE writer contract broken (flusher=%v unwrap=%v)", flusherOK, unwrapOK)
	}
	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty（SSE 绝不能压缩：逐帧 flush 被缓冲即失去实时性）", got)
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "snapshot") {
		t.Fatalf("SSE body = %q, want it to contain the event payload", body)
	}
}

// SSE 无论走 Accept 头还是路径判据，都必须旁路（双保险不能有一边失效）。
func TestGzipBypassesEventStreamByPathOnly(t *testing.T) {
	r := newGzipTestEngine("/events/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, strings.Repeat("x", 4096))
	})

	// 不带 Accept: text/event-stream，只靠路径命中。
	res := doGzipRequest(t, r, http.MethodGet, "/events/stream", map[string]string{"Accept-Encoding": "gzip"})

	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty (路径判据未生效)", got)
	}
}

// 🔴 兜底契约：即使请求**没命中**入口旁路（路径后缀不匹配、Accept 也不是
// text/event-stream），只要响应 Content-Type 是 text/event-stream，包装器就必须
// ①保住 http.Flusher（否则流式调用方直接失效）②不压缩 ③逐帧立即写出。
//
// 为什么必须单独测：上面那条 SSE 用例打在 /api/v1/events/stream，**命中了入口旁路**，
// 根本没进包装分支 —— 也就是「包装器包住流式响应」这条真实存在的路径此前零覆盖。
// `/api/v1/assistant/chat` 就是这种形态（它目前带 Accept: text/event-stream 会走旁路，
// 但旁路判定是约定、不是类型保证；新增流式端点时很容易漏）。
func TestGzipKeepsStreamingIntactWhenNotBypassed(t *testing.T) {
	var flusherOK bool
	r := newGzipTestEngine("/api/v1/assistant/chat", func(c *gin.Context) {
		f, ok := c.Writer.(http.Flusher)
		if !ok {
			t.Error("c.Writer 不再是 http.Flusher：ginSSEWriter.WriteEvent 的 Flush 会失效")
			return
		}
		flusherOK = true
		// 复刻 handler/ginSSEWriter.WriteEvent 的调用形态：小帧 -> Flush -> 大帧 -> Flush
		c.SSEvent("navigate", gin.H{"path": "/x"})
		f.Flush()
		c.SSEvent("token", gin.H{"text": strings.Repeat("z", 2048)})
		f.Flush()
	})

	res := doGzipRequest(t, r, http.MethodPost, "/api/v1/assistant/chat", map[string]string{
		"Accept-Encoding": "gzip",
		"Accept":          "*/*",
	})

	if !flusherOK {
		t.Fatal("包装后丢了 http.Flusher")
	}
	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty（text/event-stream 必须原样）", got)
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "event:navigate") || !strings.Contains(got, "event:token") {
		t.Fatalf("SSE 帧不完整（两帧都必须在，且未被缓冲吞掉）: %q", got)
	}
}

// 🔴 下载契约：http.ServeContent 的 Content-Length / Accept-Ranges 必须原样保留。
func TestGzipLeavesBinaryDownloadIntact(t *testing.T) {
	payload := bytes.Repeat([]byte{0x00, 0x01, 0x02, 0x03}, 4096) // 16 KB
	r := newGzipTestEngine("/download", func(c *gin.Context) {
		c.Header("Content-Type", "application/octet-stream")
		http.ServeContent(c.Writer, c.Request, "TrustMesh-Setup.exe", time.Now(), bytes.NewReader(payload))
	})

	res := doGzipRequest(t, r, http.MethodGet, "/download", map[string]string{"Accept-Encoding": "gzip"})

	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty (二进制下载不得压缩)", got)
	}
	if got := res.Header.Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q, want bytes (electron-updater 差分下载依赖它)", got)
	}
	if got := res.Header.Get("Content-Length"); got != "16384" {
		t.Fatalf("Content-Length = %q, want 16384", got)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, payload) {
		t.Fatalf("body mismatch: got %d bytes, want %d", len(body), len(payload))
	}
}

// 🔴 Range 请求（差分更新的实际形态）必须返回 206 且内容正确。
func TestGzipPassesThroughRangeRequests(t *testing.T) {
	payload := bytes.Repeat([]byte{0xAB}, 8192)
	r := newGzipTestEngine("/download", func(c *gin.Context) {
		c.Header("Content-Type", "application/octet-stream")
		http.ServeContent(c.Writer, c.Request, "app-64.7z", time.Now(), bytes.NewReader(payload))
	})

	res := doGzipRequest(t, r, http.MethodGet, "/download", map[string]string{
		"Accept-Encoding": "gzip",
		"Range":           "bytes=0-1023",
	})

	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.StatusCode)
	}
	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if got := res.Header.Get("Content-Range"); got != "bytes 0-1023/8192" {
		t.Fatalf("Content-Range = %q, want bytes 0-1023/8192", got)
	}
	body, _ := io.ReadAll(res.Body)
	if len(body) != 1024 {
		t.Fatalf("partial body = %d bytes, want 1024", len(body))
	}
}

// 客户端未声明 gzip 时不压（electron-updater 拉 latest.yml 就是这种请求）。
func TestGzipSkipsWithoutAcceptEncoding(t *testing.T) {
	payload := bigJSONPayload(t, 200)
	r := newGzipTestEngine("/big", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
	})

	res := doGzipRequest(t, r, http.MethodGet, "/big", nil)

	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, payload) {
		t.Fatalf("body mismatch: got %d bytes, want %d", len(body), len(payload))
	}
}

// 响应已带 Content-Encoding 时不得二次压缩（双重编码客户端解不开）。
func TestGzipSkipsWhenAlreadyEncoded(t *testing.T) {
	payload := bigJSONPayload(t, 200)
	r := newGzipTestEngine("/pre", func(c *gin.Context) {
		c.Header("Content-Encoding", "br")
		c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
	})

	res := doGzipRequest(t, r, http.MethodGet, "/pre", map[string]string{"Accept-Encoding": "gzip, br"})

	if got := res.Header.Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br (不得改写成 gzip)", got)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, payload) {
		t.Fatalf("body mismatch: got %d bytes, want %d", len(body), len(payload))
	}
}

// gzip;q=0 是显式拒绝，必须尊重。
func TestGzipHonorsExplicitQZero(t *testing.T) {
	payload := bigJSONPayload(t, 200)
	r := newGzipTestEngine("/big", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8", payload)
	})

	res := doGzipRequest(t, r, http.MethodGet, "/big", map[string]string{"Accept-Encoding": "gzip;q=0"})

	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty (q=0 表示拒绝)", got)
	}
}

func TestCompressibleContentType(t *testing.T) {
	cases := []struct {
		ct   string
		want bool
	}{
		{"application/json; charset=utf-8", true},
		{"application/json", true},
		{"APPLICATION/JSON", true},
		{"text/plain; charset=utf-8", true},
		{"text/html", true},
		{"application/vnd.api+json", true},
		{"application/xml", true},
		{"application/javascript", true},
		// 🔴 必须为 false 的一类：SSE 与一切二进制。
		{"text/event-stream", false},
		{"text/event-stream; charset=utf-8", false},
		{"application/octet-stream", false},
		{"image/png", false},
		{"image/svg+xml", false}, // 图片按二进制处理，不因 +xml 后缀放行
		{"application/vnd.android.package-archive", false},
		{"application/zip", false},
		{"", false}, // 未知类型一律不压
	}
	for _, tc := range cases {
		if got := compressibleContentType(tc.ct); got != tc.want {
			t.Errorf("compressibleContentType(%q) = %v, want %v", tc.ct, got, tc.want)
		}
	}
}

func TestRequestAcceptsGzip(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"gzip", true},
		{"gzip, deflate, br", true},
		{"deflate, gzip;q=0.8", true},
		{"GZIP", true},
		{"gzip;q=0", false},
		{"gzip; q=0.0", false},
		{"deflate, br", false},
		{"", false},
		{"identity", false},
	}
	for _, tc := range cases {
		if got := requestAcceptsGzip(tc.header); got != tc.want {
			t.Errorf("requestAcceptsGzip(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

func TestAddVaryHeaderIsIdempotent(t *testing.T) {
	h := http.Header{}
	addVaryHeader(h, "Accept-Encoding")
	addVaryHeader(h, "Accept-Encoding")
	if got := len(h.Values("Vary")); got != 1 {
		t.Fatalf("Vary entries = %d, want 1", got)
	}

	h2 := http.Header{}
	h2.Set("Vary", "Origin")
	addVaryHeader(h2, "Accept-Encoding")
	addVaryHeader(h2, "Accept-Encoding")
	joined := strings.Join(h2.Values("Vary"), ",")
	if strings.Count(joined, "Accept-Encoding") != 1 {
		t.Fatalf("Vary = %q, want exactly one Accept-Encoding", joined)
	}
	if !strings.Contains(joined, "Origin") {
		t.Fatalf("Vary = %q, lost the pre-existing Origin", joined)
	}
}
