---
layout: home

hero:
  name: NeoRuntime Web API
  text: REST · WebSocket · SSE for edge AI devices
  tagline: Bilingual reference for the NeoRuntime platform API — model management, event bus, device control, app lifecycle, media pipelines and streaming, all over one authenticated HTTP surface.
  actions:
    - theme: brand
      text: Get Started
      link: /introduction
    - theme: alt
      text: API Reference
      link: /api-reference/
    - theme: alt
      text: 简体中文
      link: /zh/

features:
  - icon: 🔐
    title: JWT Bearer authentication
    details: One login endpoint issues a bearer token; every write endpoint shares the same 400/401 error envelope with business error codes from 1000 to 10001.
  - icon: 🤖
    title: AI runtime
    details: Register, upload, parse and load HEF/ONNX/TFLite models, track apps using them, and read NPU/AI statistics.
  - icon: 📡
    title: Events & streaming
    details: Pub/sub event bus over WebSocket, zero-latency H264 video, SSE gyro attitude, log streaming and two-way audio talk.
  - icon: 🎛️
    title: Full device control
    details: PTZ, lens zoom/focus/iris with one-shot AF, GPIO, IR/white light, fan/heater/radar, RS485, alarm outputs and privacy masks.
  - icon: 📦
    title: Apps & containers
    details: App store installs, wizard-driven deployment, containerd container and image management, live logs and exec consoles.
  - icon: 🔄
    title: Stays in sync with the code
    details: The spec behind this site is CI-gated against the gin routes in the source; every API change must land with its spec and Chinese translation in the same PR.
---
