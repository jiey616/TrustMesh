package store

import (
	"sort"
	"sync"
	"sync/atomic"
)

// Aggregate 标识 store 内的一类受保护资源（聚合根）。其枚举值的**数值升序**即全局
// 固定取锁顺序：任何需要同时持多把聚合锁的代码，都必须按 Aggregate 枚举升序去重排序后
// 依次获取，释放时按逆序，以此彻底杜绝死锁（见 lockAggregates / coarseWithAggregates）。
//
// 取值范围（值与顺序均已固化为契约，新增聚合只能追加在 numAggregates 之前）：
//
//	AggUser / AggAgent / AggOrganization / AggProject / AggProjectFile /
//	AggTask / AggMeeting / AggAgentChat / AggKnowledge / AggWorkflowTemplate /
//	AggJoinRequest / AggNotification / AggExternalApp / AggOpsIncident /
//	AggProcessedMessage / numAggregates
//
// ⚠️ 2026-09-16 回退说明：T2.5 的四域调用点迁移已整体回退（见
// docs/t2.5-rollback-decision-2026-09-16.md）。本文件是**纯锁基础设施**，被完整保留，
// 但**当前没有任何生产调用点使用** —— store 包内全部 map 仍统一由 Store.mu 守护。
// 保留理由：T05 将做「原子性整批迁移」（一次迁完同一张 map 的全部访问者），届时直接复用
// 本文件的原语即可，无需重写。**在 T05 落地之前，禁止把任何调用点接回这些原语**
// （半吊子迁移 = 同一张 map 两把互不互斥的锁 = P0 数据竞争）。
// 枚举与 locks 数组已为全部聚合预留槽位，保证「固定顺序」契约在后续迁移时不位移。
type Aggregate int

const (
	AggUser Aggregate = iota
	AggAgent
	AggOrganization
	AggProject
	AggProjectFile
	AggTask
	AggMeeting
	AggAgentChat
	AggKnowledge
	AggWorkflowTemplate
	AggJoinRequest
	AggNotification
	AggExternalApp
	AggOpsIncident
	AggProcessedMessage
	numAggregates
)

// ResetLockTracking 重置 debug 持锁追踪状态（仅测试 / CI 使用）。它同时关闭追踪并清空
// 已记录持锁栈，避免用例之间互相污染。配合 SetLockTracking(true) 在单测开头调用。
func ResetLockTracking() {
	lockTrackingMu.Lock()
	lockHeldStack = nil
	lockTrackingMu.Unlock()
	SetLockTracking(false)
}

// lockAggregates 按「枚举升序 + 去重」获取给定聚合的细粒度锁，返回逆序释放闭包。
//
// 不变量（贯穿 T2.5 全程）：
//  1. 获取顺序恒为 Aggregate 枚举升序，禁止乱序 / 反向；
//  2. 调用方在持聚合锁期间**禁止**再获取 Store.mu（最外层或单独使用的兜底锁），
//     即「持聚合锁后取 s.mu」属反向嵌套，运行期不允许（不变量 2）；
//  3. 每把受保护 map 只有一个保护锁（迁移后为聚合锁），本函数不引入第二把锁。
//
// 典型用途：跨聚合读（如会议域读项目）时 lockAggregates(AggMeeting, AggProject)。
func (s *Store) lockAggregates(aggs ...Aggregate) func() {
	// 去重 + 升序排序，保证全局固定顺序（即使调用方乱序传入也不致死锁）。
	seen := make(map[Aggregate]struct{}, len(aggs))
	ordered := make([]Aggregate, 0, len(aggs))
	for _, a := range aggs {
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		ordered = append(ordered, a)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })

	if lockTrackingEnabled.Load() {
		lockTrackingMu.Lock()
		if len(lockHeldStack) > 0 {
			lockTrackingMu.Unlock()
			// 不变量 2：禁止在持有聚合锁后再取更多聚合锁（须一次 lockAggregates 取齐），
			// 否则会破坏「固定升序、单次取齐」的死锁防护约定。
			panic("t2.5: nested aggregate lock acquisition while already holding aggregate lock(s); acquire all needed locks in a single lockAggregates call")
		}
		lockTrackingMu.Unlock()
	}

	for _, a := range ordered {
		s.locks[a].Lock()
		trackLockHeld(a)
	}

	// 释放闭包：逆序解锁。
	return func() {
		for i := len(ordered) - 1; i >= 0; i-- {
			a := ordered[i]
			trackLockReleased(a)
			s.locks[a].Unlock()
		}
	}
}

// lockProject 是 lockAggregates(AggProject) 的便捷方法（单聚合读写统一入口）。
// 返回逆序释放闭包，配合 defer 使用：defer s.lockProject()()。
func (s *Store) lockProject() func() { return s.lockAggregates(AggProject) }

