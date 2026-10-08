# NE503 AIPC（NeoRuntime）发布前检查清单

> 适用范围：产品提测与正式发布（应用包、MCU 固件、OS 镜像协同交付）。
> 对外开源发布另走 [OPEN_SOURCE_SPLIT.md](../OPEN_SOURCE_SPLIT.md) 的 pre-publish 门；同时涉及产品发布与开源时，两道门都必须通过。
>
> **门禁定义**：P0 是阻断项，任一失败、未执行或证据不足均不得正式发布；P1 必须执行，确因环境无法执行时须在提测说明中记录原因、风险、责任人和补测期限；P2 是发布后的归档项。
>
> **留档要求**：每版复制本清单为 `dist/vX.Y.Z-release-checklist.md`。每个勾选项至少记录执行人、时间、结论和证据路径；不得只保留一个无证据的勾号。

## 0. 发布计划与版本矩阵（P0）

- [ ] 上一正式版本 tag 与目标分支已确定；待发布范围可由 `git log --oneline <上版tag>..HEAD` 唯一得到
- [ ] 四条版本轴已确定并写入提测说明：OS 镜像、SoC 固件、NeoRuntime 应用、MCU 固件；不得用应用版本代替其他版本轴
- [ ] 目标硬件矩阵已冻结：板卡版本、镜头型号（`af0832` / `fg2009`）、OS 版本、SoC 固件版本；本版实测覆盖和未覆盖组合已列明
- [ ] 应用兼容矩阵已确定：`machine`、`product`、`min_os_version`、`max_os_version`、`supported_data_schema`、`target_data_schema`；契约见 [deployment/os-upgrade.md](../deployment/os-upgrade.md)
- [ ] 配套 SDK（`ne503-aipc-sdk`，兄弟仓 `neoruntime-sdks`）的版本、源码 commit 和发布状态已冻结；平台 proto 与 SDK 生成代码来自同一平台 commit
- [ ] 发布镜像、SDK 和测试工具版本已冻结；发布过程中升级工具后必须重新执行受影响的门

## 1. MCU 固件冻结（P0）

- [ ] `APP_VERSION` 已设置为本版 MCU 版本；版本号与提测说明一致
- [ ] `make mcu-firmware` 构建成功（需要 `arm-none-eabi-gcc`）
- [ ] `mcu_board_prj/firmware/` 中仅保留本批次配对产物：`ne503_Main_v<版本>_<YYYYMMDD>.hex` 与 `ne503_ota_package_v<版本>.bin`
- [ ] HEX 与 OTA 包来自同一次构建；已记录文件大小和 SHA-256，且 OTA 文件名版本、包头 `app_version`、源码 `APP_VERSION` 三者一致
- [ ] 固件刷新已连同旧 HEX 删除、新 HEX 新增、OTA 更新一次性提交，提交信息建议为 `chore(mcu): refresh v0.x.y firmware artifacts`
- [ ] 已确认 MCU 交付语义：HEX 仅用于 SWD/STLink 工厂或救援烧录；现场 OTA 只升级到更高版本，不支持主动降级，详见 [deployment/baseboard-mcu-rtc-ota.md](../deployment/baseboard-mcu-rtc-ota.md)

> 如果本版不重新构建 MCU，必须说明复用来源并验证 SHA-256；随后打包显式使用 `BUILD_MCU_FW=0`。不得把“找到一个旧 OTA 文件”视为固件门通过。

## 2. 候选基线与源码质量门（P0）

