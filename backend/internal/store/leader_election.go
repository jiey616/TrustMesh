package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// T3.1 第 1 步：后台循环 leader 选举。
//
// 背景：StartTimeoutMonitor / StartDispatchReconciler / StartOpsScanner 这三个 ticker
// 各自扫全量内存状态并触发**外部副作用**（催办 / 重派 / 开运维工单 / 下发指引）。单实例
// 无碍；一旦 backend 起多实例，每个实例都会各跑一份 → N 倍重复派发、N 倍催办、N 倍工单。
// （StartCleanupTicker 只清进程内 map、无外部副作用，故**不纳入** leader 门禁——各实例
// 各自回收自己的内存才是正确的。）
//
// 机制：单文档 CAS 租约（复用 Mongo，不引入 Redis）。租约文档
//
//	{_id: leaderLeaseKey, holder, expires_at}
//
// 用一条 FindOneAndUpdate(upsert) 原子地表达「我是持有者 **或** 租约已过期 → 我抢到」：
//   - filter 命中（我是 holder / 已过期 / 文档不存在）→ 更新成功 → leader。
//   - filter 不命中（他人持有且未过期）→ upsert 试图以同一 _id 插入 → E11000 → 非 leader。
//
// fail-safe 方向（关键取舍）：**CAS 出错（含 Mongo 抖动）一律判为非 leader**。理由：这三个
// ticker 是「每 1~2min 一轮」的安全网，短暂停摆的代价远小于多实例双派发；宁可少跑一轮，
// 也不冒重复执行的风险。单实例且租约正常时，持有者每 renew 周期续租，不会误停。
//
// 灰度：整套门禁由 cfg.LeaderElectionEnabled 控制，**默认 false** → isLeaderForBackground
// 恒 true，行为与改造前逐字节一致（零风险上线）。真正要起多实例时再置 true。

const (
	// leaderLeaseKey 是租约文档的固定 _id（全局唯一一把「后台循环锁」）。
	leaderLeaseKey = "background-tickers"
	// leaderLeaseTTL 是租约有效期。必须 > renew 间隔 + Mongo 往返超时，否则持有者会在
	// 自己续租前就被判定过期、被他人抢走（flapping）。
	leaderLeaseTTL = 30 * time.Second
	// leaderRenewInterval 是续租/抢占的尝试周期。持有者在此周期把 expires_at 往后推 TTL。
	leaderRenewInterval = 10 * time.Second
)

// leaderCAS 抽象租约的原子读改写，使状态机可在无真实 Mongo 的单元测试里用 fake 驱动
// （对齐 dispatchHook / persistFailForTest 的 seam 风格）。生产实现为 mongoLeaderCAS。
type leaderCAS interface {
	// tryAcquireOrRenew 原子地尝试获取或续租租约。
	// 返回 acquired=true 表示调用后本实例持有租约；err!=nil 表示 CAS 本身失败（含 Mongo 抖动），
	// 调用方须按「非 leader」处理（fail-safe）。他人持有且未过期 → (false, nil)。
	tryAcquireOrRenew(ctx context.Context, key, holder string, now time.Time, ttl time.Duration) (acquired bool, err error)
	// release 尽力释放租约（仅当自己仍是持有者时删除），用于优雅停机，让新实例无需等 TTL。
	release(ctx context.Context, key, holder string) error
}

// mongoLeaderCAS 是 leaderCAS 的 Mongo 实现，绑定到 store 的租约集合。
type mongoLeaderCAS struct {
	col *mongo.Collection
}

func (c mongoLeaderCAS) tryAcquireOrRenew(ctx context.Context, key, holder string, now time.Time, ttl time.Duration) (bool, error) {
	filter := bson.M{
		"_id": key,
		"$or": bson.A{
			bson.M{"holder": holder},
			bson.M{"expires_at": bson.M{"$lte": now}},
		},
	}
	update := bson.M{"$set": bson.M{
		"holder":     holder,
		"expires_at": now.Add(ttl),
		"renewed_at": now,
	}}
	opts := options.FindOneAndUpdate().SetUpsert(true)
	err := c.col.FindOneAndUpdate(ctx, filter, update, opts).Err()
	// 注意驱动语义：upsert:true 且文档不存在时，服务端插入成功但返回 value:null，
	// Go 驱动会把它报成 ErrNoDocuments——这不是失败，恰恰是「抢到锁」的路径。
	if err == nil || errors.Is(err, mongo.ErrNoDocuments) {
		return true, nil
	}
	// 他人持有且未过期：filter 不命中 → upsert 以同一 _id 插入 → 唯一键冲突。判非 leader，非错误。
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return false, err
}

