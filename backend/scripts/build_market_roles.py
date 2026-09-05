#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把 agency-agents-zh 的智能体定义转换为 TrustMesh 岗位市场角色包。

用法（仓库根目录执行）：

    python backend/scripts/build_market_roles.py --src tmp/agency/repo --out data/roles

输出结构（每个角色一个目录，三件套与 store.BuildRolesIndex 约定一致）：

    <out>/<slug>/IDENTITY.md   # 第一行角色名、第二行简介（索引解析只取前两行）
    <out>/<slug>/SOUL.md       # 身份记忆 / 沟通风格 / 关键规则
    <out>/<slug>/AGENTS.md     # 核心使命 / 工作流 / 交付物

slug 规则：以源文件名为准；若文件名不带所属部门前缀，则补上（保证
store.resolveDeptID 能按前缀归入正确部门，且跨部门不重名）。
"""

import argparse
import os
import re
import shutil
import sys
from collections import Counter

# 智能体目录（= 上游部门，注意 upstream convert.sh 漏了 strategy）
AGENT_DIRS = [
    "academic", "company", "design", "engineering", "finance",
    "game-development", "gis", "hr", "legal", "marketing",
    "paid-media", "product", "project-management", "sales", "security",
    "spatial-computing", "specialized", "strategy", "supply-chain",
    "support", "testing",
]

# 归入 SOUL.md（人设）的章节关键词；其余归入 AGENTS.md（业务）
SOUL_KEYWORDS = [
    "身份", "记忆", "人格", "persona", "identity",
    "沟通", "communication", "风格", "style", "语气", "tone",
    "关键规则", "critical rule", "rules you must follow", "规则", "rule",
    "行为准则", "原则", "principle", "价值观", "边界", "红线",
]

# 明确排除在 SOUL 之外的（命中则强制归 AGENTS，避免"规则"关键词误伤业务流程）
AGENTS_OVERRIDE = [
    "工作流", "workflow", "流程", "交付", "deliverable", "输出", "output",
    "技术交付", "示例", "example", "模板", "template",
]

# 部门目录里混放的说明文档，不是岗位，跳过
SKIP_FILES = {
    "readme.md", "readme.zh-tw.md", "catalog.md", "index.md",
    "quickstart.md", "executive-brief.md", "changelog.md",
    "contributing.md", "license.md",
}

FRONTMATTER_RE = re.compile(r"^---\s*\n(.*?)\n---\s*\n", re.S)
HEADING_RE = re.compile(r"^##\s+", re.M)


def parse_frontmatter(text):
    """返回 (frontmatter dict, body)。无 frontmatter 时返回 ({}, 原文)。"""
    m = FRONTMATTER_RE.match(text)
    if not m:
        return {}, text
    fm = {}
    for line in m.group(1).splitlines():
        if ":" in line:
            k, v = line.split(":", 1)
            fm[k.strip()] = v.strip().strip('"').strip("'")
    return fm, text[m.end():]


def split_sections(body):
    """把正文切成 (prelude, [(header, content), ...])。"""
    matches = list(HEADING_RE.finditer(body))
    if not matches:
        return body.strip(), []
    prelude = body[:matches[0].start()].strip()
    sections = []
    for i, m in enumerate(matches):
        end = matches[i + 1].start() if i + 1 < len(matches) else len(body)
        header = body[m.start():body.index("\n", m.start())].strip() if "\n" in body[m.start():end] \
            else body[m.start():end].strip()
        content = body[body.index("\n", m.start()) + 1:end].rstrip() if "\n" in body[m.start():end] else ""
        sections.append((header, content))
    return prelude, sections


def is_soul_section(header):
    low = header.lower()
    for kw in AGENTS_OVERRIDE:
        if kw in low:
            return False
    for kw in SOUL_KEYWORDS:
        if kw.lower() in low:
            return True
    return False


def build_slug(dept_dir, filename):
    stem = os.path.splitext(filename)[0]
    if stem.startswith(dept_dir + "-"):
        return stem
    return f"{dept_dir}-{stem}"


def render_identity(name, description, emoji):
    lines = [f"# {name}", description or ""]
    meta = []
    if emoji:
        meta.append(f"- 标识：{emoji}")
    meta.append("- 来源：agency-agents-zh（MIT License）")
    return "\n".join(lines) + "\n\n" + "\n".join(meta) + "\n"


def render_agents(name, content):
    return (
        f"# {name} · 工作规范\n\n"
        "> 本角色包由 TrustMesh 岗位市场生成，内容源自开源项目 "
        "agency-agents-zh（MIT License）。\n"
        "> `SOUL.md` 定义你是谁，`AGENTS.md` 定义你怎么做、交付什么。\n\n"
        + content
        + "\n"
    )


def render_soul(name, content):
    return f"# {name} · 身份与行为准则\n\n{content}\n"


def convert_file(src_path, dept_dir, out_root, used_slugs):
    with open(src_path, "r", encoding="utf-8") as f:
        text = f.read()

    fm, body = parse_frontmatter(text)
    name = fm.get("name") or os.path.splitext(os.path.basename(src_path))[0]
    description = fm.get("description", "")
    emoji = fm.get("emoji", "")

    prelude, sections = split_sections(body)

    # 源文件自带的 H1 标题与角色包标题重复，去掉
    prelude = "\n".join(
        line for line in prelude.splitlines() if not line.startswith("# ")
    ).strip()

    soul_parts = [prelude] if prelude else []
    agents_parts = []
    for header, content in sections:
        block = f"{header}\n{content}\n" if content else f"{header}\n"
        if is_soul_section(header):
            soul_parts.append(block)
        else:
            agents_parts.append(block)

    # 兜底：某一边为空时把内容挪过去，避免生成空文件
    if not soul_parts and agents_parts:
        soul_parts.append(agents_parts.pop(0))
    if not agents_parts and soul_parts:
        agents_parts.append(soul_parts.pop())

    slug = build_slug(dept_dir, os.path.basename(src_path))
    if slug in used_slugs:
        base = slug
        n = 2
        while slug in used_slugs:
            slug = f"{base}-{n}"
            n += 1
    used_slugs.add(slug)

    out_dir = os.path.join(out_root, slug)
    os.makedirs(out_dir, exist_ok=True)

    with open(os.path.join(out_dir, "IDENTITY.md"), "w", encoding="utf-8", newline="\n") as f:
        f.write(render_identity(name, description, emoji))
    with open(os.path.join(out_dir, "SOUL.md"), "w", encoding="utf-8", newline="\n") as f:
        f.write(render_soul(name, "\n".join(soul_parts).strip()))
    with open(os.path.join(out_dir, "AGENTS.md"), "w", encoding="utf-8", newline="\n") as f:
        f.write(render_agents(name, "\n".join(agents_parts).strip()))

    return slug, name


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--src", required=True, help="agency-agents-zh 仓库根目录")
    ap.add_argument("--out", default="data/roles", help="输出目录（岗位市场 roles 目录）")
    ap.add_argument("--clean", action="store_true", help="生成前清空输出目录")
    args = ap.parse_args()

    if not os.path.isdir(args.src):
        print(f"源目录不存在: {args.src}", file=sys.stderr)
        return 1

    if args.clean and os.path.isdir(args.out):
        shutil.rmtree(args.out)
    os.makedirs(args.out, exist_ok=True)

    used_slugs = set()
    counter = Counter()
    skipped = []

    for dept_dir in AGENT_DIRS:
        dept_path = os.path.join(args.src, dept_dir)
        if not os.path.isdir(dept_path):
            print(f"[warn] 缺少目录: {dept_dir}")
            continue
        files = sorted(f for f in os.listdir(dept_path) if f.endswith(".md"))
        for filename in files:
            if filename.lower() in SKIP_FILES:
                skipped.append(f"{dept_dir}/{filename}")
                continue
            src_path = os.path.join(dept_path, filename)
            # 真正的智能体文件都带 name frontmatter；部门里混放的说明文档没有
            with open(src_path, "r", encoding="utf-8") as fh:
                head = fh.read(8192)
            fm, _ = parse_frontmatter(head)
            if not fm.get("name"):
                skipped.append(f"{dept_dir}/{filename}（无 name frontmatter）")
                continue
            slug, name = convert_file(src_path, dept_dir, args.out, used_slugs)
            counter[dept_dir] += 1
        print(f"  {dept_dir:<20} {counter[dept_dir]:>3} 个角色")

    print(f"\n共生成 {sum(counter.values())} 个角色 → {args.out}")
    if skipped:
        print(f"跳过 {len(skipped)} 个非角色文件: {', '.join(skipped)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
