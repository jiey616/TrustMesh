package store

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// ─── T3.1 W1：版本冲突后回源刷新（跨实例脏数据收敛）───
//
// 问题：多实例共享同一 Mongo 时，实例 A 的写若与实例 B 的并发写撞车，A 的版本化 CAS
// 未命中 → 返回 409，且 mutate*Unsafe 的快照回滚会把 A 的内存还原成**进入本次写之前**
// 的旧对象。此时 A 的内存比 Mongo 落后，而 A 的读路径只读内存（GetTask/GetProject 无
// 回查）→ **A 会持续对外返回旧数据，直到它下一次写再次撞冲突**，脏窗口无界。
//
// 单实例下这条路径几乎不可达（冲突需两个写方），故本改动对现状零影响；它补的是
// 「多实例可用」的最后一块写侧短板。
//
// 取舍（与甲方决策 3 一致）：**只刷新、不自动重试**。
//   - 重试会把「并发写」退化成「静默覆盖」，正是乐观锁要消灭的东西；
//   - 原样上交 409，让调用方（agent 回报路径靠对账器兜底、用户操作路径由前端提示）决策。
//
// 会议域不在此列：它已有 retryOnceAfterVersionRefreshLocked（meeting.go:167），且仅适用于
// 「$set 全部字段由本次输入整体派生」的会话态写入，语义与这里的通用回源不同，保持原样。
//
// 锁契约：三个 refresh* 函数均要求调用方**已持 s.mu 写锁**（沿用 *Locked 后缀约定），
// 且只在「本次 persist 已确定失败、反正要返回错误」的分支里调用 —— 成功路径零额外开销，
// 不会把 Mongo 往返塞进正常读的临界区（这是 W3 的 P0 风险，见设计文档 §1.4）。

// refreshTaskFromMongoLocked 版本冲突后把 s.tasks[taskID] 替换为 Mongo 权威文档。
// 返回是否真的刷新了（远端版本更新才算）。localVersion 传入回滚后内存对象的版本。
func (s *Store) refreshTaskFromMongoLocked(taskID string, localVersion int) bool {
	if !s.mongoEnabled || s.mongoTasks == nil {
		return false
	}
	ctx, cancel := s.mongoContext()
	defer cancel()

	var doc model.TaskDetail
	if err := s.mongoTasks.FindOne(ctx, bson.M{"_id": taskID}).Decode(&doc); err != nil {
		// 读不到（文档已删 / Mongo 抖动）→ 保持回滚后的快照，不臆造状态。
		return false
	}
	normalizeTaskVersion(&doc)
	if doc.Version <= localVersion {
		// 远端不比本地新 → 本次冲突不是「本地落后」造成的，刷新无意义。
		return false
	}
	s.tasks[taskID] = &doc
	if s.log != nil {
		s.log.Info("task refreshed from mongo after version conflict",
			zap.String("task_id", taskID),
			zap.Int("local_version", localVersion),
			zap.Int("remote_version", doc.Version))
	}
	return true
}

// refreshProjectFromMongoLocked 见 refreshTaskFromMongoLocked，作用于项目域。
func (s *Store) refreshProjectFromMongoLocked(projectID string, localVersion int) bool {
	if !s.mongoEnabled || s.mongoProjects == nil {
		return false
	}
	ctx, cancel := s.mongoContext()
	defer cancel()

	var doc model.Project
	if err := s.mongoProjects.FindOne(ctx, bson.M{"_id": projectID}).Decode(&doc); err != nil {
		return false
	}
	if doc.Version <= localVersion {
		return false
	}
	s.projects[projectID] = &doc
	if s.log != nil {
		s.log.Info("project refreshed from mongo after version conflict",
			zap.String("project_id", projectID),
			zap.Int("local_version", localVersion),
			zap.Int("remote_version", doc.Version))
	}
	return true
}

// refreshProjectFileFromMongoLocked 见 refreshTaskFromMongoLocked，作用于项目文件域。
func (s *Store) refreshProjectFileFromMongoLocked(fileID string, localVersion int) bool {
	if !s.mongoEnabled || s.mongoProjectFiles == nil {
		return false
	}
	ctx, cancel := s.mongoContext()
	defer cancel()

	var doc model.ProjectFile
	if err := s.mongoProjectFiles.FindOne(ctx, bson.M{"_id": fileID}).Decode(&doc); err != nil {
		return false
	}
	if doc.Version <= localVersion {
		return false
	}
	s.projectFiles[fileID] = &doc
	if s.log != nil {
		s.log.Info("project file refreshed from mongo after version conflict",
			zap.String("file_id", fileID),
			zap.Int("local_version", localVersion),
			zap.Int("remote_version", doc.Version))
	}
	return true
}

// asAppError 把 persist 返回的 error 归一成 *transport.AppError（非 AppError 视为写失败）。
// 存在原因：任务域提交原语返回 error 接口，直接用会踩 typed-nil 陷阱（见 persistTaskUnsafe 注释）。
func asAppError(err error) *transport.AppError {
	if err == nil {
		return nil
	}
	if ae, ok := err.(*transport.AppError); ok {
		return ae
	}
	return mongoWriteError(err)
}
