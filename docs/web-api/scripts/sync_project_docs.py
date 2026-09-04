#!/usr/bin/env python3
"""Sync the repository's project documentation (docs/**, excluding the
docs site itself) into the VitePress source tree so the whole NeoRuntime
wiki renders on the docs site, always in sync by construction: the copy
happens at build time from the same commit.

Transformations applied while copying into srcDir `project/`:
- filenames and directories are lowercased (QUICK_START.md -> quick-start
  URLs stay stable)
- markdown links that point outside the copied page tree (repo files such
  as ../LICENSE, api/swagger.yaml, shell scripts...) are rewritten to
  github.com blob/tree URLs so nothing 404s on the site
- links between copied .md pages keep working unchanged (the relative
  structure is preserved)

Also generates .vitepress/generated/project-sidebar.json consumed by
.vitepress/config.mts, so new wiki pages appear in the sidebar
automatically. Titles come from the first `# heading` of each page.

Run from docs/web-api/ via `pnpm build` / `pnpm dev` (before vitepress).
"""
import json
import os
import re
import shutil
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
SITE = HERE.parent
DOCS_ROOT = SITE.parent                      # repo docs/
REPO_ROOT = DOCS_ROOT.parent                 # repo root
DEST = SITE / "project"
SIDEBAR_OUT = SITE / ".vitepress" / "generated" / "project-sidebar.json"
GITHUB = "https://github.com/camthink-ai/neoruntime"

EXCLUDED_PARTS = {"web-api", "api"}          # site itself + raw spec dir
EXCLUDED_FILES = {"readme.md"}               # repo docs index -> our sidebar instead

SECTION_TITLES = [
    ("getting-started", "Getting Started"),
    ("architecture", "Architecture"),
    ("services", "Services"),
    ("deployment", "Deployment & OS"),
    ("references", "References"),
    ("mcu-protocol", "Protocols"),
    ("benchmarks", "Benchmarks"),
    ("testing", "Testing"),
]
SECTION_TITLES_ZH = {
    "getting-started": "快速开始",
    "architecture": "系统架构",
    "services": "平台服务",
    "deployment": "部署与 OS",
    "references": "参考手册",
    "mcu-protocol": "通信协议",
    "benchmarks": "性能基准",
    "testing": "测试",
}
ROOT_FILES = {"open-source-split.md": ("Open Source Policy", "开源策略")}

LINK_RE = re.compile(r"(?<!\!)\[([^\]]*)\]\(([^)\s]+)([^)]*)\)")


def source_files():
    for f in sorted(DOCS_ROOT.rglob("*.md")):
        rel = f.relative_to(DOCS_ROOT)
        if rel.parts[0] in EXCLUDED_PARTS:
            continue
        if str(rel).lower() in EXCLUDED_FILES:
            continue
        yield f, rel


def lower_rel(rel: Path) -> Path:
    return Path(*[p.lower() for p in rel.parts])


def first_title(text: str, fallback: str) -> str:
    for line in text.splitlines():
        if line.startswith("# "):
            return line[2:].strip()
    return fallback


def github_url(repo_rel: Path) -> str:
    rel = str(repo_rel)
    base = f"{GITHUB}/{'tree' if repo_rel.is_dir() else 'blob'}/main/{rel}"
    return base


def wiki_page_link(href: str, src_file: Path):
    """If href targets a markdown page inside the copied wiki tree (with or
    without the .md extension — repo docs use both styles), return the site
    path of that page, e.g. /project/getting-started/build.md. Else None."""
    target = href.split("#", 1)[0]
    if not target:
        return None
    resolved = (src_file.parent / target).resolve()
    candidates = [resolved] if resolved.suffix == ".md" else [resolved.with_suffix(".md"), resolved]
    for cand in candidates:
        try:
            rel = cand.relative_to(DOCS_ROOT)
        except ValueError:
            continue
        if rel.parts[0] in EXCLUDED_PARTS or str(rel).lower() in EXCLUDED_FILES:
            continue
        if cand.is_file() and cand.suffix == ".md":
            lowered = lower_rel(rel).as_posix()
            if not lowered.endswith(".md"):
                lowered += ".md"
            anchor = href.split("#", 1)[1] if "#" in href else ""
            return f"/project/{lowered}" + (f"#{anchor}" if anchor else "")
    return None


