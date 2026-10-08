# NE503 应用包安装向导（测试用）

> 适用对象：测试同学。
> 适用范围：把 NeoRuntime **应用升级包**安装（首装或升级）到 NE503 设备，并完成安装后验证与回滚。
> 不覆盖：OS 镜像烧录/升级（见 [deployment/os-upgrade.md](../deployment/os-upgrade.md)）、MCU 的 SWD/STLink 工厂烧录（见 [deployment/baseboard-mcu-rtc-ota.md](../deployment/baseboard-mcu-rtc-ota.md)）。
> 命令与行为以仓库内 `scripts/deploy.sh`、`Makefile` 为准；本文所有设备端命令均需 **root**。

---

## 1. 你会拿到什么

提测邮件（模板见 [release/release-test-email-template.md](../release/release-test-email-template.md)）附件中的应用包：

```
neoruntime-hailo15-<版本>.tar.gz        # 例如 neoruntime-hailo15-v1.1.0.tar.gz
```

包内关键内容（解压后自检用，安装脚本会自动处理，无需手工干预）：

| 路径 | 内容 |
| ---- | ---- |
| `deploy.sh` | 设备端热升级安装脚本（在设备上运行的就是它） |
| `VERSION` | 版本、git commit、平台、构建时间 |
| `opt/aipc/app-manifest.json` | 兼容信息：machine / product / OS 版本区间 / data schema |
| `opt/aipc/bin/`、`opt/aipc/etc/`、`systemd/` 等 | 服务程序、配置、systemd 单元、Web 控制台、MCU OTA 包、模型（若打包时带入了） |

## 2. 安装前提检查

### 2.1 物料与环境

- [ ] 应用包及其 **SHA-256** 已从提测邮件拿到，传输后校验一致
- [ ] NE503 设备可 SSH 登录（root），网络可达
- [ ] 设备 OS 版本在应用包兼容区间内（提测邮件"应用兼容范围"一节；当前发布默认区间 `1.12.0`–`1.12.0`）
- [ ] 设备镜头型号与包匹配（`af0832` 或 `fg2009`，以提测说明为准；不匹配时 camera-daemon 会拒绝启动）
- [ ] `/data` 分区可用空间充足：建议 **≥ 5GB**（需下载含 GenAI 的全部模型时；用 `--skip-genai` 可减约 3GB）。升级还会在 `/data/backups/` 留一份旧版本完整备份
- [ ] 若包内未内置模型（打包日志出现 `models/ source tree absent`），设备需**可访问外网**以下载模型

### 2.2 设备端安装前状态确认

```bash
ssh root@<设备IP>

uname -m                          # 预期 aarch64（Hailo-15）
cat /etc/aipc-os-release          # MACHINE=hailo15-ne503 / PRODUCT=ne503 / OS_VERSION=...
df -h /data                       # 确认可用空间
systemctl is-active containerd    # app-manager 依赖；未运行也不碍事，deploy 会自动拉起并迁移数据目录
```

如设备上已有旧版本，确认当前版本与服务状态（在任一旧解包目录里执行，见 2.3 的坑）：

```bash
./deploy.sh --status              # 输出当前版本、各服务状态、备份列表
cat /data/aipc/VERSION            # 不进解包目录也能看：version / git_commit / deploy_time
```

> **坑**：`deploy.sh --status` / `--rollback` 依赖解压出来的脚本文件本身；`/tmp` 重启后会被清理。若解压目录已丢失，重新解压一次应用包再执行即可（`--rollback` 只用脚本，不会重装包内容）。

## 3. 标准安装步骤

安装 = 升级 = 首装，同一条命令。整个过程约 2–5 分钟（含自动重启）。

### 3.1 传输与校验

```bash
# 在测试机上
scp neoruntime-hailo15-v<版本>.tar.gz root@<设备IP>:/tmp/
ssh root@<设备IP>
cd /tmp
sha256sum neoruntime-hailo15-v<版本>.tar.gz    # 与提测邮件给出的 SHA-256 比对
```

### 3.2 解压并执行安装

```bash
cd /tmp
tar xzf neoruntime-hailo15-v<版本>.tar.gz
cd neoruntime-hailo15-v<版本>
./deploy.sh
```

