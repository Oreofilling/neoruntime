# 研发自测范围与分层（NE503 AIPC / NeoRuntime）

> 定位：定义**研发对每次变更必须自测到什么程度**，以及研发自测与测试团队、性能专项的边界。
> 关联文档：[release/pre-release-checklist.md](../release/pre-release-checklist.md)（发布负责人视角的发布门）、[testing/performance-test-plan.md](performance-test-plan.md)（性能专项怎么做）。
>
> 边界一句话：**研发对自己改的路径负责——变更不引入回归、修改有效果有量化证据；测试团队对版本全量回归与验收负责。** 研发自测记录（§4 证据要求）是提测邮件"自测情况"章节的直接输入。

## 1. 自测分层

| 层 | 触发时机 | 执行者 | 内容 |
|---|---|---|---|
| L0 提交门 | 每次 push / PR | CI 自动（本机可跑同款） | 构建 + 仓库结构 + schema 漂移 + Go 单测 + swagger 同步 |
| L1 模块单测 | 按改动面 | 开发者本机 | Go `_test.go`；C++ `ctest`；web `vitest`；postprocess 重新生成 |
| L2 集成冒烟 | 改动跨服务/改接口 | 开发者本机 MVP 栈 | 集成测试 + HTTP 冒烟 |
| L3 目标机自测 | 改动触及硬件路径 | 开发者 + 目标机 | HAL/固件/媒体链路实测 + 异常恢复 |
| L4 性能专项 | 提测前 | 发布负责人组织 | 按 performance-test-plan 全量（研发日常只跑 §4.2 校准/冒烟确认链路健康） |

### L0 提交门（CI 已自动，本地提 PR 前建议同跑）

```bash
make build-native          # stub 平台全量编译
make test-basic            # 仓库结构只读检查
make postprocess-schema-check
make test-unit             # go test -race ./platform/... ./tests/unit ./tools/aipc-cli/...
```

CI（`.github/workflows/ci.yml`）额外跑：web `tsc -b` + build、swagger 同步（`scripts/check_swagger_sync.py`）和 ct-disc `go test`；commitlint 在 PR 上独立执行，CodeQL 当前只支持手工触发。
**注意：C++ 单测和 web 单测目前不在 CI 内（见 §3 缺口），属于 L1 必跑项。**

### L1 模块单测（按改动面选择，全部本机可跑）

**Go 服务**（platform-api / app-manager / device-control / onvif-device / postprocess / event-bus / device-discovery / aipc-cli）：
- 修改/新增逻辑必须配套 `_test.go`；跑 `make test-unit`（含 `-race`）

**C++ 服务（camera-daemon / ai-runtime / hal_v2）**：
- camera-daemon（14 个测试目标：overlay / DSP / daynight / injection / lens 等）——默认构建**不编译**测试，单独配置：

```bash
cmake -S platform/camera-daemon -B platform/camera-daemon/build-host-tests -DCAMERA_DAEMON_BUILD_TESTS=ON
cmake --build platform/camera-daemon/build-host-tests -j
ctest --test-dir platform/camera-daemon/build-host-tests --output-on-failure
```

- ai-runtime（3 个目标，默认构建即编译，直接跑）：

```bash
ctest --test-dir platform/ai-runtime/build-stub --output-on-failure
```

- hal_v2：配置时加 `-DHAL_V2_BUILD_TESTS=ON` 后 `ctest` 对应 build 目录

**Web 控制台**（27 个 vitest 测试文件，CI 不跑）：

```bash
cd web && pnpm test:run        # 改动模块相关的用例必须过；新组件/工具函数补测试
```

**postprocess 注册表**：改 `platform/postprocess` 注册表后跑 `make postprocess-schema` 重新生成工件并随代码一起提交（CI 只查漂移不修复）。

### L2 集成冒烟（本机 MVP 栈）

```bash
make build-native && ./scripts/start_mvp.sh
make test-integration      # tests/integration：event-bus / device-control / 服务连通性
make test-smoke            # scripts/test_all.sh：4 服务 socket + Platform API HTTP 冒烟
./scripts/stop_mvp.sh
```

