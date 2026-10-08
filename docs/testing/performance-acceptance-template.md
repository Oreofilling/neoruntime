# NE503 AIPC（NeoRuntime）性能验收文档模板

| | |
|---|---|
| 版本 | v1.13（2026-09-17，配套方案 v1.13：新增 M14 逐模型链 A 到达帧率（记录级，`--chains a` 独立窗）；横向对比表 +1 列；B31 备注互链（fps≥8 按 yolov8n 校准，逐模型不适用）；验收项 90→91，**不新增硬性门，判定规则与既有阈值不变**） |
| 基线口径 | **截至 v1.13 / run5（2026-09-18，通过·有条件）**。release/v1.1.0（2026-09-29 验收）含 A07 subscribe 根因与信用桶根修等性能相关修复，本模板阈值在修复后基线上**待重标定**（v1.14 未出）；run4 评审遗留的阈值错配修订建议亦未落本版 |
| 配套文档 | [performance-test-plan.md](performance-test-plan.md)（测试方案：怎么做） |
| 本文档 | 验收标准 + 阈值 + 判定方法 + **空白结果模板**（怎么判） |
| 用法 | 复制本模板 → 按方案执行 → 逐项回填"实测值/结论/备注" → 三层判定 |

**版本历史**：

| 版本 | 日期 | 修订要点 |
|---|---|---|
| v1.13 | 2026-09-17 | 新增 M14 逐模型链 A 到达帧率（记录级，配套方案 v1.13）：perf_demo 链 A 逐模型 300s **独立窗**（`--chains a --a-fps 10`，禁与链 B 同窗——A/B 互抢 NPU/daemon 污染两链指标）；**到达 fps=结果到达样本数÷窗口时长**（subscribe 到达口径；annotate 调用被 --annotate-hz 节流，不作交付率指标）；结构预期=到达 fps≈min(配速×交付率, 1000/e2e p50)——轻模型配速受限、yolov5m 模型天花板受限 ~8fps（非调度缺陷）；yolo_world 双输入 N/A（链 A daemon 绑定无第二输入源，U1 缺口同 M13 归因）；横向对比表 +1 列；B31 备注互链（fps≥8 线按部署模型 yolov8n 校准，逐模型不搬用）；验收项 90→91，**不新增硬性门** |
| v1.12 | 2026-09-17 | M 矩阵 keypoint 槽位模型更换（配套方案 v1.12）：face_landmarks_lite → `yolov8s_pose.hef`（单阶段 COCO 17 关键点，Model Zoo v5.4.0）。D72 实测：NHWC [1,640,640,3] uint8 RGB 单输入（`get_model_info` 冻结）；benchmark 114.2 FPS / hw_fps 129.9 / 13.8MB；SDK 冒烟 err=0；HailoRT 5.3.0 兼容 5.4.0 编译 HEF（双侧验证）。矩阵行与注同步；**验收项数与阈值不变（90）** |
| v1.11 | 2026-09-17 | 厂商工具链整合（配套方案 v1.11 §9.6/U14），全部 RECORD/诊断级、不新增验收门：①M06 改逐模型**必测** `hailortcli benchmark` + `run2 --mode raw_async`（纯 NPU 上限，两值分列）；②M07 增 run2 双模式拆分（**host 前后处理开销=full_async−raw_async**，CPU 瓶颈定量证据）；③横向对比表 +2 列（raw_async FPS / host 开销）；④元信息表增厂商工具链指纹行（板端与 Host UI 版本配对）+ U14；⑤新增附录 C 可用性矩阵（S0 preflight 回填）；⑥C36 增 DSP 负载记录级（dsp-utilization，U14） |
| v1.10 | 2026-09-16 | 新增 M13 逐模型链 B 出图帧率（记录级）：perf_demo 链 B 逐模型 300s 窗（`--zero-copy --b-fps 15`），**出图帧率=published_unique÷窗口时长**（唯一源帧口径，40Hz 重复发布不计，与 M05 推理吞吐分开）；必带准入配速（不配速=帧积压失真）；同窗 e2e/分阶段样本口径；yolo_world 双输入 N/A（U1 缺口）；横向对比表增"链 B 出图 fps"列；验收项 89→90 |
| v1.9 | 2026-09-15 | 新固件（ai-runtime 单体，兄弟会话交接）口径回灌：①A07/A07e/B31 判读增加**固件轴**注记（到达驱动闸门实交付 ≈ 设定值，旧 9/14 skew/交付参考不可比，首轮重校）；②M07 增原生 perf 分段互证（**前置 U13**：pb2 重生成+抢注坑，未落地回退探针法不判 INCONCLUSIVE）；③新增 E16 流程前置（model_variant 短名静默失配，结果非空抽检）；④元信息表增 ai-runtime 部署纪律与预处理池三键；验收项数不变（89） |
| v1.8 | 2026-09-15 | 新增 A07e fps_limit 扫描（可选，记录级；10/20/25 + 补偿档 ≈23/≈30 交付率曲线；20/25 档 90% 契约不适用——pacing 算术，见方案 7.1）；验收项 88→89 |
| v1.7 | 2026-09-15 | 新增 C38 函数级 CPU 归因（可选，记录级；py-spy 归因轮，正式测量窗禁挂采样器——见方案 9.5）；验收项 87→88 |
| v1.6 | 2026-09-15 | 新增 A41 NPU 共享组一致性审计（环境门：73/74=0 + 双 daemon 首建日志组名一致）+ 第 7 章对应契约门；验收项 86→87 |
| v1.5 | 2026-09-15 | 结构整理：编号修正（前置门 1.2.1→1.3、复跑/修订顺移）、"结果通俗摘要"转正为第 10 章、修订记录/附录顺移；与方案 v1.5 章节重排同步全部交叉引用；验收项与阈值不变（86 项） |
| v1.4 | 2026-09-15 | 新增 M12 推理期利用率（前置 U12）+ 横向对比表利用率列、C21 扩系统级字段、新增通俗摘要回填结构、验收项 85→86 |
| v1.3 | 2026-09-14 | B16=10、B08/B19 拆分、A51 公式、M11 三分、C37=FAIL、U1–U11 前置、首轮二分 |
| v1.2 | 2026-09-13 | M 系列五族矩阵 |
| v1.1 | 2026-09-12 | 五级判定+有效执行前置门 |

---

## 1. 使用说明

### 1.1 填写流程

1. 测前：填写第 3 章验收元信息表（指纹/可比性元信息/基线快照/锚点/工具就绪）
2. 测中：按方案 S0–S5 执行（命令矩阵见方案 11.3），产物落盘并**按 U10 归档**（每次独立运行前归档上一轮）
3. 测后：按报告分区行名 → 附录 A 映射，逐项回填第 4–6 章
4. 每项**数据来源列必须写全**（模块 / 报告分区 / 行名或 JSON 字段）——缺来源的回填无效
5. 每项判定前先过 **1.3 有效执行前置门**——不成立判 INCONCLUSIVE，不回填达标结论
6. 核对第 8 章豁免表（每条已知问题是否触发）
7. 按第 9 章规则出综合判定
8. 回填第 10 章**结果通俗摘要**（一页红绿灯+大白话，面向未参与测试的读者；写法规范见方案 12.1）

### 1.2 判定五级

| 级别 | 含义 | 计入失败 |
|---|---|---|
| **FAIL**（硬性） | 不达标即该项失败 | 是 |
| **WATCH**（关注） | 不达标不计失败，但**必须**在备注列归因 | 否 |
| **RECORD**（记录） | 无阈值，仅记录建立数据点 | 否 |
| **EXEMPT**（豁免） | 已知问题触发条件下不判负（触发条件见第 8 章） | 否 |
| **INCONCLUSIVE**（证据不足） | **有效执行前置门（1.3）不成立**——数据不能证明达标也不能证明不达标 | 否（但硬性项存在 INCONCLUSIVE 时，该层不得判"通过"，见第 9 章） |

### 1.3 有效执行前置门（判 INCONCLUSIVE 的统一条件）

任一条件不成立 → 该项判 INCONCLUSIVE（备注写明缺哪条），整改后重测：

1. **场景已触发**：被测路径确有活动证据（strict 会话确实建立、PushFrame 有成功样本、注入确实开启）——字段全 0 且语义上不应全 0 视为**未驱动**，不是"全过"（B05 教训）
2. **样本量/时长达标**：按方案 4.1 **逐项采样表（当前实际值列）**核对，含例外项更低样本量：infer_batch 100×1、session_lifecycle/get_topic_stats 50×1、帧操作/accel A/B 30×1、publish 200×1、delivery/annotate 100×1、publish_batch 10×1、**写租约尝试 10 次**、流式项窗口满额、B33 ≥200 编码帧
3. **字段有效**：数据来源字段非默认零/缺报（hw 字段缺报按 E12 备注不判负；其余缺报须归因或判 INCONCLUSIVE）
4. **链路持续产果**：fps>0、迭代数>0、产物非空
5. **soak 模式有效**：A51/A53 须 evidence `mode=full`；`media-only (infer degraded)` 判 INCONCLUSIVE
6. **工具就绪前提**：依赖方案第 6 章工具项的验收项，对应缺口未解决且无登记的绕过方式 → 判 INCONCLUSIVE。对应关系：**U2**→A08/A52 各轮 err；**U3**→C16 三类错误；**U4**→C31 巡检有效性；**U5**→B33 ROI 统计；**U8**→A51 重算原料（分桶序列本身即原料，U8 指内置算法不作判据）；**U9**→B24 同轮 solo；**U11**→A15/B17 单峰子项；**U12**→M12 利用率时间序列（字段缺省且代理数据也不可得时）；**U13**→M07 原生 perf 分段（未落地回退 9/12 探针法，**不判 INCONCLUSIVE**）

