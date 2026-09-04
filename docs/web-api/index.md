---
layout: home

hero:
  name: NeoRuntime
  text: Edge AI Computing Platform
  tagline: Documentation hub for NeoRuntime — an edge AI platform for smart cameras and edge devices, with a hardware abstraction layer, platform services, a web console and multi-SoC support. The project wiki and the full Web API reference live here.
  actions:
    - theme: brand
      text: Get Started
      link: /project/getting-started/quick_start
    - theme: alt
      text: Web API Reference
      link: /api-reference/
    - theme: alt
      text: 简体中文
      link: /zh/

features:
  - icon: 📖
    title: Project Wiki
    details: Getting started, build guides, architecture, service design documents, deployment & OS upgrade, protocols and benchmarks — synced from the repository on every build.
    link: /project/architecture/readme
    linkText: Browse the wiki
  - icon: 🔌
    title: Web API
    details: One authenticated HTTP surface for everything — 232 operations across 23 groups covering AI runtime, device control, media pipelines, apps and streaming. Bilingual, rendered from the OpenAPI spec.
    link: /introduction
    linkText: API guides
  - icon: 🤖
    title: AI Runtime
    details: Register, parse and load HEF/ONNX/TFLite models on the NPU, track per-app usage and inference statistics.
    link: /project/services/ai-runtime
    linkText: Service design
  - icon: 🎛️
    title: Device Control & Media
    details: PTZ, lens zoom/focus/iris with one-shot autofocus, GPIO, IR/lighting, plus ISP tuning, encoder pipelines, OSD and audio.
    link: /project/services/device-control
    linkText: Service design
  - icon: 📦
    title: Apps & Containers
    details: Application store, wizard-driven installs, containerd lifecycle management, live logs and interactive consoles.
    link: /project/services/app-manager
    linkText: Service design
  - icon: 🔄
    title: Docs That Cannot Drift
    details: CI gates keep the OpenAPI spec identical to the gin routes and the Chinese translations complete; every merge to main redeploys this site automatically.
    link: /update-mechanism
    linkText: How it works
---