- [ ] MCU 产物及本版全部改动已提交；最终候选 commit 已记录
- [ ] `git status --short` 无输出；`git rev-parse HEAD` 与提测说明中的候选 commit 一致
- [ ] 候选 commit 已推送，且针对该 commit 的必需 CI 均通过；不得用其他 commit 的绿色结果替代
- [ ] `make verify` 通过：仓库结构检查、生成物前置和 `go test -race ./platform/... ./tests/unit ./tools/aipc-cli/...`
- [ ] `make postprocess-schema-check` 通过：postprocess schema 工件与注册表无漂移
- [ ] `python3 scripts/check_swagger_sync.py` 通过：`swagger.yaml` 与 Platform API 已注册路由一致
- [ ] 涉及跨服务、升级、持久化、并发或协议链路改动时，`make test-integration` 通过；不触发时在留档中写明判定依据
- [ ] 有运行实例时，`make test-smoke` 通过；目标地址、版本和执行时间已记录
- [ ] 涉及 camera-daemon、ai-runtime 或 HAL v2 时，已按 [testing/dev-self-test-scope.md](../testing/dev-self-test-scope.md) 执行对应 native C++ `ctest`；当前这些测试尚未全部进入 CI，不得用普通编译成功替代
- [ ] 涉及 Web 控制台时，`cd web && pnpm test:run` 通过；当前 CI 的 `tsc + build` 不能替代行为测试
- [ ] `make lint` 通过，或已记录工具版本/存量告警；新增告警不得带入发布

## 3. 发布打包（P0）

正式发布应显式固定镜像和全部兼容参数。Makefile 与 GitHub release workflow 当前统一使用 `zerobot/ne503-dev-env-full:4.0.23`；命令中仍显式传入镜像，防止默认值未来变化后无法复现。

```bash
make docker-pack-release \
  DOCKER_RELEASE_IMAGE=zerobot/ne503-dev-env-full:4.0.23 \
  DOCKER_RELEASE_SDK_PATH=/opt/hailo-sdk \
  VERSION=vX.Y.Z \
  BUILD_MCU_FW=1 \
  LENS_PRODUCT=af0832 \
  AIPC_MACHINE=hailo15-ne503 \
  AIPC_PRODUCT=ne503 \
  AIPC_MIN_OS_VERSION=1.12.0 \
  AIPC_MAX_OS_VERSION=1.12.0 \
  AIPC_SUPPORTED_DATA_SCHEMAS=1 \
  AIPC_TARGET_DATA_SCHEMA=1
```

- [ ] `VERSION` 显式传入，格式与发布 tag 一致；未依赖 `git describe` 默认值
- [ ] `LENS_PRODUCT` 与本批次实机一致，仅允许 `af0832` 或 `fg2009`
- [ ] `AIPC_MACHINE`、`AIPC_PRODUCT`、OS 版本闭区间、支持的 data schema 集合和目标 schema 均显式传入并与第 0 节矩阵一致；目标 schema 必须包含在支持集合中
- [ ] `BUILD_MCU_FW` 策略已明确：`1` 表示发布环境重建；`0` 表示复用第 1 节已冻结产物。两种模式都必须校验最终包内 OTA 的 SHA-256
- [ ] `pack-release` 自带门全部通过：10 个服务二进制、`libaipc_hal.so`、release helper、nginx runtime 齐全，staged YAML 无非规范 `/opt/aipc` 绝对路径
- [ ] 打包完成后再次检查 `git status --short`；若重建 MCU 导致跟踪产物变化，必须解释并重新冻结候选基线，不能从脏工作区发布
- [ ] 产物存在：`build/release/neoruntime-hailo15-<VERSION>.tar.gz`

> `scripts/pack_release.sh` 是本机工具链封装，不是 Docker 构建的等价环境。仅用于开发调试或已有受控构建机，并在留档中注明工具链差异。

## 4. 包体机器抽检（P0）

下面命令使用临时目录，可重复执行且不会混入上次解包内容：

```bash
VERSION=vX.Y.Z
PKG="build/release/neoruntime-hailo15-${VERSION}.tar.gz"
CHECK_DIR="$(mktemp -d)"
trap 'rm -rf "$CHECK_DIR"' EXIT
tar xzf "$PKG" -C "$CHECK_DIR"
PKG_ROOT="$CHECK_DIR/neoruntime-hailo15-${VERSION}"

test -s "$PKG_ROOT/VERSION"
python3 -m json.tool "$PKG_ROOT/opt/aipc/app-manifest.json" >/dev/null
test "$(find "$PKG_ROOT/opt/aipc/web" -type f | wc -l)" -gt 0
test -s "$PKG_ROOT/opt/aipc/etc/swagger.yaml"
test "$(find "$PKG_ROOT/opt/aipc/firmware/mcu" -maxdepth 1 -name 'ne503_ota_package_*.bin' | wc -l)" -eq 1
sha256sum "$PKG"
```