### 1.4 复跑规则

硬性（FAIL 级）项单次不达标允许**复跑一次**，以复跑为准；**首跑与复跑结果都保留**（首跑写备注，不覆盖；分目录归档见方案 U10）；两次方向相反（一次过一次不过）必须在备注归因，无法归因时该项按未过处理并升级待查。WATCH/RECORD/INCONCLUSIVE 不复跑（INCONCLUSIVE 整改后重测是新数据，同规则留痕）。

### 1.5 阈值修订机制与首轮边界

本模板阈值为 **v1**。凡标注"基线待建"的项（跨设备参考线），首轮正式跑后以目标设备实测转正：登记第 11 章修订记录表（编号/原参考线/新基线阈值/依据/批准人），后续轮次按新值判。**验收范围调整（含子项拆分后的范围变化）同样须测前修订并冻结，不允许测中以"收窄判定"降低已冻结的契约要求。**

**首轮边界**：首轮正式跑的产出是「**基线建立报告**」，报告终态二选一——「**基线建立完成（全项）**」或「**基线建立（含缺项）**」（后者附缺项清单与补测计划）；综合判定只出「有条件通过（附整改）/ 不通过 / 证据不足」之一；「**性能验收通过**」自第二轮起、按冻结阈值与冻结工具版本判（见第 9 章）。

## 2. 阈值推导策略总述

### 2.1 三档基线策略（设备归属声明）

| 报告 | 设备归属 | 用作 |
|---|---|---|
| SDK 仓 `docs/test-reports/20260909-python-sdk-perf.md`（下称 **9/9 报告**） | **目标设备**（本验收对象） | L1 时延类**唯一基线** |
| SDK 仓 `docs/test-reports/20260912-platform-perf.md`（下称 **9/12 报告**） | **对照机**（另一台物理设备） | L2 时延类**跨设备参考线**（仅 WATCH 级，基线待建）+ 契约门格式 |
| SDK 仓 `docs/test-reports/20260914-perf-demo.md`（下称 **9/14 报告**） | 对照机 | B31–B35 参考值 |
| 9/11 P1-9 池驻留设备验收记录（SDK 仓 `docs/proposals/composable-pipeline-contracts.md` 条目 9，下称 **9/11 记录**） | 目标设备 | A15/B17 resize 池驻留基线（9/9 报告该项为修复前双峰数据，**不可用**） |

**防混淆声明**：9/9 与 9/12 来自两台不同设备，数值差异可达 4 倍（如 to_jpeg85：162ms vs 672ms）。**跨设备数值禁止直接互替**；L1 阈值一律从 9/9（目标设备）推导。

### 2.2 阈值公式

| 类别 | FAIL（硬性） | WATCH（关注） |
|---|---|---|
| 时延类（有目标设备基线） | ceil(基线 × 1.5) | ceil(基线 × 1.3) |
| 时延类（仅跨设备参考） | 基线待建（首轮不设 FAIL） | ceil(参考值 × 1.3)，跨设备参考线 |
| 吞吐/容量类 | ≤ 基线 × 0.85 | < 基线 × 0.9 |
| 退化比例类（并发前后） | ≥ +150% | ≥ +100% |
| 契约/不变量类 | 契约绝对值（丢帧=0、missed=0、fps≥29.5 等） | 视项另定 |
| 无基线绝对值 | 依枚举依据（85°C 告警线 / 768M 池回落 / MemAvailable≥4000MB / 500ms 采样窗净开销≤200ms） | — |

取整规则：时延阈值向上取整到整数 ms；小基线项（<6ms）×1.3 与 ×1.5 取整后可能同值，此时 WATCH 无独立裕量、直接以 FAIL 线判。
口径一致性：`gen_perf_report.py` 的内置标注（p99/p50>8×、err>0.5%、soak 衰减>10%）是**报告标注，不等于本表验收阈值**。

## 3. 验收元信息表（测前填写）

| 项 | 值 |
|---|---|
| 验收日期 / 执行人 / 复核人 | |
| 目标设备标识（占位 `<目标设备IP>`）+ 匿名设备编号 | |
| 系统与驱动版本（uname / 内核 / SoC 固件指纹） | |
| CPU 调频策略（governor）与环境温度起点 | |
| SDK 版本（设备 venv） | |
| SDK 仓 commit / 工作区状态 / **未提交补丁摘要** | |
| 部署二进制指纹（按部署清单，含 md5；**覆盖 ai-runtime 前 systemctl stop**——运行中覆盖报 text file busy）+ ai-runtime.yaml 预处理池三键（`stream_dsp_preprocess` / `stream_preprocess_slots` / `stream_preprocess_job_ms`，逐轮记录取值） | |
| 被测模型清单（文件名 + md5，M 系列五族矩阵） | |
| 流配置清单（list_streams 输出摘要） | |
| `injection.enabled` 测前 / 测后 | |
| 常驻服务清单 | |
| 空闲基线快照（CPU/CMA 双池/温度/MemAvailable/daemon RSS） | |
| journal 锚点（正式跑起始本地时间 + 设备时钟偏移 + journal 现存最早条目） | |
| **测量窗锚点（S2/S3/S4 稳态窗与 S4 故障窗各自起止——journal 分窗巡检输入）** | |
| **归档目录（run_id/阶段/模块/模型/尝试，U10）** | |
| 回滚三件套位置（venv SDK / yaml .bak / 二进制 .bak） | |
| M 系列模型矩阵盘点清单 | |
| **厂商工具链指纹（U14）**：板端 hailo-soc-profiler / hailortcli / dsp-utilization / hailo-dma-usage.sh / `/etc/hailo_noc_perf.sh` 版本 + Host 侧 hailo-soc-profiler-ui deb 版本（**板端采集器与 UI 配对登记**） | |
| 工具就绪状态（方案第 6 章 **U1–U14** 逐项：已解决/绕过方式；含 U12 利用率 preflight 间隙率、U13 pb2 重生成验证、U14 可用性矩阵回填附录 C；可选 9.5 py-spy / 9.6 厂商工具链：已装/不可装） | |

## 4. L1 验收项总表（A 系列 + M 系列）

### 4.1 A 系列

判定方法缩写：**双达标**=p50 与 p99 均须 ≤ 阈值；**计数**=事件计数；**契约**=不变量成立。

