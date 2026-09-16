package store

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/model"
)

// 权威用户读 + 账号运维字段级写 的回归测试。
//
// 背景（2026-09-16 生产实测）：双实例共享 Mongo 时，实例内存只读的
// FindUserByEmail/FindUserByID 会让「重置密码 / 禁用」时好时坏（同一密码 10 次登录 6 成功
// 4 失败），且一次重置密码会用陈旧的整文档快照把另一实例刚写的 disabled=true 覆盖丢失。
//
// 用例分两层：
//  1. 纯内存（CI 必跑）：Mongo 关闭/不可用时，权威读必须回落内存，行为与改造前完全一致。
//  2. 真 Mongo（TRUSTMESH_TEST_MONGO_URI 门控，未设置 t.Skip）：权威读以 Mongo 为准、
//     字段级写不覆盖其它实例写入的字段。

// ─── 纯内存层 ───

// Mongo 关闭（本地单机）时权威读必须等价于内存读，不能因为改动把单机环境读空。
func TestFindUserAuthoritativeFallsBackToMemoryWithoutMongo(t *testing.T) {
	s := newStoreWithSeedAdmin()

	u, appErr := s.CreateUser("auth-a@example.com", "Auth A", "hash-1")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}

	byMail, ok := s.FindUserByEmailAuthoritative("AUTH-A@example.com ")
	if !ok {
		t.Fatal("Mongo 关闭时按邮箱权威读应回落内存")
	}
	if byMail.ID != u.ID || byMail.PasswordHash != "hash-1" {
		t.Fatalf("回落结果不符: %+v", byMail)
	}

	byID, ok := s.FindUserByIDAuthoritative(u.ID)
	if !ok {
		t.Fatal("Mongo 关闭时按 id 权威读应回落内存")
	}
	if byID.Email != "auth-a@example.com" {
		t.Fatalf("回落结果不符: %+v", byID)
	}

	// 未命中同样要如实返回 false，不能臆造。
	if _, ok := s.FindUserByEmailAuthoritative("ghost@example.com"); ok {
		t.Fatal("不存在的邮箱不应命中")
	}
	if _, ok := s.FindUserByIDAuthoritative("ghost"); ok {
		t.Fatal("不存在的 id 不应命中")
	}
}

