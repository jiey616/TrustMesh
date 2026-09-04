# -*- coding: utf-8 -*-
"""A 方案：mime 双保险。
1) store_artifact.go: mimeMatchesDeclared 中 actual 为空改为不匹配；
   新增导出函数 InferMimeFromName（按扩展名推断 mime）。
2) webhook.go: artifact 构造时 MimeType 为空则按文件名兜底推断。
两个文件均为 LF。
"""
import sys, os

BASE = r"D:/AIWorkspace/TrustMesh/backend"

def load(rel):
    p = os.path.join(BASE, rel)
    d = open(p, "rb").read().decode("utf-8")
    assert "\r\n" not in d, f"{rel}: unexpected CRLF"
    return p, d

def save(p, d):
    open(p, "wb").write(d.encode("utf-8"))

fails = []

# ---------- 1. store_artifact.go ----------
p1, d = load("internal/store/store_artifact.go")

# 1a. mimeMatchesDeclared：actual 空 → 不匹配
old = """	d, a := normalizeMimeKey(declared), normalizeMimeKey(actual)
	if d == "" || a == "" {
		return true
	}"""
new = """	d, a := normalizeMimeKey(declared), normalizeMimeKey(actual)
	if d == "" {
		return true
	}
	// An unknown actual type must NOT auto-match: an upload whose MIME we
	// cannot determine is a draft/unknown file, and silently filing it into
	// the step's single slot is exactly the misclassification this package
	// must avoid (2026-09-04: .md screenplay drafts filed as the docx
	// deliverable because the agent sends mimeType:"" and empty matched all).
	if a == "" {
		return false
	}"""
if old in d:
    d = d.replace(old, new, 1)
    print("[OK] 1a. mimeMatchesDeclared: empty actual -> mismatch")
else:
    fails.append("1a")

# 1b. 新增 InferMimeFromName（放在 normalizeMimeKey 之前）
anchor = "// normalizeMimeKey reduces a MIME type or a bare extension"
helper = """// extToMime maps file extensions to canonical MIME types, used to backfill
// an upload whose transfer message omitted mimeType (the ClawSynapse CLI
// currently sends mimeType:"" for every file). Keeping the same canonical
// families as mimeToExt lets the inferred value compare equal to workflow
// slot declarations such as "docx" or "markdown".
var extToMime = map[string]string{
	".docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx":     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx":     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".md":       "text/markdown",
	".markdown": "text/markdown",
	".csv":      "text/csv",
	".txt":      "text/plain",
	".pdf":      "application/pdf",
	".json":     "application/json",
	".zip":      "application/zip",
	".jpg":      "image/jpeg",
	".jpeg":     "image/jpeg",
	".png":      "image/png",
	".mp4":      "video/mp4",
}

// InferMimeFromName returns a MIME type inferred from the file name
// extension, or "" when the extension is unknown.
func InferMimeFromName(fileName string) string {
	ext := strings.ToLower(path.Ext(strings.TrimSpace(fileName)))
	if m, ok := extToMime[ext]; ok {
		return m
	}
	return ""
}

"""
if "func InferMimeFromName" in d:
    print("[SKIP] 1b. InferMimeFromName already present")
elif anchor in d:
    d = d.replace(anchor, helper + anchor, 1)
    print("[OK] 1b. InferMimeFromName added")
else:
    fails.append("1b")

# 1c. import 补 path
old_imp = """import (
	"strings"
	"time\""""
new_imp = """import (
	"path"
	"strings"
	"time\""""
if '"path"' in d.split("var mimeToExt")[0]:
    print("[SKIP] 1c. path already imported")
elif old_imp in d:
    d = d.replace(old_imp, new_imp, 1)
    print("[OK] 1c. path import added")
else:
    fails.append("1c")

# 1d. 更新 inferOutputBinding 注释（策略描述与实现一致）
old_doc = """//   - a single slot whose mime does not match is left unbound with a warning.
func inferOutputBinding"""
new_doc = """//   - a single slot whose mime does not match is left unbound with a warning;
//     an upload with unknown mime (empty after normalisation) also fails to
//     match — call sites should backfill mime via InferMimeFromName first.
func inferOutputBinding"""
if "an upload with unknown mime" in d:
    print("[SKIP] 1d. comment already updated")
elif old_doc in d:
    d = d.replace(old_doc, new_doc, 1)
    print("[OK] 1d. policy comment updated")
else:
    fails.append("1d")

save(p1, d)

# ---------- 2. webhook.go ----------
p2, d = load("internal/clawsynapse/webhook.go")

old_art = """	artifact := model.TaskArtifact{
		TransferID: msg.TransferID,
		TaskID:     taskID,
		TodoID:     todoID,
		FileName:   msg.FileName,
		FileSize:   msg.FileSize,
		LocalPath:  msg.LocalPath,
		MimeType:   msg.MimeType,"""
new_art = """	mimeType := msg.MimeType
	if strings.TrimSpace(mimeType) == "" {
		// The ClawSynapse CLI sends mimeType:"" for every transfer; backfill
		// from the file name so downstream mime-based classification
		// (inferOutputBinding) sees the real type instead of "unknown".
		mimeType = store.InferMimeFromName(msg.FileName)
	}

	artifact := model.TaskArtifact{
		TransferID: msg.TransferID,
		TaskID:     taskID,
		TodoID:     todoID,
		FileName:   msg.FileName,
		FileSize:   msg.FileSize,
		LocalPath:  msg.LocalPath,
		MimeType:   mimeType,"""
if "mimeType = store.InferMimeFromName(msg.FileName)" in d:
    print("[SKIP] 2. webhook fallback already present")
elif old_art in d:
    d = d.replace(old_art, new_art, 1)
    print("[OK] 2. webhook: transfer mime fallback added")
else:
    fails.append("2")

save(p2, d)

if fails:
    print("[FAIL] steps:", ", ".join(fails))
    sys.exit(1)
print("ALL DONE")
