# Pausepoint

Pausepoint 是一个使用 Go 和 MyGo 原生界面的桌面休息提醒应用，默认每工作 10 分钟安排一次休息。连续两次 20 秒短休息后，下一次是 5 分钟长休息。短休息和长休息默认分别提前 10 秒和 30 秒发送桌面通知。

## 启动

在项目根目录运行以下命令。项目使用 Go 1.27.4 或更新版本，MyGo CLI 版本由 `go.mod` 固定。

```sh
go tool mygo dev
```

首次启动会打开设置窗口。点击 **Confirm settings** 保存设置并收起窗口，之后启动时只显示托盘图标。以前保存的有效配置也视为已确认。通过托盘菜单的 **Settings…** 可重新打开窗口，也可给应用的可执行文件传入 `--settings`。点击窗口的 **Close** 或系统关闭按钮会收起窗口，**Quit** 会退出应用。

可在设置窗口或托盘菜单暂停、恢复或立即开始下一次休息。时长以秒为单位，修改后点击 **Save settings** 生效。将提前提醒或推迟时长设为 `0` 可关闭该功能。设置保存到系统用户配置目录下的 `pausepoint/settings.json`，例如 Linux 上通常是 `~/.config/pausepoint/settings.json`。配置缺失时使用默认值，读取失败时会在设置窗口提示。

休息窗口显示倒计时。默认情况下，每次休息可在前 30% 的时间内推迟一次，短休息推迟 2 分钟，长休息推迟 5 分钟。严格模式禁止跳过、推迟、关掉休息窗口和在休息期间暂停计时。暂停后手动恢复、修改设置或系统睡眠后唤醒，都会从完整的工作间隔重新计时。睡眠唤醒会保留手动暂停状态。短休息和长休息的倒计时归零时均会播放 [din-ding.wav](resources/sound/din-ding.wav)。

## 桌面支持

- Linux 原生界面需要 GTK 3。托盘还需要 `libayatana-appindicator3` 和支持 StatusNotifierItem 的桌面面板。托盘初始化报错时，启动会显示设置窗口。如果桌面面板没有显示图标，可用 `--settings` 打开设置。
- 系统通知服务或权限不可用时，休息窗口仍会按时打开。Linux Wayland 上的窗口尺寸、置顶和焦点由合成器决定。Linux 的休息结束音效使用 PulseAudio（包括 PipeWire 的 PulseAudio 兼容服务）或默认的 ALSA 设备。
- 已在 Linux amd64 的 Wayland 会话验证原生窗口、调度、通知与托盘。macOS、Windows 和 Linux arm64 已验证构建产物的架构，桌面交互有待实机验证。

## 多架构打包

`go tool mygo build` 默认打包当前系统和架构。用 `-platform` 可以选择其他目标，产物按架构放在 `build/` 下。

| 系统 | 构建目标 | 主要产物 |
| --- | --- | --- |
| macOS 13 或更新版本 | `darwin/universal` | `build/darwin-universal/Pausepoint.app`，同时支持 Intel 和 Apple Silicon |
| Windows x64 和 Windows 11 on ARM | `windows/amd64`、`windows/arm64` | 对应目录中的原生 `Pausepoint.exe` 和架构对应的 `pausepoint-setup-*.exe` |
| Linux x86-64 和 ARM64 | `linux/amd64`、`linux/arm64` | 对应目录中的可执行文件、DEB、RPM、tar.gz 和 PKGBUILD |

从 Linux 交叉构建全部目标时，可以运行以下命令。Windows 安装程序需要 PATH 中提供 NSIS 的 `makensis`。只需要 Windows 可执行文件时，在命令中加入 `-skip-installer`。

```sh
go tool mygo build -platform darwin/universal,windows/amd64,windows/arm64,linux/amd64,linux/arm64 -skip-dmg
```

从 Linux 生成的 macOS 应用包未经 macOS 签名。macOS 上可以运行 `go tool mygo build -platform darwin/universal` 生成应用包和 DMG。Linux 软件包声明 GTK 3 和 Ayatana 托盘库依赖，无需安装 WebKitGTK。

## GitHub Actions 打包 macOS

将工作流推送到 GitHub 仓库的默认分支后，在 **Actions → macOS DMG → Run workflow** 手动运行。工作流在 macOS runner 上生成兼容 Intel 和 Apple Silicon 的通用 DMG，可从本次运行的 **Artifacts** 下载。默认使用 Developer ID 签名并提交 Apple 公证。取消勾选签名公证选项可以生成供测试的临时签名 DMG。

正式签名需要 Apple Developer Program 团队的 **Developer ID Application** 证书。将证书连同私钥导出为 `.p12`，在仓库的 **Settings → Secrets and variables → Actions** 添加以下 Secrets 和变量。

| 类型 | 名称 | 值 |
| --- | --- | --- |
| Secret | `MACOS_CERTIFICATE` | `.p12` 文件的 Base64 编码 |
| Secret | `MACOS_CERTIFICATE_PASSWORD` | `.p12` 文件的导出密码 |
| Secret | `APPLE_ID` | 用于公证的 Apple 账户邮箱 |
| Secret | `APPLE_TEAM_ID` | Apple Developer 团队 ID |
| Secret | `APPLE_APP_PASSWORD` | 该 Apple 账户的 App 专用密码 |
| Variable | `MACOS_SIGNING_IDENTITY` | 证书的完整名称，例如 `Developer ID Application: Your Name (TEAMID)` |

正式构建会验证应用和磁盘映像的签名及公证票据，再上传 DMG。配置见 [macOS 构建工作流](.github/workflows/macos.yml)。

## 开发与范围

```sh
go test ./...
go vet ./...
go tool mygo build
```

MVP 使用固定的「短、短、长」顺序和单个休息窗口。定时暂停、系统空闲和免打扰检测、多屏休息窗口和随机休息建议留待后续实现。行为和默认值参考 Stretchly，工程以 MyGo 的原生模板初始化。[Stretchly](https://github.com/hovancik/stretchly) · [MyGo](https://github.com/egoist/mygo/tree/v0.10.0)
