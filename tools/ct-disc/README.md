# CT-Disc 设备扫描与监控工具

`ct-disc` 是 NE503/AIPC 设备的发现、管理和资源数据记录工具。它包含两个入口：

- CLI：命令行扫描、监听、记录和 MQTT 命令发送。
- GUI：Windows 图形界面，支持扫描、手动添加设备、批量记录 CPU/内存/磁盘/NPU 数据和查看趋势图。

## 快速开始

### Windows GUI

当前构建产物：

```text
tools/ct-disc/gui/ct-disc-gui/build/bin/ct-disc-gui.exe
```

打开后可以：

1. 点击 `Scan` 扫描 CT-Disc 组播可达网络范围内的设备。
2. 点击 `Add Device` 手动添加跨网段设备，例如 `192.168.1.100`。
3. 勾选设备后点击 `Record Data`，批量记录资源数据。
4. 勾选 `One file per device`，按 IP 生成单设备记录文件。

### Windows CLI

当前构建产物：

```text
tools/ct-disc/dist/ct-disc-windows-amd64.exe
```

扫描设备：

```powershell
.\ct-disc-windows-amd64.exe scan --timeout 5 --count 3 -o json
```

记录指定设备：

```powershell
.\ct-disc-windows-amd64.exe record -a https://192.168.1.101 --token <TOKEN> --insecure-skip-tls-verify --interval 5s --samples 60 --one-file-per-device
```

## 发现机制说明

CT-Disc 扫描使用 UDP multicast：

```text
239.255.255.250:19850
```

这类发现取决于 CT-Disc 组播包是否可达，而不是简单等同于同一 IP 网段。设备可以 `ping` 通，但如果组播不可达，仍可能无法被 `scan` 发现；反过来，如果不同网段之间配置了组播路由/转发，也可以被扫描到。遇到组播不可达的情况，建议使用 GUI 的 `Add Device` 或 CLI 的 `record -a` 直接指定设备 IP/API URL。

## GUI 功能

### 扫描设备

点击 `Scan` 后，GUI 会发送 CT-Disc probe 并监听设备公告。设备列表会显示：

- IP
- FW
- Product
- MAC
- SN
- Online 状态

如果两个设备 SN 一致，记录文件不会按 SN 区分，而是优先按 IP 命名，避免互相覆盖。

### 手动添加设备

点击 `Add Device`，支持输入一行一个 IP 或 URL：

```text
192.168.1.100
https://192.168.1.101
```

添加时可以配置：

- Protocol：`HTTP` 或 `HTTPS`
- API Port：默认 HTTPS 为 `443`，HTTP 为 `8080`
- Username
- Token
- Skip HTTPS certificate verification

手动添加后，GUI 会先显示一条 `Manual` 设备，然后尝试从这些接口补全信息：

```text
/api/v1/device-info
/api/v1/network/config
/api/v1/monitor/summary
```

可补全字段包括：

- SN：从 `serial_number` / `serialNumber` / `sn` / `SN` 或 `factory.serial_number` 读取
- MAC：从 `mac_address` / `macAddress` / `mac` 或 `factory.mac_address` 读取
- FW：从 `firmware_version` / `firmwareVersion` / `firmware` / `fw` / `version` 读取
- Product / Model
- Hardware version

如果接口需要鉴权但未填写 token，设备仍会被添加，但 SN/FW/MAC 可能保持为空或显示 `manual-<ip>`。

### 记录资源数据

点击 `Record Data` 后可以设置：

- Format：`CSV` 或 `JSON Lines`
- Protocol / API Port
- Interval Seconds
- Samples
- Duration Minutes
- Username / Token
- Skip HTTPS certificate verification
- One file per device

记录接口：

```text
/api/v1/monitor/summary
/api/v1/monitor/snapshot
```

记录字段包含：

- `timestamp`
- `sn`
- `mac`
- `product`
- `ip`
- `api_url`
- `online`
- `metrics_ok`
- `error`
- `cpu_percent`
- `memory_percent`
- `disk_percent`
- `npu_percent`
- `temp_cpu`
- `temp_npu`
- `temp_board`
- `latency_ms`

