package app

import (
	"context"
	"testing"
)

// 独立验证（QA · software-qa-engineer-2）——T0.8 边界：App.FlushAll 的 nil-safety。
// main.go 在初始化失败等边界场景下可能调用到半初始化的 App；此处锁死该契约，
// 保证停机路径不会因 nil 解引用而 panic（那会把「尽力落盘」变成「必崩停机」）。
func TestAppFlushAllNilSafe(t *testing.T) {
	var nilApp *App
	if err := nilApp.FlushAll(context.Background()); err != nil {
		t.Fatalf("(*App)(nil).FlushAll = %v, want nil", err)
	}

	storelessApp := &App{} // Store == nil
	if err := storelessApp.FlushAll(context.Background()); err != nil {
		t.Fatalf("App{Store:nil}.FlushAll = %v, want nil", err)
	}

	// nil ctx 也不得 panic（Store 为 nil 时直接短路返回）。
	if err := storelessApp.FlushAll(nil); err != nil {
		t.Fatalf("App{Store:nil}.FlushAll(nil ctx) = %v, want nil", err)
	}
}