- 脚本首先做**兼容性与包完整性预检**（machine / product / OS 版本区间 / data schema / 传感器 profile），预检不过不会动设备上任何东西。
- 预检通过后提示 `Proceed with deployment? [y/N]`，输入 `y` 继续。
- 随后自动执行 8 步：建运行目录 → **备份旧版本到 `/data/backups/aipc-app/<旧版本>-<时间戳>/`**（首装无旧版本则跳过）→ 停服务 → 在暂存目录组装并校验完整新版本 → **原子切换** `/data/aipc` → 部署 systemd/nginx/containerd/内核参数 → 拷贝模型（若包内有）→ 重启服务并做 60 秒健康检查。

### 3.3 两个必须知道的行为

1. **成功后设备会自动重启**：出现 `Deploy successful!` 后约 6 秒设备重启（`AIPC_OTA_REBOOT_AFTER_SUCCESS` 默认开启）。属正常现象，等设备重启回来再验证。若本轮测试不希望重启：
   ```bash
   AIPC_OTA_REBOOT_AFTER_SUCCESS=0 ./deploy.sh
   ```
2. **失败自动回滚**：健康检查不过（或中途任一步出错），脚本自动换回旧版本并重启服务，退出码非 0。看到 `Deploy completed but health check FAILED` 时先按第 6 节查日志，再决定是否重试或报缺陷。

### 3.4 常用变体

| 场景 | 命令 |
| ---- | ---- |
| 保留设备上现有配置（提测说明要求时用） | `./deploy.sh --no-config` |
| 脚本化/自动执行，跳过交互确认 | `./deploy.sh --force` |
| 安装成功但不自动重启 | `AIPC_OTA_REBOOT_AFTER_SUCCESS=0 ./deploy.sh` |

## 4. 安装后验证

### 4.1 版本核对

```bash
cat /data/aipc/VERSION            # version / git_commit / platform / deploy_time
cat /data/aipc/app-manifest.json  # 与提测邮件"应用兼容范围"逐项核对
```

### 4.2 服务健康

```bash
systemctl list-units 'aipc-*'
# 核心服务（deploy 健康检查盯的对象）应全部 active：
#   event-bus  camera-daemon  ai-runtime  platform-api
#   app-manager  device-control  device-discovery  aipc-healthmon
#   aipc-nginx-gateway（其运行时缺失时会显示 skipped，见第 6 节 FAQ）
journalctl -u camera-daemon -n 50 --no-pager   # 逐个可疑服务看日志
```

### 4.3 Web 控制台

- 浏览器访问 `http://<设备IP>/`（nginx 网关监听 80/443；platform-api 本身只绑 `127.0.0.1:8080`）。
- 登录账号取自包内 `configs/platform-api.yaml` 默认值：**admin / password**。若提测说明另行给出了账号，以提测说明为准。
- 登录后首页能显示设备/相机信息即为通。

### 4.4 模型确认（跑 AI 功能前必做）

```bash
ls -lh /data/aipc/models/*/       # models 是指向 /data/aipc-data/models 的软链，正常
```

包内没带模型时需联网下载（GenAI 模型约 3GB，可不下载）：

```bash
/data/aipc/bin/download_models.sh              # 全量
/data/aipc/bin/download_models.sh --skip-genai # 跳过 GenAI
```

下载脚本结尾会输出 `Summary: OK=.. FAIL=.. SKIP=..`：

- `FAIL=0` 即成功，随后自动调模型扫描接口注册。
- 出现 `AI privacy mask (DPM) will NOT fully work` 警告时，说明这几个 DPM 关键文件缺失，对应隐私掩码功能会失效：`detection/hailo_yolov8n_384_640.hef`、`detection/hailo_yolov8n_384_640.json`、`segmentation/linknet_mbv1_ss_dpm_256.hef`、`detection/tiny_yolov4_license_plates.hef`。其中 `linknet` **不在公共模型库**，下载不到只能从其他正常设备拷贝——遇到请直接在测试记录里登记。

### 4.5 MCU 固件（仅核对，无需操作）