| 编号 | 验收项 | 指标与统计口径 | 硬性阈值 FAIL | 关注线 WATCH | 判定方法 | 数据来源 | 实测值 | 结论 | 备注/豁免依据 |
|---|---|---|---|---|---|---|---|---|---|
| A01 | list_models 时延 | p50/p99，中位轮 | p50≤6 / p99≤13 | p50≤6 / p99≤11 | 双达标 | test_60→P1 `list_models` | | | |
| A02 | get_model_info 时延 | p50/p99 | p50≤6 / p99≤12 | p50≤5 / p99≤11 | 双达标 | test_60→P1 `get_model_info` | | | |
| A03 | get_stats 净开销 | 净开销=p50−500ms（**缺省请求**采样窗 500ms：sampling_window_ms=0 → 服务端缺省；SDK 若显式传窗，须记录窗口值换算） | 净开销≤200ms | 净开销≤100ms | 双达标 | test_60→P1 `get_stats` | | | 偶发 max（历史 10.55s 单次）豁免，须备注 |
| A04 | infer e2e（bytes 单帧） | p50/p99 | p50≤21 / p99≤27 | p50≤18 / p99≤23 | 双达标 | test_60→P1 `infer_e2e` | | | |
| A05 | infer_batch(4) | p50/p99（100×1 采样） | p50≤51 / p99≤70 | p50≤45 / p99≤61 | 双达标 | test_60→P1 `infer_batch_4` | | | |
| A06 | 会话注册/注销往返 | p50/p99（50×1 采样） | p50≤11 / p99≤19 | p50≤10 / p99≤17 | 双达标 | test_60→P1 `session_lifecycle` | | | |
| A07 | subscribe 流式交付率 | fps_limit=10 的实交付 fps；**丢帧只按 fps 判**（HAL 序号场景序号连续性法不可用；事件/编码包序号不受此限） | fps≥9.0（90% 交付契约） | fps≥9.3 | fps 计数 | test_60→P1 `subscribe_10fps` | | | pacing 语义=距上次完成≥间隔，非缺陷（方案 10.9）——**旧固件行为**；新固件（到达驱动闸门）实交付 ≈ 设定值、skew 大幅下降，判读按部署指纹固件轴、首轮重校 |
| A07e | fps_limit 扫描（**可选**） | 各档实交付 fps（每档 60s 窗，**30fps 源流**）+ 交付率曲线；**pacing 预期线=1÷(1000/fps_limit+6.4ms)**：10→~9.4 / 20→~17.7 / 25→~21.6 | —（记录级，基线待建） | — | RECORD | 探针脚本扫描输出（方案 7.1 可选扩展） | | | **前置**：fps_limit 套件写死 10——须探针/参数化（未落地标 N/A）；**20/25 档 90% 契约不适用**（算术上限 86–89%）；要实际 20/25 用补偿档 ≈23/≈30（首轮实测校准）；gap 判读周期按档换算（100/50/40ms）；15fps 源流扫 20/25 无意义；**预期线公式=旧固件（完成锚点）口径**——新固件到达驱动下预期 ≈ 设定值（缺口主要来自源流/NPU 上限）、补偿档不需要，首轮重校 |
| A08 | L1 全模块错误率 | err 计数（时延分布不含错误样本；**各轮**，非仅中位轮） | err=0（各轮） | — | 计数 | test_60–64 全部分区 | | | **前置 U2**：采样器 evidence **只存中位轮统计、无各轮 JSON**——"人工核对各轮"不可行；U2（输出全轮 err）落地前本项判 INCONCLUSIVE |
| A11 | get_frame 主流 | p50/p99 | p50≤50 / p99≤56 | p50≤44 / p99≤48 | 双达标 | test_61→P2 `get_frame_main` | | | |
| A12 | frame.to_rgb | p50/p99（30×1 采样） | p50≤12 / p99≤28 | p50≤10 / p99≤24 | 双达标 | test_61→P2 `frame_to_rgb` | | | |
| A13 | frame.resize 半幅 | p50/p99 | p50≤14 / p99≤21 | p50≤12 / p99≤19 | 双达标 | test_61→P2 `frame_resize_half` | | | |
| A14 | frame.to_jpeg85 | p50/p99 | p50≤244 / p99≤251 | p50≤211 / p99≤218 | 双达标 | test_61→P2 `frame_to_jpeg85` | | | 阈值来自目标设备基线（对照机为 672ms 量级，禁互替） |
| A15 | accel resize（default，池驻留后） | p50/p99 + **尾部计数**（主判据）+ **单峰子项**（直方图/原始样本） | **≥43ms 样本=0**（P1-9 回归守卫，主判据，可判）；单峰子项：直方图无 p50×4 以上孤立簇 | p50≤12 / p99≤47 | 尾部计数（主）+直方图（子项） | test_61→P2 `ab_resize_nv12_default` | | | **基线=9/11 记录**（p50 8.0/p99 35.7/max 37.4）；**单峰子项前置 U11**：采样器无原始样本/直方图产物，落地前单峰子项判 INCONCLUSIVE（尾部计数与 WATCH 线仍可判） |
| A16 | accel resize（swonly）+ A/B 比值 | swonly p50/p99；比值与路由证据记录 | p50≤6 / p99≤15 | p50≤6 / p99≤13 | 双达标；比值 RECORD | test_61→P2 `ab_resize_nv12_swonly` + probes/health JSON | | | 比值>1 计价 socket 传输非算力，不作加速比结论 |
| A17 | accel encode_jpeg | default 与 swonly 各 p50/p99 | default p50≤309/p99≤423；swonly p50≤238/p99≤240 | default p50≤268/p99≤366；swonly p50≤206/p99≤208 | 双达标 | test_61→P2 `ab_encode_jpeg_default`/`_swonly` | | | |
| A21 | publish 时延 | p50/p99（200×1 采样） | p50≤5 / p99≤11 | p50≤5 / p99≤10 | 双达标 | test_62→P3 `publish` | | | |
| A22 | publish_batch(100) | p50/p99（10×1 采样） | p50≤165 / p99≤197 | p50≤143 / p99≤171 | 双达标 | test_62→P3 `publish_batch_100` | | | |
| A23 | 订阅交付 e2e | p50/p99（100×1 采样） | p50≤6 / p99≤10 | p50≤6 / p99≤8 | 双达标 | test_62→P3 `delivery_e2e` | | | |
| A24 | get_topic_stats | p50/p99（**50×1** 采样） | p50≤3 / p99≤6 | p50≤3 / p99≤5 | 双达标 | test_62→P3 `get_topic_stats` | | | |
| A25 | 10Hz 发布到达流 | fps/丢帧/gap（按 fps 交付率判丢帧；事件序号可对账） | 丢帧=0 且 fps≥9.5 且 gap_max≤200ms（2 帧周期） | gap_p95≤110ms | 契约 | test_62→P3 `arrival_10hz` | | | |
| A31 | get_capabilities | p50/p99 | p50≤3 / p99≤9 | p50≤3 / p99≤8 | 双达标 | test_63→P4 `get_capabilities` | | | |
| A32 | get_sensor_info | p50/p99 | p50≤10 / p99≤14 | p50≤9 / p99≤12 | 双达标 | test_63→P4 `get_sensor_info` | | | |
| A33 | get_stream_status | p50/p99 | p50≤5 / p99≤9 | p50≤4 / p99≤8 | 双达标 | test_63→P4 `get_stream_status` | | | |
| A34 | get_lens_status | p50/p99；**条件豁免** | 豁免未触发：p50≤13 且 p99≤21（p99 为健康参考线）；豁免触发：RECORD | p50≤11 | 双达标 / RECORD | test_63→P4 `get_lens_status` + journal | | | 豁免条件=journal 证实 af0832 bootstrap -2815 循环（设备特有，见第 8 章 E5） |
| A35 | get_autofocus_status | p50/p99 | p50≤6 / p99≤8 | p50≤5 / p99≤7 | 双达标 | test_63→P4 `get_autofocus_status` | | | |
| A36 | get_hardware_status | p50/p99 | p50≤25 / p99≤28 | p50≤22 / p99≤25 | 双达标 | test_63→P4 `get_hardware_status` | | | |
| A37 | get_infrared_status | p50/p99 | p50≤4 / p99≤8 | p50≤3 / p99≤7 | 双达标 | test_63→P4 `get_infrared_status` | | | |
| A38 | 设备事件流到达 | 帧/时长 | EXEMPT（预期 0 帧） | — | RECORD | test_63→P4 `device_event_arrivals` | | | 服务端 SubscribeEvents 未实现桩（见第 8 章 E9）；真设备事件走 event-bus（A21–A25 面） |
| A39 | GPIO 零调用审计（负面门） | 窗口内 GPIO 调用数（端点清单静态审计 + journal 佐证） | **=0**（任何调用作废窗口） | — | 计数 | 端点清单 + journal | | | issue #46：单次 GET 硬复位 |
| A41 | NPU 共享组一致性审计（环境门） | 判定窗 journal 两查：①`OUT_OF_PHYSICAL_DEVICES(74)`/`DEVICE_IN_USE(73)` 匹配数；②camera-daemon 与 ai-runtime 各自首建日志 `created shared VDevice (group='…')` 的组名——双侧一致且 =medialib `hailort.device-id`（出厂 `device0`） | ①=0 且 ②成立（任一不成立即 **FAIL**——全部推理项隐含依赖跨进程共享调度） | — | 计数+比对 | journal 取证（与 C31 同窗产物复用） | | | 2026-09-15 实测定格：组失配在 `VDevice::create` 当场 74 拒绝（hailort_server service 面 `requested: 1, found: 0`；73 为非 service 直连码）；HAL `kSharedVDeviceGroupId` = medialib device-id；ai-runtime.yaml 的 `scheduler.group_id` 死键已清除 |
| A51 | soak 速率衰减（**窗口法，参数固定**） | 从 P5 分桶序列重算，参数如下：①预热剔除：t≤120s 桶不计；②有效段=首个 t>120s 桶→末桶；③窗长 L=min(300s, 有效段时长÷2)；④首窗=有效段起点起 L、尾窗=有效段终点往前 L（尾段不足 L 并入尾窗）；⑤r=窗内迭代速率=迭代数÷窗时长；⑥**首尾窗衰减=(r_first−r_last)÷r_first×100%**；⑦最差窗=以 L 为窗、按分桶宽度步进滑动取最低 r，**最差窗衰减=(r_first−r_worst)÷r_first×100%**；有效段不足 2L 时判 INCONCLUSIVE | 首尾窗衰减 <10% | 首尾窗 <5%；最差窗衰减 >2× 首尾窗衰减须归因（首轮建立最差窗参考） | 双达标 | test_64→P5 分桶序列（重算） | | | 套件内置"首段 vs 其余均值"仅参考不作判据（U8）；有效性前提：mode=full；基线 0.68%（旧口径，新口径首轮建立） |
| A52 | soak 错误 | err 计数 | =0 | — | 计数 | test_64→P5 | | | 同 A08 的 U2 前提（各轮 err 不可得时 INCONCLUSIVE） |
| A53 | soak 客户端 RSS/fd 漂移 | ΔRSS（**统一基准点：首个 t>120s 桶 → 末桶**——"首分钟预热不计"与"首桶预热剔除"为同一判据）；fd 增量 | ΔRSS≤100MiB 且 fd≤+10 | ΔRSS≤50MiB | 双达标 | test_64→P5 分桶序列 | | | 基线 Δ13.2MiB；mode=full 前提 |
| A54 | soak daemon RSS 漂移 | 四 daemon 各自 ΔRSS（soak 窗内无重启为前提；有重启分段判） | 各 <30MiB | 各 <15MiB | 计数 | test_64→P5 daemon 表 | | | 基线：ai-runtime +0.84MiB、camera-daemon −0.72MiB |

