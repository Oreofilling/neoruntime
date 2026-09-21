# NE503 AIPC（NeoRuntime）性能测试方案

| | |
|---|---|
| 版本 | v1.13（2026-09-17，新增 M14 逐模型链 A 到达帧率（记录级，perf_demo 独立 `--chains a` 窗，与 M13 链 B 互为对照）；S4 排期 true-up 并入 M13/M14 逐模型窗；阈值在模板 v1.13） |
| 状态 | 待执行（本方案只定义方法与流程，不含实测数据） |
| 配套文档 | [performance-acceptance-template.md](performance-acceptance-template.md)（验收标准+阈值+空白结果模板，**单一事实源**） |
| 适用对象 | 在目标设备上独立执行三层性能测试的工程师 |

**版本历史**：

| 版本 | 日期 | 修订要点 |
|---|---|---|
| v1.13 | 2026-09-17 | 新增 M14 逐模型链 A 到达帧率（记录级，perf_demo 链 A 逐模型独立窗）：链 B（M13）测出图交付，链 A 测 **subscribe 平台调度到达率**——`infer.subscribe` 绑帧序→annotate 烘入主流路径逐模型覆盖。口径=**到达 fps=结果到达样本数÷窗口时长**（subscribe 到达口径；annotate 调用被 `--annotate-hz` 节流，不作交付率指标）；**独立 `--chains a --a-fps 10` 窗、禁与链 B 同窗**（A/B 互抢 NPU/daemon）；结构预期=到达 fps≈min(配速×交付率, 1000/e2e p50)——轻模型配速受限（≈9fps 级）、yolov5m（e2e ~124ms）模型天花板受限 ~8fps（非调度缺陷）；yolo_world 双输入 N/A（链 A daemon 绑定无 text embedding 第二输入源，U1 缺口同 M13）；B31 fps≥8 线按部署模型 yolov8n 校准、逐模型不搬用；11.1 S4 排期 true-up（M13 的 25min 原未入 380min 总账，一并修正 → 430min≈7h10m，排期 7.5h）；11.3 命令矩阵增 M14 行；验收项 90→91（模板 v1.13） |
| v1.12 | 2026-09-17 | M 矩阵 keypoint 槽位模型更换：`face_landmarks_lite.hef`（两阶段级联副模型）→ `yolov8s_pose.hef`（单阶段 COCO 17 关键点，Model Zoo Compiled v5.4.0 hailo15h）。D72 实测（2026-09-17）：输入契约 `get_model_info` 冻结=NHWC [1,640,640,3] uint8 RGB 单输入；benchmark 114.2 FPS（hw_fps 129.9）；SDK 链路冒烟 err=0；**HailoRT 5.3.0 运行时兼容 5.4.0 编译 HEF**（hailortcli 与 ai-runtime 双侧验证）。级联对照（可选进阶）随换型作废；7.6 适配注意与结构预期同步；S0 目标设备须同步补拷 |
| v1.11 | 2026-09-17 | 整合 Hailo-15 厂商工具链（hailo-soc-profiler / hailortcli / DFC Model Profiler / 硬件子系统脚本），**全部 RECORD/诊断级、不新增验收门**：①新增 9.6 系统级归因工作台（CPU 瓶颈两步法=run2 raw_async 定 NPU 上限→sched+applications trace 定位前后处理；与 9.5 py-spy 并列归因两板斧；Host 侧 trace 查看器 hailo-soc-profiler-ui deb :15000）；②U14 工具链 preflight 盘点（可用性矩阵；缺失静默省略不判 INCONCLUSIVE）；③7.6 第 4/5 条：逐模型必测 hailortcli benchmark+run2 双模式（host 开销=full_async−raw_async）；④9.4 厂商口径互证（hailo-dma-usage.sh/dsp-utilization）；⑤坑清单 10.15 观测者效应（正式窗禁挂 trace/monitor）；10.7 像素预算过时值 120→240（09-17 根修提额） |
| v1.10 | 2026-09-16 | 新增 M13 逐模型链 B 出图帧率（记录级，perf_demo 链 B 逐模型窗口）：**出图帧率=published_unique÷窗口时长**（唯一源帧口径，与 M05 推理吞吐分开），口径必带 `--b-fps 15` 准入配速（不配速=帧积压失真）；结构预期=轻模型出图率由 compose 固定成本主导（对照机新链路参考 ~12–13fps@配速15）、仅 yolov5m 级可见拉低、yolo_world 双输入 N/A（U1 缺口）；11.3 命令矩阵增 M13 行；验收项 89→90（模板 v1.10） |
| v1.9 | 2026-09-15 | 新固件（ai-runtime 单体，UDS 兄弟会话交接）口径回灌：①fps 闸门改**到达驱动**——A07/A07e/B31 判读按固件轴重校（旧 9/14 skew 不可比，链 A skew p50 ~51→~16ms）；②M07 新增 `StreamInferResponse.perf` 原生分段（前置 U13：pb2 重新生成+旧包抢注坑）；③坑清单 10.14 model_variant 短名静默失配（结果非空抽检）；④5.1 新增预处理池三键指纹与 ai-runtime 部署纪律（stop→scp 符号链目标）；验收项数不变 |
| v1.8 | 2026-09-15 | 新增 A07e fps_limit 扫描（可选，记录级）：10/20/25（+补偿档 ≈23/≈30）交付率曲线与拐点；三道闸门（源流 15/30fps 天花板、pacing 6.4ms 算术与补偿设定、NPU 容量预估）；20/25 档 90% 契约不适用；工具前置=探针/参数化（未落地标 N/A） |
| v1.7 | 2026-09-15 | 新增 9.5 函数级 CPU 归因（py-spy 旁挂采样，可选、记录级）：补齐"进程→链路→函数"三级 CPU 归因的最细一级；硬纪律=正式测量窗禁挂采样器（ptrace 抖动），归因轮同负载单独复现；产物=火焰图 SVG + top-10 函数表（模板 C38） |
| v1.6 | 2026-09-15 | NPU 共享组一致性回灌：5.3 检查单新增第 9 条（统一组构建确认）、9.x journal 巡检新增 A41 两查取证；验收项定义在模板 v1.6 A41 |
| v1.5 | 2026-09-15 | 结构重排：按"测什么→怎么测→准备→工具缺口→分层细则→坑→执行流程"重排章节；U 清单独立成章（第 6 章）；原附录 D 命令矩阵并入第 11 章执行流程；归档纪律/利用率口径/get_stats 采样窗等重复表述归一到所属章节；技术内容与 v1.4 一致 |
| v1.4 | 2026-09-15 | 摸底与可读性增强：新增 U12 利用率时间序列探针与 M12 推理期利用率项、横向对比表利用率列、M 窗 proc_sampler 10s、C21 系统级字段、12.1 结果通俗摘要规范、摸底最小集裁剪 |
| v1.3 | 2026-09-14 | 采样表逐项当前实际值、U1–U11 工具就绪清单、归档纪律（U10）、B16=10、hw_fps 语义、M11 三分口径、命令修正 |
| v1.2 | 2026-09-13 | M 系列五族模型矩阵 |
| v1.1 | 2026-09-12 | 五级判定+有效执行前置门+380min 预算+执行矩阵 |

本方案回答"**怎么做**"：测什么、用什么工具、按什么流程、注意什么坑。所有阈值数字、判定级别、空白结果表**一律以验收模板为准**，本文不重复维护阈值。

**阅读路径**：执行人顺读第 5 章（准备）→ 第 11 章（流程与命令）即可开跑；第 7–9 章为分层细则（每模块测什么、方法门在哪），第 6 章（工具缺口）与第 10 章（坑清单）遇到异常再查。

---

## 1. 目的与适用范围

对 NeoRuntime（NE503 AIPC）平台做三层性能测定：

- **L1 SDK 接口性能基线**：推理 / 媒体与加速 / 事件 / 设备面只读 / 长稳（soak），外加**多模型横向对比**（M 系列）
- **L2 流媒体与管线 E2E**：AI overlay 烘焙、帧注入与写租约、并发扇出、perf_demo 双链路
- **L3 服务端资源面**：控制面 REST 延迟、观测面契约、journal 死信巡检、环境长跑趋势

**范围外**（有明确理由，不要顺手测）：

| 排除项 | 理由 |
|---|---|
| GPIO 读写（REST `GET /device/gpio` 等） | 已知缺陷：单次 GET 即硬复位设备（issue #46），**全链路禁调**，只做零调用审计 |
| `metrics_port` 指标面 | 服务端未实现 |
| web 玻璃到玻璃延迟 | 支配项是浏览器 jitter buffer（100–500ms），在浏览器侧不在设备侧；需 WebRTC 另行立项 |
| ReconfigureEncoder 等重操作中断 | 会重启编码器，须独立窗口，不与性能跑混跑 |

**首轮边界**：首轮正式跑的产出是「**基线建立报告**」，终态二选一——「**基线建立完成（全项）**」或「**基线建立（含缺项，附清单与补测计划）**」；「**性能验收通过**」自第二轮起按冻结阈值与冻结工具版本判定。首轮综合判定只出「有条件通过（附整改）/ 不通过 / 证据不足」之一（规则见模板第 9 章）。

## 2. 被测对象与三层总览

| 层 | 覆盖项 | 数据来源（SDK 仓 `ne503-aipc-sdks`，下称 SDK 仓） | 验收项编号 |
|---|---|---|---|
| L1 | 推理/模型面 | `python/tests/device/test_60_perf_inference.py` → 报告 P1 | A01–A08 |
| L1 | 媒体与加速 A/B | `test_61_perf_media_accel.py` → P2 | A11–A17 |
| L1 | 事件 | `test_62_perf_events.py` → P3 | A21–A25 |
| L1 | 设备面只读 | `test_63_perf_device.py` → P4 | A31–A39 |
| L1 | 长稳 soak | `test_64_perf_soak.py` → P5 | A51–A54 |
| L1 | 多模型横向对比 | M 系列执行载体（见 7.6；M13=perf_demo 链 B / M14=perf_demo 链 A 逐模型） | M01–M14 |
| L2 | overlay 烘焙 | `test_65_perf_overlay.py` → P6 | B01–B08 |
| L2 | 帧注入与写租约 | `test_66_perf_injection.py` → P7 | B11–B19 |
| L2 | 事件枢纽与并发（扇出/慢订阅/get_frame 扇出/混合负载） | `test_67_perf_events_hub.py` → P8 | B21–B24 |
| L2 | 双链路 demo | `python/examples/perf_demo/` → 9/14 报告同构产物 | B31–B35 |
| L3 | REST 控制面 | `scripts/perf_rest_latency.py` | C11–C16 |
| L3 | 观测面契约 | GetStats / GetStreamStatus 采样 | C21–C22 |
| L3 | journal 死信 | `scripts/collect-perf-journal.sh` | C31 |
| L3 | 环境趋势 | `perf_demo/proc_sampler.py` | C32–C37 |
| L3（可选） | 函数级 CPU 归因 | py-spy 采样（Python 进程）+ perf/gdb 兜底（原生 daemon），见 9.5 | C38 |

编排入口：`scripts/run-device-tests.sh <ssh-target> 22 --perf`（构建 wheel → rsync → 设备安装 → 每模块独立进程+超时预算 → 汇总 `dist/device-perf-report.json`）。注意两点：①该入口一次跑全部 8 个模块，L1/L2 分窗与单模块复跑见 U7；②**脚本每次运行会先删除设备端与宿主机已有的模块报告并覆盖合并报告**（防混跑设计）——分阶段/逐模型执行的归档纪律见 11.2（U10）。

