---
layout: home

hero:
  name: NeoRuntime
  text: 边缘 AI 计算平台
  tagline: NeoRuntime 文档中心 —— 面向智能相机与边缘设备的边缘 AI 平台，包含硬件抽象层、平台服务、Web 控制台与多 SoC 支持。项目 wiki 与完整的 Web API 参考都在这里。
  actions:
    - theme: brand
      text: 快速开始
      link: /project/getting-started/quick_start
    - theme: alt
      text: Web API 参考
      link: /api-reference/zh/
    - theme: alt
      text: English
      link: /

features:
  - icon: 📖
    title: 项目 Wiki
    details: 快速开始、构建指南、系统架构、服务设计文档、部署与 OS 升级、通信协议与性能基准——每次构建自动与仓库同步（英文原文）。
    link: /project/architecture/readme
    linkText: 浏览 wiki
  - icon: 🔌
    title: Web API
    details: 一个带认证的 HTTP 接口面——23 个分组共 232 个操作，覆盖 AI 运行时、设备控制、媒体管线、应用与流媒体。中英双语，由 OpenAPI 规范渲染。
    link: /zh/introduction
    linkText: API 指南
  - icon: 🤖
    title: AI 运行时
    details: 在 NPU 上注册、解析并加载 HEF/ONNX/TFLite 模型，跟踪应用使用情况与推理统计。
    link: /project/services/ai-runtime
    linkText: 服务设计
  - icon: 🎛️
    title: 设备控制与媒体
    details: 云台、变焦/对焦/光圈与单次自动对焦、GPIO、红外/补光，以及 ISP 调参、编码管线、OSD 与音频。
    link: /project/services/device-control
    linkText: 服务设计
  - icon: 📦
    title: 应用与容器
    details: 应用商店、向导式安装、containerd 容器生命周期管理、实时日志与交互式控制台。
    link: /project/services/app-manager
    linkText: 服务设计
  - icon: 🔄
    title: 不会漂移的文档
    details: CI 门禁保证 OpenAPI 规范与 gin 路由一致、中文翻译完整；每次合并到 main 自动重新部署本站。
    link: /zh/update-mechanism
    linkText: 工作原理
---
