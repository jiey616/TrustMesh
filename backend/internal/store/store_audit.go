package store

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

// ────────────────────────────────────────────────────────────────────────────
// 审计日志（设计文档 §6.4）：audit_logs 集合，追加写 + 平台侧只读查询。
//
// 与其余资源不同，审计**不进全内存状态机**：它是旁路证据流，重启后无需回写，
// 查询直接打 Mongo（有 TTL 索引兜底回收，保留 180 天）。
// Mongo 不可用时降级为进程内有界环（dev/测试可用，重启即丢）。
// ─────────────────────────────────────────────────────────────────────────────

// auditMemCap 是 Mongo 不可用时内存审计环的容量（只保留最近 N 条）。
const auditMemCap = 500

// auditDefaultLimit / auditMaxLimit 控制查询返回条数上限。
const (
	auditDefaultLimit = 100
	auditMaxLimit     = 500
)

// RecordAudit 落一条审计。**永不阻断业务**：写失败只告警（审计缺失不得变成操作失败）。
// entry.ID / entry.CreatedAt 为空时由本方法补齐。
func (s *Store) RecordAudit(entry model.AuditLog) {
	if s == nil {
		return
	}
	if entry.ID == "" {
		entry.ID = "aud_" + newID()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	s.auditMu.Lock()
	s.auditMem = append(s.auditMem, entry)
	if len(s.auditMem) > auditMemCap {
		s.auditMem = append([]model.AuditLog(nil), s.auditMem[len(s.auditMem)-auditMemCap:]...)
	}
	s.auditMu.Unlock()

	if s.mongoEnabled && s.mongoAuditLogs != nil {
		ctx, cancel := s.mongoContext()
		defer cancel()
		if _, err := s.mongoAuditLogs.InsertOne(ctx, entry); err != nil && s.log != nil {
			s.log.Warn("audit log write failed",
				zap.String("action", entry.Action), zap.String("scope", entry.Scope), zap.Error(err))
		}
	}
}

// ListAuditLogs 查询审计（时间倒序）。Mongo 可用时查库（权威、含历史），
// 否则回落进程内环。
func (s *Store) ListAuditLogs(q model.AuditQuery) []model.AuditLog {
	if s == nil {
		return nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = auditDefaultLimit
	}
	if limit > auditMaxLimit {
		limit = auditMaxLimit
	}
	if s.mongoEnabled && s.mongoAuditLogs != nil {
		filter := bson.M{}
		if q.Scope != "" {
			filter["scope"] = q.Scope
		}
		if q.Action != "" {
			filter["action"] = q.Action
		}
		if q.ActorUserID != "" {
			filter["actor_user_id"] = q.ActorUserID
		}
		ctx, cancel := s.mongoContext()
		defer cancel()
		cursor, err := s.mongoAuditLogs.Find(ctx, filter,
			options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
		if err != nil {
			if s.log != nil {
				s.log.Warn("audit log query failed", zap.Error(err))
			}
		} else {
			defer cursor.Close(ctx)
			var out []model.AuditLog
			if err := cursor.All(ctx, &out); err == nil {
				if out == nil {
					out = []model.AuditLog{}
				}
				return out
			}
			if s.log != nil {
				s.log.Warn("audit log decode failed", zap.Error(err))
			}
		}
	}

	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	out := make([]model.AuditLog, 0, limit)
	for i := len(s.auditMem) - 1; i >= 0 && len(out) < limit; i-- {
		e := s.auditMem[i]
		if q.Scope != "" && e.Scope != q.Scope {
			continue
		}
		if q.Action != "" && e.Action != q.Action {
			continue
		}
		if q.ActorUserID != "" && e.ActorUserID != q.ActorUserID {
			continue
		}
		out = append(out, e)
	}
	return out
}
