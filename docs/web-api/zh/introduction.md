# 简介

NeoRuntime 在每台设备上暴露统一的 **HTTP/JSON Web API**。内置 Web 控制台使用的就是这套接口；你也可以在自己的服务、脚本或移动应用中直接调用，管理 AI 模型、应用、媒体管线与硬件外设。

## 基础地址

API 统一版本化在 `/api/v1` 下。能访问哪个基础地址取决于客户端所在位置：

| 基础地址 | 适用场景 | 说明 |
| --- | --- | --- |
| `https://{host}/api/v1` | 任意位置（生产环境） | nginx TLS 网关，443 端口。证书为**自签名**——客户端需关闭证书校验（`curl -k`、`verify=False` 等）。这是远程机器唯一可达的基础地址。 |
| `/api/v1` | 浏览器同源 | 相对于页面所在地址解析，无 CORS 问题。 |
| `http://localhost:8080/api/v1` | 仅设备本机 | `platform-api` 绑定在 `127.0.0.1:8080`，该端口不可远程访问。适合 SSH 到设备后本地调试。 |

WebSocket 端点遵循同样的规则：远程用 `wss://{host}/api/v1/...`，设备本机用 `ws://localhost:8080/api/v1/...`。

## 功能分组

API 按功能划分为以下分组（对应 [API 参考](/zh/api-reference) 中的标签）：

- **系统** —— 平台信息、健康检查、重启、OTA 固件升级、OS（A/B）升级、时间/NTP 配置。
- **AI 运行时** —— 模型注册/上传/解析、加载/卸载、使用模型的应用、AI 统计。
- **事件总线** —— 主题、发布、WebSocket 订阅流。
- **设备控制** —— 云台、变焦/对焦/光圈与单次自动对焦、GPIO、IR-CUT、白光/红外灯、风扇、加热器、雷达、RS485、告警/韦根输出、硬件能力。
- **应用管理** —— 应用安装/启动/停止/重启、权限、日志、向导安装、异步打包安装。
- **视频流** —— 基于 WebSocket 的零延迟 H264 视频。
- **媒体** —— 摄像头守护进程配置、ISP 图像参数、编码器设置、OSD、隐私遮蔽、管线 profile、动态增删流、音频采集/播放/对讲。
- **监控** —— CPU/内存/磁盘/网络、单次快照、SSE 陀螺仪姿态。
- **进程 / 文件 / 终端 / SSH / 日志** —— 进程查看与发信号、远程文件管理、网页终端、sshd 配置、系统与服务日志（含 WebSocket 实时日志流）。
- **容器 / 镜像** —— containerd 容器与镜像生命周期。
- **存储** —— 磁盘、挂载/卸载、格式化。
- **事件日志 / 调试日志** —— 带 en/zh 本地化模板的结构化事件历史、tar.gz 调试日志导出。
- **网络 / 设备信息 / 设置 / 商店 / 开发** —— 网络接口配置、设备命名与出厂信息、动态键值设置、应用商店、浏览器内开发工作台。

## 约定

- 所有响应共享一个信封：`{code, message, data, error}`。`code === 0` 表示成功，其余为业务错误码——见[错误与状态码](/zh/errors)。
- 除特别说明的端点（健康检查、公钥、登录、OTA 状态）外，所有端点都要求 Bearer 令牌——见[认证](/zh/authentication)。
- 路径使用 `snake_case`；本文档中的路径参数（如 `{model_id}`）对应 gin 源码里的 `:model_id`。

## OpenAPI 规范

本站渲染的 OpenAPI 3.0 规范位于仓库中的
[`docs/api/swagger.yaml`](https://github.com/camthink-ai/neoruntime/blob/main/docs/api/swagger.yaml)。
CI 门禁（`scripts/check_swagger_sync.py`）校验该规范与
`platform/platform-api/server/main.go` 中注册的路由完全一致，
因此你看到的参考文档不会与代码漂移。站点如何重建、翻译如何保持完整，
见[文档同步机制](/zh/update-mechanism)。