应用包内置 MCU OTA 包。重启后 `aipc-mcu-prep` 会在开机窗口自动做 RTC 同步和固件升级（只升不降，失败不阻塞开机）：

```bash
journalctl -u aipc-mcu-prep -b --no-pager | tail -20
```

MCU 版本读取与 OTA 细节见 [deployment/baseboard-mcu-rtc-ota.md](../deployment/baseboard-mcu-rtc-ota.md)。

## 5. 回滚

```bash
cd /tmp/neoruntime-hailo15-v<版本>    # 解压目录丢了就重新解压一次
./deploy.sh --rollback                # 默认回滚到最近一次备份，需输入 y 确认
./deploy.sh --rollback --force        # 免确认
```

- 备份位于 `/data/backups/aipc-app/<旧版本>-<YYYYMMDD-HHMMSS>/`，回滚同样走原子切换 + 健康检查，成功提示 `Rollback successful.`。
- 回滚只恢复应用；设备上用户数据（数据库、模型、已装应用）不动。
- 回滚目标健康检查不过时脚本会尽量保住当前版本并报错，此时保留现场日志并联系研发。

## 6. 常见问题

| 现象 | 原因与处理 |
| ---- | ---------- |
| `APP_OS_VERSION_UNSUPPORTED` / `APP_MACHINE_MISMATCH` / `APP_PRODUCT_MISMATCH` / `APP_DATA_SCHEMA_UNSUPPORTED` | 设备 OS 与应用包不配套。用 `cat /etc/aipc-os-release` 对比提测邮件兼容范围；OS 不符需先换用配套 OS 镜像，不要绕过 |
| `Deploy completed but health check FAILED`（随后自动回滚） | 先 `journalctl -u '<失败服务>' -n 50` 看日志；若已自动回滚，设备仍在旧版本上运行。保留日志、记录包 SHA-256 后报缺陷 |
| `camera-daemon` 反复退出 | 常见为镜头型号不匹配：`cat /data/aipc/etc/product.yaml`（`lens:` 值须为实机镜头 `af0832`/`fg2009`），或传感器 profile 预检问题（看 journalctl） |
| Web 控制台打不开 | `systemctl status aipc-nginx-gateway`；若为 skipped，检查 `/data/nginx/bin/nginx` 是否存在；另确认客户端到设备 80/443 端口可达 |
| 模型下载 FAIL | 设备外网不通或源站失效。恢复网络后**重跑** `download_models.sh`（已下载的会自动 SKIP，不会重复下载） |
| `exec format error` | 包与设备架构不匹配（本包仅适用 aarch64/Hailo-15），确认拿错包后更换 |
| `/data` 空间不足 | 清理 `/data/backups/aipc-app/` 下不再需要的旧备份（确认当前版本稳定后再清），或换更大存储的设备 |

## 7. 安装验收最小清单

- [ ] 包 SHA-256 与提测邮件一致
- [ ] `./deploy.sh` 预检与安装全程无 ERROR，结尾 `Deploy successful!`，设备自动重启后可重新 SSH
- [ ] `cat /data/aipc/VERSION` 的 version / git_commit 与提测说明一致
- [ ] `systemctl list-units 'aipc-*'` 核心服务全部 active（nginx-gateway 若 skipped 需说明原因）
- [ ] Web 控制台可登录、设备信息可见
- [ ] （涉及 AI 用例时）模型文件齐备，`download_models.sh` 输出 `FAIL=0`
- [ ] （升级场景）执行一次 `./deploy.sh --rollback` 验证可回到旧版本，再重新安装新版本
- [ ] 测试记录中留档：安装控制台输出、`/data/aipc/VERSION`、服务状态截图、异常时的 `journalctl` 片段

## 8. 参考

- [deployment/DEPLOYMENT.md](../deployment/DEPLOYMENT.md)：部署机制与手动部署（研发向）
- [deployment/baseboard-mcu-rtc-ota.md](../deployment/baseboard-mcu-rtc-ota.md)：MCU OTA / RTC 说明
- [release/pre-release-checklist.md](../release/pre-release-checklist.md) 第 7.1 节：发布侧对同一条安装路径的验收要求
- [api/swagger.yaml](../api/swagger.yaml)：Platform API 参考
