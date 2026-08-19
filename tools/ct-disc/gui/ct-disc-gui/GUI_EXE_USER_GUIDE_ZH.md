# CT-Disc GUI exe 使用说明

适用程序：

```text
ct-disc-gui.exe
```

默认构建路径：

```text
tools/ct-disc/gui/ct-disc-gui/build/bin/ct-disc-gui.exe
```

## 1. 工具用途

CT-Disc GUI 用于发现 NE503/AIPC 设备，并记录每台设备的资源监控数据，包括 CPU、内存、磁盘、NPU、温度和请求延迟。

它支持两种添加设备方式：

- `Scan`：扫描 CT-Disc 组播可达网络范围内的设备。设备不一定必须和本机在同一 IP 网段；如果跨网段组播路由/转发可达，也可以扫描到。
- `Add Device`：手动添加指定 IP 或 URL，适合 IP 可访问但组播不可达的设备。

## 2. 启动程序

在 Windows 上双击运行：

```text
ct-disc-gui.exe
```

如果 Windows 安全提示拦截，选择允许运行。首次运行时如果需要访问网络，请允许防火墙放行。

## 3. 扫描设备

1. 在顶部网卡下拉框选择网卡，或保持 `All Interfaces`。
2. 点击 `Scan`。
3. 等待扫描完成。
4. 设备列表会显示 `IP`、`FW`、`Product`、`MAC`、`SN` 和在线状态。

CT-Disc 扫描依赖 UDP multicast：

```text
239.255.255.250:19850
```

能 `ping` 通不代表一定能扫描到。`ping` 是单播，`Scan` 依赖组播。如果不同网段之间配置了组播转发/路由，跨网段设备也能扫描到；如果组播不可达，请使用 `Add Device` 手动添加。

## 4. 手动添加设备

点击 `Add Device`，输入一行一个 IP 或 URL，例如：

```text
192.168.1.100
https://192.168.1.101
```

可配置项：

- `Protocol`：选择 `HTTPS` 或 `HTTP`。
- `API Port`：HTTPS 常用 `443`，HTTP 常用 `8080`。
- `Username`：设备 API 需要用户名时填写。
- `Token`：设备 API 需要 token 时填写。可以填写裸 token，也可以填写 `Bearer <token>`。
- `Skip HTTPS certificate verification`：设备使用自签 HTTPS 证书时建议勾选。

添加后，设备会先以 `Manual` 形式出现在列表中。GUI 会自动尝试读取以下接口补全设备信息：

```text
/api/v1/device-info
/api/v1/network/config
/api/v1/monitor/summary
```

如果接口可达且鉴权正确，会自动显示真实 `SN`、`FW`、`MAC`、`Product`、`HW`。如果读取失败，设备仍会保留在列表里，但可能显示为 `manual-<ip>`。

## 5. 查看设备详情

点击设备行可以打开详情窗口。详情中可以查看：

- Serial Number
- Product
- Firmware
- Hardware
- IP Address
- Port
- MAC Address
- First Seen
- Last Seen
- Capabilities

手动添加的设备会显示 `Manual` 标记，并支持在详情里移除。

## 6. 批量选择设备

设备列表左侧有复选框：

- 勾选单台设备：加入批量操作。
- 勾选表头复选框：选择全部在线设备。
- 点击批量工具条中的 `Clear Selection`：清除选择。

批量选择后可以执行 `Record Data`。

## 7. 记录设备资源数据

选择设备后点击 `Record Data`。

常用设置：

- `Format`：推荐使用 `CSV`；也支持 `JSON Lines`。
- `Protocol` / `API Port`：和设备 API 保持一致。
- `Interval Seconds`：采样间隔。短期排查可用 `5` 秒；长时间挂测建议 `10-30` 秒。
- `Samples`：采样次数。填 `0` 表示不限制次数。
- `Duration Minutes`：记录时长。填 `0` 表示不限制时长。
- `Username` / `Token`：用于访问设备 API。
- `One file per device`：推荐勾选，每台设备生成一个记录文件。

点击 `Start Recording` 开始记录，点击 `Stop Recording` 停止记录。

如果 `Samples` 和 `Duration Minutes` 都为 `0`，程序会一直记录，直到手动停止或关闭程序。

## 8. 记录文件

默认输出文件为：

```text
ct-disc-metrics.csv
```

可以点击 `Browse` 选择保存路径。

勾选 `One file per device` 后，文件名优先按 IP 生成，例如：

```text
ct-disc-metrics_192-168-1-101.csv
ct-disc-metrics_192-168-1-100.csv
```

这样即使两台设备 SN 相同，也不会写入同一个文件。

## 9. 记录字段说明

CSV/JSON Lines 中常见字段：

- `timestamp`：采样时间。
- `sn`：设备 SN。
- `mac`：设备 MAC。
- `product`：产品型号。
- `ip`：设备 IP。
- `api_url`：采集使用的 API 地址。
- `online`：设备是否作为记录目标存在。
- `metrics_ok`：本次是否成功采集监控数据。
- `error`：采集失败原因。
- `cpu_percent`：CPU 使用率。
- `memory_percent`：内存使用率。
- `disk_percent`：磁盘使用率。
- `npu_percent`：NPU 使用率。
- `temp_cpu` / `temp_npu` / `temp_board`：温度数据。
- `latency_ms`：请求延迟。

判断采集是否成功时，请优先看 `metrics_ok`。手动添加设备时，`online=true` 只表示设备被加入记录目标，不代表监控接口读取成功。

## 10. 趋势图

记录过程中，窗口下方会显示每台设备的趋势图，展示：

- CPU
- Memory
- Disk
- NPU

趋势图只使用 `metrics_ok=true` 的成功采样点。

## 11. 建议挂测时长

程序本身没有固定挂测时长限制，主要取决于采样间隔、设备数量和磁盘空间。

参考值：

- 5 秒间隔：每台设备每天约数 MB 到 10 MB。
- 10 秒间隔：每台设备每天约 2.5 MB 到 5 MB。
- 60 秒间隔：每台设备每天约 0.5 MB 到 1 MB。

建议：

- 短期问题定位：5 秒间隔，挂测数小时到 1 天。
- 批量老化测试：10-30 秒间隔，挂测 3-7 天。
- 长期趋势观察：60 秒间隔，可挂测数周。

## 12. 常见问题

### 可以 ping 通，但扫描不到设备

这是正常现象。`ping` 是单播，CT-Disc 扫描依赖组播。跨网段、防火墙、VPN、WSL 虚拟网卡或交换机组播策略都可能导致扫描不到。

如果不同网段之间已经配置组播路由/转发，并且 CT-Disc 组播包可以到达本机，`Scan` 也可以发现跨网段设备。

处理方式：

- 使用 `Add Device` 手动添加设备 IP。
- 在设备所在网络或组播可达的位置运行扫描工具。
- 检查 Windows 防火墙是否允许程序访问网络。
- 如需跨网段自动扫描，请检查网络侧组播路由、IGMP、ACL 和交换机策略。

### 手动添加后 SN/FW/MAC 不显示

通常原因：

- 协议或端口不正确。
- 设备 API 不可达。
- 设备返回 `401 Unauthorized`，需要填写 token。
- HTTPS 使用自签证书，但未勾选跳过证书校验。

### 记录文件里 metrics_ok=false

表示本次采集失败。请查看同一行的 `error` 字段，常见原因包括超时、连接拒绝、401 未授权、接口返回非 2xx 状态码。

### 多台设备 SN 相同怎么办

勾选 `One file per device`。记录文件会优先按 IP 命名，不会因为 SN 相同而互相覆盖。