### 4.2 M 系列（多模型横向对比）

执行载体与方法见方案 7.6（M01–M12：`PERF_MODEL_FILE` 覆盖或探针脚本；M13/M14：perf_demo 链 B/链 A 逐模型窗口——方案 11.3 M13/M14 行；同口径采样协议；**前置 U1 模型适配清单**，覆盖 M01–M14）。首轮回填后即为各模型基线，经修订表转正。

**模型矩阵（五族，每族一个；NPU 参考=主仓 `docs/benchmarks/ai-model-benchmark-hailo15h.md`，2026-05-18，batch=1）**：

| 系列 | 模型文件 | 输入契约（**参考值；正式以 `get_model_info` 实测冻结**——文件名解析不可靠，实测 yolo_world_v2s_540→960×540、tiny_yolov4 无法解析回退摄像头原始尺寸） | NPU 参考 | 现状/来源 |
|---|---|---|---|---|
| 轻量单输入 NV12 检测 | `hailo_yolov8n_384_640.hef` | NV12 640×384 | 444.9 FPS / 2.25ms / 2 contexts / 4.7MB | 设备已有 |
| 中型大几何检测 | `yolov5m_vehicles.hef` | 1080×1920（F8CR） | 46.6 FPS / 21.5ms / 5 contexts / 18MB | 本地工作区补拷 |
| 零样本双输入 | `yolo_world_v2s_540.hef` | image（文件名解析=960×540，**以实测为准**）+ text embedding [1,1,80,512] | （未含于 5/18 报告；对照机 SDK 链路 ~42–47 FPS） | 设备已有 |
| 超轻量 RGB 检测 | `tiny_yolov4_license_plates.hef` | RGB 416×416（文件名**无法解析**——须实测冻结） | 908.4 FPS / 1.10ms / 1 context | 本地工作区补拷 |
| 姿态/关键点（单阶段） | `yolov8s_pose.hef` | **NHWC [1,640,640,3] uint8 RGB 单输入**（D72 `get_model_info` 实测冻结 2026-09-17；文件名无法解析几何；9 输出=3 尺度×box64+conf1+kpts51，COCO 17 关键点） | 114.2 FPS / ≈8.8ms / 13.8MB（D72 benchmark 2026-09-17；daemon hw_fps 129.9） | Model Zoo **v5.4.0**（hailo15h）；D72 已入 `models/keypoint/` 并实测载入——目标设备 S0 同步补拷 |

注：yolov8s_pose 为单阶段单输入（RGB 640×640，2026-09-17 更替 face_landmarks_lite，其两阶段级联口径作废）——`infer_batch` 路径可用，不受双输入批量 U1 缺口约束（该缺口仅影响 yolo_world，不可用标 N/A 并归因）。

**横向对比表（每模型一行；v1.3 列口径修订）**：

| 模型文件（id=文件名派生） | 输入契约（几何/格式，实测冻结） | e2e p50/p90/p99/max | keep-fd e2e p50（可选） | batch(4) p50/p99 与每帧比 | GetStats avg_latency_us / **hw 时延倒数 ÷ NPU FPS（参考比）** | **实际吞吐（完成推理数÷窗口时长）** | **链 B 出图 fps（M13：published_unique÷窗口，配速 15）** | **链 A 到达 fps / annotate_errors（M14：--chains a 独立窗，a-fps 10）** | **推理期利用率 NPU%/CPU%（U12 时间序列分位）** | NPU 纯执行（benchmark FPS / **raw_async FPS**，U14） | 平台开销 ms 与占比（开销÷e2e） | **host 开销 ms（full_async−raw_async，U14）** | 加载时延（冷加载/缓存命中/首次推理） | 文件大小/contexts/RSS 增量 | err | 结论 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| | | | | | | | | | | | | | | | | |

| 编号 | 验收项 | 指标与统计口径 | 硬性阈值 FAIL | 关注线 WATCH | 判定方法 | 数据来源 | 实测值 | 结论 | 备注/豁免依据 |
|---|---|---|---|---|---|---|---|---|---|
| M01 | 模型矩阵盘点 | /data/aipc-data/models 全部 HEF 清单（几何/格式/大小）+ 五族到位核对 | 五族全部到位（缺=补拷后复测） | — | 清单核对 | 盘点命令输出 | | | 矩阵从本地工作区补拷扩大（文件不入库，md5 记元信息表） |
| M02 | 逐模型 infer e2e | 每模型 p50/p99（输入按冻结契约） | 基线待建（首轮不设） | p50 ≤ NPU latency + 20ms（跨设备参考） | 双达标 | 横向对比表 | | | 参考量级：对照机 yolov8n e2e 16.7 − NPU 2.25 ≈ 开销 14.5ms |
| M03 | 逐模型错误率 | err 计数 | **err=0（每模型）** | — | 计数 | 横向对比表 | | | 输入契约不满足的 -2799/-2811 是**流程错误**非性能——整改输入后重跑，不得记豁免 |
| M04 | infer_batch(4) 横向 | 每模型 p50/p99 与单帧比值 | 基线待建 | — | RECORD | 横向对比表 | | | 参考比值：对照机 38.4ms/4=9.6ms/帧（比单帧省 36%）；双输入批量受 U1 缺口约束 |
| M05 | GetStats per-model 对照 | avg_latency_us 与 e2e 一致性；**hw 时延倒数**（=1÷avg_hw_latency，HAL 理论峰值语义，非交付吞吐）；**参考比=hw 时延倒数 ÷ NPU FPS**；**实际吞吐=完成推理数÷窗口时长（单独计）** | 字段在位 | 偏差 ≤±30% | 一致性 | GetStats 采样 | | | hw 字段对部分模型族不上报（HAL latency 标记，E12），缺报备注不判负；**实际吞吐不用 hw 字段替代**；系统级利用率字段见 M12/U12 |
| M06 | NPU 纯执行对照（**前置 U14**） | 逐模型 `hailortcli benchmark <hef>`（当前镜像版本）；`run2 --mode raw_async set-net <hef>` 稳态帧率（**纯 NPU 上限**，input/output 不经 host 前后处理） | — | — | RECORD | 复测输出（两值分列：benchmark FPS / raw_async FPS） | | | 5/18 报告仅作历史参考不再直接引用；工具缺失（U14 矩阵）标省略不判 INCONCLUSIVE；raw_async=CPU 瓶颈两步法第一步（方案 9.6） |
| M07 | 平台开销分解 | e2e − NPU；四块分解（可选，方法见 9/12 报告补充章节）；**run2 双模式拆分（前置 U14）：host 前后处理开销 = full_async 帧时延 − raw_async 帧时延**（差值占比过大="前后处理挤爆 CPU"定量证据，下钻走方案 9.6 两步法第二步）；**新固件原生分段 `StreamInferResponse.perf`**（repack/dsp/queue/infer/post/write，**前置 U13**）可与探针法互证 | — | — | RECORD | 探针输出 + run2 双模式输出 + 原生 perf 分段（U13 落地时） | | | U13/U14 未落地不阻塞——回退 9/12 探针法，不判 INCONCLUSIVE |
| M08 | 资源占用 | 模型大小/contexts/加载后 ai-runtime RSS 增量 | — | RSS 增量 <100MiB/模型 | RECORD | 横向对比表 | | | |
| M09 | 多模型共存交替（可选） | max_model_cache=3 下交替推理的切换影响 | — | — | RECORD | 探针输出 | | | 可选项，不执行标注 N/A |
| M10 | batch on/off 对照（可选） | 同模型 batch(4) vs 4×单帧 | — | — | RECORD | 探针输出 | | | 可选项 |
| M11 | 模型加载时延（**三分口径**） | ①**冷加载**=重启 ai-runtime 或注册服务端未加载文件后的 register→ready；②**缓存命中**=同文件再注册的 register→ready（服务端对已加载模型与**同文件别名**都会复用跳过加载——不区分口径会只测到一次注册查询）；③**首次推理**=冷加载后首次 infer e2e − 稳态 p50 | 基线待建 | — | RECORD | 横向对比表 + **注册前后 `list_models` 清单 + journal（already loaded, skipping）** | | | 三段分别记录；频繁换模型场景关注项（与 M09 互补）；与文件大小/contexts 数对照 |
| M12 | 推理期 NPU/CPU 利用率（**前置 U12**） | 推理窗内 `device_utilization`（NPU 真实利用率，HAL nnc 计数器经 GetStats 归一 0–1）时间序列分位；CPU **两口径分开记**（GetStats SoC 口径 `cpu_utilization` / proc_sampler Linux 口径 ai-runtime 进程 CPU%，**不同源不混写**） | 基线待建（首轮建立参考） | — | RECORD | U12 探针 JSONL + proc_sampler（M 窗口 10s 粒度） | | | 字段缺省窗回退**推导代理**：NPU busy% ≈ 实际吞吐 × avg_hw_latency（忙时占比下界）；U12 preflight 间隙率记元信息表；并发下的饱和信号=hw latency 膨胀 |
| M13 | 逐模型链 B 出图帧率 | perf_demo 链 B（keep-fd→DSP resize→infer→render/blend→publish）逐模型 300s 窗（`--zero-copy --b-fps 15`）：**出图帧率 = published_unique ÷ 窗口时长**（唯一源帧口径，40Hz 重复发布与 zero-copy push_slot 均不计新帧）；同窗 e2e p50/p90/p99（样本口径，非 console 窗口均值）+ 分阶段（resize/infer/render/blend/pub） | 基线待建（首轮不设） | —（结构预期见备注，不作 WATCH 线） | RECORD | perf-demo samples JSONL（方案 11.3 M13 行） | | | 前置=U1 契约冻结 + injection 已开 + 流适配（同 B31）；**口径必带配速**——不配速=帧积压 e2e 虚高失真；结构预期：轻模型出图率由 compose 固定成本主导趋同，仅 yolov5m 级可见拉低；**yolo_world 双输入 N/A**（链 B 单输入张量，U1 缺口）；对照机新链路参考（yolov8n 配速 15，2026-09-16）：实交付 ~12–13fps、compose p50 ~50ms、e2e p50 62.4ms |
| M14 | 逐模型链 A 到达帧率 | perf_demo 链 A（subscribe 平台调度：infer.subscribe 绑帧序→annotate 烘入主流）逐模型 300s **独立窗**（`--chains a --a-fps 10`，**禁与链 B 同窗**——A/B 互抢 NPU/daemon 污染两链指标）：**到达 fps=结果到达样本数÷窗口时长**；annotate_errors 全窗计数；Δbake_skips（B33 同源）；skew 分位（新固件到达驱动轴） | 基线待建（首轮不设） | —（结构预期见备注，不作 WATCH 线） | RECORD | perf-demo samples JSONL + perf-demo.json（方案 11.3 M14 行） | | | 前置=流适配+injection 已开（同 B31）；**到达 fps≈min(a-fps 配速×交付率, 1000/e2e p50)**——轻模型配速受限（≈9fps 级）、yolov5m（e2e ~124ms）**模型天花板受限 ~8fps，非调度缺陷**；yolo_world 双输入 N/A（链 A daemon 绑定无 text embedding 第二输入源，U1 缺口同 M13 归因）；B31 fps≥8 线按部署模型 yolov8n 校准，逐模型不搬用 |

