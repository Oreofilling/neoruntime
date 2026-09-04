# NeoRuntime Web API Documentation Site

Bilingual (English default, 简体中文) documentation for the NeoRuntime platform
Web API, rendered with **VitePress** (guides) + **Redoc** (full OpenAPI
reference) and deployed to GitHub Pages.

- English (default): `https://<site>/`
- 中文: `https://<site>/zh/`

## How it stays in sync with the code

The site never hand-copies API details. It renders the repository's master
OpenAPI spec `docs/api/swagger.yaml`, which is itself CI-gated against the gin
routes in `platform/platform-api/server/main.go` (`scripts/check_swagger_sync.py`).
On top of that:

1. `docs/web-api/scripts/build_specs.py` regenerates `public/swagger.json`
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
pnpm dev          # regenerate specs + vitepress dev server (hot reload)
pnpm build        # regenerate specs + production build into .vitepress/dist
pnpm preview      # serve the production build locally
```

The generated spec files under `public/` are gitignored build artifacts.

## Adding a translation

Edit `i18n/zh.yaml`. Per operation (`paths.<path>.<method>`) the keys
`summary` and `description` are **required**; optional keys translate
parameter descriptions (`parameters.<name>.description`), response
descriptions (`responses.<status>.description`) and `requestBody.description`.
New functional groups need a `tags.<TagName>` entry with `name` + `description`.
Untranslated strings fall back to English at build time.
