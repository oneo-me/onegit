# Onegit

Onegit 是基于 [Onevra](https://github.com/oneo-me/onevra) 构建的 macOS 桌面应用，目标是以 worktree 为中心，在一个界面中管理多个 Git 仓库，实时查看各 worktree 当前分支的状态，并通过侧边栏进入各 worktree。

目标是替代日常工作中对 Fork 的部分使用，不追求完整复刻 Fork。

## 当前阶段

应用提供原生窗口、可折叠和调整宽度的侧边栏，以及欢迎空状态。可通过侧边栏底部的设置按钮、应用菜单“设置…”或 `⌘,` 打开设置弹窗，选择项目父目录、主题和语言；修改自动保存，重启后恢复。侧边栏底部也可切换主题。点击侧边栏顶部搜索框或按 `⌘E` 打开命令面板，按名称或完整路径查找项目，也可搜索并打开主题、语言和项目父目录设置；使用方向键选择、回车确认、Esc 关闭。

启动先显示上次的项目树，再后台扫描并加入新项目；不存在的路径保留并标记。更换目录后也扫描一次，各父目录分别保留缓存。侧边栏节点默认折叠，记住各节点的展开状态，支持选择查看路径。右键项目可置顶或取消置顶；“置顶”分组按名称排序（忽略大小写）并平铺置顶项目，“其他项目”分组保留其余项目的目录层级，两个分组默认展开，可点击标题折叠；置顶与分组折叠状态重启后恢复。配置和状态分别保存到 `~/.oneo/onegit/config.yaml` 与 `state.yaml`，首次使用新路径时自动导入旧设置。支持普通仓库、linked worktree、已初始化子模块及裸仓库；遇到有效仓库即停止向内扫描，仓库内的嵌套项目不会单独列出；选择项目后，右侧显示全部关联 worktree（包括父目录外的），主仓库排在最上方，以侧边栏风格的简洁列表显示名称，点击可选中并高亮。项目概览作为独立层级，选中 worktree 后的子界面尚未实现；不持续扫描或刷新分支状态。运行时需能从 PATH 找到 Git。

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

[打包配置](source/packer.yaml) 默认生成 Apple Silicon 的 macOS ZIP，位于 `dist/releases/0.1.0/`，最低系统版本声明为 macOS 13。Intel 目标可在配置中改为 `darwin/amd64/zip`，尚未做本机运行验证。产物仅作本地开发验证，使用临时签名，未经公证；`0.1.0` 是初始构建版本，不表示已发布。`go test` 覆盖设置交互与持久化、Git 项目识别、worktree 列表读取与项目切换、目录树交互、扫描取消、缓存合并与恢复及旧设置迁移；原生目录选择器仍需人工验证。

## 项目文档

- [产品范围](docs/product-scope.md)：已确认的方向与暂不展开的细节。
- [架构概览](ARCHITECTURE.md)：技术基础、初步职责边界与核心流程。
- [协作约定](AGENTS.md)：后续开发和文档维护规则。
- [更新记录](CHANGELOG.md)：面向应用使用者的版本变化。

开始开发前，完整阅读架构概览，再阅读相关主题文档。
