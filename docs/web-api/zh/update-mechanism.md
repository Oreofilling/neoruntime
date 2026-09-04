# 文档同步机制

本站由单一事实来源生成，不会与代码静默漂移。这依靠两道门禁和一次自动部署。

## 更新链路

```
platform/platform-api/server/main.go 中的路由变更
        │
        ▼  CI 门禁 1 — swagger-sync  (scripts/check_swagger_sync.py)
同一 PR 内必须更新 docs/api/swagger.yaml
        │
        ▼  CI 门禁 2 — docs-site     (docs/web-api/scripts/build_specs.py)
同一 PR 内必须补齐 docs/web-api/i18n/zh.yaml 的中文翻译
        │
        ▼  合并到 main
GitHub Action docs-pages.yml 构建站点（VitePress + Redoc）
        │
        ▼
GitHub Pages 自动重新部署
```

### 门禁 1 —— 规范与代码一致

`scripts/check_swagger_sync.py` 解析 `platform/platform-api/server/main.go` 中的 gin 路由注册，与 `docs/api/swagger.yaml` 中记录的路径和方法逐一比对。新增路由没有配套规范条目、或规范条目对应的路由已被删除，CI 即失败。该门禁在文档站之前就已存在；文档站只是渲染它的产出。

### 门禁 2 —— 每个操作都有翻译

`docs/web-api/scripts/build_specs.py` 校验中文 overlay `docs/web-api/i18n/zh.yaml` 覆盖主规范中**每个**操作的 `summary` 与 `description`，且 overlay 中不存在已删除操作的残留条目。翻译缺失或过期都会让 CI 失败并列出具体路径，新端点不可能不带中文翻译合入 `main`。

### 构建与部署

每当 `main` 上有触及 API 面、文档或工作流本身的推送，`docs-pages.yml` 会重新生成双语规范 JSON、构建 VitePress 站点并发布到 GitHub Pages。没有任何手工拷贝：`/api-reference/` 全屏参考页直接渲染 `swagger.json`（英文）与 overlay 合并生成的 `swagger.zh.json`（中文）（未翻译的字符串自动回退英文）。

## 修改 API 的检查清单

1. 在 `platform/platform-api/server/main.go` 中注册/注销路由并实现 handler。
2. 同一 PR 内更新 `docs/api/swagger.yaml`——路径、方法、参数、请求/响应 schema、示例，尽量写到与代码一样细。
3. 在 `docs/web-api/i18n/zh.yaml` 的 `paths:` 下新增（或删除）对应条目——`summary` 与 `description` 必填；可选键支持翻译参数描述、响应描述与示例：

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

4. 新增了标签？在 `tags:` 下补充翻译后的 `name` 与 `description`。
5. 提交 PR——CI 会跑两道门禁和站点构建；合并后 Pages 自动重新部署。

## 本地运行站点

```bash
cd docs/web-api
pnpm install
pnpm dev        # 重新生成规范 + 启动带热更新的 VitePress
```

生成的规范文件（`public/swagger*.json`）是构建产物——已加入 .gitignore，始终从 `docs/api/swagger.yaml` + `i18n/zh.yaml` 重新生成。

## 仓库布局

| 路径 | 职责 |
| --- | --- |
| `docs/`（wiki） | 项目文档树——每次构建由 `scripts/sync_project_docs.py` 同步到站点 `/project/...`；在那里修改文件，下次合并后站点自动更新。 |
| `docs/api/swagger.yaml` | 主 OpenAPI 规范（事实来源，英文）。 |
| `docs/web-api/i18n/zh.yaml` | 构建时合并到主规范的中文 overlay。 |
| `docs/web-api/scripts/build_specs.py` | 校验翻译覆盖率并生成 `public/swagger.json` / `public/swagger.zh.json`。 |
| `docs/web-api/.vitepress/` | 站点配置与主题。 |
| `docs/web-api/public/api-reference/` | 独立全屏 Redoc 参考页（中英），跟随系统深浅色。 |
| `.github/workflows/docs-pages.yml` | 合并到 `main` 后构建并部署站点到 GitHub Pages。 |