`online` 表示目标是否作为记录目标存在；`metrics_ok` 表示本次是否成功读取到监控数据。判断采集是否成功时优先看 `metrics_ok` 和 `error`。

### 单设备单文件

勾选 `One file per device` 后，文件名优先按 IP 生成：

```text
ct-disc-metrics_192-168-1-101.csv
ct-disc-metrics_192-168-1-100.csv
```

这可以避免 SN 重复时写入同一个文件。

### 趋势图

记录过程中，GUI 会按设备显示 CPU、Memory、Disk、NPU 的变化趋势。趋势图来自本次运行期间采集到的成功记录，即 `metrics_ok=true` 的数据点。

## CLI 用法

### scan

主动发送 probe 并等待响应：

```bash
./ct-disc scan --timeout 5 --count 3
./ct-disc scan --timeout 5 --count 3 -o json
```

常用参数：

- `--timeout`：等待响应秒数
- `--count`：发送 probe 次数
- `--iface`：指定网卡名称
- `-o, --output`：`table` / `json` / `yaml`

### list

只监听设备公告：

```bash
./ct-disc list --timeout 10
./ct-disc list --product NE503 --sn CT2026
```

### watch

持续监听设备上下线和更新：

```bash
./ct-disc watch
./ct-disc watch --timeout 300
```

### record

记录资源数据：

```bash
./ct-disc record -a http://192.168.1.101:8080 --interval 5s --samples 60 --file metrics.csv
```

HTTPS 自签证书：

```bash
./ct-disc record -a https://192.168.1.101 --token <TOKEN> --insecure-skip-tls-verify --interval 5s --duration 1h
```

自动发现后记录：

```bash
./ct-disc record --scan-timeout 5s --count 3 --interval 10s --samples 360
```

一台设备一个文件：

```bash
./ct-disc record -a https://192.168.1.101 --token <TOKEN> --one-file-per-device
```

### send

通过 MQTT 给指定 SN 发送命令：

```bash
./ct-disc send CT2026-000812 reboot --broker tcp://192.168.1.102:1883 --payload '{}'
```

### announce

在设备侧模拟发送 CT-Disc 公告：

```bash
./ct-disc announce --product NE503 --sn CT2026-000812 --ip 192.168.1.101 --port 8080 --fw v1.3.8
```

## 构建

### CLI

```bash
cd tools/ct-disc
make build
make build-all
make build-windows-amd64
```

### GUI

```bash
cd tools/ct-disc/gui/ct-disc-gui
wails build -platform windows/amd64 -clean
```

或从 `tools/ct-disc` 目录构建当前平台 GUI：

```bash
make gui
```

## 常见问题

### 能 ping 通但扫描不到

正常。`ping` 是单播，CT-Disc 扫描依赖 multicast。跨网段、VPN、WSL/虚拟网卡、防火墙、交换机组播策略都可能阻止发现；如果跨网段组播路由/转发可达，也可以扫描到。处理方式：

- GUI 使用 `Add Device` 手动添加 IP。
- CLI 使用 `record -a <设备URL>` 指定目标。
- 如果必须跨网段自动发现，需要网络侧支持组播路由/转发，或在目标网络/组播可达位置运行扫描工具。

### 记录里的 online 是 true，但没有数据

看 `metrics_ok`。手动指定设备时，`online=true` 只表示它被加入记录目标；真正采集是否成功由 `metrics_ok` 和 `error` 判断。

### 手动添加后 SN/FW/MAC 不显示

一般是设备 API 不通、协议/端口不对，或缺少 token。先确认：

- HTTPS 设备是否勾选 `Skip HTTPS certificate verification`
- API Port 是否正确
- Token 是否正确
- `/api/v1/device-info` 或 `/api/v1/network/config` 是否能返回 SN/MAC

### 可以挂测多久

没有固定限制，主要取决于采样间隔、设备数量和磁盘空间。经验值：

- 5 秒间隔：每台设备每天约数 MB 到 10 MB
- 10 秒间隔：每台设备每天约 2.5 MB 到 5 MB
- 60 秒间隔：每台设备每天约 0.5 MB 到 1 MB

批量老化测试建议使用 `10-30` 秒间隔，并勾选 `One file per device`。
