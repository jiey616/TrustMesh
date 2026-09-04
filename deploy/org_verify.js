// 多租户阶段 1：回填结果校验。
//   mongosh --quiet trustmesh /tmp/org_verify.js
//
// 校验三件事：
//   1. 每个 user 恰好 1 个 personal org + 1 条 owner membership；
//   2. 每个集合 org_id 覆盖率（未回填计数应为 0，除非无主数据）；
//   3. 跨集合一致性：artifact 跟随 task、project_file/meeting 跟随 project、
//      meeting_message 跟随 meeting。

function pad(s, n) {
  s = String(s);
  while (s.length < n) s += " ";
  return s;
}

var problems = [];
function check(cond, msg) {
  if (!cond) problems.push(msg);
}

print("=== 1. 个人租户覆盖 ===");
var userCount = db.users.countDocuments({});
var okUsers = 0;
db.users.find({}).forEach(function (u) {
  var mids = db.org_memberships.find({ user_id: u._id }).toArray();
  var personal = [];
  mids.forEach(function (m) {
    var o = db.organizations.findOne({ _id: m.org_id });
    if (o && o.kind === "personal") personal.push({ org: o, role: m.role });
  });
  if (personal.length !== 1) {
    problems.push("user " + u._id + " has " + personal.length + " personal org(s)");
    return;
  }
  if (personal[0].role !== "owner") {
    problems.push("user " + u._id + " personal membership role = " + personal[0].role);
    return;
  }
  okUsers++;
});
print("users=" + userCount + "  with_personal_org=" + okUsers);
check(okUsers === userCount, "not every user has exactly one personal org");

print("");
print("=== 2. org_id 覆盖率 ===");
var colls = [
  "projects", "tasks", "agents", "agent_chats", "comments", "notifications",
  "events", "join_requests", "knowledge_documents", "knowledge_chunks",
  "workflow_templates", "external_apps", "artifacts", "project_files",
  "meetings", "meeting_messages",
];
var totalMissing = 0;
colls.forEach(function (c) {
  var total = db.getCollection(c).countDocuments({});
  var missing = db.getCollection(c).countDocuments({
    $or: [{ org_id: { $exists: false } }, { org_id: null }, { org_id: "" }],
  });
  totalMissing += missing;
  print(pad(c, 22) + " total=" + pad(total, 6) + " missing=" + pad(missing, 6));
});

print("");
print("=== 3. 跨集合一致性 ===");
function consistency(name, coll, link, targetColl, targetKey) {
  var bad = 0, checked = 0;
  db.getCollection(coll)
    .find({ org_id: { $exists: true, $ne: "" } })
    .forEach(function (d) {
      var key = d[link];
      if (!key) return;
      var t = db.getCollection(targetColl).findOne({ [targetKey]: key });
      if (!t) return;
      checked++;
      if (t.org_id && t.org_id !== d.org_id) bad++;
    });
  print(pad(name, 34) + " checked=" + pad(checked, 6) + " mismatched=" + pad(bad, 4));
  check(bad === 0, name + ": " + bad + " org mismatch");
}
consistency("artifact.org == task.org", "artifacts", "task_id", "tasks", "_id");
consistency("project_file.org == project.org", "project_files", "project_id", "projects", "_id");
consistency("meeting.org == project.org", "meetings", "project_id", "projects", "_id");
consistency("meeting_message.org == meeting.org", "meeting_messages", "meeting_id", "meetings", "_id");

print("");
print("=== 4. 孤儿归属（有 org_id 但 user 无此 membership）===");
var orphan = 0;
db.tasks.find({ org_id: { $exists: true, $ne: "" } }).forEach(function (t) {
  if (!t.user_id) return;
  var m = db.org_memberships.findOne({ org_id: t.org_id, user_id: t.user_id });
  if (!m) orphan++;
});
print("tasks with org the user is not a member of: " + orphan);
check(orphan === 0, orphan + " tasks point to an org the owner does not belong to");

print("");
if (totalMissing > 0) problems.push(totalMissing + " documents still have no org_id");
if (problems.length === 0) {
  print("=== VERIFY OK ===");
} else {
  print("=== VERIFY FAILED ===");
  problems.forEach(function (p) {
    print(" - " + p);
  });
}
