package store

import (
	"context"
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"trustmesh/backend/internal/model"
)

// ─── T3.1 W2：SSE 跨实例广播（Mongo 事件总线）───
//
// 问题：userSubscribers 是 per-process 的 channel map（user_streams.go），
// publishUserEventUnsafe 只能投给**本实例**的订阅者。多实例下 webhook 打到实例 B，
// 而用户的 SSE 连在实例 A → 事件永久丢失。读回源 Mongo 解决不了这个（见设计文档 §0）。
//
// 约束（甲方决策 1）：**backend 不得直连 NATS**，保持「一切经 clawsynapsed」的架构边界。
// 又因 Mongo 是 standalone（docker-compose.yml:4-12 无 --replSet）→ Change Streams 不可用。
// 故本工作流用 **Mongo 轮询式 outbox + tailer**：零新基建，跨实例延迟 ≈ 轮询周期(500ms)。
//
// 数据流：
//
//	本地产事件 → ① 立即投本地订阅者（零延迟，行为与改造前一致）
//	            → ② 非阻塞塞进 outboxCh → 后台 writer 取 seq 并落 Mongo
//	其他实例   → tailer 每 500ms 拉 seq > cursor → 投给本地订阅者（跳过自己发的）
//
// 三条硬不变量：
//  1. **广播关闭（默认）→ 行为与改造前逐字节一致**：outboxCh 为 nil，publish 只走本地投递。
//  2. **绝不阻塞持锁路径**：publishUserEventUnsafe 在 s.mu 持锁内被调用，塞 outbox 用
//     select+default；channel 满则丢弃并计数告警。SSE 是尽力而为的提示，前端已有轮询兜底，
//     丢事件的后果是「最坏 10s 后由轮询补上」，绝不允许为此拖慢持锁写路径。
//  3. **载荷以 JSON 字符串存储**：payload 内嵌 model.Event / model.Agent 等 struct，
//     若走原生 BSON 往返，解码成 map[string]any 后字段名会退回 bson tag，与本地直投路径
//     的 json tag 不一致 → 前端解析断裂。存 json.Marshal 后的整条事件，跨实例与本地投递
//     的 JSON 才逐字节等价。

const (
	// sseEventSeqKey 是全局自增序号所在计数器文档的 _id。
	sseEventSeqKey = "user-events"
	// sseOutboxCapacity 是 outbox 缓冲深度。满时丢弃最新事件（保已入队者）。
	sseOutboxCapacity = 256
	// sseTailInterval 是 tailer 轮询周期 = 跨实例投递延迟上界。
	sseTailInterval = 500 * time.Millisecond
	// sseTailBatch 是单轮最多拉取的事件数（背压保护）。
	sseTailBatch = 200
	// sseSeqGapWaits 是遇到 seq 空洞时最多等几轮再跳过。
	// 空洞来源：$inc 取号与 insert 落库是两步，两步之间崩溃或被并发反超都会留下缺口。
	// 等 3 轮（1.5s）足够反超的插入就位；超上限则跳过，宁可丢一个事件也不让总线永久卡死。
	sseSeqGapWaits = 3
)

// sseUserEventDoc 是 outbox 集合里的物理文档。
type sseUserEventDoc struct {
	Seq       int64     `bson:"_id"`
	OriginID  string    `bson:"origin_id"`
	UserID    string    `bson:"user_id"`
	EventJSON string    `bson:"event_json"`
	CreatedAt time.Time `bson:"created_at"`
}

// StartSSEBroadcast 启动 outbox writer + tailer 两个 goroutine。
// 仅当 cfg.SSEBroadcastEnabled 且 Mongo 可用时由 app 层启动（见 SSEBroadcastArmed）。
func (s *Store) StartSSEBroadcast(ctx context.Context) {
	if !s.SSEBroadcastArmed() {
		return
	}
	if s.log != nil {
		s.log.Info("sse broadcast started",
			zap.String("instance", s.sseInstanceID),
			zap.Duration("tail_interval", sseTailInterval))
	}
	go s.runSSEOutboxWriter(ctx)
	go s.runSSETailer(ctx)
}

// SSEBroadcastArmed 报告是否应启动跨实例广播：开关开启 + 集合已接线（Mongo 可用）。
func (s *Store) SSEBroadcastArmed() bool {
	return s.sseBroadcastEnabled && s.mongoUserEvents != nil
}

