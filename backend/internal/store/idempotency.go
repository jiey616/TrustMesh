package store

import (
	"crypto/sha1"
	"encoding/hex"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// idemCheckOrRecord 是 T2.6 通用幂等键原语。
//
// 契约（调用方必须遵守）：
//   - seen=true  → 该 key 已存在（本进程或另一实例记录过）。调用方应跳过本次处理 /
//     返回首结果，绝不能重复落库或重复派发。
//   - seen=false → 首次出现，且已完成记录（慢路径写 Mongo + 写进程内缓存）。调用方
//     继续正常处理。
//   - err!=nil   → 记录失败。按「宁可失败也不静默放行重复」致命化（对齐 T2.x 的
//     持久化错误处理），调用方须立即返回，不得继续。
//
// 设计要点：
//   - 快路径：持自有锁 idemCacheMu 查进程内缓存，未过期直接返回 seen=true（零 Mongo 往返）。
//   - 仅快路径查/写缓存在锁内；Mongo 慢路径在锁外执行，避免把一次 InsertOne 的临界区
//     串行化到所有幂等检查（含命中缓存的并发调用）。
//   - 慢路径（Mongo 启用时）：InsertOne({_id:key, expire_at:now+ttl})。成功→写缓存、
//     seen=false；E11000（dup）→ seen=true（跨实例命中，不报错）；其他 Mongo 错→
//     mongoWriteError 返回。
//   - Mongo 未启用：仅写缓存、seen=false（退化为内存语义，与现状等价，不阻断）。
func (s *Store) idemCheckOrRecord(key string, ttl time.Duration) (bool, *transport.AppError) {
	if key == "" {
		// 空键无意义：既不记录也不误判，按「首次、不阻断」处理，避免把不同请求
		// 混到同一个空键上相互误杀。
		return false, nil
	}

	now := time.Now().UTC()
	expireAt := now.Add(ttl)

	// 快路径（持自有锁）。读 mongoEnabled 也在此锁内，给启动期的写提供 happens-before，
	// 避免与 enableMongo 的写产生无同步的数据竞争（-race 友好）。
	s.idemCacheMu.Lock()
	if exp, ok := s.idemCache[key]; ok {
		if exp.After(now) {
			s.idemCacheMu.Unlock()
			return true, nil
		}
		// 已过期：删掉，走慢路径重新记录（不返回 seen，避免窗口外误杀）。
		delete(s.idemCache, key)
	}
	mongoOn := s.mongoEnabled && s.mongoIdempotencyKeys != nil
	s.idemCacheMu.Unlock()

	if !mongoOn {
		// 内存模式：仅写缓存、返回首次。
		s.idemCacheMu.Lock()
		s.touchIdemCacheLocked(key, expireAt)
		s.idemCacheMu.Unlock()
		return false, nil
	}

	// 慢路径：Mongo 唯一键插入。持锁外执行，避免阻塞命中缓存的并发调用。
	ctx, cancel := s.mongoContext()
	defer cancel()
	_, err := s.mongoIdempotencyKeys.InsertOne(ctx, bson.M{"_id": key, "expire_at": expireAt})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			// 跨实例命中：另一实例已记录同键。不报错，返回 seen=true。
			return true, nil
		}
		// 其他 Mongo 错误：宁可失败也不静默放行重复（与 T2.x「致命化」一致）。
		return false, mongoWriteError(err)
	}

	s.idemCacheMu.Lock()
	s.touchIdemCacheLocked(key, expireAt)
	s.idemCacheMu.Unlock()
	return false, nil
}

// touchIdemCacheLocked 在已持有 idemCacheMu 的前提下写入/更新缓存项，并在超过上限时
// 按「近似 LRU」淘汰一项，避免进程内缓存无界增长。不追踪精确访问序（仅容量上限），
// 与 cleanup.go 对 processedMessages 的淘汰策略一致。调用方必须持有 idemCacheMu。
func (s *Store) touchIdemCacheLocked(key string, expireAt time.Time) {
	s.idemCache[key] = expireAt
	if len(s.idemCache) <= maxProcessedMessages {
		return
	}
	// 容量超限：优先删一个已过期项；无过期项则删任意一个。
	for k, exp := range s.idemCache {
		if exp.Before(time.Now().UTC()) {
			delete(s.idemCache, k)
			return
		}
	}
	for k := range s.idemCache {
		delete(s.idemCache, k)
		return
	}
}

// meetingMessageSoftKey 为未带客户端幂等键的会议消息派生软键（T2.6 B）。
//
// 组成：meetingID|senderType|senderID|sha1(content)。不含时间窗——超时重试的去重
// 窗口由 idemCheckOrRecord 的 ttl 参数（会议消息 5m）控制，软键本身只表达
// 「同会议、同发送者、同内容」这一稳定语义。客户端显式携带 IdempotencyKey 时不调用
// 本函数（直接用它作 key）。
func meetingMessageSoftKey(meetingID, senderType, senderID, content string) string {
	sum := sha1.Sum([]byte(content))
	return meetingID + "|" + senderType + "|" + senderID + "|" + hex.EncodeToString(sum[:])
}

// findStoredMeetingMessageByKeyUnsafe 在持锁内按幂等键找回已存消息（T2.6 B 的 seen 路径）。
//
// 先扫进程内消息 map（同进程重复重试，命中即返回），未命中且 Mongo 启用时回查
// meeting_messages 集合（跨实例命中 / 重启后命中）。返回 nil 表示库中确实没有该键对应
// 的消息——seen=true 却查不到属极端情况（TTL 窗口内被并发清理），调用方应按「首次」
// 继续处理，避免丢消息。
//
// 调用方必须持有 s.mu（AddMeetingMessage 的写锁上下文）。
func (s *Store) findStoredMeetingMessageByKeyUnsafe(key string) *model.MeetingMessage {
	for _, m := range s.meetingMessages {
		if m != nil && m.IdempotencyKey == key {
			cp := *m
			return &cp
		}
	}
	if s.mongoEnabled && s.mongoMeetingMessages != nil {
		ctx, cancel := s.mongoContext()
		defer cancel()
		var m model.MeetingMessage
		if err := s.mongoMeetingMessages.FindOne(ctx, bson.M{"idempotency_key": key}).Decode(&m); err == nil {
			return &m
		}
	}
	return nil
}