## 3. 术语与统计口径

### 3.1 统计口径速查

| 术语 | 定义 |
|---|---|
| p50/p90/p95/p99/max | 百分位与最大值；**p50 取 3 轮中的中位轮**（套件采样协议），回填模板时不得混用全样本值 |
| fps | 实际交付帧率（按帧计数 ÷ 窗口时长） |
| 丢帧 | **只按 fps 交付率判**（HAL 帧序号场景序号连续性法不可用，详见 3.2-①） |
| hw 时延倒数 | GetStats `hw_fps` = 1 ÷ avg_hw_latency，**理论峰值 fps 非交付吞吐**（详见 3.2-②） |
| 利用率口径 | GetStats SoC 口径与 proc_sampler Linux /proc 口径**不同源不混写**（详见 3.2-③） |
| skew_us | `now_us() − frame.timestamp_ns/1000`，两端同在设备 CLOCK_MONOTONIC 域；0 = 未测量 |
| bake / strict / write-lease | overlay 结果烘入编码帧；strict=对帧门控模式；write-lease=注入帧写租约（outstanding ≤3、200ms 租期） |
| keep-fd | 取帧不经像素拷贝、以 buffer_id 引用传入推理的路径 |
| TTL | overlay 结果有效期（链：per-result > per-stream > global > round(2000/fps) > 500ms） |
| 占空比 | 烧录验证指标：带合成像素的编码帧 / 总编码帧 |
| CMA 双池 | `hailo_media`（768MB，camera-daemon 管线 dma-buf）与 `linux,cma`（512MB，内核通用） |

### 3.2 关键口径详解

**① 丢帧判定与 HAL 共享序号空间**。HAL 帧序号活在共享计数空间（跨全部流 ~75–81 步/s），按序号连续性计丢必然误报——实测 10fps 采样被误报 87.5%「丢帧」，实际 9.37fps 正常交付。因此 HAL 帧序号场景丢帧只按 fps 交付率判；**事件序号与编码包序号不受此限**，仍可对账（A25 gap、C22 恒等式）。

**② hw_fps 语义**。GetStats `hw_fps` 实为 1 ÷ avg_hw_latency（HAL 语义：纯 NPU 硬件平均时延的倒数，即理论峰值 fps，`hailo15_inference_impl.cpp`），**不是 SDK 实际交付吞吐**；实际吞吐 = 完成推理数 ÷ 窗口时长，单独计（M 系列）。

**③ 利用率两口径**。GetStats `device_utilization`（NPU，来自 HAL `nnc_utilization` **真实 NNC 计数器**）与 `cpu_utilization`/`dsp_utilization` 是 HailoRT 采样窗内的测量值（窗长=sampling_window_ms，便宜快照用 1–50ms 小窗）；proc_sampler 采集的是 Linux /proc 口径——**不同源不可混写**。NPU busy 推导代理 = 实际吞吐 × avg_hw_latency（忙时占比下界）；采样链路细节见 U12。

**④ get_stats 采样窗**。缺省请求即 500ms 阻塞采样窗（`sampling_window_ms=0` → 服务端缺省）。服务端 GetStats 已支持 `sampling_window_ms` 参数、clamp [1,5000]（主仓 `platform/ai-runtime/src/grpc_service.cpp`），SDK 侧先 clamp。验收看**净开销 = p50 − 500ms**（A03/C13，须用缺省窗调用的实测）；服务端窗口边界验收（C21）须**绕过客户端 clamp** 直接构造请求。

### 3.3 口径一致表（三处对照）

| 位置 | 阈值语义 | 与验收的关系 |
|---|---|---|
| 本方案 + 验收模板 | FAIL=基线×1.5（向上取整）、WATCH=×1.3；契约绝对值另列；INCONCLUSIVE=有效执行前置门不成立 | **验收判定唯一依据** |
| `gen_perf_report.py` 内置标注 | p99/p50>8× 长尾标注、err>0.5% 标注、soak 衰减>10% 标注 | 仅报告**标注**，辅助读数，**≠ 验收阈值** |
| 套件内部断言 | soak 衰减阈值 10% 等 | 套件自检，结论以模板回填为准 |

## 4. 测量方法学

### 4.1 采样协议（逐项**当前实际值**，已逐条对照代码；回填按本表核对样本量有效性）

