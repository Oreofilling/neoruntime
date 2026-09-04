# NeoRuntime Documentation Site

The project documentation hub for NeoRuntime, built with **Astro Starlight**
and deployed to GitHub Pages:

- **Project wiki** — everything under the repository's `docs/` tree
  (getting started, architecture, services, deployment, references,
  benchmarks) is synced into the site at build time by
  `scripts/sync_project_docs.py`, so it can never go stale. Wiki pages
  live under `/project/...` and are English (the source documents).
- **Web API guides** — bilingual (English default, 简体中文 at `/zh/`).
- **Web API reference** — a standalone full-page Redoc instance at
  `/api-reference/` (`/api-reference/zh/` in Chinese), rendered from the
  OpenAPI spec and following the system colour scheme.

The site lives in `docs/web-api/` for historical reasons; it hosts the
whole documentation site, not only the Web API part.

## How it stays in sync with the code

The site never hand-copies API details. It renders the repository's master
OpenAPI spec `docs/api/swagger.yaml`, which is itself CI-gated against the
gin routes in `platform/platform-api/server/main.go` (`scripts/check_swagger_sync.py`).
On top of that:

1. `scripts/build_specs.py` regenerates `public/swagger.json`
   (English) and `public/swagger.zh.json` (Chinese) from the master spec +
   the translation overlay `i18n/zh.yaml`.
2. The same script **fails CI** if any operation lacks a Chinese
   summary/description, or if the overlay contains stale entries — an API
   change must land with its translation in the same PR.
3. `.github/workflows/docs-pages.yml` rebuilds and redeploys GitHub Pages on
   every merge to `main` that touches the API surface or docs.

See the rendered explanation: *Keeping Docs in Sync* / *文档同步机制*.

## Local development

Requirements: Node 20+, pnpm 10, Python 3 with PyYAML.

```bash
cd docs/web-api
pnpm install
pnpm dev          # regenerate specs + wiki, start the Starlight dev server
pnpm build        # regenerate everything + production build into dist/
pnpm preview      # serve the production build locally
```

Generated artifacts (`public/swagger*.json`, `public/redoc.standalone.js`,
`src/content/docs/project/`) are gitignored and always rebuilt from source.

## Adding a translation

Edit `i18n/zh.yaml`. Per operation (`paths.<path>.<method>`) the keys
`summary` and `description` are **required**; optional keys translate
parameter descriptions (`parameters.<name>.description`), response
descriptions (`responses.<status>.description`) and `requestBody.description`.
New functional groups need a `tags.<TagName>` entry with `name` + `description`.
Untranslated strings fall back to English at build time.
