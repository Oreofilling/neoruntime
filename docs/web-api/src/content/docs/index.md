---
title: Home
template: splash
hero:
  tagline: "Documentation hub for NeoRuntime — an edge AI computing platform for smart cameras and edge devices: HAL, platform services, web console, multi-SoC support. Project wiki and the full bilingual Web API reference live here."
  actions:
    - text: Get Started
      link: /neoruntime/project/getting-started/quick_start/
      variant: primary
    - text: Web API Reference
      link: /neoruntime/api-reference/
      variant: secondary
    - text: 简体中文
      link: /neoruntime/zh/
      variant: minimal
headlines:
  - title: Project Wiki
    items:
      - title: Getting Started
        link: /neoruntime/project/getting-started/quick_start/
        description: Local startup, build targets, MVP bring-up and Windows setup.
      - title: Architecture
        link: /neoruntime/project/architecture/readme/
        description: System overview, HAL v2 design and the security model.
      - title: Platform Services
        link: /neoruntime/project/services/platform-api/
        description: AI runtime, device control, event bus, camera daemon, app manager and more.
      - title: Deployment & OS
        link: /neoruntime/project/deployment/deployment/
        description: On-device release, Yocto builds, A/B OS upgrade and factory tooling.
  - title: Web API
    items:
      - title: API Guides
        link: /neoruntime/introduction/
        description: Base URLs, conventions, authentication, error codes — bilingual.
      - title: Full Reference (232 operations)
        link: /neoruntime/api-reference/
        description: Standalone OpenAPI-rendered reference with search and examples.
      - title: Keeping Docs in Sync
        link: /neoruntime/update-mechanism/
        description: CI gates keep the spec identical to the routes and translations complete.
  - title: Project
    items:
      - title: GitHub Repository
        link: https://github.com/camthink-ai/neoruntime
        description: Source code, issues and releases.
      - title: OpenAPI Spec Source
        link: https://github.com/camthink-ai/neoruntime/blob/main/docs/api/swagger.yaml
        description: The single source of truth behind the API reference.
      - title: Benchmarks
        link: /neoruntime/project/benchmarks/ai-model-benchmark-hailo15h/
        description: AI model and NPU performance reports.
---