- [ ] `VERSION` 中 `version`、`git_commit`、`platform` 和 `build_date` 与候选基线一致
- [ ] `app-manifest.json` 中 app version、machine、product、OS 闭区间和 data schema 与版本矩阵完全一致；不再以旧 `required_compat_level` 作为兼容依据
- [ ] 包内恰有一个目标 MCU OTA 包；文件名、包头版本及 SHA-256 与第 1 节冻结产物一致
- [ ] `opt/aipc/web/` 非空，`opt/aipc/etc/swagger.yaml` 为本候选版本
- [ ] nginx runtime、部署脚本、systemd units、HAL 动态库和 10 个服务程序均存在且权限正确
- [ ] 对 `models/ source tree absent` 已确认目标机的模型供给方式与首次联网条件；出现 `imu calibration missing` 时必须查明，不能直接忽略
- [ ] 包的 SHA-256、字节数、构建日志和包体检查日志已归档

## 5. 测试工具就绪门（正式性能测试前 P0）

- [ ] 已对 [testing/performance-test-plan.md](../testing/performance-test-plan.md) 的 U1–U14 逐项记录状态：已修复 / 有效替代证据 / 不适用 / 未解决
- [ ] 每个未解决缺口已映射到受影响验收项；任何会使硬性项变成 `INCONCLUSIVE` 的缺口均已在正式跑前关闭，否则本版结论只能是“证据不足”，不得发布
- [ ] 尤其确认跨轮错误计数、HTTP/业务错误识别、journal 窗口、ROI 像素证据、同轮 solo/mixed 对照及分阶段产物归档能力可用；不能以 `err=0` 或空报告替代有效性检查
- [ ] 测试工具、SDK、proto、验收模板均已冻结版本；测试期间发生变化时，受影响窗口全部重跑
- [ ] 每次运行使用唯一 `run_id`，元信息至少包含：候选 commit、包 SHA-256、SDK 版本、测试计划版本、设备/镜头、OS/SoC/MCU 版本、工具版本、起止时间
- [ ] 测试框架自身异常与被测产品失败分开记录；超时、探针崩溃、证据缺字段不得误判为产品 PASS

## 6. 性能与稳定性自测（正式发布 P0）

- [ ] 按 [testing/performance-test-plan.md](../testing/performance-test-plan.md) 执行，并用 [testing/performance-acceptance-template.md](../testing/performance-acceptance-template.md) 回填
- [ ] FAIL / WATCH / RECORD / EXEMPT / INCONCLUSIVE 均按模板定义判定；硬性项存在 FAIL 或 INCONCLUSIVE 时不得判“通过”或“通过（有条件）”
- [ ] 原始样本、模块 evidence、journal、环境指纹和汇总报告均按 `run_id/阶段/模块/模型/尝试` 归档；开始下一轮前先封存上一轮
- [ ] 对照上一正式版核对关键指标无意外回退；若硬件、SDK、OS 或测试工具不同，先判定是否可比，不能直接计算回退比例
- [ ] 允许复跑的项目同时保留首跑与复跑证据，并写明复跑触发原因；不得覆盖失败结果
- [ ] 最终报告明确区分：产品结论、测试框架健康度、已知缺口和条件项
- [ ] 报告及原始数据打包为 `perf-<VERSION>-<run_id>.tar.gz`，记录 SHA-256并作为提测附件

## 7. 实机功能、升级与恢复（P0）

### 7.1 应用包

