// 多租户阶段 1：个人租户创建 + org_id 全集合回填。
//
// 用法（在 mongo 容器内）：
//   mongosh --quiet trustmesh --eval "var APPLY=false;" /tmp/org_backfill.js   // 预演
//   mongosh --quiet trustmesh --eval "var APPLY=true;"  /tmp/org_backfill.js   // 落盘
//
// 幂等：只回填 org_id 缺失/为空的文档；重复执行不会改写已有归属。
// 归属解析优先级：user_id 直连 > 经 task_id > 经 project_id > 经 meeting_id > created_by。

var APPLY = (typeof APPLY !== "undefined" && APPLY === true);
var now = new Date();

function hex() {
  return new ObjectId().toString(); // 24 hex chars，与后端 newID() 同形
}

function pad(s, n) {
  s = String(s);
  while (s.length < n) s += " ";
  return s;
}

function report(coll, total, updated, already, orphan) {
  print(
    pad(coll, 22) +
      " total=" + pad(total, 6) +
      " updated=" + pad(updated, 6) +
      " already=" + pad(already, 6) +
      " no-owner=" + pad(orphan, 6)
  );
}

print("=== APPLY = " + APPLY + " ===");

// ---------- 1. 为每个 user 建个人租户（幂等） ----------
var userToOrg = {};
var orgCreated = 0;
var orgReused = 0;

db.users.find({}).forEach(function (u) {
  var mids = db.org_memberships.find({ user_id: u._id }).toArray();
  for (var i = 0; i < mids.length; i++) {
    var o = db.organizations.findOne({ _id: mids[i].org_id });
    if (o && o.kind === "personal") {
      userToOrg[u._id] = o._id;
      orgReused++;
      return;
    }
  }
  var orgId = "org_" + hex();
  var display = u.name || u.email || String(u._id);
  if (APPLY) {
    db.organizations.insertOne({
      _id: orgId,
      name: display,
      slug: "u-" + u._id,
      kind: "personal",
      owner_id: u._id,
      quota: { max_members: -1, max_nodes: -1, max_projects: -1, max_storage_bytes: -1 },
      created_at: now,
      updated_at: now,
    });
    db.org_memberships.insertOne({
      _id: "om_" + hex(),
      org_id: orgId,
      user_id: u._id,
      role: "owner",
      joined_at: now,
    });
  }
  userToOrg[u._id] = orgId;
  orgCreated++;
});

print("organizations: reused=" + orgReused + " created=" + orgCreated);
print("");

// ---------- 2. 派生索引 ----------
var taskUser = {};
db.tasks.find({}, { _id: 1, user_id: 1 }).forEach(function (t) {
  taskUser[t._id] = t.user_id;
});
var projUser = {};
db.projects.find({}, { _id: 1, user_id: 1 }).forEach(function (p) {
  projUser[p._id] = p.user_id;
});
var meetingOrg = {};
db.meetings.find({}, { _id: 1, org_id: 1, project_id: 1 }).forEach(function (m) {
  meetingOrg[m._id] = m.org_id || userToOrg[projUser[m.project_id]];
});

// ---------- 3. 回填 ----------
function backfill(coll, resolver) {
  var total = 0, updated = 0, already = 0, orphan = 0;
  db.getCollection(coll).find({}).forEach(function (d) {
    total++;
    if (d.org_id) {
      already++;
      return;
    }
    var org = resolver(d);
    if (!org) {
      orphan++;
      return;
    }
    if (APPLY) db.getCollection(coll).updateOne({ _id: d._id }, { $set: { org_id: org } });
    updated++;
  });
  report(coll, total, updated, already, orphan);
  return { total: total, updated: updated, already: already, orphan: orphan };
}

var byUser = function (d) {
  return userToOrg[d.user_id];
};

print("--- 直连 user_id ---");
[
  "projects",
  "tasks",
  "agents",
  "agent_chats",
  "comments",
  "notifications",
  "events",
  "join_requests",
  "knowledge_documents",
  "knowledge_chunks",
  "workflow_templates",
].forEach(function (c) {
  backfill(c, byUser);
});

print("--- 派生归属 ---");
backfill("external_apps", function (d) {
  return userToOrg[d.created_by];
});
backfill("artifacts", function (d) {
  return userToOrg[taskUser[d.task_id]];
});
backfill("project_files", function (d) {
  return userToOrg[projUser[d.project_id]];
});
backfill("meetings", function (d) {
  return userToOrg[projUser[d.project_id]];
});
backfill("meeting_messages", function (d) {
  return meetingOrg[d.meeting_id];
});

print("");
print("=== done (APPLY=" + APPLY + ") ===");