// Mongo 已启用但不可达（点查必然超时）时，权威读同样回落内存 —— 不能让 Mongo 抖动
// 把登录整体打挂。
func TestFindUserAuthoritativeFallsBackWhenMongoUnreachable(t *testing.T) {
	cli, err := mongo.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:1/"))
	if err != nil {
		t.Fatalf("mongo.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cli.Disconnect(context.Background()) })

	s := New()
	s.log = zap.NewNop()
	s.mongoEnabled = true
	s.mongoUsers = cli.Database("trustmesh_auth_test").Collection("users")
	// 1ns 超时：任何 Mongo 往返立刻返回 deadline exceeded（手法同 task 域测试）。
	s.mongoTimeout = time.Nanosecond

	now := time.Now().UTC()
	s.mu.Lock()
	s.users["u1"] = &model.User{
		ID: "u1", Email: "fallback@example.com", Name: "Fallback",
		PasswordHash: "hash-fb", CreatedAt: now, UpdatedAt: now,
	}
	s.usersByMail["fallback@example.com"] = "u1"
	s.mu.Unlock()

	got, ok := s.FindUserByEmailAuthoritative("fallback@example.com")
	if !ok {
		t.Fatal("Mongo 不可达时应回落内存")
	}
	if got.PasswordHash != "hash-fb" {
		t.Fatalf("回落结果不符: %+v", got)
	}
	if _, ok := s.FindUserByIDAuthoritative("u1"); !ok {
		t.Fatal("Mongo 不可达时按 id 权威读应回落内存")
	}

	// 读侧宽容、写侧严格：Mongo 已启用但不可达时字段级写必须把驱动错误上抛，
	// 由调用方（SetUserDisabled 等）转成写失败 —— 静默当成功会造成「禁用成功」的假象。
	if err := s.persistUserFieldsUnsafe("u1", bson.M{"disabled": true}, nil); err == nil {
		t.Fatal("Mongo 已启用但不可达时字段级写必须上抛错误，不能静默当成功")
	}
}

// Mongo 关闭时字段级写是 no-op（保证本地开发不因缺少 Mongo 失败）。
func TestPersistUserFieldsUnsafeNoopWithoutMongo(t *testing.T) {
	s := New()
	if err := s.persistUserFieldsUnsafe("u1", bson.M{"disabled": true}, bson.M{"disabled_at": ""}); err != nil {
		t.Fatalf("Mongo 关闭时字段级写应返回 nil，实际 %v", err)
	}
	if err := s.persistUserFieldsUnsafe("", bson.M{"disabled": true}, nil); err != nil {
		t.Fatalf("空 userID 应返回 nil，实际 %v", err)
	}
	if err := s.persistUserFieldsUnsafe("u1", nil, nil); err != nil {
		t.Fatalf("空变更集应返回 nil，实际 %v", err)
	}
}

// ─── 真 Mongo 层 ───

// newLiveUserMongoStore 连接 TRUSTMESH_TEST_MONGO_URI 指向的测试库，
// 清空 users 集合并重置内存用户表，保证用例之间互不干扰。
// 环境变量未设置 / Mongo 不可达时跳过（不让 CI 因缺 Mongo 变红）。
func newLiveUserMongoStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("TRUSTMESH_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("skip: set TRUSTMESH_TEST_MONGO_URI to a reachable MongoDB to enable this test")
	}
	db := os.Getenv("TRUSTMESH_TEST_MONGO_DB")
	if db == "" {
		db = "trustmesh_auth_test"
	}

	s := New()
	cfg := config.Config{
		MongoEnabled:  true,
		MongoURI:      uri,
		MongoDatabase: db,
		MongoTimeout:  10 * time.Second,
	}
	if err := s.enableMongo(cfg, zap.NewNop()); err != nil {
		t.Skipf("skip: mongo unavailable: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoUsers.DeleteMany(ctx, bson.D{}); err != nil {
		t.Fatalf("clean users collection: %v", err)
	}
	// enableMongo 会先把库里的用户载入内存（上面才清库），必须一并清掉内存残留。
	s.mu.Lock()
	s.users = make(map[string]*model.User)
	s.usersByMail = make(map[string]string)
	s.mu.Unlock()

	// 播种一个与本用例无关的种子管理员：种子模式开启后，CreateUser 不再把
	//「首个注册用户」自动提升为平台管理员，用例里的账号才是可禁用的普通账号
	//（SetUserDisabled 对平台管理员直接 403）。
	s.SeedPlatformAdmins([]string{"seed-admin@example.com"})
	return s
}

// 权威读必须压过陈旧内存：模拟「另一个实例改了 Mongo」。
func TestFindUserAuthoritativePrefersMongoOverStaleMemory(t *testing.T) {
	s := newLiveUserMongoStore(t)

	u, appErr := s.CreateUser("stale@example.com", "Stale", "hash-old")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}

	// 模拟另一实例：直接改库（本实例内存保持陈旧）。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.mongoUsers.UpdateOne(ctx, bson.M{"_id": u.ID}, bson.M{"$set": bson.M{
		"password_hash": "hash-new",
		"disabled":      true,
		"name":          "Renamed By Other Instance",
	}}); err != nil {
		t.Fatalf("simulate cross-instance write: %v", err)
	}

	// 内存读仍然是旧值（这正是缺陷本体，断言它以防有人误以为内存读已修好）。
	mem, ok := s.FindUserByEmail("stale@example.com")
	if !ok || mem.PasswordHash != "hash-old" || mem.Disabled {
		t.Fatalf("内存读应保持陈旧（说明用例前提失效）: %+v", mem)
	}

	auth, ok := s.FindUserByEmailAuthoritative("stale@example.com")
	if !ok {
		t.Fatal("权威读未命中")
	}
	if auth.PasswordHash != "hash-new" || !auth.Disabled || auth.Name != "Renamed By Other Instance" {
		t.Fatalf("权威读应取 Mongo 值: %+v", auth)
	}

	authByID, ok := s.FindUserByIDAuthoritative(u.ID)
	if !ok || authByID.PasswordHash != "hash-new" || !authByID.Disabled {
		t.Fatalf("按 id 权威读应取 Mongo 值: %+v", authByID)
	}

	// 权威读不写内存（刻意不回填，避免再引入一条「用陈旧副本覆盖内存」的整文档写路径）。
	time.Sleep(10 * time.Millisecond)
	mem2, _ := s.FindUserByID(u.ID)
	if mem2.PasswordHash != "hash-old" {
		t.Fatalf("权威读不应回填内存: %+v", mem2)
	}
}