func (c mongoLeaderCAS) release(ctx context.Context, key, holder string) error {
	_, err := c.col.DeleteOne(ctx, bson.M{"_id": key, "holder": holder})
	return err
}

// StartLeaderElection 驱动租约状态机：每 leaderRenewInterval 尝试获取/续租，维护
// s.isLeader。仅当 cfg.LeaderElectionEnabled 且 Mongo 可用时由 app 层启动；否则三个
// ticker 走 isLeaderForBackground 的「恒 leader」分支（单实例现状）。
//
// 启动顺序不变量（T0.10b 同款）：必须在所有 Set*Hook 之后、且早于依赖
// isLeaderForBackground 的三个 ticker 首轮触发；renew 间隔(10s) 远小于 ticker 间隔，
// 首轮 ticker 前 isLeader 已完成首次 CAS。
func (s *Store) StartLeaderElection(ctx context.Context) {
	if s.leaderCAS == nil {
		// 未接线 CAS（理论上 app 层已保证不会发生）：保守起见保持 isLeader=true，
		// 等价于门禁关闭，绝不因选举组件缺失而停摆安全网。
		return
	}
	if s.log != nil {
		s.log.Info("leader election started",
			zap.String("holder", s.leaderID),
			zap.Duration("renew_interval", leaderRenewInterval),
			zap.Duration("lease_ttl", leaderLeaseTTL))
	}
	// 立即抢一次，避免等满一个 renew 周期才确立身份（缩短冷启动空窗）。
	s.renewLeadershipOnce(time.Now().UTC())
	ticker := time.NewTicker(leaderRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.onLeadershipShutdown()
			return
		case <-ticker.C:
			s.renewLeadershipOnce(time.Now().UTC())
		}
	}
}

// renewLeadershipOnce 执行一次 CAS 获取/续租并更新 leader 身份。抽出为方法便于单测直接驱动
// （注入 fake CAS + 显式 now）。CAS 出错 → isLeader=false（fail-safe，见文件头取舍）。
func (s *Store) renewLeadershipOnce(now time.Time) {
	ctx, cancel := s.mongoContext()
	defer cancel()

	acquired, err := s.leaderCAS.tryAcquireOrRenew(ctx, leaderLeaseKey, s.leaderID, now, leaderLeaseTTL)
	if err != nil {
		// Mongo 抖动/超时：判非 leader（宁可暂停安全网，不冒双派风险）。
		if wasLeader := s.isLeader.Swap(false); wasLeader && s.log != nil {
			s.log.Warn("leadership lost: lease CAS error, background tickers paused",
				zap.String("holder", s.leaderID), zap.Error(err))
		}
		return
	}
	was := s.isLeader.Swap(acquired)
	if acquired && !was && s.log != nil {
		s.log.Info("leadership acquired: background tickers enabled", zap.String("holder", s.leaderID))
	} else if !acquired && was && s.log != nil {
		s.log.Info("leadership lost: another instance holds the lease, background tickers paused",
			zap.String("holder", s.leaderID))
	}
}

// onLeadershipShutdown 在 ctx 取消时尽力释放租约，让新实例无需等 TTL 即可接管。
// 释放失败不影响正确性（租约会自然过期），故只记日志。
func (s *Store) onLeadershipShutdown() {
	if s.leaderCAS == nil {
		return
	}
	s.isLeader.Store(false)
	ctx, cancel := s.mongoContext()
	defer cancel()
	if err := s.leaderCAS.release(ctx, leaderLeaseKey, s.leaderID); err != nil && s.log != nil {
		s.log.Warn("leader election: release lease failed (will expire by TTL)",
			zap.String("holder", s.leaderID), zap.Error(err))
	}
}

// isLeaderForBackground 是三个有外部副作用的 ticker 的门禁。
//   - 门禁未启用 / Mongo 不可用：恒 true → 单实例现状，零行为变化。
//   - 门禁启用：仅 leader 返回 true。
func (s *Store) isLeaderForBackground() bool {
	if !s.leaderElectionEnabled {
		return true
	}
	return s.isLeader.Load()
}

// LeaderElectionArmed 报告是否应启动 leader 选举循环：门禁开启且 CAS 已接线
// （即 Mongo 可用）。app 层据此决定是否 go StartLeaderElection。
func (s *Store) LeaderElectionArmed() bool {
	return s.leaderElectionEnabled && s.leaderCAS != nil
}
