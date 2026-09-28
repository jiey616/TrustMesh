package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// 响应体压缩（gzip）。
//
// 背景（2026-09-24 生产实测，非推测）：任务事件流 GET /tasks/:id/events 在真实
// 任务上返回 **542 KB 未压缩 JSON**，公网耗时 1.4–2.3 s；而同一请求在服务端容器内
// 直连 localhost:8080 只要 8–10 ms ⇒ 延迟几乎全部出在传输上。后端此前**完全没有**
// gzip（带 Accept-Encoding: gzip 请求，响应仍无 Content-Encoding），前端 4 s 一次
// 的轮询因此每次重传 542 KB，「待人工确认」状态与确认弹框长期错位。
//
// 策略：**延迟决策 + 响应类型判定**，刻意不枚举路径。
//
//   - 只有 Content-Type 明确是 JSON/文本时才压；
//   - SSE（text/event-stream）与一切二进制（图片 / octet-stream / apk / 安装包）
//     天然免疫 —— 这点很关键：下载端点走 http.ServeContent，必须保留
//     Range / Content-Length 语义（electron-updater 的差分更新依赖 .blockmap +
//     Range），把它压坏会**静默**退化成每次全量 87 MB；
//   - 小于 minGzipBytes 的响应不压（压缩小体反而更大，还白耗 CPU）；
//   - 客户端未声明 Accept-Encoding: gzip 时不压（electron-updater 就不声明）。
//
// 🔴 绝不可改为「无条件包装 c.Writer 一路压到底」：handler/sse.go 里
//
//	flusher, ok := c.Writer.(http.Flusher)          // 硬断言，不含 ok 分支的兜底
//	http.NewResponseController(c.Writer).SetWriteDeadline(...)  // 依赖 Unwrap 链
//
// 包装器只要漏实现其中任何一个，SSE 就会静默失效 —— 而那正是本次要修的头号问题。
// 因此本中间件在入口就对 SSE 前置旁路（根本不进包装分支），同时包装器完整实现
// Write / WriteString / WriteHeaderNow / Flush / Unwrap；对应的契约用例见
// gzip_test.go 的 TestGzipLeavesEventStreamIntact。
const minGzipBytes = 1024

// Gzip 返回按需 gzip 压缩响应体的 gin 中间件。应挂在全局（engine.Use）。
func Gzip() gin.HandlerFunc {
	pool := &sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}

	return func(c *gin.Context) {
		if !requestAcceptsGzip(c.Request.Header.Get("Accept-Encoding")) {
			c.Next()
			return
		}
		// SSE 长连接前置旁路：它需要逐帧立即 flush，任何缓冲都会破坏实时性，
		// 而且它绝不该进入「可能包装 c.Writer」的分支（见上方硬断言说明）。
		if requestWantsEventStream(c.Request) {
			c.Next()
			return
		}

		gw := &gzipWriter{ResponseWriter: c.Writer, pool: pool}
		c.Writer = gw
		defer gw.close()
		c.Next()
	}
}

// gzipWriter 包装 gin.ResponseWriter，在第一次写 body 时才决定是否压缩。
//
// 为什么必须「延迟」到写 body 而不是 WriteHeader：gin 的渲染链路是
//
//	c.Status(code) -> r.Render(w) -> writeContentType(w, ...) -> w.Write(body)
//
// 即 WriteHeader 被调用时 Content-Type **还没设**（c.Status 早于 writeContentType），
// 只有到 Write 时类型才是可信的。
type gzipWriter struct {
	gin.ResponseWriter
	pool *sync.Pool

	// buf 缓存「尚未确定是否压缩」期间写下的字节；达到 minGzipBytes 即结算。
	buf []byte
	// decided 为 true 后，compress 才代表最终结论。
	decided  bool
	compress bool
	gz       *gzip.Writer
	closed   bool
}