## 5. L2 验收项总表（B 系列）

时延类基线=对照机（跨设备参考，**仅 WATCH 级，FAIL 待目标设备基线建立后修订**）；不变量类=契约绝对值（FAIL）。**子项拆分原则**：C5/D4 契约拆出的独立子项（B08/B19）缺证据判 INCONCLUSIVE，**不以"收窄判定"降低原契约**；范围调整须测前修订冻结（1.5）。

| 编号 | 验收项 | 指标与统计口径 | 硬性阈值 FAIL | 关注线 WATCH | 判定方法 | 数据来源 | 实测值 | 结论 | 备注/豁免依据 |
|---|---|---|---|---|---|---|---|---|---|
| B01 | annotate（10 框） | p50/p99（100×1 采样） | 基线待建 | p50≤7 / p99≤15 | 双达标 | test_65→P6 `annotate_10det` | | | 参考值 5.1/10.9 |
| B02 | annotate（50 框） | p50/p99 | 基线待建 | p50≤10 / p99≤16 | 双达标 | test_65→P6 `annotate_50det` | | | 参考值 7.5/11.8 |
| B03 | annotate（16 多边形） | p50/p99 | 基线待建 | p50≤10 / p99≤15 | 双达标 | test_65→P6 `annotate_16poly` | | | 参考值 7.1/11.2 |
| B04 | 烘焙流帧率不变量（C5 门·帧率连续性子项） | level_small/mid/big + noop on/off 各档 fps/丢帧/gap | 每档 fps≥29.5 且丢帧=0 且 gap_max≤100ms（3 帧周期） | gap_max>66.7ms（2 帧周期） | 契约 | test_65→P6 流表 | | | 本项只判帧率连续性；**每帧成本子项独立为 B08**（v1.3 拆分） |
| B05 | strict 计数 | strict_locked/degraded/skips | —（覆盖缺口） | — | **INCONCLUSIVE**（未驱动时） | test_65→P6 `strict_app_layer` | | | **套件未驱动 strict 会话**（全 0=未驱动非全过）→ 判 INCONCLUSIVE；语义门 bake_skips<frames 成立时 RECORD；实锁验收靠 B31/B34 或独立驱动（方案 8.1） |
| B06 | TTL 衰减 / session sweep | 定量记录字段 | — | — | RECORD | test_65→P6 `ttl_decay_*`/`session_sweep` | | | |
| B07 | 帧绑定命中率 | hit_rate_pct | — | — | RECORD | test_65→P6 `frame_binding` | | | **方法门：必须 FrameHandle.sequence 锚定**（last_packet_seq 恒判 too-old，方案 10.10）；参考带 31.7–38% |
| B08 | overlay 计算成本子项（C5 门） | on/off 两档**每帧成本对照**（编码耗时或等效每帧成本字段） | 证据可得时：每帧成本差 <1% | — | 契约 | 编码耗时/每帧成本字段 | | | **前置：现产物无编码耗时/成本字段——缺证据判 INCONCLUSIVE**（不反向收窄 B04）；字段落地后本项转硬性 |
| B11 | push_frame RTT（720p NV12） | p50/p99（200×1 采样） | 基线待建 | p50≤10 / p99≤15 | 双达标 | test_66→P7 `push_frame_720p_nv12` | | | 参考值 7.0/10.8 |
| B12 | paced 注入不变量 | **38 帧 paced 场景**：丢弃计数 + fps 对照 | 丢弃=0 且 frames_dropped=0 且 fps 与对照差 <2% | — | 契约 | test_66→P7 `paced_invariants_38` | | | D3 门；有效性前提=注入已开且 PushFrame 有成功样本（1.3-1）；**与 B16 的租约收敛是两个不同场景分别定义** |
| B13 | 4K 全幅 REPLACE | 池分配结果 | EXEMPT：预期被 DSP 每客户端像素预算拒绝（记录 error 即过） | — | RECORD | test_66→P7 `compose_replace_4k_nv12` | | | 见第 8 章 E7；4K 用 OVERLAY(inset) |
| B14 | compose 不改帧率（D4 门·帧率子项） | 720p replace/overlay、4K overlay 各档 fps/丢帧 | 每档 fps≥29.5 且丢帧=0 | — | 契约 | test_66→P7 流表 | | | 本项只判帧率/丢帧；**码率子项独立为 B19**（v1.3 拆分） |
| B15 | EOS 恢复 | EOS 后首帧延迟 / 最大间隙 | 首帧≤100ms 且 gap≤100ms（3 帧周期@30fps） | — | 契约 | test_66→P7 `eos_recovery` | | | 参考值 36.5/64.9ms |
| B16 | 写租约收敛与稳态 | 收敛率=**n_conv/(n_conv+n_timeout)**；**尝试数=10**（测试固定 `range(10)`，对齐代码；与 B12 的 38 帧 paced 场景分别定义）；in_flight_max；收敛 p99 | 收敛率=100% 且 n_timeout=0 且 **n_conv+n_timeout=10** 且 in_flight_max≤3 | 收敛 p99≤36ms | 契约+双达标 | test_66→P7 `perf:lease_convergence`（n_conv/n_timeout）+ `lease_steady_state` | | | percentile 统计的 n 只含收敛样本，**禁用 n 作分母**；需更高尝试数属工具参数化项（方案 4.1）；配额=每客户端 outstanding≤3、租期 200ms |
| B17 | resize 池驻留回归门 | paced_resize_pool p99 + **尾部样本**（主判据）+ **单峰子项** | **p99≤35.7ms 且 ≥43ms 样本=0**（主判据，可判）；单峰子项：直方图无孤立簇 | — | 契约 | test_66→P7 `paced_resize_pool` | | | **基线=9/11 记录（目标设备）**；回归即 FAIL（B3 门）；**单峰子项前置 U11**（无原始样本/直方图产物，落地前该子项 INCONCLUSIVE，p99 与尾部计数仍可判） |
| B18 | 注入与 DSP 并发退化 | p99 退化百分比（concurrent vs solo） | <150% | <100% | 退化比例 | test_66→P7 `injection_dsp_concurrency` | | | 参考值：dsp +18.6%、publish +94.6% |
| B19 | compose 码率子项（D4 门） | 各档**码率或编码字节对照**（compose on/off 或前后窗） | 证据可得时：波动 <5% | — | 契约 | GetStreamStatus/流表 bytes 字段 | | | **前置：现产物无码率/字节字段——缺证据判 INCONCLUSIVE**（不反向收窄 B14）；字段落地后本项转硬性 |
| B21 | 事件扇出 | 1/4/16 订阅者 missed + p99 | 每档 missed=0 | p99：1 档≤11 / 4 档≤15 / 16 档≤24 | 计数+双达标 | test_67→P8 `event_fanout` | | | 参考值 8.3/11.5/17.9ms |
| B22 | 慢订阅者隔离 | fast 收到数 / slow 排水语义 | fast 100/100；只丢 slow 自己 | — | 契约 | test_67→P8 `slow_subscriber` | | | E4 门 |
| B23 | get_frame 扇出包络 | 1/2/4/8 客户端 p50 + 每客户端帧数 | 2 客户端 p50 ≤ 1 客户端 ×1.5 | 同左 | 包络 | test_67→P8 `getframe_fanout` | | | 4/8 客户端退化=帧配额 by design，RECORD（参考：4 客户端每客户端 ~15fps、8 客户端 ~7.5fps） |
| B24 | 混合负载退化（**前置 U9**） | **同轮序列 solo-before → mixed → solo-after**；各面 p99 相对前后 solo **均值**退化；err；每面 attempted/succeeded/failed 计数；**漂移门公式：drift_pct=\|solo_after.p99 − solo_before.p99\| ÷ solo_before.p99 × 100% ≤ 30%** | 各面 err=0 且退化 <150% 且漂移门成立 | <100% | 退化比例 | test_67→P8 `mixed_load` + 同轮前后 solo 快照 | | | **U9 未落地前整项 INCONCLUSIVE**（现测试无同轮 solo、异常被忽略、无 attempted/succeeded/failed）；漂移门超差判该面 INCONCLUSIVE 重测（U9 落地后） |
| B31 | 链 A 稳态（subscribe） | fps/dropped/annotate_errors/skew | dropped=0 且 annotate_errors=0 | fps≥8；skew p99≤120ms | 契约+双达标 | perf_demo `/run/aipc/perf-demo.json` 10s 滚动窗 | | | 参考值 fps 9.2/skew p99 91.5ms（9/14 报告，**旧固件口径**）；丢帧按 fps 判；**有效性前提：subscribe 链路 fps>0 且 annotate_calls>0（1.3-1/4）**；**新固件（到达驱动闸门）skew 大幅下降（参考 p99 ~22ms）——旧 9/14 skew 参考不可比，首轮重校**；逐模型链 A 见 M14（本行 fps≥8 线按部署模型 yolov8n/配速 10 校准，逐模型不适用） |
| B32 | 链 B 稳态（keep-fd） | fps/frames_err/降级；hw_infer p99；e2e | frames_err=0 且无降级 | fps≥12；hw_infer p99≤17ms | 契约+双达标 | 同上 | | | e2e RECORD（含 fd 队列年龄，非纯处理时延）；参考值 fps 13.3/hw_infer p99 12.4ms；**有效性前提：frames>0 且 cpu_fallback 关闭状态已记录** |
| B33 | 烧录占空比（**前置 U5 选定方案**） | **采集时直接计算 ROI 亮像素统计**（ROI=合成文本带/框区域，Y>200 且像素数 >150 判"带合成像素"，随采样逐帧输出计数）；**空白对照**（无合成时段占空比 ≈0）；**样本量 ≥200 编码帧**；可复核视频段存档可选 | B 链 ≥90% 且空白对照 ≈0 | B 链 ≥95% | 契约 | video_observer 采集时 ROI 统计输出 | | | **方法门：pacer 40Hz + pts=now+40ms 前置**（方案 8.4）；A 链 Δbake_skips=0（FAIL）；U5=采集时计算（video_observer 只存元数据无像素，事后分析不可行） |
| B34 | 韧性三注入 | ①SIGKILL app（`systemctl kill -s SIGKILL perf-demo`）②yaml 移除 injection + 重启 camera-daemon ③还原 yaml 冷却重建 | 三契约：①重启后 **≤120s** 双链路重建健康（NRestarts +1 为预期）②注入断供时**明确降级不崩**（日志含明确报因）③恢复后 **≤120s** 全量重建 | — | 契约 | journal + NRestarts + `/run/aipc/perf-demo.json` 重建后首条 | | | **手工操作（前置 U6：phase_runner 不做故障注入）**；**故障窗起止记元信息表**供 C31 分窗巡检 |
| B35 | 过夜 soak（可选） | afps/bfps 衰减；RSS；NRestarts | 衰减 <10% 且 RSS 无单调爬升 且 NRestarts 仅预期 +1 | — | 契约 | soak 采样日志（10min 周期） | | | 不执行标 N/A |