触发条件：改动跨服务接口（proto/gRPC 契约）、改 platform-api 路由、改服务启动/依赖关系。

### L3 目标机自测（硬件路径必测）

触发条件：改动触及 HAL 后端、DSP/NPU 数据通路、镜头/传感器、MCU 协议、媒体链路（camera-daemon/ai-runtime 核心路径）。最低要求：

1. 变更路径在真机走通（如：overlay 烘焙出图、模型加载推理出结果、镜头移动到位）
2. **异常恢复**：相关进程 `kill -9` 后 ≤15s 自愈（订阅恢复、注册自清理）；断电重启后状态一致（配置持久化项）
3. DMA/DSP 路径改动需确认无句柄/内存泄漏趋势（长跑 30 分钟级观察）
4. MCU 协议改动：协议文档（`docs/mcu_protocol/`）同步更新 + 真机指令往返验证；固件验证 OTA 升级、升级后不重复刷写和失败保持原固件。当前 OTA 禁止主动降级；如需救援恢复，另走配对 HEX + SWD/STLink

## 2. 按改动类型的必测矩阵

| 改动类型 | L0 | L1 | L2 | L3 | 特定要求 |
|---|---|---|---|---|---|
| Go 服务逻辑 | ✅ | `make test-unit` + 补测试 | 视接口面 | — | — |
| platform-api 路由 | ✅ | platform-api 用例（一致性基线的扩展点） | ✅ smoke | — | `check_swagger_sync.py` 必过；新路由补 swagger.yaml |
| ai-runtime 推理/后处理 | ✅ | ctest 3 目标 + postprocess-schema-check | ✅ | ✅ 真机模型冒烟 | 模型加载冒烟须含 post_result 校验；新增模型类型补 unified schema |
| camera-daemon 视频/DSP/overlay | ✅ | ctest 14 目标 | — | ✅ 链路出图 | 涉及 DMA 提交的改动确认 hw-in-use 语义 |
| HAL v2 | ✅ | stub ctest | — | ✅ hailo15 实测 | 后端能力声明与实测一致 |
| MCU 固件 / 协议 | 构建 | ms_bridging/host_link selftest | — | ✅ OTA 往返 | hex/OTA 配对；验证升级/不重复刷写/失败保持原固件；协议文档同步；`chore(mcu)` 提交 |
| Web 控制台 | tsc+build | `pnpm test:run` | — | 手动 console 验证 | — |
| 打包/部署脚本 | ✅ | — | — | ✅ 实机部署 | 解包抽检 + deploy.sh preflight 过 |
| os-updater / 升级路径 | ✅ | `go test ./platform/osupgrade/...` | 视改动面 | ✅ 升级+回滚演练 | app-manifest 兼容字段核对；真机 A/B 或 single-recovery 仍不可由单测替代 |

## 3. 测试框架现状基线与缺口（2026-09 盘点）

**已有的**：CI 四件套（build/basic/schema/unit）+ swagger 同步；PR 另有 commitlint；CodeQL 目前为手工触发。platform-api 有 74 个 Go 测试文件；性能验收体系为方案 v1.13 / 91 项 / 五级判定。

**缺口（按修复收益排序）**：

