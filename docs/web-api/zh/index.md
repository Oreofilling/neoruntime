---
layout: home

hero:
  name: NeoRuntime Web API
  text: 面向边缘 AI 设备的 REST · WebSocket · SSE 接口
  tagline: NeoRuntime 平台 API 双语参考 —— 模型管理、事件总线、设备控制、应用生命周期、媒体管线与流媒体，统一收敛在一个带认证的 HTTP 接口层。
  actions:
    - theme: brand
      text: 快速开始
      link: /zh/introduction
    - theme: alt
      text: API 参考
      link: /api-reference/zh/
    - theme: alt
      text: English
      link: /
features:
  - icon: 🔐
    title: JWT Bearer 认证
    details: 一个登录接口签发 Bearer 令牌；所有写接口共享同一套 400/401 错误信封与 1000～10001 的业务错误码。
  - icon: 🤖
    title: AI 运行时
    details: 注册、上传、解析并加载 HEF/ONNX/TFLite 模型，查看使用模型的应用，读取 NPU/AI 统计。
  - icon: 📡
    title: 事件与流媒体
    details: 基于 WebSocket 的发布/订阅事件总线、零延迟 H264 视频、SSE 陀螺仪姿态、日志流与双向语音对讲。
  - icon: 🎛️
    title: 完整设备控制
    details: 云台、镜头变焦/对焦/光圈与单次自动对焦、GPIO、红外/白光、风扇/加热器/雷达、RS485、告警输出与隐私遮蔽。
  - icon: 📦
    title: 应用与容器
    details: 应用商店安装、向导式部署、containerd 容器与镜像管理、实时日志与交互式控制台。
  - icon: 🔄
    title: 与代码保持同步
    details: 本站渲染的 spec 由 CI 与源码中的 gin 路由强校验；任何 API 变更都必须在同一 PR 内带上 spec 更新与中文翻译。
---
