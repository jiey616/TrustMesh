#!/usr/bin/env bash
set -euo pipefail

EMAIL=""
MONGO_CONTAINER="${MONGO_CONTAINER:-trustmesh-mongo}"
MONGO_DATABASE="${MONGO_DATABASE:-trustmesh}"
APPLY_DELETE=0
AUTO_YES=0

usage() {
  cat <<'EOF'
Usage:
  scripts/delete-account-mongo-by-email.sh --email user@example.com [options]

Options:
  --email <email>              Login email to delete.
  --mongo-container <name>     MongoDB Docker container name. Default: trustmesh-mongo
  --db <name>                  MongoDB database name. Default: trustmesh
  --execute                    Execute deletion. Default is dry-run.
  --dry-run                    Only print the matched MongoDB data scope.
  --yes, -y                    Skip interactive confirmation when used with --execute.
  --help, -h                   Show this help.

Notes:
  - This script only cleans MongoDB data.
  - It intentionally preserves knowledge files and vector index data.
  - The following MongoDB collections are cleaned:
    users, agents, projects, conversations, tasks, events,
    comments, notifications, artifacts, processed_messages
EOF
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --email)
      EMAIL="${2:-}"
      shift 2
      ;;
    --mongo-container)
      MONGO_CONTAINER="${2:-}"
      shift 2
      ;;
    --db)
      MONGO_DATABASE="${2:-}"
      shift 2
      ;;
    --execute)
      APPLY_DELETE=1
      shift
      ;;
    --dry-run)
      APPLY_DELETE=0
      shift
      ;;
    --yes|-y)
      AUTO_YES=1
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [ -z "$EMAIL" ]; then
  echo "--email is required" >&2
  usage >&2
  exit 1
fi

require_command docker

if ! docker container inspect "$MONGO_CONTAINER" >/dev/null 2>&1; then
  echo "mongo container not found: $MONGO_CONTAINER" >&2
  exit 1
fi

if [ "$APPLY_DELETE" -eq 1 ] && [ "$AUTO_YES" -ne 1 ]; then
  printf 'About to delete MongoDB account data for "%s". Type "yes" to continue: ' "$EMAIL" >&2
  read -r answer
  if [ "$answer" != "yes" ]; then
    echo "aborted" >&2
    exit 1
  fi
fi

docker exec \
  -e ACCOUNT_EMAIL="$EMAIL" \
  -e APPLY_DELETE="$APPLY_DELETE" \
  -i "$MONGO_CONTAINER" \
  mongosh --quiet "$MONGO_DATABASE" --file /dev/stdin <<'MONGO'
const email = (process.env.ACCOUNT_EMAIL || "").trim();
const applyDelete = process.env.APPLY_DELETE === "1";

function impossibleFilter() {
  return { _id: { $exists: false } };
}

function orFilter(filters) {
  const cleaned = filters.filter(Boolean);
  if (cleaned.length === 0) {
    return impossibleFilter();
  }
  if (cleaned.length === 1) {
    return cleaned[0];
  }
  return { $or: cleaned };
}

function uniqueStrings(values) {
  return [...new Set(values.filter((value) => typeof value === "string" && value !== ""))];
}

function collectIDs(collectionName, filter) {
  return uniqueStrings(
    db.getCollection(collectionName)
      .find(filter, { _id: 1 })
      .toArray()
      .map((doc) => doc._id)
  );
}

function printKV(kind, key, value) {
  print([kind, key, value].join("\t"));
}

if (!email) {
  printKV("ERROR", "invalid_email", "ACCOUNT_EMAIL is empty");
  quit(2);
}

const user = db.users.findOne({ email }, { _id: 1, email: 1, name: 1 });
if (!user) {
  printKV("ERROR", "user_not_found", email);
  quit(3);
}

const userId = user._id;
const projectIDs = collectIDs("projects", { user_id: userId });
const conversationFilter = orFilter([
  { user_id: userId },
  projectIDs.length > 0 ? { project_id: { $in: projectIDs } } : null,
]);
const conversationIDs = collectIDs("conversations", conversationFilter);
const taskFilter = orFilter([
  { user_id: userId },
  projectIDs.length > 0 ? { project_id: { $in: projectIDs } } : null,
  conversationIDs.length > 0 ? { conversation_id: { $in: conversationIDs } } : null,
]);
const taskIDs = collectIDs("tasks", taskFilter);

const filters = {
  users: { _id: userId },
  agents: { user_id: userId },
  projects: { user_id: userId },
  conversations: conversationFilter,
  tasks: taskFilter,
  events: orFilter([
    { user_id: userId },
    projectIDs.length > 0 ? { project_id: { $in: projectIDs } } : null,
    taskIDs.length > 0 ? { task_id: { $in: taskIDs } } : null,
  ]),
  comments: orFilter([
    { user_id: userId },
    taskIDs.length > 0 ? { task_id: { $in: taskIDs } } : null,
  ]),
  notifications: { user_id: userId },
  artifacts: taskIDs.length > 0 ? { task_id: { $in: taskIDs } } : impossibleFilter(),
  processed_messages: taskIDs.length > 0 ? { resource_id: { $in: taskIDs } } : impossibleFilter(),
};

const counts = {
  users: db.users.countDocuments(filters.users),
  agents: db.agents.countDocuments(filters.agents),
  projects: db.projects.countDocuments(filters.projects),
  conversations: db.conversations.countDocuments(filters.conversations),
  tasks: db.tasks.countDocuments(filters.tasks),
  events: db.events.countDocuments(filters.events),
  comments: db.comments.countDocuments(filters.comments),
  notifications: db.notifications.countDocuments(filters.notifications),
  artifacts: db.artifacts.countDocuments(filters.artifacts),
  processed_messages: db.processed_messages.countDocuments(filters.processed_messages),
};

printKV("MODE", "value", applyDelete ? "execute" : "dry-run");
printKV("USER", "id", userId);
printKV("USER", "email", user.email);
printKV("USER", "name", user.name || "");
printKV("NOTE", "preserved", "knowledge_documents, knowledge_chunks, files, vector_index");

Object.entries(counts).forEach(([name, count]) => {
  printKV("COUNT", name, count);
});

if (!applyDelete) {
  printKV("SUMMARY", "status", "dry-run only");
  quit(0);
}

const deleted = {
  artifacts: db.artifacts.deleteMany(filters.artifacts).deletedCount,
  comments: db.comments.deleteMany(filters.comments).deletedCount,
  notifications: db.notifications.deleteMany(filters.notifications).deletedCount,
  events: db.events.deleteMany(filters.events).deletedCount,
  processed_messages: db.processed_messages.deleteMany(filters.processed_messages).deletedCount,
  tasks: db.tasks.deleteMany(filters.tasks).deletedCount,
  conversations: db.conversations.deleteMany(filters.conversations).deletedCount,
  projects: db.projects.deleteMany(filters.projects).deletedCount,
  agents: db.agents.deleteMany(filters.agents).deletedCount,
  users: db.users.deleteOne(filters.users).deletedCount,
};

Object.entries(deleted).forEach(([name, count]) => {
  printKV("DELETED", name, count);
});

printKV("SUMMARY", "status", "delete completed");
MONGO