func (w *gzipWriter) Write(p []byte) (int, error) {
	if w.closed {
		return w.ResponseWriter.Write(p)
	}
	if !w.decided {
		if len(w.buf)+len(p) < minGzipBytes {
			w.buf = append(w.buf, p...)
			return len(p), nil
		}
		w.commit(compressibleContentType(w.Header().Get("Content-Type")))
	}
	if w.compress {
		return w.gz.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

// WriteString 必须重写：gin 的 c.String() 走 ResponseWriter.WriteString，
// 不重写会绕过压缩逻辑直接写底层（内嵌接口的方法提升），造成「有时压有时不压」。
func (w *gzipWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// WriteHeaderNow 被显式调用说明调用方要求立即发头（流式响应）。
// SSE 已在入口旁路，走到这里说明类型可压，直接结算，避免头迟迟不发
// 让客户端 fetch 长时间挂住。
func (w *gzipWriter) WriteHeaderNow() {
	if !w.decided {
		w.commit(compressibleContentType(w.Header().Get("Content-Type")))
		return
	}
	w.ResponseWriter.WriteHeaderNow()
}

// Flush 必须重写：未达阈值就要求 flush 的说明是流式小帧，原样输出、绝不压缩；
// 已压缩时先 flush gzip 缓冲，再透传底层 Flush。
func (w *gzipWriter) Flush() {
	if !w.decided {
		w.commit(false)
	} else if w.compress && w.gz != nil {
		_ = w.gz.Flush()
	}
	w.ResponseWriter.Flush()
}

// Unwrap 暴露被包装的 writer，使 http.NewResponseController（如 sse.go 用它清除
// 写超时）能继续向下找到真正支持 SetWriteDeadline 的底层实现。
func (w *gzipWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// commit 结算压缩决策，并把已缓冲的字节写出去（同时立即发出响应头）。
func (w *gzipWriter) commit(enable bool) {
	w.decided = true

	h := w.Header()
	// 已有 Content-Encoding（上游代理已压 / handler 自行压）时放弃：
	// 双重编码客户端解不开。
	if enable && h.Get("Content-Encoding") != "" {
		enable = false
	}
	if enable {
		// 压缩后长度未知，必须撤掉 Content-Length，让 net/http 回落 chunked。
		h.Del("Content-Length")
		h.Set("Content-Encoding", "gzip")
		addVaryHeader(h, "Accept-Encoding")
		w.gz = w.pool.Get().(*gzip.Writer)
		w.gz.Reset(w.ResponseWriter)
	}
	w.compress = enable

	w.ResponseWriter.WriteHeaderNow()

	if len(w.buf) == 0 {
		return
	}
	buf := w.buf
	w.buf = nil
	if enable {
		_, _ = w.gz.Write(buf)
		return
	}
	_, _ = w.ResponseWriter.Write(buf)
}

// close 由中间件 defer 调用：结算尚未压缩的尾量并归还 gzip.Writer。
func (w *gzipWriter) close() {
	if w.closed {
		return
	}
	w.closed = true

	if !w.decided {
		// 整个响应体都没到阈值：原样输出。
		w.decided = true
		if len(w.buf) > 0 {
			_, _ = w.ResponseWriter.Write(w.buf)
			w.buf = nil
		}
		return
	}
	if w.gz != nil {
		_ = w.gz.Close()
		w.pool.Put(w.gz)
		w.gz = nil
	}
}

// requestAcceptsGzip 判断 Accept-Encoding 是否允许 gzip（含 q=0 显式拒绝）。
func requestAcceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		for _, p := range strings.Split(params, ";") {
			p = strings.TrimSpace(p)
			v, ok := strings.CutPrefix(p, "q=")
			if !ok {
				continue
			}
			// 🔴 必须按数值判：`q=0`、`q=0.0`、`q=0.000` 都是显式拒绝，
			// 只比对字面 "0" 会漏掉后两种。
			if q, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && q <= 0 {
				return false
			}
		}
		return true
	}
	return false
}

// requestWantsEventStream 识别 SSE 请求，供入口旁路使用。
// 双判据：前端显式带 Accept: text/event-stream；路径兜底。
func requestWantsEventStream(r *http.Request) bool {
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		return true
	}
	return strings.HasSuffix(r.URL.Path, "/events/stream")
}

// compressibleContentType 只对明确的文本/JSON 类型放行压缩。
//
// 🔴 未知类型一律不压：宁可少压一个接口，也绝不能把带 Range 语义的二进制下载压坏
// （那类损坏是静默的 —— 客户端只会变慢或校验失败，不会报错）。
func compressibleContentType(ct string) bool {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" || ct == "text/event-stream" {
		return false
	}
	if strings.HasPrefix(ct, "text/") {
		return true
	}
	// 🔴 只认 application/*：`image/svg+xml` 这类虽然名字里带 +xml，但它是图片，
	// 走的是 http.ServeContent（要保留 Content-Length / Range），一律按二进制处理。
	if !strings.HasPrefix(ct, "application/") {
		return false
	}
	switch ct {
	case "application/json", "application/javascript", "application/xml":
		return true
	}
	return strings.HasSuffix(ct, "+json") || strings.HasSuffix(ct, "+xml")
}

// addVaryHeader 幂等地追加 Vary 值（响应可能已带其它 Vary 项）。
func addVaryHeader(h http.Header, value string) {
	for _, existing := range h.Values("Vary") {
		for _, part := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	h.Add("Vary", value)
}