def rewrite_links(text: str, src_file: Path, report: list) -> str:
    def repl(m):
        label, href, rest = m.group(1), m.group(2), m.group(3)
        if href.startswith(("http://", "https://", "#", "mailto:")):
            return m.group(0)
        page = wiki_page_link(href, src_file)
        if page:
            return f"[{label}]({page})"
        target = href.split("#", 1)[0]
        resolved = (src_file.parent / target).resolve() if target else None
        # anything else that lives in the repo -> link to GitHub
        if resolved is not None:
            try:
                rel_in_repo = resolved.relative_to(REPO_ROOT)
                if (REPO_ROOT / rel_in_repo).exists():
                    return f"[{label}]({github_url(rel_in_repo)}{rest})"
            except ValueError:
                pass
        report.append((str(src_file.relative_to(DOCS_ROOT)), href))
        return m.group(0)

    return LINK_RE.sub(repl, text)


def build_sidebar(pages):
    """pages: list of {rel (lowered posix), title} sorted by rel."""
    sections = {dirn: [] for dirn, _ in SECTION_TITLES}
    root_items = []
    for rel, title in pages:
        parts = rel.split("/")
        if parts[0] == "mcu_protocol":
            sections["mcu-protocol"].append((title, f"/project/{rel[:-3]}"))
            continue
        if len(parts) == 1:
            zh = ROOT_FILES.get(rel, (title, title))[1]
            root_items.append({"text": title, "link": f"/project/{rel[:-3]}", "_zh": zh})
            continue
        if parts[0] in sections and len(parts) == 2:
            sections[parts[0]].append((title, f"/project/{rel[:-3]}"))

    out, out_zh = [], []
    for dirn, title in SECTION_TITLES:
        items = sections[dirn]
        if not items:
            continue
        out.append({
            "text": title,
            "collapsed": True,
            "items": [{"text": t, "link": l} for t, l in items],
        })
        out_zh.append({
            "text": SECTION_TITLES_ZH[dirn],
            "collapsed": True,
            "items": [{"text": t, "link": l} for t, l in items],
        })
    for item in root_items:
        zh = item.pop("_zh")
        out.append(item)
        out_zh.append({"text": zh, "link": item["link"]})
    return out, out_zh


def main():
    if not DOCS_ROOT.is_dir():
        sys.exit(f"docs root not found: {DOCS_ROOT}")

    shutil.rmtree(DEST, ignore_errors=True)
    DEST.mkdir(parents=True)

    pages, report = [], []
    for src, rel in source_files():
        dest_rel = lower_rel(rel)
        dest = DEST / dest_rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        text = src.read_text(encoding="utf-8", errors="replace")
        text = rewrite_links(text, src, report)
        dest.write_text(text, encoding="utf-8")
        pages.append((dest_rel.as_posix(), first_title(text, dest.stem.replace("-", " ").title())))

    pages.sort()
    sidebar_en, sidebar_zh = build_sidebar(pages)

    SIDEBAR_OUT.parent.mkdir(parents=True, exist_ok=True)
    SIDEBAR_OUT.write_text(
        json.dumps({"en": sidebar_en, "zh": sidebar_zh}, ensure_ascii=False, indent=1),
        encoding="utf-8",
    )

    print(f"synced {len(pages)} project docs -> project/ (sidebar sections: {len(sidebar_en)})")
    if report:
        print(f"NOTE: {len(report)} link(s) not resolvable to repo paths were left as-is:")
        for src, href in report[:10]:
            print(f"  {src}: {href}")


if __name__ == "__main__":
    main()