// 停机语义：outbox channel **永不关闭**。publishUserEventUnsafe 在持锁路径上随时可能
// enqueue，而后台 ticker（ops 巡检 / 超时提醒）在 HTTP drain 之后仍会发事件；若停机时
// close(ch)，check-then-send 无法原子消除「send on closed channel」panic 窗口。
// 故 writer 改为由 ctx 驱动退出并尽力排空，未落库的事件留在缓冲里被 GC（前端轮询兜底）。

// enqueueSSEEventUnsafe 把事件非阻塞塞进 outbox。**必须在持锁路径内调用且不得做任何 I/O**。
// 返回是否入队（未启用/已满都返回 false，调用方无需处理——广播是尽力而为）。
func (s *Store) enqueueSSEEventUnsafe(event model.UserStreamEvent, userID string) bool {
	ch := s.outboxCh // 本地快照：channel 永不关闭，故 select+default 安全
	if ch == nil {
		return false
	}
	item := sseUserEventDoc{
		OriginID:  s.sseInstanceID,
		UserID:    userID,
		EventJSON: s.marshalUserEvent(event),
		CreatedAt: time.Now().UTC(),
	}
	select {
	case ch <- item:
		return true
	default:
		// 缓冲满：丢弃并计数。SSE 是提示通道，前端轮询兜底，绝不为此拖慢持锁写路径。
		n := s.sseDropped.Add(1)
		if s.log != nil && n%64 == 1 {
			s.log.Warn("sse outbox full, event dropped (poll fallback covers it)",
				zap.String("user_id", userID),
				zap.String("type", event.Type),
				zap.Int64("dropped_total", n))
		}
		return false
	}
}

func (s *Store) marshalUserEvent(event model.UserStreamEvent) string {
	raw, err := json.Marshal(event)
	if err != nil {
		// UserStreamEvent 由内部构造（map/struct/time），正常不可能失败；
		// 真遇到就跳过广播而非中断业务写。
		if s.log != nil {
			s.log.Warn("sse event marshal failed, skip broadcast",
				zap.String("type", event.Type), zap.Error(err))
		}
		return ""
	}
	return string(raw)
}

// runSSEOutboxWriter 串行消费 outbox：取 seq → 落 Mongo。
// 单 goroutine 消费保证同实例内入队顺序即 seq 顺序（跨实例的反超由 tailer 的 gap 等待兜）。
//
// outbox channel 永不关闭（见文件头不变量 2），故退出只由 ctx 驱动：取消后尽力排空
// 缓冲里已入队的事件，然后返回；没来得及落库的事件留在缓冲里被 GC，由前端轮询兜底。
func (s *Store) runSSEOutboxWriter(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			s.drainSSEOutbox()
			return
		case item := <-s.outboxCh:
			s.writeSSEEvent(ctx, item)
		}
	}
}

// drainSSEOutbox 在停机时尽力把缓冲里剩余事件落库（不再取新事件）。
func (s *Store) drainSSEOutbox() {
	for {
		select {
		case item := <-s.outboxCh:
			s.writeSSEEvent(context.Background(), item)
		default:
			return
		}
	}
}

func (s *Store) writeSSEEvent(ctx context.Context, item sseUserEventDoc) {
	if item.EventJSON == "" {
		return // marshal 失败已告警，不落库
	}
	if ctx.Err() != nil {
		return // 已取消：不再发起新的 Mongo 往返
	}
	seq, err := s.nextSSESeq()
	if err != nil {
		s.noteSSEWriteFailure("allocate seq", err)
		return
	}
	item.Seq = seq
	writeCtx, cancel := s.mongoContext()
	defer cancel()
	if _, err := s.mongoUserEvents.InsertOne(writeCtx, item); err != nil {
		// seq 已消耗但文档未落 → 留下空洞，由 tailer 的 gap 等待机制跳过。
		s.noteSSEWriteFailure("insert event", err)
	}
}

