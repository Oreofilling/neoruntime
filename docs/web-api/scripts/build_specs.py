#!/usr/bin/env python3
"""Build the API spec assets consumed by the docs site, and gate on
Chinese translation coverage.

What it does
------------
1. Reads the single source of truth ``docs/api/swagger.yaml`` (kept in
   sync with the gin routes in platform/platform-api/server/main.go by
   scripts/check_swagger_sync.py) and writes it as ``public/swagger.json``
   for the English Redoc page.
2. Reads the Chinese overlay ``i18n/zh.yaml`` and deep-merges it onto a
   copy of the master spec, producing ``public/swagger.zh.json`` for the
   Chinese Redoc page. Untranslated strings simply fall back to English.
3. FAILS (exit 1) when the overlay is out of sync with the master spec:
   - an operation (path + method) missing a zh ``summary`` or ``description``
   - an overlay entry for a path/method that no longer exists in the master
   - an overlay tag for a tag that no longer exists (or a master tag
     without an overlay entry)

This makes the docs update chain enforceable in CI:

    route change -> swagger.yaml updated (check_swagger_sync.py)
                 -> i18n/zh.yaml updated (this script)
                 -> merge to main -> GitHub Pages redeploy (docs-pages.yml)

Run from docs/web-api/ (or anywhere; paths resolve from this file).
"""
import json
import os
import sys

import yaml

HERE = os.path.dirname(os.path.abspath(__file__))
MASTER = os.path.join(HERE, "..", "..", "api", "swagger.yaml")
OVERLAY = os.path.join(HERE, "..", "i18n", "zh.yaml")
PUBLIC = os.path.join(HERE, "..", "public")
HTTP = ("get", "post", "put", "delete", "patch", "head", "options")


def load_yaml(path):
    with open(path, encoding="utf-8") as f:
        return yaml.safe_load(f)


def iter_ops(spec):
    for path, item in (spec.get("paths") or {}).items():
        for method, op in item.items():
            if method in HTTP and isinstance(op, dict):
                yield path, method, op


def validate_overlay(master, overlay):
    """Return a list of human-readable problems; empty means in sync."""
    problems = []
    zh_paths = overlay.get("paths") or {}
    zh_tags = overlay.get("tags") or {}

    master_tags = {t["name"] for t in master.get("tags") or []}
    for name in sorted(master_tags - set(zh_tags)):
        problems.append(f"tag {name!r} has no zh entry (i18n/zh.yaml: tags)")
    for name in sorted(set(zh_tags) - master_tags):
        problems.append(f"zh tag {name!r} not in swagger.yaml anymore (stale entry)")

    for path, method, op in iter_ops(master):
        entry = (zh_paths.get(path) or {}).get(method)
        if entry is None:
            problems.append(f"zh missing: {method.upper():7s} {path} (no entry)")
            continue
        for field in ("summary", "description"):
            if not str(entry.get(field) or "").strip():
                problems.append(
                    f"zh missing: {method.upper():7s} {path} (empty {field})"
                )

    master_paths = master.get("paths") or {}
    master_ops = {(p, m) for p, m, _ in iter_ops(master)}
    for path, item in zh_paths.items():
        if path not in master_paths:
            problems.append(f"zh stale: path {path!r} not in swagger.yaml anymore")
            continue
        for method in item:
            if method not in HTTP:
                problems.append(f"zh stale: {path}: unsupported key {method!r}")
            elif (path, method) not in master_ops:
                problems.append(
                    f"zh stale: {method.upper():7s} {path} not in swagger.yaml anymore"
                )
    return problems


def apply_overlay(master, overlay):
    """Return a zh copy of the spec with overlay strings merged in.

    Within an operation the overlay may set: summary, description,
    parameters (dict keyed by parameter name -> {description, example}),
    responses (dict keyed by status code -> {description}) and
    requestBody.description. Anything not covered falls back to English.
    """
    zh = json.loads(json.dumps(master))  # deep copy, yaml -> json-safe

    info = overlay.get("info") or {}
    if info.get("title"):
        zh["info"]["title"] = info["title"]
    if info.get("description"):
        zh["info"]["description"] = info["description"]

    name_map = {}
    for i, tag in enumerate(zh.get("tags") or []):
        entry = (overlay.get("tags") or {}).get(tag["name"]) or {}
        if entry.get("name"):
            name_map[tag["name"]] = entry["name"]
            tag["name"] = entry["name"]
        if entry.get("description"):
            tag["description"] = entry["description"]

    def remap_tags(holder):
        if isinstance(holder.get("tags"), list):
            holder["tags"] = [name_map.get(t, t) if isinstance(t, str) else t for t in holder["tags"]]

    zh_paths = overlay.get("paths") or {}
    for path, item in zh.get("paths", {}).items():
        remap_tags(item)
        for method, op in item.items():
            if method not in HTTP or not isinstance(op, dict):
                continue
            remap_tags(op)
            entry = zh_paths.get(path, {}).get(method) or {}
            for field in ("summary", "description"):
                if entry.get(field):
                    op[field] = entry[field]

            for p in op.get("parameters") or []:
                pv = (entry.get("parameters") or {}).get(p.get("name"))
                if not pv:
                    continue
                if pv.get("description"):
                    p["description"] = pv["description"]
                if "example" in pv:
                    p["example"] = pv["example"]

            for code, resp in (op.get("responses") or {}).items():
                rv = (entry.get("responses") or {}).get(str(code))
                if rv and rv.get("description"):
                    resp["description"] = rv["description"]

            rb = op.get("requestBody")
            rbv = entry.get("requestBody")
            if rb and rbv and rbv.get("description"):
                rb["description"] = rbv["description"]
    return zh


def main():
    if not os.path.isfile(MASTER):
        sys.exit(f"master spec not found: {os.path.abspath(MASTER)}")
    master = load_yaml(MASTER)
    overlay = load_yaml(OVERLAY) if os.path.isfile(OVERLAY) else {}

    problems = validate_overlay(master, overlay)
    for p in problems:
        print(p)
    if problems:
        print(
            f"\ni18n/zh.yaml out of sync with docs/api/swagger.yaml: "
            f"{len(problems)} problem(s) — add/fix the zh translations above "
            "in the same PR as the swagger change "
            "(docs/web-api/i18n/zh.yaml)",
            file=sys.stderr,
        )
        sys.exit(1)

    os.makedirs(PUBLIC, exist_ok=True)
    with open(os.path.join(PUBLIC, "swagger.json"), "w", encoding="utf-8") as f:
        json.dump(master, f, ensure_ascii=False, indent=1)

    zh = apply_overlay(master, overlay)
    with open(os.path.join(PUBLIC, "swagger.zh.json"), "w", encoding="utf-8") as f:
        json.dump(zh, f, ensure_ascii=False, indent=1)

    n = sum(1 for _ in iter_ops(master))
    print(f"specs generated: swagger.json + swagger.zh.json ({n} operations, zh 100% covered)")


if __name__ == "__main__":
    main()
