# Keeping Docs in Sync

This site is generated from a single source of truth and cannot silently drift from the code. Three gates and one deployment make that true.

## The update chain

```
route change in platform/platform-api/server/main.go
        │
        ▼  CI gate 1 — swagger-sync  (scripts/check_swagger_sync.py)
docs/api/swagger.yaml must be updated in the same PR
        │
        ▼  CI gate 2 — docs-site     (docs/web-api/scripts/build_specs.py)
docs/web-api/i18n/zh.yaml must translate every operation in the same PR
        │
        ▼  merge to main
GitHub Action docs-pages.yml builds the site (VitePress + Redoc)
        │
        ▼
GitHub Pages is redeployed automatically
```

### Gate 1 — spec matches the code

`scripts/check_swagger_sync.py` parses the gin route registrations in `platform/platform-api/server/main.go` and compares them, one by one, with the paths and methods documented in `docs/api/swagger.yaml`. A route that lands without its spec entry — or a spec entry whose route was removed — fails CI. This gate already existed before the docs site; the site simply renders its output.

### Gate 2 — every operation is translated

`docs/web-api/scripts/build_specs.py` verifies that the Chinese overlay `docs/web-api/i18n/zh.yaml` has a `summary` and a `description` for **every** operation in the master spec, and that the overlay contains no entries for operations that no longer exist. Missing or stale translations fail CI with the exact paths listed, so a new endpoint cannot reach `main` without its Chinese translation.

### Build & deploy

On every push to `main` that touches the API surface, the docs, or the workflow itself, `docs-pages.yml` regenerates the bilingual spec JSONs, builds the VitePress site and publishes it to GitHub Pages. Nothing is hand-copied: the English page renders `swagger.json` verbatim and the Chinese page renders `swagger.zh.json`, produced by merging the overlay onto the master spec (untranslated strings fall back to English).

## How to change the API (checklist)

1. Register/deregister the route in `platform/platform-api/server/main.go` and implement the handler.
2. Update `docs/api/swagger.yaml` in the same PR — path, method, parameters, request/response schemas, examples. Keep it as detailed as the code.
3. Add (or remove) the matching entry in `docs/web-api/i18n/zh.yaml` under `paths:` — `summary` and `description` are mandatory; optional keys let you translate parameter descriptions, response descriptions and examples:

   ```yaml
   paths:
     /example/new-endpoint:
       post:
         summary: 创建示例资源
         description: 创建一个示例资源并返回其 ID。
         parameters:
           verbose: { description: 返回详细信息 }
         responses:
           '201': { description: 创建成功 }
   ```

4. New tag? Add it under `tags:` with a translated `name` and `description`.
5. Open the PR — CI runs both gates and a docs-site build; once merged, Pages redeploys by itself.

## Running the site locally

```bash
cd docs/web-api
pnpm install
pnpm dev        # regenerate specs + start VitePress with live reload
```

The generated spec files (`public/swagger*.json`) are build artifacts — they are gitignored and always recreated from `docs/api/swagger.yaml` + `i18n/zh.yaml`.

## Repo layout

| Path | Role |
| --- | --- |
| `docs/api/swagger.yaml` | Master OpenAPI spec (source of truth, English). |
| `docs/web-api/i18n/zh.yaml` | Chinese overlay merged onto the master spec at build time. |
| `docs/web-api/scripts/build_specs.py` | Validates overlay coverage and generates `public/swagger.json` / `public/swagger.zh.json`. |
| `docs/web-api/.vitepress/` | Site configuration, theme, Redoc embed component. |
| `.github/workflows/docs-pages.yml` | Builds and deploys the site to GitHub Pages on merges to `main`. |
