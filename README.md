# Onegit

Onegit 是基于 [Onevra](https://github.com/oneo-me/onevra) 构建的 macOS 桌面应用，目标是以 worktree 为中心，在一个界面中管理多个 Git 仓库，实时查看各 worktree 当前分支的状态，并通过侧边栏进入各 worktree。

目标是替代日常工作中对 Fork 的部分使用，不追求完整复刻 Fork。

## 当前阶段

项目已完成最小应用初始化：提供跟随系统主题的原生窗口、可折叠和调整宽度的侧边栏，以及欢迎空状态。仓库接入、worktree 导航和 Git 状态读取尚未实现。

## 开发与构建

需要 macOS、Go 1.27.1 或更新版本及 Xcode Command Line Tools。应用代码和 Go 模块位于 [source](source/)，依赖固定为 Onevra `v0.2026.1`。

同版本的 Onevra Packer 已通过 `go.mod` 的 `tool` 指令声明为项目工具依赖，无需全局安装。以下命令均从仓库根目录进入 `source` 后执行：

```sh
cd source
GOWORK=off go mod download
go tool packer run
```

`go tool packer run` 构建并运行 `dist/Onegit Dev.app`，关闭窗口退出应用；不提供热更新。也可用 `go run .` 直接启动。

```sh
go tool packer build
GOWORK=off go test ./...
GOWORK=off go vet ./...
```

[打包配置](source/packer.yaml) 默认生成 Apple Silicon 的 macOS ZIP，位于 `dist/releases/0.1.0/`，最低系统版本声明为 macOS 13。Intel 目标可在配置中改为 `darwin/amd64/zip`，尚未做本机运行验证。产物仅作本地开发验证，使用临时签名，未经公证；`0.1.0` 是初始构建版本，不表示已发布。当前没有自动化测试用例，`go test` 仅检查包能否编译。

## 项目文档

- [产品范围](docs/product-scope.md)：已确认的方向与暂不展开的细节。
- [架构概览](ARCHITECTURE.md)：技术基础、初步职责边界与核心流程。
- [协作约定](AGENTS.md)：后续开发和文档维护规则。
- [更新记录](CHANGELOG.md)：面向应用使用者的版本变化。

开始开发前，完整阅读架构概览，再阅读相关主题文档。
