package store

import (
	"strings"
	"testing"
)

// 上传校验是纯函数，直接表驱动。文件名会进公开下载 URL 路径段，
// 与桌面包同一套 ASCII 守门；version 必须出现在文件名里（配对校验，
// 防「1.2.0 的包挂 1.3.0 的版本号」这类错发）。
func TestValidateMobileAppUpload(t *testing.T) {
	ok := []struct {
		name     string
		version  string
		fileName string
	}{
		{"常规", "1.0.0", "TrustMesh-1.0.0.apk"},
		{"prerelease", "1.0.0-beta.1", "TrustMesh-1.0.0-beta.1.apk"},
		{"大写扩展名", "2.3.4", "trustmesh-2.3.4.APK"},
	}
	for _, c := range ok {
		if err := validateMobileAppUpload(c.version, c.fileName); err != nil {
			t.Errorf("%s: 期望通过，实际 %v", c.name, err)
		}
	}

	bad := []struct {
		name     string
		version  string
		fileName string
		want     string
	}{
		{"非 semver", "v1", "TrustMesh-v1.apk", "invalid version"},
		{"空版本", "", "TrustMesh-1.0.0.apk", "invalid version"},
		{"非 apk", "1.0.0", "TrustMesh-1.0.0.exe", "invalid file name"},
		{"文件名不含版本", "1.0.0", "TrustMesh-latest.apk", "invalid file name"},
		{"含空格", "1.0.0", "TrustMesh 1.0.0.apk", "invalid file name"},
		{"含路径分隔符", "1.0.0", "a/b/TrustMesh-1.0.0.apk", "invalid file name"},
		{"含引号", "1.0.0", `TrustMesh"1.0.0.apk`, "invalid file name"},
	}
	for _, c := range bad {
		err := validateMobileAppUpload(c.version, c.fileName)
		if err == nil {
			t.Errorf("%s: 期望拒绝，实际通过", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: 错误信息应含 %q，实际 %v", c.name, c.want, err)
		}
	}
}