1. **C++ 单测不在 CI**：camera-daemon 14 个、hal_v2 测试目标默认构建不编译（需 `-D..._BUILD_TESTS=ON`），ai-runtime 3 个目标编译了但 CI 不执行 `ctest`。测试写了但不在门里＝回退保护靠自觉。→ 建议：ci.yml 增加native-tests job（零新代码）；中期把测试纳入默认 host 构建。
2. **web vitest 不在 CI**：27 个测试文件存在，CI 只跑 `tsc + build`。→ ci.yml web job 加一行 `pnpm test:run`。
3. **OS 升级缺少系统级故障注入门**：`platform/osupgrade` 已有 runner、backup、compatibility、layout、recovery、store、validate 等 Go 单测，但 CI 仍不能覆盖真实 SWUpdate、分区切换、断电点和 bootloader boot-attempt。→ 保留现有单测门，并建设目标机台架的 A/B / single-recovery 故障注入套件。
4. **MCU 无自动测试门**：`ms_bridging_test` / `host_link_posix_selftest` 是示例代码不是测试框架，构建门只查"OTA 包存在"。→ 至少把 selftest 纳入 host 编译运行；协议回归靠 L3 真机。
5. **集成层薄且存在假绿风险**：`make test-integration` 实际只执行两个 Go 文件中的 5 个入口，`test_camera.cpp` 未接入该目标；`TestServiceConnectivity` 连接失败只写日志不失败。跨服务契约（如 camera-daemon↔ai-runtime 帧流）也无自动覆盖。→ 先消除“服务不在也通过”的路径，再补关键契约的 stub 集成用例。
6. **无覆盖率基线**：`go test` 未统计 `-cover`。→ 先加统计出基线数字，暂不设阈值门。

**在缺口补齐前**：第 1、2 条对应的测试已列入发布门（pre-release-checklist 第 2 节），发布前必须按改动面手动执行。

### 建议落地顺序

| 优先级 | 建议 | 完成标志 |
|---|---|---|
| P0（下一迭代） | Web vitest 和三组 C++ `ctest` 接入 CI；修掉 integration 的假绿路径；关闭会使性能硬性项 `INCONCLUSIVE` 的 U 系列工具缺口 | PR 无需人工补跑 host 测试；服务未启动、探针失败或证据缺字段时测试明确失败/证据不足 |
| P1 | 建立统一 runner 和版本化 `run-manifest.json`，统一生成 `run_id/阶段/模块/尝试` 目录；报告由结构化 evidence 自动生成 | 重跑不覆盖首跑；任意结论可反查候选 commit、包 SHA、设备和工具版本 |
| P1 | 发布候选“构建一次、测试同一制品、批准后提升”，避免 tag 流水线重新构建一个未经实测的新包 | 提测、制品库和最终 Release 的包 SHA-256 相同 |
| P1 | 将测试框架健康度与产品判定分开：探针/环境失败不能记产品 PASS，也不能混记产品 FAIL | 报告同时输出 product verdict 与 harness verdict |
| P2 | 建设目标机 HIL 调度：按板卡/镜头/OS/SoC/MCU 版本矩阵分配台架，支持断电、进程杀死、A/B 切换等故障注入 | 每个候选能自动给出覆盖矩阵与未覆盖组合 |
| P2 | 建立覆盖率、耗时、失败率和 flaky 趋势；覆盖率先记录基线，不立即用单一百分比卡发布 | 能区分真实回归、环境故障与波动测试 |

建议按执行成本分三条流水线，而不是让一套 7 小时级性能方案承担所有反馈：

- **PR 快门（目标 ≤15 分钟）**：编译、静态检查、Go race、Web/C++ 单测、stub 契约测试。
- **夜间目标机门**：关键媒体/NPU/MCU 链路、30 分钟稳定性、故障恢复和兼容负例。
- **发布候选门**：对冻结制品执行完整性能方案、升级/回滚和硬件矩阵抽检；诊断型 RECORD 项可按变更面裁剪，但硬性项不可裁剪。

## 4. 证据要求（PR 与提测邮件的输入）

沿用提测邮件模板的写作口径：

1. **量化前后对比**：缺陷修复给"修复前 → 修复后"实测数据；性能优化给较上版变化幅度；不用"显著改善"类模糊词
2. **写实判定**：自测结论写"通过 / 通过（有条件+条件）"，不写笼统"自测通过"
3. **数据来源可追溯**：关键论断注明测试项编号 / 产物路径 / 提交链（进提测邮件附录追溯表）
4. **PR 描述包含**：跑了哪些层（L0–L3）、命令、结果；自动化覆盖不到的写明手工验证步骤（对齐 CONTRIBUTING.md 的 PR 要求）
5. **仅用合成本地数据**，不提交日志/抓包/客户数据/设备凭据（TESTING_GUIDE 既有纪律）
