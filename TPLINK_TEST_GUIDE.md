# TP-Link MULTITRANS 协议测试指南 (Windows)

本文档指导如何在 Windows 环境下编译、配置并测试新实现的 TP-Link 双向语音功能。

## 1. 编译 (Compilation)

确保你已经安装了 Go 语言环境。

打开终端 (PowerShell 或 CMD)，进入 `go2rtc` 项目目录：

```powershell
cd e:\Project\homeassistant_projects\go2rtc
```

执行编译命令：

```powershell
# 禁用 CGO 以确保生成的 exe 更通用 (可选)
$env:CGO_ENABLED="0" 
go build -o go2rtc_tplink.exe
```

如果编译成功，目录下会生成 `go2rtc_tplink.exe`。

## 2. 配置 (Configuration)

在同一目录下创建一个名为 `go2rtc.yaml` 的配置文件（如果已有，请备份并修改）。

根据你提供的设备信息，配置如下：

```yaml
streams:
  tplink_cam:
    # 视频流 (通常使用 standard RTSP)
    - rtsp://admin:admin@192.168.1.202:554/stream1
    # 语音对讲通道 (使用新实现的 MULTITRANS 协议)
    - multitrans://admin:admin@192.168.1.202:554/
```

> [!NOTE]
> `multitrans` 链接主要用于建立语音回传通道 (Backchannel)。

## 3. 运行与测试 (Running & Testing)

1.  **启动程序**:
    在终端中运行编译好的程序：
    ```powershell
    .\go2rtc_tplink.exe
    ```
    观察控制台输出，确认程序启动且没有报错。

2.  **访问 Web 界面**:
    打开浏览器访问: [http://localhost:1984](http://localhost:1984)

3.  **开始对讲**:
    - 在 Web 界面中找到 `tplink_cam`。
    - 点击 `stream` 链接进入预览页面。
    - 确保页面显示了视频画面。
    - 点击视频窗口下方的 **麦克风图标** (Microphone)。
    - 浏览器可能会请求麦克风权限，请点击“允许”。
    - 对着麦克风说话。

4.  **验证结果**:
    - 你应该能从摄像机端听到你的声音。
    - **查看日志**: 回到运行 `go2rtc` 的终端窗口，观察是否有类似以下的日志（表示链接成功）：
      ```text
      [streams] prod=multitrans://...
      ```
      或者如果没有报错信息，通常也代表连接正常。

## 故障排查

- **无法听到声音**: 
  - 检查 Web 界面麦克风图标是否激活。
  - 检查 go2rtc 终端是否有 `multitrans: auth failed` 或其他错误。
  - 尝试使用 `rtsp://admin:admin@192.168.1.202/stream2` 作为主视频流试试（有些相机子码流更稳定）。

- **编译失败**:
  - 确保 Go 环境已确安装 (`go version`)。
  - 如果提示缺少依赖，运行 `go mod tidy`。
