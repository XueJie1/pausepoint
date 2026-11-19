# 开发与打包

Pausepoint 用 Go 和 MyGo 原生界面实现，工程从 MyGo 原生模板初始化。休息调度、提醒和托盘交互参考 Stretchly。 [MyGo v0.10.0](https://github.com/egoist/mygo/tree/v0.10.0) · [Stretchly](https://github.com/hovancik/stretchly)

## 本地开发

按 README 准备 Go 工具链和桌面运行依赖。在仓库根目录运行开发模式。 [从源码构建](README.md#从源码构建)

```sh
go tool mygo dev
```

提交前用 `gofmt` 格式化改动的 Go 文件，再运行以下检查。

```sh
go test -mod=readonly -race -count=1 ./...
go vet -mod=readonly ./...
git diff --check
```

涉及平台后端时，交叉编译验证 Windows amd64 和 macOS arm64。涉及托盘或窗口时，用开发模式在桌面手动验证。架构边界和完整验证要求见项目说明。 [AGENTS.md](AGENTS.md)

## 多架构打包

`go tool mygo build` 默认构建当前系统和架构。用 `-platform` 选择目标，产物按目标放在 `build/` 下。

| 构建目标 | 输出目录 | 主要产物 |
| --- | --- | --- |
| `darwin/universal` | `build/darwin-universal/` | `Pausepoint.app`，兼容 Intel 和 Apple Silicon。在 macOS 上构建时另有 DMG |
| `windows/amd64`、`windows/arm64` | `build/windows-amd64/`、`build/windows-arm64/` | `Pausepoint.exe`、`pausepoint-setup-*.exe` |
| `linux/amd64`、`linux/arm64` | `build/linux-amd64/`、`build/linux-arm64/` | `pausepoint`、DEB、RPM、tar.gz、PKGBUILD |

在 Linux 上可以用以下命令交叉构建所有目标。生成 Windows 安装程序需要在 `PATH` 中提供 NSIS 的 `makensis`。只需 Windows 可执行文件时，加入 `-skip-installer`。

```sh
go tool mygo build -platform darwin/universal,windows/amd64,windows/arm64,linux/amd64,linux/arm64 -skip-dmg
```

从 Linux 生成的 macOS 应用包未经 macOS 签名。在 macOS 上运行 `go tool mygo build -platform darwin/universal` 可生成应用包和 DMG。Linux 软件包声明 GTK 3 和 Ayatana 托盘库依赖，无需 WebKitGTK。

## 用 GitHub Actions 生成 macOS DMG

将工作流推送到仓库默认分支后，在 **Actions → macOS DMG → Run workflow** 手动运行。工作流在 macOS runner 上生成兼容 Intel 和 Apple Silicon 的通用 DMG，可从该次运行的 **Artifacts** 下载。默认使用 Developer ID 签名并提交 Apple 公证。取消勾选签名公证选项可生成采用 ad hoc 签名的测试 DMG。 [macOS 构建工作流](.github/workflows/macos.yml)

正式签名需要 Apple Developer Program 团队的 **Developer ID Application** 证书。将证书和私钥一起导出为 `.p12`，并在仓库的 **Settings → Secrets and variables → Actions** 配置下表中的 Secrets 和变量。

| 类型 | 名称 | 值 |
| --- | --- | --- |
| Secret | `MACOS_CERTIFICATE` | `.p12` 文件的 Base64 编码 |
| Secret | `MACOS_CERTIFICATE_PASSWORD` | `.p12` 文件的导出密码 |
| Secret | `APPLE_ID` | 用于公证的 Apple 账户邮箱 |
| Secret | `APPLE_TEAM_ID` | Apple Developer 团队 ID |
| Secret | `APPLE_APP_PASSWORD` | 该 Apple 账户的 App 专用密码 |
| Variable | `MACOS_SIGNING_IDENTITY` | 证书完整名称，格式如 `Developer ID Application: Your Name (TEAMID)` |

正式构建会验证应用包和 DMG 的签名及公证票据，再上传 DMG。