// 禁用/启用必须字段级落库：disabled 与 disabled_at 正确落盘，且启用时清掉 disabled_at。
func TestSetUserDisabledPersistsFieldScoped(t *testing.T) {
	s := newLiveUserMongoStore(t)

	u, appErr := s.CreateUser("switch@example.com", "Switch", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, appErr := s.SetUserDisabled(u.ID, true); appErr != nil {
		t.Fatalf("disable: %v", appErr)
	}
	var doc model.User
	if err := s.mongoUsers.FindOne(ctx, bson.M{"_id": u.ID}).Decode(&doc); err != nil {
		t.Fatalf("read mongo: %v", err)
	}
	if !doc.Disabled || doc.DisabledAt == nil {
		t.Fatalf("禁用未落库: %+v", doc)
	}

	if _, appErr := s.SetUserDisabled(u.ID, false); appErr != nil {
		t.Fatalf("enable: %v", appErr)
	}
	var raw bson.M
	if err := s.mongoUsers.FindOne(ctx, bson.M{"_id": u.ID}).Decode(&raw); err != nil {
		t.Fatalf("read mongo: %v", err)
	}
	if disabled, _ := raw["disabled"].(bool); disabled {
		t.Fatalf("启用后 disabled 应为 false: %+v", raw)
	}
	if _, exists := raw["disabled_at"]; exists {
		t.Fatalf("启用后 disabled_at 应被 $unset 清掉: %+v", raw)
	}
}

// 回归：账号运维写不得覆盖其它实例写入的字段（生产事故本体）。
//
// 旧实现用整文档 ReplaceOne，本实例内存陈旧时会把这些字段一并写回旧值：
// 实测「一次重置密码把另一实例刚写的 disabled=true 覆盖丢失」。
func TestAccountOpsWriteDoesNotClobberCrossInstanceFields(t *testing.T) {
	s := newLiveUserMongoStore(t)

	u, appErr := s.CreateUser("clobber@example.com", "Clobber", "hash-old")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 另一实例：禁用该账号 + 改名（本实例内存不知情）。
	if _, err := s.mongoUsers.UpdateOne(ctx, bson.M{"_id": u.ID}, bson.M{"$set": bson.M{
		"disabled":    true,
		"disabled_at": time.Now().UTC(),
		"name":        "Renamed Elsewhere",
	}}); err != nil {
		t.Fatalf("simulate cross-instance write: %v", err)
	}

	// 本实例（内存陈旧）执行重置密码 —— 绝不能把 disabled / name 覆盖回旧值。
	if appErr := s.UpdateUserPassword(u.ID, "hash-reset"); appErr != nil {
		t.Fatalf("update password: %v", appErr)
	}
	if _, appErr := s.SetUserDisabled(u.ID, true); appErr != nil {
		t.Fatalf("disable (idempotent, 内存为 false 故会写一次): %v", appErr)
	}

	var doc model.User
	if err := s.mongoUsers.FindOne(ctx, bson.M{"_id": u.ID}).Decode(&doc); err != nil {
		t.Fatalf("read mongo: %v", err)
	}
	if !doc.Disabled {
		t.Fatal("disabled 被本地陈旧快照覆盖丢失（字段级写回归）")
	}
	if doc.Name != "Renamed Elsewhere" {
		t.Fatalf("name 被本地陈旧快照覆盖: %q", doc.Name)
	}
	if doc.PasswordHash != "hash-reset" {
		t.Fatalf("重置后的 password_hash 未落库: %q", doc.PasswordHash)
	}

	// 权威读能看到禁用态 —— 这才是「禁用后无法登录」的前提。
	auth, ok := s.FindUserByEmailAuthoritative("clobber@example.com")
	if !ok || !auth.Disabled || auth.PasswordHash != "hash-reset" {
		t.Fatalf("权威读结果不符: %+v", auth)
	}
}