## 6. L3 验收项总表（C 系列）

REST 类参考值来自对照机 unix 面实测（n=100）；绝对值类直接判。

| 编号 | 验收项 | 指标与统计口径 | 硬性阈值 FAIL | 关注线 WATCH | 判定方法 | 数据来源 | 实测值 | 结论 | 备注/豁免依据 |
|---|---|---|---|---|---|---|---|---|---|
| C11 | REST 快读类 | /system/health、/system/info、/monitor/network、/monitor/cpu、/monitor/memory、/ai/capabilities、/events/topics 各 p99 | 基线待建 | p99≤10ms | 双达标 | perf_rest_latency.py JSON | | | 参考带 4.2–6.7ms；探针请求间隔 ≥0.05s |
| C12 | REST 已知慢路径 | /monitor/summary、/monitor/disk、/ai/models p99 | 基线待建 | p99：summary≤850 / disk≤385 / models≤460 | 双达标 | 同上 | | | 慢=路径构成非缺陷（第 8 章 E8）；UI 轮询建议加缓存为跟进项 |
| C13 | REST 采样窗端点 | /system/stats、/ai/stats 净开销（p50−500ms，缺省窗口径同 A03） | 净开销≤200ms | 净开销≤100ms | 双达标 | 同上 | | | 口径同 A03 |
| C14 | REST 跨服务链 | /media/status、/media/profiles、/device/infrared/status、/device/lens/status p99；/device/status p99 | 基线待建 | 轻链 p99≤15ms；/device/status p99≤41ms | 双达标 | 同上 | | | 参考带 6.3–9.9ms；/device/status 27.2ms |
| C15 | REST GPIO 排除审计 | GPIO 端点调用数 | **=0**（任何调用作废窗口） | — | 计数 | 探针端点清单 + journal | | | 同 A39（issue #46） |
| C16 | REST 错误率（三类） | ①传输错误（连接失败/超时）②HTTP 错误（状态码非 2xx；status_hist 须全 2xx）③业务错误（抽检响应 error 字段空、data 在位，每类端点 ≥3 次） | 三类均=0 且 status_hist 全 2xx | — | 计数+抽检 | perf_rest_latency.py JSON | | | **前置 U3**：现版本 HTTP 4xx/5xx 计成功——err=0 不证明成功；修复前本项判 INCONCLUSIVE |
| C21 | GetStats 观测面契约 | **缺省请求=500ms**（sampling_window_ms=0 → 服务端缺省）；服务端 sampling_window_ms clamp [1,5000]（`grpc_service.cpp` GetStats）；**SDK 侧先 clamp——服务端边界验收须绕过客户端 clamp 直接构造请求**；per-model 字段在位（avg_latency_us/hw_fps/queue_depth/**avg_skew_us/max_skew_us/skew_samples**）**与系统级字段在位（device_utilization/cpu_utilization/dsp_utilization/device_temperature）** | clamp 契约成立且字段在位 | — | 契约 | SDK 探测调用 + 绕过 clamp 的直接请求 | | | 探测建议显式小窗（如 100ms）；hw_fps 字段语义=hw 时延倒数（方案 3.2-②）；系统级字段仅 ≥0 才置值（HAL −1 缺省）——缺省记间隙率，时间序列采样见 U12/M12；REST `/monitor` NPU 读数同源（默认 500ms 窗，轮询用 SDK 小窗） |
| C22 | GetStreamStatus 四层丢包对账 | 清窗 drain 后恒等式 received+missing==packets_published delta；**稳态窗**（剔除启动/epoch 切换/drain 边界）四层 drop 计数 | 恒等式精确成立 且 稳态窗异常丢弃全零 | — | 契约 | GET /api/v1/media/status（SDK 同源） | | | 四层=publisher 7 项+bake 4 项+注入层+SDK 层；**注入层 frames_dropped 含良性取代（newest-due-wins）非丢显**——异常丢弃与良性取代区分判 |
| C31 | journal 死信巡检（**分窗**） | 七类死信**逐窗**匹配数：①稳态窗（S2/S3/S5 各测量窗）全零为判定窗；②故障窗（S4 三注入）只核**预期事件清单**（camera-daemon 重启日志、`injection disabled` 明确报因、perf-demo systemd 拉起、epoch 重绑/会话替换）——预期事件不算死信，意外死信单独归因 | 稳态窗**全零**（故障窗意外死信=0 另计） | — | 计数 | collect-perf-journal.sh 分窗产物（journal-<窗名>.txt） | | | **前置 U4**（脚本引号 bug + stderr 屏蔽 + 远端命令须 `sh -s --`）；**有效性核对**：退出码=0 + 输出窗口覆盖锚点窗 + journal 现存最早条目早于锚点；`--since` **不会自动排除故障日志**——必须分窗；七类清单见方案 9.3；event_hub_drops 为 Debug 级单独标注 |
| C32 | CMA hailo_media（768M 池） | 模块结束后回落（相对 S1 基线快照） | 30min 内回落至基线 +10% 以内 | — | 契约 | proc_sampler JSONL | | | 对照机参考：400→峰 737→回落 400；目标设备基线以 S1 快照为准 |
| C33 | 温度 | tz0/tz1 峰值 | <85°C（告警线） | ≥80°C | 契约 | proc_sampler JSONL | | | 对照机峰 79.9/77.8°C |
| C34 | MemAvailable | 全程最低值 | ≥4000MB | ≥5000MB | 契约 | proc_sampler JSONL | | | 对照机最低 5827MB |
| C35 | CMA linux,cma（512M 池） | 全程水位 | — | — | RECORD | proc_sampler JSONL | | | 对照机恒 281MB（55%）；历史高位碎片化未复现，持续观察 |
| C36 | 空闲 CPU 基线 | S1 空闲快照（CPU 占用/负载） | — | — | RECORD | proc_sampler JSONL | | | **基线待建**：首轮快照经修订表转正后供后续轮判定；另记 GetStats SoC 口径 `cpu_utilization` 空闲参考（与 Linux /proc 口径不同源，不混写）；**DSP 负载**（C21 字段口径 + `dsp-utilization` 厂商口径快照，U14 有则采）同窗记录，基线待建 |
| C37 | daemon RSS 全程漂移（生命周期分段） | 四 daemon 按**进程生命周期分段**：记录 PID/启动时间/重启数；S4 韧性注入的预期重启段剔除 | **非预期重启=0——确认发生即 FAIL**；该非预期重启段的 RSS 统计单独标"证据不足"；其余段各 ΔRSS <30MiB | 各 <15MiB | 分段双达标 | proc_sampler JSONL | | | 与 A54（soak 窗）互补，本项覆盖全程；预期重启段（B34 三注入）以元信息表故障窗锚点为准 |
| C38 | 函数级 CPU 归因（**可选**，载体=方案 9.5） | Python 进程：py-spy 火焰图 **self CPU top-10 函数**（采样率 50Hz、采样窗 ≥60s、负载场景与正式轮一致或注明差异）；原生 daemon：perf/gdb 兜底（镜像内工具可得时），不可得登记工具缺口 | —（记录级，不设阈值） | — | RECORD | 火焰图 SVG + top-10 函数表（`run_id/归因/prof/` 归档） | | | **仅在归因轮执行——正式测量窗禁挂采样器**（ptrace 停顿引入微抖动，方案 9.5 纪律）；未执行标 N/A；结论须与 proc_sampler 同窗进程 CPU% 三角互证 |

## 7. 契约门汇总表

门定义与实测格式参照 9/12 报告"契约门汇总"节；编号沿用平台契约文档。**拆分原则**：C5/D4 的"每帧成本/码率"证据独立为 B08/B19 子项——缺证据子项判 INCONCLUSIVE，**不以收窄判定降低原契约**。

| 门 | 契约内容 | 对应验收项 | 实测 | 判定 |
|---|---|---|---|---|
| B2 编码流（空载） | 30fps：丢帧=0、gap_max≤3 帧周期 | B04 | | |
| B3 池驻留回归 | p99≤35.7ms、≥43ms 样本=0、单峰 | B17（单峰子项前置 U11） | | |
| C5 overlay 恒定开销 | on/off 每帧成本差 <1%（**拆分**：帧率连续性→B04；计算成本→B08；B08 缺证据判 INCONCLUSIVE，不收窄 B04） | B04 + B08 | | |
| C7 strict 新语义 | gate SKIP 时 app 层仍绘制：bake_skips<frames | B05 | | |
| D3 注入不变量 | paced：丢弃=0、fps 不变 | B12 | | |
| D4 compose 不改码率/帧率 | 各档 fps 持平、0 丢帧；码率不变须码率/字节字段佐证（**拆分**：帧率→B14；码率→B19；B19 缺证据判 INCONCLUSIVE，不收窄 B14） | B14 + B19 | | |
| E4 慢订阅者隔离 | 只丢慢者自己 | B22 | | |
| I1 长稳衰减 | <10%（窗口法口径，A51 参数化公式） | A51 | | |
| — GPIO 零调用审计 | 窗口内 GPIO 调用=0 | A39/C15 | | |
| — NPU 共享组一致性 | 稳态窗 73/74=0 且双 daemon 首建日志组名一致（=medialib `hailort.device-id`） | A41 | | |
| — journal 死信零 | 稳态窗七类死信 0 匹配（含 U4 有效性核对；故障窗按预期事件清单另判） | C31 | | |
| — CMA 回落 | 模块结束后 30min 内回基线 | C32 | | |
| — 帧纪律 | retained_frames ≤2（perf_demo） | B31–B34 | | |

## 8. 预期豁免与已知问题核对表

**四类处理语义**：**范围排除**=不测不判；**已知缺陷**=触发条件下 EXEMPT/RECORD；**流程前置**=不满足即无效跑/重测（**不是豁免**）；**统计口径**=口径内建，按替代口径判。每条：触发条件成立 → 按类别处理；不成立 → 按原阈值判。

| 类别 | # | 已知问题（方案第 10 章） | 受影响项 | 触发条件 | 本轮是否触发（填） | 证据（填） |
|---|---|---|---|---|---|---|
| 范围排除 | E13 | web 玻璃到玻璃 / ReconfigureEncoder 重操作 / metrics_port 未实现 | —（不在本轮范围） | 恒成立 | | 方案第 1 章范围表 |
| 已知缺陷 | E1 | GPIO 单调用硬复位（issue #46） | A39/C15（负面门） | 恒成立（排除策略本身） | | 端点清单审计输出 |
| 已知缺陷 | E5 | 镜头 af0832 bootstrap -2815 死循环（设备特有） | A34 | journal 证实循环存在 | | journal grep |
| 已知缺陷 | E7 | 4K 全幅 REPLACE 超 DSP 每客户端像素预算 | B13 | 恒成立（设计约束） | | `DspError: client limit exceeded` 记录 |
| 已知缺陷 | E9 | SubscribeEvents 服务端桩未实现 | A38 | 恒成立（上游缺口） | | |
| 已知缺陷 | E12 | hw_infer_time_us 部分模型族不上报 | M05 | GetStats 缺报时 | | |
| 已知缺陷 | E2 | get_stats 500ms 阻塞采样窗；偶发 max | A03/C13 | 净开销口径已内建；偶发 max 单次事件 | | |
| 已知缺陷 | E14 | 服务端模型复用：已加载模型/同文件别名注册直接跳过加载（根因记载：方案 7.6/M11） | M11 | 恒成立（三分口径已内建：冷加载/缓存命中/首次推理） | | 注册前后 list_models 清单 + journal |
| 流程前置 | E4 | 模型 id 注册陷阱（自愈复活） | 全部推理项 | 流程纪律（id 文件名派生）；**违反=重跑非豁免** | | 注册清单 |
| 流程前置 | E6 | injection.enabled 出厂 false | B11–B19/B31–B35 | 未开启即无效跑（**前置条件，非豁免**） | | yaml 与 .bak 记录 |
| 流程前置 | E15 | 分阶段执行删除上一轮报告（编排脚本双清+覆盖合并） | 全部分阶段产物 | **归档纪律（U10）：每次运行前归档上一轮，违反=证据缺失，相关项 INCONCLUSIVE** | | 归档目录清单（元信息表） |
| 流程前置 | E16 | model_variant 短名静默失配（写 `yolov8n` 不报错但 post_result 恒空，须真实 dlsym 符号如 `hailo_yolov8n`；方案 10.14） | 全部推理项 | variant 用真实符号名（get_model_info/插件清单核对）+ **结果非空抽检**（objects/post 输出非空）；违反=重跑非豁免（err=0 掩盖不了空结果，M03 不覆盖此坑） | | 抽检记录 |
| 统计口径 | E3 | subscribe 丢帧统计伪影（HAL 共享序号空间） | A07/B31 | 恒成立（口径规避：只按 fps 判；事件/编码包序号不受此限） | | |
| 统计口径 | E8 | 三「快类」REST 端点实为慢路径 | C12 | 恒成立（路径构成） | | |
| 统计口径 | E10 | subscribe pacing 语义（完成锚点） | A07 | 恒成立（90% 契约已内建） | | |
| 统计口径 | E11 | device hub lag 钟域无效（~20.7 天偏差） | P8 device_hub 记录 | 恒成立（lag 不判） | | |

## 9. 综合判定规则

1. **有效执行前置门先行**：任一条件（1.3）不成立的项判 **INCONCLUSIVE**——不计失败，但该层存在硬性级 INCONCLUSIVE 时，该层**不得判"通过"**
2. **分层判定**：某层全部**硬性（FAIL 级）项 PASS**（豁免项不计失败、硬性项 INCONCLUSIVE 为 0）→ 该层通过；任一硬性项经复跑仍 FAIL → 该层不通过；硬性项存在 INCONCLUSIVE 且未闭环 → 该层判"证据不足（附整改清单）"
3. **总判定**：L1（含 M03）+ L2 + L3 三层均通过 → **验收通过**
4. **有条件通过**：三层硬性项均 PASS，但存在下列之一 → 有条件通过（条件=整改清单+复测项+期限，写入最终结论栏）：a) 未归因的 WATCH 项；b) 已列整改的非硬性 INCONCLUSIVE / 工具缺口（不影响硬性项有效性）
5. WATCH 项不达标**不计失败**，但每条必须在备注列归因；无法归因的 WATCH 升级为待查事项（不阻塞判定，列入跟进）
6. RECORD 项不参与判定，仅建立数据点
7. 流程性失效（GPIO 被调、injection 未开、测量窗被干扰且无空闲基线佐证、U10 归档纪律违反致证据缺失）→ 相应窗口数据作废重测
8. **非预期重启（C37）**：确认发生的非预期 daemon 重启直接判 **FAIL**（不因该段 RSS 统计缺失而降级为 INCONCLUSIVE——重启本身即硬性失败事件；缺失的只是该段 RSS 证据，单独标注）
9. 复跑规则见 1.4（首跑与复跑结果都保留，分目录归档）；阈值修订见 1.5 与第 11 章
10. **首轮边界**：首轮正式跑只出「有条件通过 / 不通过 / 证据不足」，产出=**基线建立报告**（终态注明「基线建立完成（全项）」或「基线建立（含缺项，附清单与补测计划）」——独立状态，不与综合判定混写）+修订表回填；第二轮起按冻结阈值与冻结工具版本方可判「验收通过」

## 10. 结果通俗摘要（回填后必填，一页，面向未参与测试的读者）

写法规范见方案 12.1（结论先行 / 关键数字 / 问题三句话）；本节给回填结构。

**一句话总评（填）**：______

| 层 | 红绿灯 🟢🟡🔴⚪ | 一句话结论（大白话，不用缩写术语） |
|---|---|---|
| L1 SDK 接口性能 | | |
| M 多模型对比 | | |
| L2 流媒体与管线 | | |
| L3 服务端资源 | | |

**关键数字（填，≤8 行，指标用通俗名，超标写倍数）**：

| 指标（通俗名） | 实测 | 参考线 | 直观解读（如"比参考慢 2.3 倍"） |
|---|---|---|---|
| | | | |

**问题清单（按影响排序，每条三句：现象 / 影响 / 建议）**：

1. ______

**最终结论（填）**：□ 验收通过　□ 有条件通过（条件：______）　□ 不通过　□ 证据不足（整改后复测）

**首轮基线建立状态（填，仅首轮）**：□ 基线建立完成（全项）　□ 基线建立（含缺项，清单与补测计划：______）

**签署（填）**：执行人 ______　复核人 ______　日期 ______

## 11. 阈值修订记录表

| 日期 | 验收项编号 | 原阈值（v1） | 新阈值 | 依据（数据来源） | 批准人 |
|---|---|---|---|---|---|
| | | | | | |

## 12. 附录

### 附录 A：验收项 ↔ 模块 ↔ 报告分区映射（单一事实源）

| 编号段 | 模块/载体 | 报告分区 | 行名/字段 |
|---|---|---|---|
| A01–A08 | test_60 | P1 | 表行名与接口同名；`subscribe_10fps` 流行 |
| A07e | 探针脚本 fps_limit 扫描（方案 7.1 可选扩展） | 扫描输出 | 各档实交付 fps |
| A11–A17 | test_61 | P2 | 表行名与操作同名；probes/health JSON |
| A21–A25 | test_62 | P3 | 表行名 + `arrival_10hz` 流行 |
| A31–A39 | test_63 | P4 | 表行名 + `device_event_arrivals` 流行；journal |
| A51–A54 | test_64 | P5 | 长稳结果 + **分桶序列（A51 重算原料）** + daemon 表 |
| M01–M14 | M 系列载体（方案 7.6；M13=perf_demo 链 B 逐模型、M14=perf_demo 链 A 逐模型） | 横向对比表 / 探针输出 / U12 利用率 JSONL / M13、M14 samples JSONL | 见 4.2 |
| B01–B08 | test_65 | P6 | 表行名 + 流表 + 定量记录字段 |
| B11–B19 | test_66 | P7 | 表行名 + 流表 + 定量记录字段（lease_convergence：n_conv/n_timeout） |
| B21–B24 | test_67 | P8 | 定量记录字段（event_fanout/slow_subscriber/getframe_fanout/mixed_load） |
| B31–B35 | perf_demo | `/run/aipc/perf-demo.json` + video_observer ROI 统计 + soak 日志 | 10s 滚动窗字段 |
| C11–C16 | perf_rest_latency.py | 探针 JSON | 按端点（status_hist/err） |
| C21–C22 | SDK 观测面探测 | GetStats/GetStreamStatus 响应 | 按字段 |
| C31 | collect-perf-journal.sh | 分窗巡检输出（journal-<窗名>.txt） | 按死信类 |
| C32–C37 | proc_sampler | 趋势 JSONL | 按采样字段 |
| C38 | py-spy 归因轮（方案 9.5） | 火焰图 SVG + top-10 函数表 | self CPU 占比 |

### 附录 B：基线数据引用与设备归属声明

见方案附录 B（两处内容一致，以方案附录 B 为准）：9/9 报告=**目标设备**（L1 唯一基线）；9/12 报告=**对照机**（跨设备参考，仅 WATCH 级）；9/14 报告=对照机（perf_demo 参考）；9/11 记录=目标设备（池驻留基线）；主仓 `docs/benchmarks/ai-model-benchmark-hailo15h.md`=NPU 裸层（旧时间点）。

### 附录 C：厂商工具链可用性矩阵（U14 preflight 产物，S0 回填）

来源=方案 9.6 厂商工具链。**缺失项相关采集静默省略并在产物标注**——RECORD/诊断级，不判 INCONCLUSIVE；相关归因降级到方案 9.5 py-spy / 探针法。

> 下表已预填 **D72（对照机）2026-09-17 盘点值**作参考；正式跑 S0 须在**目标设备**复核一遍（版本随镜像漂移，以目标设备实测为准）。
>
> **D72 链路验证注记（2026-09-17，最小 trace 端到端实测）**：采集→scp→Perfetto trace_processor 解析（与 UI 同引擎）→Host UI `:15000` 全链路打通。数据源分轨结论：**noc-bandwidth-\* 与 thermal 实测可用**（5s 窗 34.7 万计数器样本：dsp/Total BW/NN Core TX·RX·config/ISP Fast Bus/Encoder 八轨道 ~8.7kHz + pvt-ts-0/1）；**sched 配置在位但该内核无 tracefs**（`mount -t tracefs` 报 unknown filesystem type；虽 `CONFIG_FTRACE=y`，内核 5.15.32-yocto-standard-g7919a266e480 未编 tracefs 实例）→**静默产出空 trace（691B/0 事件），勿当正常数据**；**applications（track_event）负载下无 slices**（`hailortcli benchmark` 同窗仍 0——库侧打点未激活，跟进项：需 Hailo 库 tracing 激活方式或 instrumented 构建）；mem_tracker 空闲窗无输出（须带分配负载复验）。预置配置全表比文档描述多（applications/applications_detailed/hailo15-imaging/mem_tracker/noc-bandwidth-\*×12/sched/thermal/v4l2）。

| 工具 | 位置（板端/Host/PC） | 在位（是/否） | 版本 | 配对/备注 |
|---|---|---|---|---|
| hailo-soc-profiler（采集器） | 板端 | **是**（D72：`/usr/bin/hailo-soc-profiler`；另有 `hailo-soc-profiler-service`） | D72：opkg 包 1.0-r0（recipe 版本，无 CLI `--version`；须 `-t` 才运行） | trace 类型：applications / sched / noc-bandwidth-dsp-encoder-isp / mem_tracker |
| hailo-soc-profiler-ui（查看器 deb） | Host | **是**（已装） | 1.12.0 | **版本配对轴=板端 HailoRT ↔ Host UI 交付线**（soc-profiler 包 1.0-r0 是 recipe 编号、与 UI 编号不同体系）；`:15000` 冒烟通过（2026-09-17）；**首采 trace 后立即在 UI 打开验证一次**——打不开即版本失配，矩阵记失配并换 UI 版本 |
| hailortcli（benchmark / run2 / monitor） | 板端 | **是**（D72：`/usr/bin/hailortcli`，run2/benchmark/monitor 子命令全在） | D72：**HailoRT-CLI 5.3.0**（opkg：hailortcli/libhailort5.3.0 均 5.3.0-r0） | run2 双模式=M06/M07；monitor 仅诊断窗（方案 10.15） |
| hailo-dma-usage.sh | 板端 | **是**（D72：`/usr/bin/hailo-dma-usage.sh`；同目录另有 `hailo-cma-usage.sh` 可一并采） | —（脚本无版本串） | CMA/DMA 快照（9.4 互证） |
| dsp-utilization | 板端 | **是**（D72：`/usr/bin/dsp-utilization`） | — | DSP 负载（C36 记录级） |
| /etc/hailo_noc_perf.sh | 板端 | **是**（D72：在位，`noc_set_counter_filter` / `noc_measure_sleep` 函数齐） | — | NoC 计数器（source 后调用函数集） |
| DFC Model Profiler（hailo profiler） | PC | 待盘点 | — | 可选：har + runtime_data 逐层报告；未装则 M07 可选进阶标省略 |
