#!/usr/bin/env python3
"""Sync the repository's project documentation (docs/**, excluding the
docs site itself) into the Astro/Starlight content collection so the whole
NeoRuntime wiki renders on the docs site, always in sync by construction:
the copy happens at build time from the same commit.

Transformations applied while copying into src/content/docs/project/:
- filenames and directories are lowercased; mcu_protocol becomes
  mcu-protocol (stable, clean URLs)
- a Starlight frontmatter block is injected: `title` from the first
  `# heading` (which is then removed — Starlight renders its own h1) and a
  `sidebar.order` for curated ordering inside a directory
- markdown links that point outside the copied page tree (repo files such
  as ../LICENSE, api/swagger.yaml, shell scripts...) are rewritten to
  github.com blob/tree URLs so nothing 404s on the site
- links between copied wiki pages are rewritten to base-prefixed site URLs
  (/neoruntime/project/.../)

The sidebar picks these pages up automatically via the `autogenerate`
entry for directory `project` in astro.config.mjs — new wiki pages appear
without any config change.

Run from docs/web-api/ via `pnpm build` / `pnpm dev` (before astro).
"""
import re
import shutil
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
SITE = HERE.parent
DOCS_ROOT = SITE.parent                      # repo docs/
REPO_ROOT = DOCS_ROOT.parent                 # repo root
DEST = SITE / "src" / "content" / "docs" / "project"
GITHUB = "https://github.com/camthink-ai/neoruntime"
BASE = "/neoruntime"

EXCLUDED_PARTS = {"web-api", "api"}          # site itself + raw spec dir
EXCLUDED_FILES = {"readme.md"}               # repo docs index -> our sidebar instead

DIR_MAP = {"mcu_protocol": "mcu-protocol"}

# Curated ordering inside each directory (lower ranks first); files not
# listed keep alphabetical order after ordered ones via frontmatter order.
ORDER = {
    "getting-started": {"quick_start.md": 1, "build.md": 2, "mvp_guide.md": 3, "windows_setup.md": 4},
    "architecture": {"readme.md": 1, "hal_v2_overview.md": 2, "security-architecture.md": 3},
    "services": {
        "platform-api.md": 1, "web-console.md": 2, "ai-runtime.md": 3,
        "app-manager.md": 4, "device-control.md": 5, "event-bus.md": 6,
        "media-streaming.md": 7, "device-discovery.md": 8, "camera_daemon_design.md": 9,
    },
    "deployment": {
        "deployment.md": 1, "yocto_deployment.md": 2, "os-upgrade.md": 3,
        "os-image-aipc-restore-design.md": 4, "baseboard-mcu-rtc-ota.md": 5,
    },
    "references": {
        "config-reference.md": 1, "cli-guide.md": 2, "faq.md": 3,
        "hal-v2-api-reference.md": 4, "systemd-services.md": 5,
        "troubleshooting.md": 6, "web-troubleshooting.md": 7,
        "gyro-attitude-sse.md": 8, "seccomp_profile_explanation.md": 9,
        "ct_disc_protocol.md": 10,
    },
    "benchmarks": {
        "ai-model-benchmark-hailo15h.md": 1, "ai-runtime-performance-params.md": 2,
        "npu-parallelism-benchmark.md": 3, "video-decode-capability-assessment.md": 4,
    },
    "testing": {"hal_lens_af0832_usage.md": 1},
}

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
    return Path(*(DIR_MAP.get(p, p).lower() for p in rel.parts))


def wiki_page_link(href: str, src_file: Path):
    """If href targets a markdown page inside the copied wiki tree (with or
    without the .md extension — repo docs use both styles), return the
    base-prefixed site URL, e.g. /neoruntime/project/x/y/. Else None."""
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
            url = lower_rel(rel).as_posix()[:-3]
            anchor = href.split("#", 1)[1] if "#" in href else ""
            return f"{BASE}/project/{url}/" + (f"#{anchor}" if anchor else "")
    return None


def github_url(repo_rel: Path) -> str:
    rel = str(repo_rel)
    kind = "tree" if (REPO_ROOT / repo_rel).is_dir() else "blob"
    return f"{GITHUB}/{kind}/main/{rel}"


def rewrite_links(text: str, src_file: Path, report: list) -> str:
    def repl(m):
        label, href, rest = m.group(1), m.group(2), m.group(3)
        if href.startswith(("http://", "https://", "#", "mailto:")):
            return m.group(0)
        page = wiki_page_link(href, src_file)
        if page:
            return f"[{label}]({page})"
        target = href.split("#", 1)[0]
        if not target:
            return m.group(0)
        resolved = (src_file.parent / target).resolve()
        try:
            rel_in_repo = resolved.relative_to(REPO_ROOT)
            if (REPO_ROOT / rel_in_repo).exists():
                return f"[{label}]({github_url(rel_in_repo)}{rest})"
        except ValueError:
            pass
        report.append((str(src_file.relative_to(DOCS_ROOT)), href))
        return m.group(0)

    return LINK_RE.sub(repl, text)


def split_h1(text: str):
    """Return (title, body_without_h1). Fails loudly on unexpected
    frontmatter so the sync never silently mis-renders a page."""
    if text.startswith("---"):
        sys.exit("unexpected frontmatter in a wiki source file — extend this script")
    lines = text.splitlines(keepends=True)
    for i, line in enumerate(lines):
        if line.startswith("# "):
            title = line[2:].strip()
            body = "".join(lines[i + 1:]).lstrip("\n")
            return title, body
    return None, text


def main():
    if not DOCS_ROOT.is_dir():
        sys.exit(f"docs root not found: {DOCS_ROOT}")

    shutil.rmtree(DEST, ignore_errors=True)
    DEST.mkdir(parents=True)

    report = []
    count = 0
    for src, rel in source_files():
        dest_rel = lower_rel(rel)
        dest = DEST / dest_rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        text = src.read_text(encoding="utf-8", errors="replace")
        text = rewrite_links(text, src, report)
        title, body = split_h1(text)
        front = [f"title: {json_str(title or dest.stem)}"]
        order = ORDER.get(dest_rel.parts[0], {}).get(dest_rel.parts[-1])
        if order is not None:
            front.append("sidebar:")
            front.append(f"  order: {order}")
        dest.write_text("---\n" + "\n".join(front) + "\n---\n\n" + body, encoding="utf-8")
        count += 1

    print(f"synced {count} project docs -> src/content/docs/project/")
    if report:
        print(f"NOTE: {len(report)} link(s) not resolvable to repo paths were left as-is:")
        for src, href in report[:10]:
            print(f"  {src}: {href}")


def json_str(s: str) -> str:
    # YAML double-quoted scalar with escaping — titles may contain quotes/colons.
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


if __name__ == "__main__":
    main()