`perf_sample` 默认：n=`PERF_SAMPLE_N`（300）、rounds=1、**预热额外执行 warmup=max(5, n//10) 次后丢弃**；p50 headline 取多轮的中位轮（跨轮保真限制见 U2）。

| 验收项 | 项 | 当前实际采样 | 计划调整（拟；落地前按"当前实际"核对样本量门） |
|---|---|---|---|
| A01–A03 | list_models / get_model_info / get_stats | 300×1 | 300×3 |
| A04 | infer_e2e | **300×3** | 维持 |
| A05 | infer_batch(4) | 100×1 | 维持（批量语义） |
| A06 | session_lifecycle | **50×1** | 100×1 |
| A07 | subscribe | 窗口制 60s（`PERF_STREAM_S`） | 维持 |
| A11–A17 | get_frame / to_rgb / resize / to_jpeg85 / accel A/B | **30×1** | 100×1 |
| A21 | publish | **200×1** | 维持 |
| A22 | publish_batch(100) | 10×1 | 维持（批量语义） |
| A23 | delivery_e2e | **100×1** | 维持 |
| A24 | get_topic_stats | **50×1** | 100×1 |
| A25 | arrival_10hz | 窗口制 60s | 维持 |
| A31–A37 | 设备面只读 RPC | 300×1 | 300×3 |
| B01–B03 | annotate 三档 | 100×1 | 维持 |
| B11 | push_frame | 200×1 | 维持 |
| B12 | paced 注入不变量 | 38 帧场景 | 维持 |
| B16 | 租约收敛 | **10 次尝试**（`range(10)`） | 可参数化提高（工具项） |
| B24 | 混合负载 | 15s 窗（**现无同轮 solo**，见 U9） | solo-before→mixed→solo-after |
| A51–A54 | soak 全项 | 1800s（`PERF_SOAK_S`）分桶 | 维持 |
| C11–C14 | REST 探针 | n=100，间隔 ≥0.05s | 维持 |
| M 系列 | 逐模型 | 与对应项同口径 | 同左 |

> v1.2 之前把常规时延项统述为 300×3 与代码不符（多数项实为 300×1 或更低）；错误采样表会把正常执行误判 INCONCLUSIVE，故本表逐项列出。"计划调整"列为拟议值，属工具演进项，**落地并冻结前一律按"当前实际"核对**。

- 错误**只计数不中断**（err 计入 err_pct，时延分布不含错误样本）
- **已知保真限制**：headline 只保留**中位轮**统计且 evidence **无各轮 JSON**（见 U2）
- 采样配置 env（编排脚本**只透传 4 个** `PERF_SAMPLE_N / PERF_SAMPLE_ROUNDS / PERF_STREAM_S / PERF_SOAK_S`，其余 PERF_* 变量不会到达设备）

### 4.2 校准/冒烟参数（先跑这个确认链路健康）

```bash
PERF_SAMPLE_N=50 PERF_SAMPLE_ROUNDS=1 PERF_STREAM_S=10 PERF_SOAK_S=120 \
  scripts/run-device-tests.sh <目标设备IP> 22 --perf
```

对照机全量默认参数跑 wall ≈ 2672s（45 用例）可作 S2+S3 时长参照。校准跑**结果不回填模板**。

### 4.3 模块超时预算与分窗执行

| 模块 | 预算 | 模块 | 预算 |
|---|---|---|---|
| test_60 | 1800s | test_65 | 900s |
| test_61 | 900s | test_66 | 1800s |
| test_62 | 600s | test_67 | 1500s |
| test_63 | 600s | — | — |
| test_64 | 2400s | — | — |

L1（60–64）预算和 105min；L2（65–67）预算和 70min。超时按模块失败计，进入复跑规则。

**S2/S3 分窗执行**：编排入口默认一次跑全部 8 模块；L1 与 L2 分开窗口执行时按 U7 的单模块子集方式裁剪，两窗之间重采空闲基线；每次运行前先归档上一轮报告（11.2，U10）。单模块复跑同法。

### 4.4 perf_demo 专用工具

- `proc_sampler.py`：设备资源采集（CPU/CMA/温度/MemAvailable/daemon RSS），全程后台（`--interval 60`；**产物为 JSONL**，每行一条记录）；**M 系列逐模型窗口内提到 `--interval 10`**（60s 粒度对 5–8min 窗口仅 6–8 样本且会漏 CPU 峰）
- `phase_runner.py`：**单阶段守护运行**（只跑一个 guarded phase、采集 CPU 基线与契约产物；**不做故障注入**，也从不操作平台服务——B34 三注入是手工操作，见 8.4 与 U6）
- `video_observer.py`：宿主机侧码流**帧采样器**（**只保存时间/PTS/尺寸等元数据，不含图像像素**——B33 的 ROI 统计须按 U5 选定方案落地）
- 部署：`python/examples/perf_demo/deploy/install.sh <ssh-target> [ssh-port] --model <设备侧模型路径> [-- <app 参数>]`（**宿主机执行**：本地构建 wheel → 部署 → yaml 备份后启用 injection → 装 systemd 单元）；回滚 `uninstall.sh <ssh-target> [ssh-port]`

### 4.5 判定级别与有效执行前置门

判定分**五级**（FAIL / WATCH / RECORD / EXEMPT / **INCONCLUSIVE**），定义与"有效执行前置门"的统一条件见验收模板 1.2/1.3——本方案只强调执行侧含义：**一项测量若场景未真正触发（如 strict 会话未建立、注入未开、字段全 0 且语义上不应全 0），该数据不是"全过"而是"证据不足"**，须整改后重测或在模板登记 INCONCLUSIVE。

## 5. 执行准备

### 5.1 环境指纹与可比性元信息（测前必采，回填模板第 3 章元信息表）

**指纹类**：

- SDK 仓 commit 与工作区状态；**SDK 未提交补丁摘要**（若设备 venv 装的是工作区版，逐条列出与 commit 的差异——否则两轮结果不可比）
- 部署二进制指纹（以**部署清单**为准，逐个 `readlink /proc/<pid>/exe` + md5 终判——camera-daemon 的 systemd unit 指向 `/usr/bin` 真文件，scp 到 `/data` 是 no-op 陷阱；ai-runtime 相反是 symlink；**覆盖 ai-runtime 前先 `systemctl stop ai-runtime`**——scp 到运行中文件报 text file busy）
- ai-runtime.yaml 预处理池三键：`stream_dsp_preprocess` / `stream_preprocess_slots` / `stream_preprocess_job_ms`（级 2 DSP 预处理池，当前默认启用，直接影响推理时延口径——逐轮记录取值）
- 被测模型文件名与 md5；SDK 版本（设备 venv 内）
- `injection.enabled` 状态（测前/测后各记一次）
- 常驻服务清单、journal 现存最早条目时间（--since 锚点有效性）
- **各测量窗起止锚点**（S2/S3/S4 稳态窗与 S4 故障窗分开记录——journal 分窗巡检的输入，见 9.3）

**可比性类**（跨轮/跨设备对比的前提，缺项须在报告注明证据局限）：

- 匿名设备编号（区分目标设备与对照机记录；不写网络标识）
- 系统与驱动版本：`uname -a`、内核版本、SoC 固件指纹（随部署指纹）
- 流配置清单：SDK `list_streams()` 输出摘要（id/几何/fps——对照机与目标设备流清单不同，perf_demo 参数据此适配）
- 模型输入契约（每被测模型的几何/格式/输入数——以 `get_model_info` **实测**为准，文件名解析不可靠，见 7.6 与 U1）
- CPU 调频策略：governor 与策略记录（performance/ondemand 差异足以淹没 ms 级时延对比）
- 环境温度起点（S1 快照，温度漂移影响长跑趋势项）
- **空闲基线快照**：CPU/CMA 双池/双温区/MemAvailable/各 daemon RSS（S1 采集）

### 5.2 访问

- 设备地址一律记 `<目标设备IP>`；SSH 凭据见部署清单（不写入本仓库任何文件）
- REST 控制面探针**面预检**：优先 UDS `/run/aipc/platform-api.sock`（免鉴权）；若部署版本只开 HTTP 面，则 `POST /api/login` 取 Bearer token（凭据占位符），探针脚本带 token 头——**该兜底路径当前未实现**，见 U3

### 5.3 目标设备适配检查单（逐项打勾后才允许开跑）

1. **注入总闸**：`injection.enabled` 出厂 false → 测前在 camera-daemon 配置追加 `injection:\n  enabled: true` 并重启（先备份 .bak）；**测后恢复**。不开则 PushFrame 全拒 error_code=-1，test_66 全灭
2. **流清单盘点**：SDK `list_streams()` 记录实际流（id/几何/fps）。perf_demo 的 `--a-stream third`（640×384@15 推理源）是**按对照机流清单设计的**；目标设备按实际流调整参数（订阅源选几何与模型匹配的流）
3. **模型盘点**：`ls /data/aipc-data/models`（M 系列矩阵输入，见 7.6 矩阵表）；确认矩阵五族模型到位（缺的从本地工作区 `models/detection/`、`models/keypoint/` 补拷，文件不入库）；**逐模型按 U1 适配清单冻结输入契约并留存冒烟成功证据**
4. **模型 id 纪律**：注册 id 一律**文件名派生**（固定 id 换文件会静默打到旧模型，见 10.4）
5. **推理输入契约**：NV12 模型必须喂几何匹配 NV12（RGB 或尺寸不符报 -2799/-2811）；原始路径用 `resize_hw(stretch)` 产出精确几何 NV12；**双输入模型（yolo_world 系）须同时喂 text embedding 张量**（`infer_with_tensors`）
6. **测量窗独占**：正式跑窗口内禁止其他操作（soak 期间尤其）；每层前后各采 2min 空闲基线
7. **L3 探针面预检**（见 5.2）+ REST 探针请求间隔 ≥0.05s（防自我排队/看门狗）
8. **回滚预案**：设备 venv SDK 恢复命令、camera-daemon.yaml .bak、部署二进制 .bak 位置（记入模板元信息表）
9. **NPU 共享组一致性（A41 前置）**：确认设备跑统一组构建——journal 双侧出现 `created shared VDevice (group='device0'`（camera-daemon 与 ai-runtime 各一）、`/data/aipc/etc/ai-runtime.yaml` 无 `scheduler.group_id` 键、HAL .so `strings` 无 `group=aipc` 残留；组失配会在 `VDevice::create` 当场 74 拒绝，全部推理项作废

## 6. 工具就绪清单（U1–U14，正式跑前的"执行工具版本冻结"项）

以下缺口均已对照代码核实。**处理顺序约定**：先解决本表有效性问题 → 冻结阈值与工具版本 → 再安排正式跑。对应工具改动属 SDK 仓代码改动，提交按仓库规约另行审批；未解决项须在元信息表登记"绕过方式"或接受相关验收项 INCONCLUSIVE。

| # | 缺口（已核实现状） | 影响验收项 | 正式跑前动作 |
|---|---|---|---|
| U1 | M 系列模型覆盖与输入适配：`pick_model` 无模型覆盖入口（透传仅 4 变量）；**输入几何靠文件名推测且不可靠**（实测 `yolo_world_v2s_540` 被解析为 960×540；`tiny_yolov4_license_plates`/`face_landmarks_lite` 无法解析→回退摄像头原始尺寸）；`infer_batch` 每项只构造一个输入张量，**双输入批量不可用** | M01–M11、M13 | 扩为**模型适配清单**：逐模型固定输入名/W/H/格式/dtype/shape（以 `get_model_info` 实测冻结）、单次与 batch 调用方式（双输入单次可用 `infer_with_tensors`；批量路径须另行实现或明示能力缺口）、预/后处理是否计入时延、冒烟成功证据；外加 `PERF_MODEL_FILE` 覆盖+透传 |
| U2 | 采样器跨轮保真：evidence **只存中位轮统计与 p50 spread，无各轮 JSON**——"人工核对各轮"不可行 | A08（及一切 err=0 判据） | 扩展 `perf_sample` 输出全轮 err/p99；落地前 A08 按模板 1.3 判 INCONCLUSIVE |
| U3 | REST 探针成功判定：仅传输异常计 err；HTTP 4xx/5xx 计成功时延（status_hist 不参与判定）；HTTP+Bearer 兜底未实现 | C16 | 状态码非 2xx 计错误；补 HTTP+Bearer 兜底；输出三类错误计数（传输/HTTP/业务） |
| U4 | journal 巡检脚本：①脚本内 `JBASE` 未加引号（`--since` 带空格拆词）+ `2>/dev/null` 屏蔽失败——失败呈现为零匹配；②远端调用命令须写 `sh -s --`（参数终止符），否则 `--since` 被 sh 当自身选项解析报 `Illegal option --` | C31 | 修复①②；每次执行后核对退出码与窗口覆盖（见 9.3） |
| U5 | 烧录占空比分析：video_observer 只存元数据**无图像像素** | B33 | **选定方案：采集时直接计算 ROI 亮像素统计**（ROI 内 Y>200 且像素数>150 判"带合成像素"，随采样逐帧输出计数）；可复核视频段存档为可选佐证 |
| U6 | 韧性注入载体：phase_runner 不做故障注入 | B34 | 三注入按 8.4 手工操作 + journal/NRestarts 取证 |
| U7 | 单模块复跑入口：编排脚本 `MODULES` 数组硬编码，无模块子集参数 | 复跑规则 / S2、S3 分窗 | 复跑或分窗时临时裁剪 MODULES 数组，或在设备 venv 直跑单模块（`NEORUNTIME_DEVICE=1` + PERF_* env） |
| U8 | soak 衰减算法：test_64 内置=首段速率 vs 其余均值（buckets[1] 分界），非等长窗口对比 | A51 | 判据改为从 P5 分桶序列**重算**，参数化公式见模板 A51（窗长/首窗起点/步长/尾窗处理已固定）；内置值仅参考输出 |
| U9 | 混合负载同轮 solo 对照：test_67 `mixed_load` **无模块内 solo 前后数据**（evidence 自述"跨模块对比"），异常被忽略、无 attempted/succeeded/failed 计数 | B24 | 输出每面 `solo_before / mixed / solo_after` 三段与 `attempted / succeeded / failed` 三计数；落地前 B24 判 INCONCLUSIVE |
| U10 | 报告归档：编排脚本每次运行**删除设备端与宿主机 `report-test_*.json` 并覆盖合并报告**（防混跑设计）——分阶段/逐模型执行会丢上一轮证据 | 全部分阶段产物 | 每次运行前把上一轮产物归档到独立目录（`run_id/阶段/模块/模型/尝试次数`）；**本次归档完成才允许下一次运行**（11.2）；S6 按产物清单合并并核对缺项。整改方向=脚本原生支持 run_id 目录 |
| U11 | 原始样本/直方图输出：`perf_sample` 只输出聚合分位，**无原始时延数组或直方图**（B17 有 ≥43ms 计数，A15/B17 的"单峰"判据无原料） | A15/B17 单峰子项 | `perf_sample` 可选输出直方图或尾部原始样本；落地前单峰子项判 INCONCLUSIVE（尾部计数与 p99 线仍可判） |
| U12 | NPU/CPU/DSP 利用率时间序列探针：**链路已核实存在但从未按方案采样**——HAL `nnc_utilization→npu_utilization`（HailoRT 5.3 IntegratedDevice 返回**真实 NNC 计数器**，hailort_server 多进程模式亦有效，`hailo15_inference_impl.cpp`）→ GetStats `device_utilization`（0–1）/`cpu_utilization`/`dsp_utilization`/`device_temperature`（`grpc_service.cpp`，字段仅当 ≥0 才置值、HAL 不支持返回 −1 缺省）；SDK `get_stats(sampling_window_ms=50)` 小窗快照（docstring 明示便宜快照用 1–50ms 窗），perf_demo `statusline.py` 有现成范式；**风险**：9/14 报告实测读数间隙（0.077/0.0） | M12（推理期利用率）、C21（系统级字段在位）、NPU 消耗摸底全局 | ①**preflight**：目标设备连续 ≥10 次 50ms 快照，验证字段非缺省且非恒 0，间隙率记入元信息表；②探针脚本逐相（空闲/逐模型/并发）循环小窗快照，输出分位时间序列 JSONL；③字段缺省窗回退**推导代理**：NPU busy% ≈ 实际吞吐 × avg_hw_latency |
| U13 | 客户端 pb2 陈旧且**抢注**：SDK 包内 `inference_pb2` 落后于新固件 proto——**无 `StreamInferResponse.perf`（PerfBreakdown 字段 8）**，且旧 pb2 会**抢注 descriptor**（重新生成后仍可能加载到旧定义）。来源：dma-buf-zero-copy 会话交接（2026-09-15） | M07 原生分段（及一切 perf 字段消费） | 按新固件 proto 重新生成 python pb2 并装入设备 venv 与测试环境，验证 `StreamInferResponse` 含 perf 字段；**未落地不阻塞**——M07 回退 9/12 探针法（RECORD 项不判 INCONCLUSIVE） |
| U14 | **厂商工具链可用性未盘点**：板端 `hailo-soc-profiler` / `hailortcli` / `dsp-utilization` / `hailo-dma-usage.sh` / `/etc/hailo_noc_perf.sh`（DFC Model Profiler 在 PC 端）随 OS/SoC 固件镜像交付，在位性与版本随镜像漂移，从未按方案登记；Host 侧 trace 查看器 `hailo-soc-profiler-ui`（amd64 deb，如 1.12.0）与板端采集器**版本须配对**（不匹配可能 trace 打不开） | 9.4 厂商口径互证、9.6 系统级归因工作台、M 系列第 4/5 条（NPU 对照与 host 开销拆分） | **preflight 盘点**（并入 S0 检查单）：板端逐项 `command -v` + 存在性 + 版本记录，Host 侧记录 UI deb 版本并**配对登记**，产出**可用性矩阵**（回填模板附录）；缺失项相关采集**静默省略并在产物标注**——厂商工具是 RECORD/诊断级数据源、不是验收门，缺失不判 INCONCLUSIVE，相关归因降级到 9.5 py-spy / 探针法 |

## 7. L1：SDK 接口性能基线

每模块统一结构：目的 → 覆盖接口 → 执行命令 → 产出字段（报告分区行名）→ 判定指向（模板编号）。

### 7.1 test_60 推理与模型面

- 覆盖：`list_models` / `get_model_info` / `get_stats` / `infer` e2e / `infer_batch(4)` / 会话注册注销 / `subscribe` 流式
- 产出：报告 P1 表（行名与接口同名）+ `subscribe_10fps` 流行
- **get_stats 口径**：采样窗语义见 3.2-④；验收看净开销（A03）；C21 须绕过客户端 clamp 探测服务端边界。返回字段含 avg_skew_us / max_skew_us / skew_samples
- **subscribe 交付口径**：fps_limit 语义是"距上次**完成** ≥1000/fps"（完成链 ~6.4ms 叠加），fps_limit=10 实交付 ~9.4fps 是已知 pacing 语义非缺陷；按 **90% 交付率契约**判（A07：≥9.0fps）；丢帧只按 fps 判（3.2-①）。**固件轴**：完成锚点是**旧固件**行为——新固件（ai-runtime 到达驱动闸门）实交付 ≈ 设定值、skew 大幅下降（链 A skew p50 ~51→~16ms、p99 84→22ms），判读按部署指纹的固件轴区分，**旧 9/14 skew 参考值不可直接比**
- **A07 可选扩展：fps_limit 扫描（A07e，记录级、基线待建，不参与判定）**——回答"平台交付率上限/拐点在哪"：
  - **方法**：同窗口制，fps_limit ∈ {10, 20, 25} 各一窗（60s），可加**补偿档 ≈23/≈30**（对齐"实际交付 20/25"）；输出各档实交付 fps → 交付率曲线
  - **三道闸门**：①**源流天花板**：交付 ≤ 源流帧率——20/25 档必须订 30fps 流（sub/main，几何与模型匹配，按 5.3-2 盘点选）；third 类 15fps 源上限 15，扫 20/25 无意义；②**pacing 算术**：完成链 ~6.4ms 恒定，**预期交付 ≈ 1÷(1000/fps_limit + 6.4ms)**：10→~9.4（94%）/ 20→~17.7（88.7%）/ 25→~21.6（86%）——**25 档 90% 交付契约不适用**（算术上限即 86%，非性能不达标）；③**NPU 容量**：小模型（yolov8n 2.25ms/帧）25fps 仅占 NPU ~6% 无压力；大模型按 hw latency×目标速率预估（yolov5m 21.5ms×25fps≈54%，走得动但余量缩）
  - **判读**：首轮只建参考值（修订表转正）；gap 判读周期按档换算（10fps=100ms / 20=50ms / 25=40ms）；交付**显著低于 pacing 预期线**（且排除源流/NPU 因素）才视为异常信号
  - **固件轴**：上述 pacing 预期线公式基于**旧固件**（完成锚点）；新固件（到达驱动闸门）下预期交付 ≈ 设定值（缺口主要来自源流/NPU 上限），**补偿档 ≈23/≈30 属旧固件方案**——新固件直接设目标值；首轮实测重校所在固件轴的预期线
  - **工具前置**：fps_limit 在套件测试里写死 10——扫描须探针脚本或测试参数化（可选工具项，未落地标 N/A；不进 U 清单、不阻塞正式跑）；补偿档数值以首轮实测校准
- 判定 → A01–A08

### 7.2 test_61 媒体与加速 A/B

- 覆盖：`get_frame`（主流/子流）、`to_rgb` / `resize` / `to_jpeg85`；accel 路由 A/B（default vs swonly）+ 路由决策证据（probes/health JSON）
- 产出：报告 P2 表（**单峰判据的直方图/原始样本产物待 U11 落地**）
- **A15 resize 池驻留项**阈值用修复后基线（9/11 P1-9 设备验收记录），非 9/9 报告的双峰数据
- **A/B 比值语义**：数组输入场景硬件腿走 UDS 像素 socket 传输，比值>1 计价的是传输不是算力——记录级，不作加速比结论
- 判定 → A11–A17

### 7.3 test_62 事件

- 覆盖：`publish` / `publish_batch(100)` / 订阅交付 e2e / `get_topic_stats` / 10Hz 发布到达流
- 产出：报告 P3 表
- 判定 → A21–A25

### 7.4 test_63 设备面只读

- 覆盖：`get_lens_status` / `get_autofocus_status` / `get_capabilities` / `get_sensor_info` / `get_stream_status` / `get_hardware_status` / `get_infrared_status` / 设备事件流
- 产出：报告 P4 表
- **A34 lens 条件豁免**：目标设备历史上存在镜头 af0832 bootstrap -2815 死循环（p99 203ms/max 2.2s 长尾的根因，设备特有）。豁免触发条件=journal 证实循环存在；不存在则按健康参考线判
- **A38 设备事件流**：服务端 SubscribeEvents 是未实现桩（初始开源即如此），预期 0 帧——豁免记录，真设备事件走 event-bus（P3 面）
- **A39 GPIO 零调用审计**（负面门）：套件与探针端点清单静态确认不含 GPIO；窗口内 journal 无 gpio 请求记录（如观测面可用）。**任何 GPIO 调用 = 流程违规，当场作废窗口**
- 判定 → A31–A39

### 7.5 test_64 长稳 soak

- 全量模式 1800s（full：取帧+推理+事件混合迭代）；产出：速率衰减、客户端 RSS/fd 分桶序列、四 daemon RSS Δ
- **A51 衰减口径**：套件内置衰减算法是"首段速率 vs 其余均值"（buckets[1] 分界，test_64 内实现），**不作判据**；判据从 P5 分桶序列重算，**窗长/首窗起点/最差窗步长/尾窗处理按模板 A51 固定公式**（见 U8）
- **有效性前提**：A51/A53 只在 evidence `mode=full` 时判定；`media-only (infer degraded)` 为降级跑，A51/A53 判 INCONCLUSIVE 并归因
- **RSS 漂移归因（A53 基准点统一）**：ΔRSS 一律取**首个 t>120s 的桶 → 末桶**（预热 120s 剔除后的首末差，与"首分钟预热不计"统一为同一判据）；首分钟上涨是预热（分配器/缓存到位）；daemon RSS Δ 单独记录（A54）
- 判定 → A51–A54

### 7.6 M 系列：多模型横向对比

**背景**：NPU 裸层已有横向数据（主仓 `docs/benchmarks/ai-model-benchmark-hailo15h.md`，hailortcli benchmark，2026-05-18），但**不走 SDK/平台链路**；SDK 套件 test_60 是单模型降级链。M 系列补齐 SDK 层横向。

**模型矩阵（五族）**：

| 系列 | 模型文件 | 输入契约（**参考值，正式以 `get_model_info` 实测冻结**） | NPU 裸层参考（5/18 报告，batch=1） | 现状/来源 |
|---|---|---|---|---|
| 轻量单输入 NV12 检测 | `hailo_yolov8n_384_640.hef` | NV12 640×384 | 444.9 FPS / 2.25ms / 2 contexts / 4.7MB | 设备已有（以 M01 盘点为准） |
| 中型大几何检测 | `yolov5m_vehicles.hef` | 1080×1920（F8CR 压缩格式） | 46.6 FPS / 21.5ms / 5 contexts / 18MB | 本地工作区 `models/detection/` 补拷（文件不入库） |
| 零样本双输入 | `yolo_world_v2s_540.hef` | image（文件名解析=960×540，**以实测为准**）+ text embedding [1,1,80,512] | （5/18 报告未含；对照机 SDK 链路实测 ~42–47 FPS） | 设备已有 |
| 超轻量 RGB 检测 | `tiny_yolov4_license_plates.hef` | RGB 416×416（文件名**无法解析**，回退摄像头原始尺寸——须实测冻结） | 908.4 FPS / 1.10ms / 1 context | 本地工作区补拷 |
| 姿态/关键点（单阶段） | `yolov8s_pose.hef` | **NHWC [1,640,640,3] uint8 RGB 单输入**（D72 `get_model_info` 实测冻结 2026-09-17；文件名无法解析几何，恰证 U1 警告；9 输出=3 尺度×box64+conf1+kpts51，COCO 17 关键点） | 114.2 FPS / ≈8.8ms / 13.8MB（D72 benchmark 2026-09-17；daemon hw_fps 129.9） | Hailo Model Zoo **v5.4.0**（hailo15h）；D72 已入 `models/keypoint/` 并实测载入（5.3.0 运行时兼容 5.4.0 编译 HEF，hailortcli 与 ai-runtime 双侧验证）——目标设备 S0 同步补拷 |

> **输入契约警告**：套件现用 `model_input_geometry()` 从文件名推测几何，实测 `yolo_world_v2s_540→960×540`、后两个文件名无法解析（回退摄像头原始尺寸喂入）。**U1 适配清单落地前，矩阵表数值一律视为参考**，逐模型以 `get_model_info` 实测冻结并留存冒烟成功证据。

输入适配注意：yolo_world 双输入须同时喂 text embedding 张量（单次推理可用 `infer_with_tensors`；**`infer_batch` 现仅构造单输入张量，双输入批量须另行实现或明示能力缺口**——U1）；NV12 模型喂几何匹配 NV12；RGB 模型走原始 bytes 路径。yolov8s_pose 为单阶段单输入 RGB 640×640——喂 `resize_nv12→640×640 + nv12_to_rgb`（契约 D72 `get_model_info` 实测冻结 2026-09-17，SDK 链路冒烟 err=0），`infer_batch` 路径可用、不受双输入批量缺口约束。

**执行准备项（二选一）**：

- 路径 a（推荐）：U1 模型适配清单 + `PERF_MODEL_FILE` 覆盖（**改动点**：`perf_common.pick_model` 加覆盖入口 + `run-device-tests.sh` 透传列表加该变量 + 逐模型适配清单落地），逐模型重跑 test_60
- 路径 b：探针脚本逐模型跑（同口径采样协议：预热 max(5, n//10) 后丢弃 → 按对应项样本量；参考 9/12 报告"推理面耗时分解"探针方法）

**每模型窗口资源采集**：proc_sampler `--interval 10` 并行挂起 + U12 利用率探针同步采样——逐模型 CPU/NPU 消耗摸底的数据源（横向对比表利用率列）。

**每模型统一测**（输入按冻结契约；结果列定义与横向对比表对齐）：

1. SDK `infer` e2e 分位 **p50/p90/p99/max**（bytes 路径）；keep-fd 路径 p50 对照（可选）
2. `infer_batch(4)`（true batch 落地后的横向）+ **每帧比 = batch(4) p50 ÷ 4 ÷ 单帧 p50**（双输入批量受 U1 缺口约束，不可用则标 N/A 并归因）
3. GetStats per-model `avg_latency_us` / **hw 时延倒数**（理论峰值语义，3.2-②）+ **参考比 = hw 时延倒数 ÷ NPU FPS**（hw 字段对部分模型族不上报，E12）；**实际吞吐 = 完成推理数 ÷ 窗口时长**（单独计，不用 hw_fps 替代）
4. NPU 纯执行对照（**前置 U14**）：逐模型必测 `hailortcli benchmark <hef>`（当前镜像版本；5/18 报告时间点旧，仅作历史参考不再直接引用）；并补 `hailortcli run2 --mode raw_async set-net <hef>`——**raw_async 的 input/output 不经 host 前后处理**，其稳态帧率即**纯 NPU 帧率上限**（排除 CPU 干扰的对照锚点；与 SDK 链路实际帧率对比即可判断"慢在 NPU 还是慢在 CPU"，见 9.6 两步法第一步）
5. **平台开销 = SDK e2e − NPU latency** 与 **开销占比 = 开销 ÷ e2e**（方法与四块分解参考 9/12 报告补充章节）；**run2 双模式拆分（前置 U14）**：`hailortcli run2 --mode full_async set-net <hef>`（全流水线，含 host 侧前后处理）与 raw_async 同 HEF 对跑，**host 前后处理开销 = full_async 帧时延 − raw_async 帧时延**——CPU 满载嫌疑的第一定量证据（差值占比过大即"前后处理挤爆 CPU"，下钻走 9.6 两步法第二步）；**新固件原生分段 `StreamInferResponse.perf`**（PerfBreakdown：repack/dsp/queue/infer/post/write，**前置 U13**——pb2 须重新生成）可与探针法互证；可选进阶：板端 `hailortcli run2 --mode raw set-net <hef> measure-fw-actions` 产出 runtime_data_*.json，scp 回 PC 用 DFC Model Profiler 联合出逐层报告（`hailo profiler <model>.har --runtime-data <json> --out-path <html>`）
6. **模型加载时延（三分口径，M11）**：**冷加载**=重启 ai-runtime 或注册未加载文件后的 register→ready；**缓存命中**=同模型再注册（注意服务端对已加载模型与同文件**别名**会直接复用跳过加载，`model_manager.cpp`——不区分会只测到一次注册查询）；**首次推理**=首次 infer e2e 与稳态 p50 之差
7. 资源占用：模型文件大小 / contexts 数 / 加载后 ai-runtime RSS 增量
8. **推理期利用率（M12，前置 U12）**：推理窗内 `device_utilization`（NPU 真实利用率）时间序列分位 + CPU **两口径分开记**（3.2-③）；字段缺省窗用推导代理（NPU busy% ≈ 实际吞吐 × avg_hw_latency）

**逐模型链 B 出图帧率（M13，新增）**：回答"每个模型推理后，出图能到多少帧"——这是**链路交付帧率**，与第 3 条的**推理吞吐**（完成推理数÷窗口）是两个口径：出图帧率受 compose 全链（DSP resize / 推理 / render+blend / 发布）与准入配速共同约束。载体=perf_demo 链 B（keep-fd→DSP resize→infer→render/blend→publish，8.4 同链路）逐模型窗口：

- 命令与参数见 11.3 M13 行（`--chains b --zero-copy --b-fps 15 --duration 300`，逐模型 `--model-path`）
- 口径：**出图帧率 = published_unique ÷ 窗口时长**（唯一源帧出图数；40Hz 发布节拍的重复发布与 zero-copy push_slot 均按唯一源帧计，不按 publish 调用数）；同窗记录 e2e p50/p90/p99 与分阶段时延（resize/infer/render/blend/pub）——一律**样本口径**，console 的 e2e 行是被尾部拉高的窗口均值，不作判据
- **配速语义（口径必带）**：不配速则帧积压（compose 耗时 > 帧距时 e2e 虚高、出图率失真）；配速下出图率 ≈ min(源流 fps, 配速值, 链路容量)。首轮统一 `--b-fps 15` + sub 源流
- 结构预期（对照机 2026-09-16 新链路参考，yolov8n 配速 15：实交付 ~12–13fps、compose p50 ~50ms、e2e p50 62.4ms）：轻量模型（yolov8n / tiny_yolov4 / yolov8s_pose）出图率**不随模型显著变化**——compose 由 draw/blend 与 RPC 固定成本主导，非 NPU 时延；仅 yolov5m 级大几何模型（NPU 21.5ms 参考）会可见拉低
- **yolo_world 双输入标 N/A 并归因**：链 B infer 腿现仅构造单输入张量（U1 缺口，同上方输入适配注意）
- 前置：U1 输入契约冻结 + injection 已开 + 流适配（同 8.4 B31 前置）；首轮 RECORD 级建基线，阈值经修订表转正

**逐模型链 A 到达帧率（M14，新增）**：与 M13 链 B 互为对照——链 B 测**出图交付**（compose 全链后发布），链 A 测**平台调度到达**（`infer.subscribe` 绑帧序→annotate 烘入主流，8.4 链 A 同路径）。载体=perf_demo 链 A 逐模型 300s **独立窗**：

- 命令面：`--chains a --a-fps 10 --duration 300`，逐模型 `--model-path`（与 M13 同模型矩阵、同归档纪律）；**禁与链 B 同窗**——`--chains ab` 下 A/B 互抢 NPU 与 daemon，污染两链指标
- 口径：**到达 fps = 结果到达样本数 ÷ 窗口时长**（subscribe 到达样本；**annotate 调用被 `--annotate-hz`（默认 3.0）节流，annotate_calls 不作交付率指标**）；annotate_errors 全窗计数；Δbake_skips（B33 同源）；skew 分位（新固件到达驱动轴）
- 结构预期：**到达 fps ≈ min(a-fps 配速×交付率, 1000/e2e p50)**——轻量模型（yolov8n / tiny / yolov8s_pose）配速受限（≈9fps 级）；yolov5m（e2e ~124ms）**模型天花板受限 ~8fps，属模型算力约束非调度缺陷**，判读勿套 B31 的 fps≥8 线（该线按部署模型 yolov8n/配速 10 校准，逐模型不适用）
- **yolo_world 双输入标 N/A 并归因**：链 A daemon 绑定路径无 text embedding 第二输入源（U1 缺口，同 M13 归因）
- 前置：流适配 + injection 已开（同 B31）；首轮 RECORD 级建基线（模板 M14 行）

**可选进阶**：多模型共存交替推理（`max_model_cache=3` 约束下 contexts 切换影响）、batch on/off 对照。

**阈值策略**：首轮以横向对比表+记录级为主；硬性项=每模型 err=0（M03）；e2e 参考线=NPU latency+平台开销预算（对照机 yolov8n 实测开销 ~14.5ms 的参考上浮，见模板 M02）。首轮正式跑后经模板修订表转正为各模型基线。

判定 → M01–M14。

## 8. L2：流媒体与管线 E2E

### 8.1 test_65 overlay 烘焙

- 覆盖：`annotate_result`（10 框/50 框/16 多边形三档）、烘焙流帧率不变量（level_small/mid/big + noop on/off）、TTL 衰减、session sweep、strict 计数、帧绑定
- **B07 锚源方法门（必守）**：frame-bound 绑定**必须从 `FrameHandle.sequence`（HAL 共享计数空间）锚定**；用 `last_packet_seq`（每流编码包计数）恒判 too-old（两空间 ~2.5× 错位，对照机曾因此把命中率从 31.7–38% 误测成 1.7%）
- **B05 覆盖缺口**：套件未建立 strict 订阅会话（strict_locked/degraded 全 0 是**未驱动**不是全过）→ 该项判 **INCONCLUSIVE**；strict 实锁语义验收依赖 perf_demo 或独立驱动
- **B08 拆分**：C5「overlay 恒定开销」不再并入 B04 判据——独立子项 B08（计算成本），缺证据判 INCONCLUSIVE，**不反向收窄 B04**（帧率连续性独立可判）
- 判定 → B01–B08

### 8.2 test_66 帧注入与写租约

- 覆盖：`push_frame` RTT、paced 不变量、compose REPLACE/OVERLAY、EOS 恢复、写租约收敛/稳态、池驻留回归、注入与 DSP 并发
- **B13 预期豁免**：4K 全幅 REPLACE 池分配被 DSP 每客户端像素预算拒绝（`DspError: client limit exceeded`）是已知约束非缺陷；4K 场景用 OVERLAY(inset)
- **B16 收敛率分母与尝试数（对齐代码）**：收敛率=**n_conv/(n_conv+n_timeout)**（evidence 字段 `perf:lease_convergence`；percentile 统计里的 n 只含收敛样本，禁用作分母）；**尝试数=10**（测试固定 `range(10)`，与 B12 的 38 帧 paced 场景是两个不同场景分别定义）；需提高尝试数属工具参数化项
- **B17 池驻留回归门**是 P1-9 修复的守卫项（阈值来源 9/11 设备验收记录），**回归即 FAIL**；单峰子项前置 U11
- **B19 拆分**：D4「compose 不改码率」的码率证据独立为 B19 子项，缺证据判 INCONCLUSIVE，**不反向收窄 B14**
- **B18 退化口径**：并发退化按 p99 相对同跑 solo 的百分比（关注 ≥100%、FAIL ≥150%）
- 判定 → B11–B19

### 8.3 test_67 事件枢纽与并发

- 覆盖：事件扇出（1/4/16 订阅者）、慢订阅者隔离、get_frame 扇出（1/2/4/8 客户端）、device hub 事件、混合负载
- **get_frame 扇出语义**：4 客户端起每客户端 ~15fps、8 客户端 ~7.5fps 是**帧配额（每客户端 outstanding ≤3）主导，by design**——4/8 客户端按记录级包络判，不作 FAIL
- **device hub lag 无效**：DeviceEvent.timestamp_ns 与 monotonic 不同钟域（偏差 ~20.7 天），lag 记录取消；hub 无丢弃计数器（journal-only 观测缺口）
- **B24 混合负载方法（完整公式；前置 U9）**：**同轮内 solo-before → mixed → solo-after 三段序列**；退化分母=前后两段 solo 的均值；**漂移门公式：drift_pct = |solo_after.p99 − solo_before.p99| ÷ solo_before.p99 × 100% ≤ 30%**，超差判该面 INCONCLUSIVE 重测；每面输出 `attempted / succeeded / failed` 三计数。U9 未落地前 B24 整项 INCONCLUSIVE（现测试无同轮 solo、异常被忽略）
- 判定 → B21–B24

### 8.4 perf_demo 双链路

- 链 A（subscribe 平台调度）：推理结果绑帧序烘入主流；指标=fps/dropped/annotate_errors/skew
- 链 B（keep-fd 自有管线）：取帧→DSP stretch→infer→自绘→40Hz 前置 pts 注入替换子流；指标=分阶段时延（pull/infer/hw_infer/draw/pub/e2e）/fps/frames_err
- **占空比方法门**：B 链烧录占空比达标依赖 **pacer 40Hz + pts=now+40ms 前置**（daemon 逐编码槽 bake、持有 1–2 帧未来帧吸收到达间隙；低频发布必闪断）
- **B33 测量方法（选定方案，前置 U5）**：**采集时直接计算 ROI 亮像素统计**（ROI=合成文本带/框区域，Y>200 且像素数 >150 判"该帧带合成像素"，随采样逐帧输出）；**空白对照**=无合成时段占空比应 ≈0（证明判据不误报）；**样本量 ≥200 编码帧**；可复核视频段存档为可选佐证。A 链以 Δbake_skips=0 判（FAIL 级）
- **B34 韧性三注入（手工操作，U6）**：
  1. SIGKILL app：`systemctl kill -s SIGKILL perf-demo` → 契约=systemd 拉起（NRestarts +1 为预期）后 **≤120s** 双链路重建健康
  2. 拔注入配置+重启 daemon：yaml 移除 `injection:` → `systemctl restart camera-daemon` → 契约=链 B **明确降级不崩**（日志含明确报因），链 A 经 epoch 重绑存活
  3. 恢复：还原 yaml → 重启/冷却 → 契约=**≤120s** 全量重建

  证据字段：NRestarts、journal 对应时段、`/run/aipc/perf-demo.json` 重建后首条、链路状态输出。**故障窗起止时间记入元信息表**（journal 分窗巡检依据，见 9.3）
- **同步 trace 采集（可选诊断窗，前置 U14）**：perf_demo 各 phase（稳态/占空比/韧性）可在**诊断复测窗**同步录系统 trace——板端 `hailo-soc-profiler applications noc-bandwidth-dsp-encoder-isp -t <Ns> -o <phase>.trace`（applications=进程/CPU 线程；noc-dsp-encoder-isp=NoC 带宽+DSP/编码/ISP 负载，H15H 建议带 noc 组）。trace 与时延采样**不同窗**（观测开销，10.15）：正式判定窗零旁挂。产物 .trace scp 回宿主机，装 `hailo-soc-profiler-ui_<V>_amd64.deb` 起 UI 服务后浏览器 ：15000 拖入离线读（同 9.6）；留档不设阈值
- 过夜 soak 可选（B35）：采样器 10min 周期，次晨补录
- **流适配**：按 5.3-2 盘点结果调整 `--a-stream/--b-stream`
- 判定 → B31–B35

### 8.5 契约门汇总

B2（编码流空载）/ B3（池驻留）/ C5（overlay 恒定开销→B08 子项）/ C7（strict 新语义）/ D3（注入不变量）/ D4（compose 不改码率帧率→B14+B19）/ E4（慢订阅者隔离）/ I1（长稳衰减）——门定义与实测格式见 9/12 报告"契约门汇总"节；模板第 7 章汇总回填。**C5/D5 证据原则**：契约门拆分为独立子项（帧率连续性 / 计算成本 / 码率），**哪个子项缺证据哪个标 INCONCLUSIVE，不以"收窄判定"降低原契约要求**；若决定调整验收范围，须测前修订并冻结（模板 1.5/第 11 章）。

## 9. L3：服务端资源面

### 9.1 REST 控制面延迟探针

- 工具：`scripts/perf_rest_latency.py`（`--socket` 默认 UDS 路径、`-n 100`、`--request-delay 0.05`、`--endpoints` 可选过滤）
- **端点四分类**（阈值按类给，见模板 C11–C14）：快读 / 已知慢路径（/monitor/summary、/monitor/disk、/ai/models——慢是路径构成非缺陷）/ 采样窗端点（/system/stats、/ai/stats，净开销口径同 A03）/ 跨服务链
- **GPIO 端点一律不打**（C15 审计）
- **成功判定（前置 U3）**：现版本仅传输异常计 err，**HTTP 4xx/5xx 计成功时延**——C16 的 err=0 不证明成功。修复后按三类错误判：传输错误 / HTTP 状态码错误（status_hist 须全 2xx）/ 业务错误（响应 error 字段空、data 在位，抽检）
- 面预检：UDS 优先，HTTP+Bearer 兜底（5.2；兜底未实现前仅 UDS 面可测）

### 9.2 观测面契约采样

- **GetStats**：采样窗与 clamp 契约见 3.2-④；per-model 字段（avg_latency_us/hw_fps/queue_depth/avg_skew_us/max_skew_us/skew_samples）与**系统级字段（device_utilization/cpu_utilization/dsp_utilization/device_temperature）在位（C21）**；系统级字段时间序列采样见 U12/M12（字段仅 ≥0 才置值，缺省记录间隙率；REST `/monitor` 的 NPU 读数走同一 GetStats 默认 500ms 阻塞窗——轮询用 SDK 小窗更优）
- **GetStreamStatus**（REST `GET /api/v1/media/status` 同源）：四层丢包计数（publisher 层 7 项 + bake 层 4 项 + 注入层 + SDK 层）；清窗 drain 后对账恒等式 `received + missing == packets_published delta` 精确成立（C22 硬性）。**注入层 frames_dropped 语义**：含**良性取代**（newest-due-wins 取代未到期帧）非丢显——异常丢弃判定须取稳态窗（剔除启动/epoch 切换/drain 边界）

### 9.3 journal 死信巡检（分窗巡检）

- 工具命令（**注意 `sh -s --` 参数终止符**，否则 `--since` 被 sh 当自身选项解析报 `Illegal option --`；远端日期参数保留引号）：

```bash
ssh root@<目标设备IP> 'sh -s --' < scripts/collect-perf-journal.sh \
    --since "<窗口起点>" --until "<窗口终点>" > journal-<窗名>.txt
```

- **分窗巡检**：`--since` 不会自动排除故障日志（它覆盖整个正式窗口）——巡检必须**逐窗执行**：①**稳态窗**（S2/S3/S5 各测量窗，起止锚点测时记入元信息表）：七类死信全零匹配（C31 判定窗）；②**故障窗**（S4 三注入，起止锚点同上）：只核对**预期事件清单**（camera-daemon 重启日志、`injection disabled` 明确报因、perf-demo systemd 拉起、epoch 重绑/会话替换）与意外死信——预期事件**不算死信**，意外死信单独归因
- 七类死信：strict_gate_milestones 异常 / FdPublisher quota·lease rejects / fd_client_drops / frame_router / frame_watchdog / dsp_service_warn / oom_history
- **共享组取证（A41）**：同批 journal 产物追加两查：①`grep -cE "OUT_OF_PHYSICAL_DEVICES|DEVICE_IN_USE|status=7[34]"`（判定窗内须=0）；②`grep "created shared VDevice (group="`——camera-daemon 与 ai-runtime 双侧组名一致且 =medialib `hailort.device-id`（出厂 `device0`）
- **有效性核对（前置 U4）**：脚本存在 `--since` 值未加引号的拆词 bug 且 `2>/dev/null` 屏蔽读取失败——**读取失败会呈现为零匹配**。每次执行后必须核对：①脚本退出码=0 ②输出窗口覆盖（首末条目时间戳覆盖该窗起止）③S0 记录的 journal 现存最早条目早于窗口起点
- 注意 event_hub_drops 为 Debug 级：**未记录 ≠ 未发生**，单独标注

### 9.4 环境长跑趋势

- `proc_sampler.py` 全程后台（`--interval 60`；**产物 JSONL**）：CMA 双池 / 双温区 / MemAvailable / daemon RSS
- 厂商口径互证快照（前置 U14，有则采无则跳过）：`hailo-dma-usage.sh -v`（官方 CMA/DMA 池占用快照，与 proc_sampler debugfs 读数互证口径差）；`dsp-utilization`（DSP 负载快照；记录级，基线待建）
- 判定 → C32–C37（绝对值：85°C 告警线、768M 池回落、MemAvailable 下限）
- **C37 口径**：daemon RSS 全程漂移按**进程生命周期分段**判（记录 PID/启动时间/重启数；S4 韧性注入产生的预期重启段剔除；**非预期重启=FAIL**，该段 RSS 统计单独标证据不足，其余段照常判）

### 9.5 函数级 CPU 归因（可选，py-spy）

**定位**：proc_sampler（9.4）只到**进程**粒度，perf_demo 分阶段时延（8.4）只到**链路段**粒度；需要回答"进程内部**哪个函数**吃的 CPU"时，对 Python 进程旁挂采样器做函数级归因。**记录级（C38）**：不设阈值、不参与判定，未执行标 N/A。

**适用面与工具**：

| 对象 | 工具 | 说明 |
|---|---|---|
| Python 进程（perf 套件各模块、perf_demo app） | **py-spy**（PyPI 有 aarch64 wheel；设备 venv `pip install py-spy`） | 不改代码、不重启进程，旁挂采样 |
| 原生 daemon（camera-daemon / ai-runtime / platform-api） | py-spy **不适用**（非 Python） | 镜像内有 `perf` 则 `perf record -g -p <PID>`；有 gdb 则 `gdb -batch -p <PID> -ex 'thread apply all bt'` 多次抓栈人工聚合；两者皆无则原生进程本轮不做函数级归因（元信息表登记工具缺口），仅 Python 进程覆盖 |

**三种用法**（设备上执行；`<PID>` = 被测进程）：

```bash
# 实时热点（现场排查）
py-spy top --pid <PID>
# 火焰图（标准归档产物；50Hz、60s）
py-spy record --pid <PID> -r 50 -d 60 -o <run_id>/归因/prof/<进程名>-<场景>.svg
# 一次性抓栈（CPU 瞬间飙升时看"正在干什么"）
py-spy dump --pid <PID>
```

**执行纪律（硬约束）**：

- **正式测量窗内禁止挂采样器**：py-spy 采样经 ptrace 短暂停起被测线程，会引入微抖动（proc_sampler 只是读 /proc、不触碰被测进程，两者性质不同）；基线判定轮（S2/S3/S4 正式窗）必须"零旁挂"纯净跑。函数级归因只在**归因轮**做：发现 CPU 异常（proc_sampler 进程 CPU 异常、M12 CPU 口径异常、链路阶段时延异常）后，**同一负载单独复现一次**、采样器旁挂
- 采样率 **50Hz** 足够（默认 100Hz 无增益、停顿翻倍）；`record` 采样窗 ≥60s 或覆盖一个完整负载周期
- **三角互证**：火焰图 top 函数 self% 与 proc_sampler 同窗该进程 CPU% 量级应一致；不一致先核对采样窗与负载对齐，再下结论
- ptrace 被拒（`kernel.yama.ptrace_scope`）：root 不受限；非 root 需同父进程或临时 sysctl 放开（测后还原）
- 归因轮产物归档到 `run_id/归因/`，**不与正式轮目录混放**（与 U10 同款纪律）

**典型触发场景**：

1. proc_sampler 显示某 Python 进程 CPU 异常（如 perf_demo 单核 >100%）→ 归因轮定位到函数（历史先例：9/14 报告链 B 画框环节 blend 固件拒绝、SDK 回退 CPU 绘制，单核 ~111%——进程级采样能发现异常，函数级才能点名到绘制函数）
2. M12 显示"推理时 CPU 远高于 NPU"→ 对套件/探针进程采样，分清平台开销落在哪个函数（RPC 序列化 / 像素拷贝 / GIL 竞争等）
3. 链 B 阶段时延（pull/infer/draw/pub）某段慢且疑似 CPU 忙等 → 采样确认该段是 CPU 密集还是等硬件

**产物与回填**：火焰图 SVG + self CPU top-10 函数表 → 模板 **C38**；通俗摘要问题清单可直接引结论（"CPU 大头是 XX 进程的 XX 函数"，12.1）。

### 9.6 系统级归因工作台（Hailo-15 厂商工具链，前置 U14）

**定位**：9.5 py-spy 回答"哪个 **Python 函数**吃的 CPU"；本节工具回答"**全系统**谁吃的 CPU/带宽/内存——NPU/DSP/硬件线程/内核调度无一遗漏"。两者是归因两板斧：**py-spy=进程内 Python 面，soc-profiler=系统面**；原生 daemon（camera-daemon/ai-runtime，非 Python）的归因只能走本节（9.5 适用面表已列其局限）。**全部 RECORD/诊断级**：不设阈值、不参与判定，工具缺失即省略（U14）。

**CPU 瓶颈标准归因两步法**（"CPU 容易满载"的定位流程）：

1. **第一步——raw_async 评估纯 NPU 帧率上限**：`hailortcli run2 --mode raw_async set-net <hef>`，input/output 不经 host 前后处理，稳态帧率即 NPU 拓扑天花板。与 SDK 链路实际帧率对比：raw_async 远高于实际 → 瓶颈不在 NPU 而在 CPU 侧（前后处理/拷贝/调度），进第二步；两者接近 → NPU 就是瓶颈，优化方向在模型而非代码。（与 7.6 第 4 条同源——M 系列逐模型必测）
2. **第二步——sched+applications trace 定位挤爆 CPU 的代码**：`hailo-soc-profiler sched applications -t <Ns> -o cpu.trace` 录包含内核调度与进程线程的 Trace，在 UI 里看 CPU 满载时段**每个核上跑的是什么线程**、就绪队列排队、被抢占的推理线程——精确定位是**哪一部分前/后处理代码挤爆了 CPU**（Python 侧预处理、C++ 拷贝线程、还是常驻进程抢核）。（"host 开销大"（7.6 第 5 条 full_async−raw_async 差值）到此细化到线程级）

**工具四类与用法**（在位性按 U14 矩阵）：

| 工具 | 用途 | 命令/流程 |
|---|---|---|
| hailo-soc-profiler | 系统级 Perfetto trace：applications（进程/线程 CPU）/ sched（内核调度）/ noc-bandwidth-dsp-encoder-isp（NoC 带宽+DSP/编码/ISP 负载，H15H 建议带 noc 组）/ mem_tracker（大块内存 alloc/free 时间线） | 板端 `hailo-soc-profiler applications sched noc-bandwidth-dsp-encoder-isp -t 10s -o <场景>.trace` → scp 回宿主机 → 查看器打开（见下） |
| hailo-soc-profiler-ui（Host） | .trace 查看器：内嵌 Perfetto 前端的本地 HTTP 服务（amd64 deb，如 1.12.0；与板端采集器**版本配对**，U14） | Host 装 deb 后 `hailo-soc-profiler-ui`（或 systemd 服务 `hailo-soc-profiler-ui.service`）起服务，默认 **:15000**（`-p` 改端口），浏览器打开拖入 .trace |
| hailortcli | NPU 基准与吞吐：`benchmark <hef>`（单模型 FPS/时延）；`run2` 双模式（7.6-4/5、两步法第一步）；`HAILO_MONITOR=1` + `hailortcli monitor -v` 实时温度/功耗/利用率（仅诊断窗，10.15） | 板端直跑；结果入 M 系列表 |
| DFC Model Profiler（PC） | 模型逐层离线剖析：PC 端 `hailo profiler <model>.har` 出 HTML 报告；板端 `run2 --mode raw set-net <hef> measure-fw-actions` 产 runtime_data_*.json，回 PC 联合 `hailo profiler <har> --runtime-data <json> --out-path <html>` | PC 端为主；har 不可得时记录省略 |
| 硬件子系统脚本 | `hailo-dma-usage.sh -v`（CMA/DMA 池，9.4 互证）；`dsp-utilization`（DSP 负载）；`. /etc/hailo_noc_perf.sh` 后 `noc_set_counter_filter` / `noc_measure_sleep` 读 NoC 带宽计数器 | 板端直跑；快照入趋势日志 |

**归因示例（run4 A15 案例回看）**：A15 resize 回归（per-call 371MB 池块反复重建，时延双峰 187.9ms）若有 mem_tracker track，能**直接看到大块 alloc/free 的时间线**，无需从时延双峰反推——内存 churn 型退化 mem_tracker 一眼定性；CPU 满载型退化看 sched+applications（两步法）；带宽型退化看 noc。

**执行纪律**：与 9.5 py-spy 同款——**正式测量窗零旁挂**（trace 写盘+计数器有观测开销，10.15），只在归因轮/诊断复测窗挂；trace 产物与火焰图同归 `run_id/归因/`，不与正式轮混放（U10 同款纪律）。

**在位性实测注记（D72，2026-09-17 链路验证）**：采集→传输→解析→UI 全链路已打通；**数据源分轨可用性**——①noc-bandwidth-\*/thermal 计数器**实测可用**（5s 窗 34.7 万样本，带宽型退化归因主力）；②**sched 在该固件内核不可用**（无 tracefs 实例，`CONFIG_FTRACE=y` 但 `mount -t tracefs` 报 unknown filesystem type）且**静默产空 trace**——两步法第二步的调度时间线在此固件上缺失，CPU 定位降级为 py-spy（9.5）+ noc/thermal 硬件面佐证，**内核补编 tracefs 列为固件跟进项**；③applications（track_event）负载下无 slices——Hailo 库侧打点未激活，跟进项；④空 trace ≠ 链路故障，判定前先看文件大小（<1KB 即空）。逐项最新状态见模板附录 C。

## 10. 已知问题与规避（坑清单）

逐条格式：现象 / 根因 / 规避 / 判定处理。**判定处理列即模板第 8 章豁免核对表的触发条件**（按四类归档：范围排除/已知缺陷/流程前置/统计口径）。

| # | 现象 | 根因 | 规避 | 判定处理 |
|---|---|---|---|---|
| 10.1 | `GET /device/gpio` 单次调用设备硬复位 | issue #46 已知缺陷 | 全链路禁调；端点清单静态审计 | A39/C15 零调用审计，任何调用作废窗口 |
| 10.2 | get_stats p50 ~536ms | 缺省请求 500ms 阻塞采样窗（3.2-④）；max 10.55s 为单次 ioctl 争用偶发 | 验收看净开销；不反复高频打 | A03 净开销口径；偶发 max 豁免 |
| 10.3 | subscribe「丢帧率」高（如 87.5%） | HAL 共享序号空间伪影（3.2-①） | 只按 fps 交付率判（事件/编码包序号不受此限） | A07/B31 按 fps；drop_pct 不判 |
| 10.4 | 固定 model_id 换模型文件后 infer -2799 | daemon 重复注册不校验 path + 自愈复活无主注册 | id 文件名派生；或注册前先注销（REST DELETE 会删模型文件，**禁用**） | 流程纪律，违反则重跑 |
| 10.5 | get_lens_status p99 203ms/max 2.2s | 该机镜头 af0832 bootstrap -2815 死循环（设备特有） | journal 查循环存在性 | A34 条件豁免（循环在→记录级） |
| 10.6 | PushFrame 全拒 error_code=-1 | `injection.enabled` 出厂 false | 测前开启+重启（备份 .bak），测后恢复 | B 系列前置条件，未开=无效跑 |
| 10.7 | 4K 全幅 REPLACE 池分配失败 | DSP 每客户端像素预算（240 MPix/s 量级；2026-09-17 根修自 120 提额——120 为 720p 时代校准值，4K@30 派生管线 ≈249 会被饿死） | 4K 用 OVERLAY(inset) | B13 预期豁免 |
| 10.8 | /monitor/summary ~543ms 等三端点慢 | 内联采集/跨服务链路径构成 | 归慢类阈值；UI 轮询建议加缓存（跟进项） | C12 已知慢路径，参考线判 |
| 10.9 | fps_limit=10 实交付 ~9.4 | pacing 语义=距上次完成 ≥间隔（完成链 ~6.4ms 叠加）；30fps 场景缺口放大到 ~16% | 按 90% 交付率契约判；修复方向已记录（打点改派发时刻） | A07 契约阈值 |
| 10.10 | 帧绑定恒判 too-old | 绑定空间用错（last_packet_seq vs HAL 共享序号，~2.5× 错位） | 从 FrameHandle.sequence 锚定 | B07 方法门 |
| 10.11 | 测量窗内结果抖动 | 并发操作/常驻负载干扰 | 测量窗独占；前后空闲基线；部署指纹记录 | 元信息表佐证 |
| 10.12 | 探针连不上/被拒 | 部署版本 REST 面差异（UDS vs HTTP+Bearer，兜底未实现） | 5.2 面预检先行 | C 系列前置条件 |
| 10.13 | 逐模型/分阶段跑丢上一轮报告 | 编排脚本每次运行删除设备端与宿主机模块报告并覆盖合并报告（防混跑设计） | 每次运行前归档到 `run_id/阶段/模块/模型/尝试` 独立目录（11.2） | U10 归档纪律；违反=证据缺失，相关项 INCONCLUSIVE |
| 10.14 | RegisterModel 的 model_variant 写短名（如 `yolov8n`）：**不报错但 post_result 永远为空** | variant 须写**真实 dlsym 符号**（如 `hailo_yolov8n`），短名静默失配（新固件交接确认） | variant 用插件真实符号名（get_model_info/插件清单核对） | 流程纪律 + **结果非空抽检**——err=0 掩盖不了空结果，推理项抽检 objects/post 输出非空（M03 的 err=0 不覆盖此坑） |
| 10.15 | 时延/吞吐验收样本被 trace/监控工具污染（系统性偏慢或抖动增大） | 观测者效应：soc-profiler trace 写盘 + NoC 计数器 + `hailortcli monitor` 均有开销（py-spy 的 ptrace 停顿同理，9.5 已列） | 正式判定窗（S2/S3/S4/S5）零旁挂；trace/monitor 只在归因轮/诊断复测窗挂（9.5/9.6 执行纪律） | 流程纪律，违反则相关窗重跑 |

工具层缺口（采样器各轮输出、REST 状态码判定、journal 引号与远端命令、占空比 ROI、单模块复跑、衰减算法、mixed solo 对照、原始样本/直方图、利用率探针、pb2 陈旧抢注、厂商工具链在位性）不在本表——见**第 6 章工具就绪清单 U1–U14**。另有部署面坑（scp 路径不对称、时钟偏移等）在部署清单维护，本表不重复。

## 11. 执行流程与时间预算

### 11.1 阶段时间表

| 阶段 | 内容 | 预计 | 上限 |
|---|---|---|---|
| S0 预检 | 指纹与可比性元信息 / .bak / injection 开启 / 流清单盘点 / 探针面预检 / 校准跑 | 45min | 60min |
| S1 空闲基线 | proc_sampler 后台挂起 + REST/CMA/温度快照 | 15min | 20min |
| S2 L1 套件 | test_60–64 默认参数（含 soak 1800s）+ M 系列模型矩阵（每模型 5–8min） | ~2.5h | 105min 预算和 + 矩阵 30–60min |
| S3 L2 套件 | test_65–67 默认参数 | ~1h | 70min 预算和+缓冲 |
| S4 perf_demo | 部署 + 双链路稳态 + 占空比 + 韧性 + M13/M14 逐模型窗（各 5×300s，独立窗纪律） | ~1h35m | 1h50m |
| S5 L3 采集 | REST 探针 + journal 分窗巡检 + 趋势回收 | 35min | 45min |
| S6 报告与恢复 | gen_perf_report + 脱敏审计 + 模板回填 + **通俗摘要（12.1）** + 回滚 | 35min | 45min |

### 11.2 执行纪律

- **合计 430min ≈ 7h10m**（未计复跑与 B35 过夜 soak；含 M13/M14 逐模型窗各 25min——M13 的 25min 此前未入总账，v1.13 一并 true-up）；含缓冲与 M 矩阵建议按 7.5h 排期
- **归档纪律（U10）**：S2 的 L1 / 逐模型 / S3 的 L2 等每次独立运行之间，**先把上一轮产物归档到独立目录再开跑**；**本次归档完成才允许下一次运行**；S6 按第 12 章产物清单合并并核对缺项
- **顺序刚性**：S4 韧性注入的重启必须发生在 S5 journal 巡检之前，且故障窗起止锚点记入元信息表供分窗巡检（9.3）；soak 进行中禁止其他操作；B35 过夜 soak 可选并行（不占当日窗口）
- 硬性项单次 FAIL 允许**复跑一次**，以复跑为准（首跑与复跑结果都保留，规则见模板 1.4）
- **首轮边界**：见第 1 章——首轮产出「基线建立报告」，验收判定自第二轮起
- **摸底裁剪（非验收）**：若目标只是摸底推理性能与 CPU/NPU 消耗，最小集 = S0 预检 + S1 空闲基线 + test_60 + M 系列（含 U12 探针、proc_sampler 10s）≈ 1.5–2h；L2/L3 与 soak 可后置。**正式验收仍须全流程**；摸底数据可复用为基线（经模板修订表转正）；需回答"进程内哪个函数吃 CPU"时加做 9.5 归因轮（每进程约 +10min，同负载复现）

### 11.3 分阶段命令矩阵

每行=完整命令 + 运行位置 + 前置 + 产物。占位符一律 `<目标设备IP>`。归档纪律见 11.2。

| 阶段（载体） | 完整命令 | 运行位置 | 前置条件 | 产物 | 工具就绪 |
|---|---|---|---|---|---|
| S0 校准跑（套件） | `PERF_SAMPLE_N=50 PERF_SAMPLE_ROUNDS=1 PERF_STREAM_S=10 PERF_SOAK_S=120 scripts/run-device-tests.sh <目标设备IP> 22 --perf` | 宿主机（编排 rsync → 设备执行） | 5.3 检查单全过 | dist/device-perf-report.json | — |
| S2 L1（套件） | `scripts/run-device-tests.sh <目标设备IP> 22 --perf`（仅 60–64：按 U7 裁剪 MODULES；跑前归档上一轮，U10） | 宿主机 | S1 基线快照 | report-test_6[0-4]*.json | U2（A08） |
| S2 M 系列（套件/探针） | `PERF_MODEL_FILE=/data/aipc-data/models/<模型文件> scripts/run-device-tests.sh <目标设备IP> 22 --perf`（仅 test_60；**逐模型归档**，U10） | 宿主机 | U1 适配清单落地 + 矩阵五族到位（7.6）+ **U12 preflight 通过** | 横向对比表 + 利用率 JSONL | U1、U12 |
| S3 L2（套件） | 同入口（仅 65–67：U7 裁剪；跑前归档 S2 产物，U10）；与 S2 之间重采空闲基线 | 宿主机 | S2 完成 | report-test_6[5-7]*.json | U9（B24） |
| S4 perf_demo（app） | 宿主机部署：`python/examples/perf_demo/deploy/install.sh root@<目标设备IP> 22 --model /data/aipc-data/models/hailo_yolov8n_384_640.hef -- --a-stream <A源> --b-stream <B源>`（SSHPASS 环境变量备用）；设备观测 `cat /run/aipc/perf-demo.json` | **宿主机**（install）/ 设备（app 运行） | 流适配（5.3-2）+ injection 已开 | perf-demo.json（fps/skew/分阶段时延） | U5（B33） |
| S4 B34 三注入（手工） | ①`systemctl kill -s SIGKILL perf-demo` ②yaml 去 injection + `systemctl restart camera-daemon` ③还原 yaml 重启 | 设备 | S4 稳态完成；故障窗起止记元信息表 | NRestarts / journal / 重建后首条 JSON | U6 |
| S4 M13 逐模型链 B（perf_demo） | 设备端：`cd /data/aipc/perf-demo && /data/venv-sdk/bin/python3 app.py --model-path /data/aipc-data/models/<模型文件> --chains b --zero-copy --b-fps 15 --duration 300 --samples-path <run_id>/m13/<模型名>-samples.jsonl`（**逐模型归档**，U10；五族逐个跑，7.6 M13） | 设备 | 7.6 M13 前置：U1 契约冻结 + 流适配 + injection 已开 | samples JSONL（出图帧率/e2e/分阶段） | —（app 自带 samples 落盘） |
| S4 M14 逐模型链 A（perf_demo） | 设备端：`cd /data/aipc/perf-demo && /data/venv-sdk/bin/python3 app.py --model-path /data/aipc-data/models/<模型文件> --chains a --a-stream <A源> --a-fps 10 --duration 300 --samples-path <run_id>/m14/<模型名>-samples.jsonl`（**逐模型归档**，U10；五族逐个跑，7.6 M14；**独立窗，禁与链 B 同窗**） | 设备 | 7.6 M14 前置：流适配 + injection 已开（同 B31） | samples JSONL + perf-demo.json（到达 fps/annotate_errors/bake_skips/skew） | —（app 自带 samples 落盘） |
| S5 REST 探针（探针） | 设备 venv：`python scripts/perf_rest_latency.py --socket /run/aipc/platform-api.sock -n 100 --request-delay 0.05`（HTTP 面加 `--base`+token） | 设备 | 面预检（5.2） | JSON（分位/status_hist/err） | U3（C16） |
| S5 journal（巡检） | 宿主机：`ssh root@<目标设备IP> 'sh -s --' < scripts/collect-perf-journal.sh --since "<稳态窗起>" --until "<稳态窗止>" > journal-稳态.txt`（故障窗同法另跑一份） | 宿主机发起、设备执行 | 各窗起止锚点已记（5.1） | 七类计数 + 样本行 | U4 |
| S1/S5 趋势（采样器） | 设备 venv：`python proc_sampler.py --output <路径>.jsonl --interval 60 --duration <秒> --run-id <id> --phase <phase>` | 设备 | — | **JSONL** 趋势记录 | — |
| 归因轮（可选，py-spy） | 设备 venv 装 py-spy 后：`py-spy record --pid <PID> -r 50 -d 60 -o <run_id>/归因/prof/<进程名>-<场景>.svg`（现场排查用 `py-spy top/dump --pid <PID>`） | 设备 | **同负载归因复现轮——正式测量窗禁挂**（9.5 纪律） | 火焰图 SVG + self CPU top-10 函数表 | — |
| S6 报告（生成器） | 宿主机：`scripts/gen_perf_report.py dist/device-perf-report.json` | 宿主机 | 全模块 JSON 齐（按 U10 归档合并，核对缺项） | Markdown 报告（IPv4 已脱敏） | — |

## 12. 产物清单与报告生成

| 产物 | 生成方式 | 去向 |
|---|---|---|
| 性能报告 Markdown | `scripts/gen_perf_report.py dist/device-perf-report.json` | SDK 仓 `docs/test-reports/<日期>-python-sdk-perf.md` |
| 分模块 evidence JSON | 编排脚本（**按 U10 归档**：`run_id/阶段/模块/模型/尝试`） | `dist/device-reports/report-test_6*.json` |
| REST 延迟 JSON | perf_rest_latency.py | 本地暂存，摘要回填模板 |
| journal 死信计数 | collect-perf-journal.sh（分窗产物 journal-<窗名>.txt） | 同上 |
| 环境趋势 JSONL | proc_sampler | 同上（52+ 样本 × 60s；M 系列窗 10s 粒度） |
| 利用率时间序列 JSONL | U12 探针（逐相分位：空闲/逐模型/并发） | 同上 |
| 占空比 ROI 统计 | video_observer 采集时计算（U5） | 同上 |
| 函数级 CPU 火焰图（可选） | py-spy record 归因轮（9.5） | `run_id/归因/prof/`；top-10 函数表回填模板 C38 |
| 交付率扫描曲线（可选） | 探针脚本 fps_limit 扫描（7.1 A07e） | 同上；各档交付 fps 回填模板 A07e |
| 验收文档（回填后） | 本方案配套模板 | 主仓 `docs/testing/` |

**提交前强制审计**：`grep -En '(192\.168|10\.0\.[0-9]|172\.(1[6-9]|2[0-9]|3[01])\.)' <全部产物>` 必须**零命中**（生成器内置 IPv4 脱敏只覆盖其产出，手工产物自查）。

**回填流程**：报告分区行名 → 模板验收项（映射见模板附录 A）；每项数据来源列必须写全（模块/报告分区/行名或 JSON 字段），缺来源的回填无效。

### 12.1 结果通俗摘要规范（每轮报告必含，一页）

报告与回填后的验收文档必须附一页**通俗摘要**（模板第 10 章有对应回填结构）——面向未参与测试的读者（管理/硬件/客户接口人），要求"不看正文也能知道结论"：

1. **结论先行**：一句话总评 + 四行红绿灯（L1 / M 系列 / L2 / L3；🟢 通过、🟡 有条件通过或关注、🔴 不通过、⚪ 证据不足），每盏灯配一句话理由——大白话，不用缩写术语（写"infer_e2e p99 超标"不行，写"单帧推理最慢的 1% 比参考慢 2.3 倍"可以）
2. **关键数字表（≤8 行）**：只挑决策相关的核心指标，指标名用通俗名（"单帧推理耗时"、"NPU 利用率"、"推理时整机 CPU"），对照参考线，超标用倍数直观表达
3. **问题清单按影响排序**：每条固定三句——现象 / 影响 / 建议；不堆原始数据、不写排查过程；CPU 类问题若做过函数级归因（9.5），结论直接点名到"XX 进程的 XX 函数"
4. 术语首次出现给一句话解释；原始数据与判据留在正文各表，摘要只放结论与关键数字；INCONCLUSIVE 项在摘要里写"没测成，原因一句话"，不写"INCONCLUSIVE"

## 13. 安全与合规

- 全文与产物**不得出现设备网络标识**（IP/主机名/序列号）：占位符 `<目标设备IP>`；基线引用只写报告文件名
- 仓库安全规约：文档提交须先经批准，文档与代码分开提交；不含凭据（ssh 口令、登录 token 等只存在部署清单）
- 回滚三件套（venv SDK / yaml / 二进制 .bak）位置记入模板元信息表，S6 执行恢复并记录

## 14. 附录

### 附录 A：验收项 ↔ 模块 ↔ 报告分区映射

单一事实源在**验收模板附录 A**，本文第 2 章总表与之对齐（A/B/M/C 编号可互相回指）。

### 附录 B：基线数据引用与设备归属声明

| 报告（SDK 仓 `docs/test-reports/`） | 设备归属 | 用途 |
|---|---|---|
| `20260909-python-sdk-perf.md` | **目标设备**（本方案验收对象） | **L1 时延类阈值唯一基线**（21 PASS，test_60–64） |
| `20260912-platform-perf.md` | **对照机**（部署验证机） | L2 时延类跨设备参考线（仅 WATCH 级）+ 契约门格式 + M 系列方法参考；含"推理面耗时分解"探针方法 |
| `20260914-perf-demo.md` | 对照机 | perf_demo（B31–B35）参考值与方法 |
| 9/11 P1-9 池驻留设备验收记录（`docs/proposals/composable-pipeline-contracts.md` 条目 9） | 目标设备 | **A15/B17 resize 池驻留基线**（9/9 报告该项为修复前双峰，不可用） |
| 主仓 `docs/benchmarks/ai-model-benchmark-hailo15h.md` | NPU 裸层（旧时间点） | M 系列 NPU 对照 |

**归属声明（防混淆）**：两份平台报告来自**两台不同物理设备**——9/9 报告是目标设备正式跑（其 to_jpeg85=162ms、lens 长尾、subscribe 缺席均为该机当时状态）；9/12 报告是对照机（to_jpeg85=672ms 量级、subscribe 连通）。**跨设备数值禁止直接互替**：9/12 的 4 倍差距项若误当目标设备基线，阈值将整体错 4 倍。

### 附录 C：校准参数表

| env | 默认 | 校准 | 作用 |
|---|---|---|---|
| `PERF_SAMPLE_N` | 300 | 50 | 每轮样本数 |
| `PERF_SAMPLE_ROUNDS` | 3 | 1 | 轮数（p50 取中位轮；仅 A04 等多轮项生效，见 4.1） |
| `PERF_STREAM_S` | 60 | 10 | 流式项窗口秒数 |
| `PERF_SOAK_S` | 1800 | 120 | soak 时长 |

编排脚本透传的 PERF_* 变量**仅上表 4 个**；`PERF_MODEL_FILE` 为 7.6/U1 执行准备项，落地时须同步扩展透传列表。