// lockProjectFile 是 lockAggregates(AggProjectFile) 的便捷方法。
func (s *Store) lockProjectFile() func() { return s.lockAggregates(AggProjectFile) }

// lockTask 是 lockAggregates(AggTask) 的便捷方法。
func (s *Store) lockTask() func() { return s.lockAggregates(AggTask) }

// lockMeeting 是 lockAggregates(AggMeeting) 的便捷方法。
func (s *Store) lockMeeting() func() { return s.lockAggregates(AggMeeting) }

// coarseWithAggregates 先取最外层兜底锁 Store.mu，再按升序取给定聚合锁（内层），
// 返回逆序释放闭包（先释聚合、后释 s.mu）。
//
// 用于「既触碰已迁入聚合锁的聚合（需聚合锁），又触碰仍由 s.mu 守护的 map（如
// agents / organizations / projectVisible 内部的 org 校验等）」的跨聚合写路径。
// 它显式体现不变量 2 的**允许方向**：s.mu 作为最外层，聚合锁作为内层，绝不反向。
//
// ⚠️ 2026-09-16 回退后：**当前没有任何聚合已迁入聚合锁**，所有 map 仍由 s.mu 守护，
// 故本函数暂未被任何运行时路径使用（仅被死锁压测覆盖），留待 T05 原子性整批迁移启用。
func (s *Store) coarseWithAggregates(aggs ...Aggregate) func() {
	if lockTrackingEnabled.Load() {
		lockTrackingMu.Lock()
		if len(lockHeldStack) > 0 {
			lockTrackingMu.Unlock()
			// 不变量 2 的禁止方向：已持聚合锁后再取 s.mu（反向嵌套）直接 panic。
			panic("t2.5: reverse lock nesting: coarseWithAggregates acquired s.mu while holding aggregate lock(s)")
		}
		lockTrackingMu.Unlock()
	}
	s.mu.Lock()
	releaseAggs := s.lockAggregates(aggs...)
	return func() {
		releaseAggs()
		s.mu.Unlock()
	}
}

// flushSectionLock 为 FlushPersistAll 的「按段取锁」（T04 / AC-5）提供统一入口：
//   - agg == numAggregates（哨兵）：本段仅触碰仍未迁移、由 s.mu 守护的聚合（users / agents /
//     joinRequests / agentChats / knowledgeDocs / workflowTemplates / organizations /
//     orgMemberships / projectMembers / opsIncidents / llmConfigs / processedMessages /
//     notifications / externalApps）→ 仅取 s.mu；
//   - 否则：coarseWithAggregates(agg)（s.mu 外层 + 聚合锁内层），使段内对 projectMembers 等
//     仍由 s.mu 守护的 map 的读取同样安全。
//
// 返回逆序释放闭包；每段处理完即释放，整轮 sweep 不再持单一长锁（AC-5 临界区收敛）。
func (s *Store) flushSectionLock(agg Aggregate) func() {
	if agg == numAggregates {
		s.mu.Lock()
		return func() { s.mu.Unlock() }
	}
	return s.coarseWithAggregates(agg)
}

// flushSection 描述 FlushPersistAll 的「一类实体回写段」：agg 为该段保护锁对应的聚合
// （numAggregates 表示仅由 s.mu 守护），run 为段内回写闭包。
type flushSection struct {
	agg Aggregate
	run func()
}

// ─── 可选：debug 构建/测试下的持锁追踪（零成本运行路径，仅测试/CI 开启） ───

// lockTrackingEnabled 控制持锁追踪。生产路径恒为 false（原子读即返回，无互斥开销）；
// 测试或 debug 构建可调用 SetLockTracking(true) 开启，用于顺序 / 嵌套断言（AC-6 静态与
// 运行期双重保险）。
var lockTrackingEnabled atomic.Bool

// SetLockTracking 开关 debug 持锁追踪（仅测试 / CI 使用）。
func SetLockTracking(on bool) { lockTrackingEnabled.Store(on) }

var lockTrackingMu sync.Mutex
var lockHeldStack []Aggregate // 受 lockTrackingMu 保护，LIFO 记录当前持锁（debug 用）

func trackLockHeld(a Aggregate) {
	if !lockTrackingEnabled.Load() {
		return
	}
	lockTrackingMu.Lock()
	lockHeldStack = append(lockHeldStack, a)
	lockTrackingMu.Unlock()
}

func trackLockReleased(a Aggregate) {
	if !lockTrackingEnabled.Load() {
		return
	}
	lockTrackingMu.Lock()
	for i := len(lockHeldStack) - 1; i >= 0; i-- {
		if lockHeldStack[i] == a {
			lockHeldStack = append(lockHeldStack[:i], lockHeldStack[i+1:]...)
			break
		}
	}
	lockTrackingMu.Unlock()
}
