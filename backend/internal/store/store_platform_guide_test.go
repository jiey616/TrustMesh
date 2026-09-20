package store

import (
	"strings"
	"testing"
)

// 上传校验是纯函数，直接表驱动；这层是「管理端误传工程产物 / 非法文件名」
// 的第一道闸（渲染安全由前端 sandbox iframe 兜底，见 model 注释）。
func TestValidateGuideUpload(t *testing.T) {
	ok := []struct {
		name     string
		fileName string
		html     []byte
	}{
		{"常规 html", "guide.html", []byte("<html><body>ok</body></html>")},
		{"大写扩展名", "GUIDE.HTML", []byte("<p>x</p>")},
		{"带路径前缀取后缀判定", "some/dir/guide.html", []byte("<p>x</p>")},
	}
	for _, c := range ok {
		if err := validateGuideUpload(c.fileName, c.html); err != nil {
			t.Errorf("%s: 期望通过，实际 %v", c.name, err)
		}
	}

	bad := []struct {
		name     string
		fileName string
		html     []byte
		want     string
	}{
		{"空文件名", "  ", []byte("<p>x</p>"), "invalid file name"},
		{"非 html 扩展名", "guide.pdf", []byte("<p>x</p>"), "invalid file name"},
		{"无扩展名", "guide", []byte("<p>x</p>"), "invalid file name"},
		{"空文档", "guide.html", nil, "empty document"},
		{"超限文档", "guide.html", make([]byte, platformGuideMaxUpload+1), "document too large"},
	}
	for _, c := range bad {
		err := validateGuideUpload(c.fileName, c.html)
		if err == nil {
			t.Errorf("%s: 期望拒绝，实际通过", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: 错误信息应含字段 %q，实际 %v", c.name, c.want, err)
		}
	}
}

// 2MiB 恰好等于上限：边界值必须通过（off-by-one 是这类校验最典型的回归）。
func TestValidateGuideUploadExactBoundary(t *testing.T) {
	exact := make([]byte, platformGuideMaxUpload)
	if err := validateGuideUpload("guide.html", exact); err != nil {
		t.Errorf("恰好 2MiB 应通过，实际 %v", err)
	}
}
