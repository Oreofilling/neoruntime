# CT-Disc GUI

CT-Disc GUI 是设备扫描和资源数据记录的 Windows 图形界面。

完整使用说明见：

```text
../../README.md
```

GUI exe 中文使用说明：

```text
GUI_EXE_USER_GUIDE_ZH.md
```

GUI exe English user guide:

```text
GUI_EXE_USER_GUIDE_EN.md
```

## 功能

- 扫描 CT-Disc 组播可达网络范围内的设备
- 手动添加跨网段设备 IP 或 URL
- 自动补全手动设备的 SN、FW、MAC、Product、HW
- 批量选择设备
- 记录 CPU、内存、磁盘、NPU、温度和请求延迟
- 支持 CSV / JSON Lines
- 支持一台设备一个记录文件，文件名优先按 IP 生成
- 记录过程中显示每台设备的趋势图

## 运行开发模式

```bash
cd tools/ct-disc/gui/ct-disc-gui
wails dev
```

## 构建 Windows GUI exe

```bash
cd tools/ct-disc/gui/ct-disc-gui
wails build -platform windows/amd64 -clean
```

构建产物：

```text
build/bin/ct-disc-gui.exe
```

## 手动设备补全

添加设备时可以配置协议、端口、username、token 和 HTTPS 证书校验选项。GUI 会尝试读取：

```text
/api/v1/device-info
/api/v1/network/config
/api/v1/monitor/summary
```

如果接口可达且鉴权正确，设备列表会显示真实 SN/FW/MAC。读取失败时设备仍会保留为 `Manual`，不影响记录数据。
