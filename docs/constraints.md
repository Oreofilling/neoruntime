# NE503 (Hailo-15) 平台约束清单

> 基准：2026-09-24 工作区内容（含未提交改动），反映本地最新状态，不完全等同于上游 `develop`。
> 本文档是各领域工程约束的汇总入口；与旧文档冲突时，一律以当前代码、systemd unit 和实际配置解析逻辑为准。

## 0. 红线速览

最容易踩、后果最重的硬约束，详见对应章节：

| # | 约束 | 章节 |
|---|---|---|
| 1 | MCU 独占 `/dev/ttyS0 @ 921600`；RTC/OTA 初始化必须先于 camera-daemon、device-control 打开串口 | [§4](#4-硬件与-mcu) |
| 2 | Hailo medialib 的 dewarp DSP 优化必须保持关闭，否则约 20–30 分钟后 ISP/DSP 硬死、watchdog 循环重启 | [§4](#4-硬件与-mcu) |
| 3 | Hailo-15 正式发布构建必须使用 Hailo Yocto/Poky SDK 4.0.23；普通主机 ARM64 工具链不足以完成发布构建 | [§2](#2-构建与发布) |
| 4 | 默认发布严格锁定 OS `1.12.0`、machine `hailo15-ne503`、product `ne503`、data schema `1` | [§2](#2-构建与发布) |
| 5 | A/B 在线升级依赖固定五分区布局；旧三分区设备只能走 single-recovery | [§3](#3-部署与升级) |
| 6 | 模型仅接受 Hailo HEF：裸 `.hef`，或内含 HEF 的 AMPK v1 `.bin`；ONNX 等不接受 | [§6](#6-ai-与模型) |
| 7 | 编码流协议为 V3（38 字节头）；daemon 与 SDK 客户端必须同步部署，V2 客户端不兼容 | [§5](#5-视频缓冲区与-ipc) |
| 8 | 跨进程零拷贝只能用 `buffer_id` + 显式 lookup lease；`Tensor.dma_fd` 会被拒绝 | [§5](#5-视频缓冲区与-ipc) |
| 9 | MCU OTA 只在当前版本严格低于包版本时执行，不允许降级；失败不阻塞平台启动 | [§4](#4-硬件与-mcu) |
| 10 | 仓库自带固定默认凭据，且 OS 升级无真实签名校验；生产部署必须覆盖凭据并评估升级链路风险 | [§8](#8-安全与已知高风险项) |

## 1. 项目与平台边界

| 类型 | 约束 |
|---|---|
| 仓库范围 | 本仓库只负责平台服务、HAL、部署资源、MCU 固件和 Web 控制台；SDK 与示例应用在独立仓库（[README](../README.md#L7)） |
| 生产平台 | 实际实现的 HAL 后端只有 `hailo15` 和 `stub`；README 中的 RK3588、Jetson 仅为架构目标，不是现成可发布后端 |
| Stub 限制 | Stub 只适合单测、CI 和接口验证；功能、错误行为、性能和内存管理均可能与真机不同（[HAL Stub 说明](references/hal-v2-api-reference.md#L797)） |
| 运行架构 | Hailo-15 发布产物固定为 Linux ARM64；Go 服务无 CGO，C++/HAL 仍依赖厂商运行库（[Makefile](../Makefile#L59)） |
| 固定目录 | 设备可变数据与运行时安装根为 `/data/aipc`，临时 Unix Socket 位于 `/run/aipc`；大量服务、脚本和配置依赖这两个路径 |

## 2. 构建与发布

- **工具链最低要求**：Go 1.25、Node.js 24、pnpm 10、CMake 3.16、GCC/G++ 10、protoc 3.15、Python 3.8；HAL 使用 C++20（[构建要求](getting-started/BUILD.md#L57)）。
- **交叉编译**：Hailo-15 正式构建必须使用 Hailo Yocto/Poky SDK 4.0.23 及其 sysroot，发布与本地复现统一使用 Docker 镜像 `zerobot/ne503-dev-env-full:4.0.23`；普通主机 ARM64 编译器不足以完成完整发布构建（[交叉编译](getting-started/BUILD.md#L100)）。
- **`make pack-release`**（[Makefile](../Makefile#L319)）：
  - 强制使用 Hailo SDK；
  - 默认重新构建 MCU 固件，需要 `arm-none-eabi-gcc`；
  - 只有明确指定 `BUILD_MCU_FW=0` 才复用现有 MCU 产物；
  - Hailo 发布包要求随包携带 nginx ARM64 运行时。
- **默认发布兼容范围严格锁定**（[Makefile](../Makefile#L25)）：
  - OS：`1.12.0`，且 `min_os_version == max_os_version == 1.12.0`（严格 `x.y.z` 闭区间）；
  - machine：`hailo15-ne503`；
  - product：`ne503`；
  - data schema：1。
- **模型不入库**：仓库不跟踪模型权重；部署时需要另外下载或导入模型。

## 3. 部署与升级

- **应用包安装强校验**：安装前强制校验 machine、product、严格的 `x.y.z` OS 闭区间和数据 schema；不兼容的应用包不能安装（[兼容模型](deployment/os-upgrade.md#L85)）。
- **OS 升级是无条件升级**：
  - 应用不兼容不会阻止或回滚 OS；
  - 升级后仅保留 `platform-api` 作为救援通道；
  - 其余不兼容服务保持停止，由运维重新安装兼容应用。
- **A/B 在线升级要求固定分区布局**（[分区布局](deployment/os-upgrade.md#L263)）：
  - `mmcblk1p1/p2`：A；`mmcblk1p3/p4`：B；`mmcblk1p5`：持久化数据；
  - 旧版三分区设备只能走 single-recovery。
- **Bootloader 约束**：必须提供有限启动尝试次数；若新镜像无法启动到 systemd，用户态验证无法恢复（[升级启动约束](deployment/os-upgrade.md#L82)）。
- **资源要求**：OS 上传要求"包大小 + 2 GiB"可用磁盘空间，并至少 512 MiB 可用内存（[资源要求](deployment/os-upgrade.md#L32)）。
- **热部署**：在设备本机以 root 执行，围绕 `/data/aipc` 做备份、替换和回滚（[部署指南](deployment/DEPLOYMENT.md#L107)）。

## 4. 硬件与 MCU

- **串口独占与启动顺序**：MCU 通信独占 `/dev/ttyS0 @ 921600`；MCU RTC/OTA 准备必须在 camera-daemon 和 device-control 打开串口前完成，不能在运行中途启动（[MCU 启动顺序](deployment/baseboard-mcu-rtc-ota.md#L43)）。
- **RTC**：仅支持主机向 MCU 同步，且主机年份必须不小于 2024。
- **MCU OTA**（[MCU OTA](deployment/baseboard-mcu-rtc-ota.md#L95)）：
  - 只在当前版本严格低于包版本时升级，不允许降级；
  - 失败不会阻塞整个平台启动；
  - wrapper 默认 115 秒超时，并始终向 systemd 返回成功。
- **PTZ**：当前平台没有电动 PTZ，MCU 协议也没有 PTZ 指令，必须保持禁用（[设备能力](../configs/platform/device-control.yaml#L41)）。
- **GPIO**：GPIO API 当前没有可用引脚目录；MCU `0x31` 是 SoC 复位/掉电指令，不能当 GPIO 使用（[GPIO 约束](../configs/platform/device-control.yaml#L63)）。
- **FG2009 镜头**：仅支持相对焦距、变焦及 IR-cut（[FG2009 说明](../mcu_board_prj/FG2009_BRINGUP.md#L3)）：
  - 无 PI/home 反馈，不支持绝对移动、homing 和 iris；
  - 镜头类型选择不会持久化到 MCU，每次 MCU 重启都要重新选择；
  - 位置值只是相对诊断计数，断电后不能视为真实光学位置。
- **Dewarp DSP**：Hailo medialib 的 dewarp DSP 优化必须保持关闭；已知启用后约 20–30 分钟会导致 ISP/DSP 硬死和 watchdog 循环重启（[camera-daemon service](../systemd/camera-daemon.service#L29)）。

## 5. 视频、缓冲区与 IPC

- **流配置事实来源**：编码器配置是流配置的唯一事实来源；主码流不可关闭，仅 sub/third 可动态启停。默认流参数见 [camera-daemon.yaml](../configs/platform/camera-daemon.yaml#L169)。
- **原始帧分发**：通过 Unix Socket + `SCM_RIGHTS` 传递 DMA-BUF（[FD Publisher](../platform/camera-daemon/include/fd_publisher.h#L32)）：
  - 最多 16 个客户端；每客户端最多 3 个未释放帧；默认租约 200 ms；
  - 发生背压、超额或租约超时时直接丢帧，不阻塞摄像头线程。
- **租约纪律**：每个已接收的 frame/buffer lease 必须准确释放；客户端卡死或忘记释放会被限流，最终由 watchdog 回收。
- **编码流协议 V3**（[Encoded Publisher](../platform/camera-daemon/include/encoded_publisher.h#L8)）：
  - 38 字节头，携带 `packet_seq`；
  - 老 V2 客户端不兼容，daemon 与 SDK 客户端必须同步部署；
  - 发布队列上限 120；溢出后客户端跳过预测帧并等待下一个 IDR。
- **gRPC 消息上限**：AI Runtime 单条收发消息上限 64 MiB；大图像优先使用 DMA buffer ID，不应直接塞入 protobuf（[AI Runtime](../platform/ai-runtime/src/main.cpp#L169)）。
- **FD 不跨进程**：文件描述符数值不能跨进程通过 gRPC 使用；`Tensor.dma_fd` 会被拒绝，跨进程零拷贝必须使用 `buffer_id` 和显式 lookup lease。
- **超时语义**：超时不代表底层硬件操作已取消；厂商 DSP 操作一旦进入执行阶段无法中止，只能丢弃迟到结果并释放资源。

## 6. AI 与模型

- **仅支持 Hailo HEF**（[HEF 上传限制](../platform/platform-api/handlers/ai.go#L1109)）：
  - 直接上传 `.hef`；或 AMPK v1 `.bin`，内部仍必须包含 HEF；
  - ONNX 等其他格式不接受。
- **AMPK 包格式**（[模型包格式](../platform/platform-api/storage/modelpackage.go#L13)）：
  - magic 为 `AMPK`；当前只支持 version 1、flags 0；
  - JSON metadata 最大 1 MiB；使用 SHA-256 校验 JSON + HEF；
  - metadata 为封闭 schema，禁止携带任意动态库路径。
- **默认调度限制**（[AI 配置](../configs/ai/ai-runtime.yaml#L35)）：
  - 全局 100 QPS、并发 8；单会话默认 30 QPS、并发 2；
  - 队列 64，超时 5 秒；
  - StreamInfer 最多 16 个活动 RPC、单流 4 个订阅者；
  - 流式预处理默认关闭。
- **会话标识**：只允许 ASCII `[A-Za-z0-9._-]`，最多 128 字节；全局最多 1024 会话，每应用最多 128（[Session Manager](../platform/ai-runtime/include/session_manager.h#L42)）。
- **流式推理语义**：实时/latest-only；过早、过载或超过 FPS 限制的帧可能直接丢弃，不提供无限积压保证。
- **模型 preload 最小化**：Hailo network group/NPU 资源不会及时释放，加载过多重模型可能耗尽 NPU context（[模型预加载](../configs/ai/ai-runtime.yaml#L23)）。

## 7. 应用容器

- **运行时依赖**：containerd、`io.containerd.runc.v2` 和 overlayfs。
- **默认容器策略**（[App Manager 配置](../configs/platform/app-manager.yaml#L11)）：
  - 只读 rootfs、`no_new_privileges`、默认无网络；
  - 单容器：50% 单核 CPU、256 MiB 内存、128 PID；
  - 全局上限：2 CPU core、2 GiB 内存。
- **Manifest 校验**：必须严格使用 `apiVersion: v1`；kind 仅限 `Application`、`ModelService`、`BusinessService`；要求 ID、名称、版本及镜像（[Manifest 校验](../platform/app-manager/manifest/manifest.go#L339)）。
- **内置模型路径**：必须是规范化绝对路径并以 `.bin` 结尾；裸 `.hef` 已不允许作为应用内置模型（[模型映射](../platform/app-manager/manifest/manifest.go#L413)）。
- **上传限制**：应用镜像或 `.neoapp` 请求最大 2 GiB，并额外要求 1 GiB 磁盘余量；包内 `app.yaml` 最大 4 MiB（[上传限制](../platform/platform-api/handlers/app.go#L726)）。

## 8. 安全与已知高风险项

1. **Hailo-15 OS 升级没有真实签名校验**：当前 `AIPC_OS_REQUIRE_SIGNATURE=false`，只检查签名文件存在；CMS 校验和设备公钥尚未接通（[签名限制](deployment/os-upgrade.md#L191)）。
2. **仓库配置仍包含固定默认凭据**：`token_key=aipc-secure-token-secret`、`admin/password`；app-manager systemd 也固定使用相同 token。生产部署必须通过环境变量覆盖（[platform-api 配置](../configs/platform-api.yaml#L55)、[app-manager service](../systemd/app-manager.service#L9)）。
3. **API 网络边界**：Platform API 只在 `127.0.0.1:8080` 提供 HTTP，由 nginx 终止外部 HTTPS；本地 Unix Socket 按文件权限信任且免认证（[platform-api 配置](../configs/platform-api.yaml#L1)）。
4. **Session 不持久化**：登录 session 只保存在进程内存中，platform-api 重启会使已签发的 Web session 失效。
5. **开源发布许可检查**：开源前必须检查 MCU/boot/nginx 二进制的再分发许可；当前开源检查文档已落后于实际固件版本，且声称存在的 `SECURITY.md` 实际不存在（[开源检查表](OPEN_SOURCE_SPLIT.md#L38)）。

## 9. 尚未闭环的实现缺口

- 多容器共享 network namespace 仍是 TODO（[runtime.go](../platform/app-manager/containerd/runtime.go#L174)）。
- AI Runtime 应用模型权限自动注册目前只打印日志，没有真正注册权限（[server.go](../platform/app-manager/server/server.go#L297)）。
- 插件依赖的 `min_version` 尚未进行 semver 比较（[resolver.go](../platform/app-manager/plugin/resolver.go#L57)）。
- containerd task IO 日志读取未实现，只能依赖日志文件（[client.go](../platform/app-manager/containerd/client.go#L1136)）。
- Web 导入向导只能识别多容器包，不能完整编辑多容器 manifest。
- CI 默认验证 host stub，不覆盖真实 Hailo NPU、ISP、MCU、DMA-BUF、组播发现和硬件资源耗尽场景；这些仍需设备台架测试（[CI](../.github/workflows/ci.yml#L13)）。

## 10. 文档与配置漂移

以下漂移**不应作为运行时事实**；做发布或接口联调时，以当前代码、systemd unit 和实际配置解析逻辑为准：

- Node 版本口径不一：根文档和 CI 要求 Node 24，`scripts/setup_env.sh` 安装 Node 22，Web README 仍写 18/20（[setup_env.sh](../scripts/setup_env.sh#L89)、[Web README](../web/README.md#L5)）。
- README 提到 RK3588/Jetson，代码中没有对应 HAL 后端。
- Makefile 仍用 `LENS_PRODUCT` 生成 `product.yaml`，camera 配置则说明镜头型号已改由 EEPROM 决定。
- 部分旧文档仍描述编码协议 V2，当前实现已是 V3（见 [§5](#5-视频缓冲区与-ipc)）。
- 开源检查表中的固件版本、二进制状态及 `SECURITY.md` 状态已过期（见 [§8](#8-安全与已知高风险项)）。