// nextSSESeq 用单文档 $inc 原子取号。返回的是**新**序号（从 1 起）。
// 自持 mongoContext() 有界超时：writer 的 ctx 来自 context.Background()，若直接沿用
// 就没有任何超时，Mongo 卡死会让 writer goroutine 永久阻塞、outbox 随即塞满。
func (s *Store) nextSSESeq() (int64, error) {
	ctx, cancel := s.mongoContext()
	defer cancel()

	var res struct {
		Seq int64 `bson:"seq"`
	}
	err := s.mongoUserEventsSeq.FindOneAndUpdate(ctx,
		bson.M{"_id": sseEventSeqKey},
		bson.M{"$inc": bson.M{"seq": int64(1)}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&res)
	if err != nil {
		return 0, err
	}
	return res.Seq, nil
}

func (s *Store) noteSSEWriteFailure(stage string, err error) {
	n := s.sseWriteErrs.Add(1)
	if s.log != nil && n%64 == 1 {
		s.log.Warn("sse broadcast write failed (poll fallback covers it)",
			zap.String("stage", stage),
			zap.Int64("failures_total", n),
			zap.Error(err))
	}
}

// runSSETailer 每 sseTailInterval 拉取增量事件并投给本地订阅者。
//
// 游标语义：s.sseCursor = 本实例**已确认处理过**的最大 seq。首轮启动时对齐到当前最大 seq
// （不回放过期历史），之后严格单调推进。
//
// seq 空洞处理：取号与落库分离，并发反超或中途失败都会留下缺口。若直接跳过，落后的那条
// 会被游标越过而永久丢失；因此遇到空洞先等 sseSeqGapWaits 轮，仍不来才跳过并告警。
func (s *Store) runSSETailer(ctx context.Context) {
	if !s.awaitSSECursor(ctx) {
		return
	}
	ticker := time.NewTicker(sseTailInterval)
	defer ticker.Stop()
	gapWaits := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gapWaits = s.tailSSEBatchOnce(ctx, gapWaits)
		}
	}
}

// awaitSSECursor 等 Mongo 可用并把游标对齐到当前最大 seq；ctx 取消返回 false。
func (s *Store) awaitSSECursor(ctx context.Context) bool {
	for {
		cursor, err := s.maxSSESeq(ctx)
		if err == nil {
			s.sseCursor = cursor
			if s.log != nil {
				s.log.Info("sse tailer cursor aligned", zap.Int64("seq", cursor))
			}
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		s.noteSSEWriteFailure("init tailer cursor", err)
		select {
		case <-ctx.Done():
			return false
		case <-time.After(sseTailInterval):
		}
	}
}

func (s *Store) maxSSESeq(ctx context.Context) (int64, error) {
	readCtx, cancel := s.mongoContext()
	defer cancel()
	var res struct {
		Seq int64 `bson:"seq"`
	}
	err := s.mongoUserEventsSeq.FindOne(readCtx, bson.M{"_id": sseEventSeqKey}).Decode(&res)
	if err == mongo.ErrNoDocuments {
		return 0, nil // 还没有任何事件
	}
	if err != nil {
		return 0, err
	}
	return res.Seq, nil
}

// tailSSEBatchOnce 处理一轮增量拉取，返回下一轮应使用的 gapWaits。
func (s *Store) tailSSEBatchOnce(ctx context.Context, gapWaits int) int {
	readCtx, cancel := s.mongoContext()
	defer cancel()

	filter := bson.M{"_id": bson.M{"$gt": s.sseCursor}}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(sseTailBatch))
	cur, err := s.mongoUserEvents.Find(readCtx, filter, opts)
	if err != nil {
		s.noteSSEWriteFailure("tailer query", err)
		return gapWaits
	}
	defer func() { _ = cur.Close(ctx) }()

	batch := sseTailBatch
	for batch > 0 && cur.TryNext(readCtx) {
		var doc sseUserEventDoc
		if err := cur.Decode(&doc); err != nil {
			s.noteSSEWriteFailure("tailer decode", err)
			continue
		}
		batch--

		if doc.Seq > s.sseCursor+1 {
			// 空洞：期望 cursor+1，实际更大。先等几轮给落后的插入就位。
			if gapWaits < sseSeqGapWaits {
				return gapWaits + 1
			}
			if s.log != nil {
				s.log.Warn("sse tailer skipping seq gap after max waits",
					zap.Int64("cursor", s.sseCursor), zap.Int64("got_seq", doc.Seq))
			}
		}

		s.sseCursor = doc.Seq
		gapWaits = 0
		if doc.OriginID == s.sseInstanceID {
			continue // 自己发的已本地直投，跳过避免重复
		}
		s.deliverRemoteSSEEvent(doc)
	}
	if err := cur.Err(); err != nil {
		s.noteSSEWriteFailure("tailer cursor iterate", err)
	}
	return gapWaits
}

// deliverRemoteSSEEvent 把远端事件投给本实例订阅者。跳过自己发的、忽略反序列化失败的。
func (s *Store) deliverRemoteSSEEvent(doc sseUserEventDoc) {
	if doc.EventJSON == "" {
		return
	}
	var event model.UserStreamEvent
	if err := json.Unmarshal([]byte(doc.EventJSON), &event); err != nil {
		s.noteSSEWriteFailure("tailer unmarshal", err)
		return
	}
	s.deliverUserEvent(doc.UserID, event)
}
