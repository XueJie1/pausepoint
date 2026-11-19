# <img src="resources/icon.png" width="48" height="48" alt=""> Pausepoint

Pausepoint 是一款桌面休息提醒应用。它通过托盘和倒计时窗口提醒你定时离开屏幕。默认每工作 10 分钟休息一次，按「短、短、长」循环。

| 默认安排 | 休息时长 | 提前通知 | 推迟时长 |
| --- | --- | --- | --- |
| 短休息 | 20 秒 | 10 秒 | 2 分钟 |
| 长休息 | 5 分钟 | 30 秒 | 5 分钟 |

## 日常使用

1. 首次启动会打开设置。可以保留默认值，也可修改「休息计划」。关闭窗口后，应用会留在托盘。完成首次设置后，再次启动通常只显示托盘图标。
2. 从托盘打开「设置…」，也可选择「暂停提醒」「恢复提醒」「立即开始下次休息」或「退出」。如果托盘图标未显示，启动可执行文件时加上 `--settings` 即可打开设置。
3. 休息开始后，窗口会显示倒计时。默认可以跳过休息，或在倒计时的前 30% 内推迟一次。严格模式下，休息期间无法推迟、跳过、关闭休息窗口或暂停提醒。倒计时自然结束时会播放提示音。 [试听提示音](resources/sound/din-ding.wav)

## 从源码构建

构建需要 Git 和 Go 1.27.4 或更新版本。Go 会按项目配置获取固定版本的 MyGo CLI。 [go.mod](go.mod)

```sh
git clone https://github.com/XueJie1/pausepoint.git
cd pausepoint
go tool mygo build
```

产物位于 `build/` 下对应系统和架构的目录中。

| 平台 | 如何启动 | 运行要求 |
| --- | --- | --- |
| Linux x86-64、ARM64 | 运行 `pausepoint`，或安装对应的软件包 | GTK 3。托盘还需要 Ayatana AppIndicator 库和支持 StatusNotifierItem 的桌面面板 |
| macOS Intel、Apple Silicon | 打开 `Pausepoint.app` | macOS 13 或更新版本 |
| Windows x64、ARM64 | 运行 `Pausepoint.exe`，或使用 `pausepoint-setup-*.exe` 安装 | ARM64 产物面向 Windows 11 on ARM |

## 设置

- **休息计划**中的时长以秒为单位。修改会自动保存，数字输入会在短暂停顿后应用。无效输入会显示错误并保留上次生效的配置。将对应时长设为 `0` 可关闭提前通知或推迟。在等待阶段修改休息计划会重置计时，已开始的休息沿用开始时的规则。
- **通用**可开关休息提醒和选择界面语言。支持简体中文、英文、阿拉伯文、法文、俄文和西班牙文。默认跟随系统，找不到对应语言时使用英文，阿拉伯文界面从右到左排列。
- **外观**可选深色、浅色或跟随系统，默认深色。预览使用独立的短休息倒计时。在设置窗口内可用 `⌘⇧B`（macOS）或 `Ctrl+Shift+B`（Windows、Linux）打开预览，也可在「快捷键」中重新录制。

设置保存在系统用户配置目录下的 `pausepoint/settings.json`，例如 Linux 上通常是 `~/.config/pausepoint/settings.json`。如果文件缺失，应用会使用默认值。读取失败时，设置窗口会显示提示。

## 平台与功能限制

- 已在 Linux x86-64 的 Wayland 会话验证窗口、调度、通知和托盘。macOS、Windows 和 Linux ARM64 的构建产物已验证架构，桌面交互有待实机验证。
- 桌面通知不可用时，休息窗口仍会按时显示。Linux 通过会话 D-Bus 发送通知，结束提示音使用 PulseAudio（含 PipeWire 的 PulseAudio 兼容服务）或默认 ALSA 设备。Wayland 下的窗口尺寸、置顶和焦点由合成器决定。
- 手动恢复提醒后，应用会从完整的工作间隔重新计时。收到系统唤醒事件时，正在运行的计划也会重新计时，手动暂停状态会保留。
- 目前使用单个休息窗口。定时暂停、空闲和免打扰检测、多屏休息窗口、随机休息建议尚未实现。

## 参与开发

开发模式、检查命令、多架构打包及 macOS 签名和公证流程见贡献指南。 [CONTRIBUTING.md](CONTRIBUTING.md)

## 许可证

源代码采用 MIT 许可证。随附的 MyGo 许可和 Freesound 提示音的署名及许可见第三方声明。 [MIT 许可证](LICENSE) · [第三方声明](resources/THIRD-PARTY-NOTICES.txt)