- [ ] 抽一台目标机按 [deployment/DEPLOYMENT.md](../deployment/DEPLOYMENT.md) 从上一应用版本升级到候选版本，基本功能和服务健康检查通过
- [ ] `/data/backups/aipc-app/<旧版本>-<YYYYMMDD-HHMMSS>/` 完整，`latest` 指向本次备份；执行一次 `deploy.sh --rollback` 并验证服务恢复
- [ ] 应用兼容负例至少覆盖：OS 版本超出闭区间、machine/product 不匹配、data schema 不受支持；安装或服务启动按契约被拒绝

### 7.2 OS 镜像

- [ ] 按 [deployment/os-upgrade.md](../deployment/os-upgrade.md) 验证本设备布局对应的 A/B 或 single-recovery 路径
- [ ] `/data/backups/aipc-os-upgrade/<job-id>/` 中 `manifest.json`、`SHA256SUMS`、`status` 和所需归档存在且校验通过
- [ ] A/B 设备至少完成一次成功切换和一次验证失败回滚；同时确认 boot-attempt 回滚路径可用
- [ ] 已验证 OS 升级中的应用兼容性是告警而非 OS 安装阻断；升级后不兼容应用服务被门禁停住，`platform-api` 救援通道仍可用

### 7.3 MCU

- [ ] 在底板验证 MCU OTA 升级、升级后版本读取、下一次启动不重复刷写，以及 OTA 失败时保持原固件/可恢复
- [ ] 不把“OTA 降级”列为能力：当前 `--upgrade-if older` 明确禁止降级
- [ ] 如产品要求救援恢复，另行用配对 HEX 验证 SWD/STLink 烧录；该路径与现场 OTA 分开记录

## 8. 提测与发布物料（P1）

- [ ] 复制 [release-test-email-template.md](release-test-email-template.md) 为 `dist/vX.Y.Z-release-test-email.md` 并补全所有【待填】项
- [ ] 附件清单齐备：OS 镜像、应用包、MCU HEX + OTA 配对件、性能报告包、提测说明及各文件 SHA-256
- [ ] 历史遗留和注意事项逐条重新验证；失效项删除，新增风险补入，不得整段照抄上一版
- [ ] 内部追溯表已回填：邮件论断 ↔ PR/commit ↔ 测试项 ↔ 原始证据路径
- [ ] 测试覆盖矩阵明确列出已覆盖与未覆盖的硬件/版本组合，以及未覆盖风险的责任人和计划

## 9. 发布与归档（P2）

- [ ] 发布批准后再创建 `vX.Y.Z` tag；tag 必须直接指向已测试的候选 commit
- [ ] tag 触发的最终构建成功；对最终发布包重新执行第 4 节机器抽检，记录最终 SHA-256
- [ ] GitHub Release、制品库和提测附件指向同一最终包；若 tag 流水线重新构建导致包内容变化，必须重新确认而不是沿用候选包结论
- [ ] 提测说明、勾选清单、构建日志、测试报告、原始证据索引和 SHA-256 清单存入 `dist/` 或团队指定的不可变归档位置
- [ ] 若同时对外发布开源仓，完成 [OPEN_SOURCE_SPLIT.md](../OPEN_SOURCE_SPLIT.md) pre-publish 门

## 附：最小机器门

```bash
make verify \
  && make postprocess-schema-check \
  && python3 scripts/check_swagger_sync.py

make docker-pack-release \
  DOCKER_RELEASE_IMAGE=zerobot/ne503-dev-env-full:4.0.23 \
  DOCKER_RELEASE_SDK_PATH=/opt/hailo-sdk \
  VERSION=vX.Y.Z BUILD_MCU_FW=1 LENS_PRODUCT=af0832 \
  AIPC_MACHINE=hailo15-ne503 AIPC_PRODUCT=ne503 \
  AIPC_MIN_OS_VERSION=1.12.0 AIPC_MAX_OS_VERSION=1.12.0 \
  AIPC_SUPPORTED_DATA_SCHEMAS=1 AIPC_TARGET_DATA_SCHEMA=1
```

> 最小机器门只覆盖源码检查与打包。测试工具有效性、包体检查、实机性能、升级和恢复仍必须按上文留证。
