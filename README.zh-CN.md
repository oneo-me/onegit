# Onegit

[English](README.md) | **简体中文**

Onegit 是以 worktree 为中心的 macOS 应用，在一个窗口中汇集多个 Git 仓库。它承接日常的仓库浏览与 worktree 切换，不追求覆盖通用 Git 客户端的全部功能。

## 功能

- 扫描项目父目录中的 Git 仓库，并在侧边栏以目录树浏览。
- 将项目置顶到目录树上方按名称排序的平铺分组。
- 通过命令面板（`⌘E`）查找项目与设置。
- 以标签页在项目的全部 worktree 之间切换。
- 分别查看已暂存与未暂存的改动，按文件、改动块或所选行暂存，并填写信息提交。
- 浏览可折叠合并的完整提交图谱、历史查找，以及含差异与文件内容的提交详情。
- 浏览本地分支、远程分支与标签，并跳转到对应提交。
- 双击引用或提交即可检出，也可在其菜单中检出到当前或新的 worktree，拉取、合并或变基到它，新建分支、标签，删除或复制名称。
- 获取、拉取、推送、贮藏和菜单操作等 Git 写入仅在点击时执行。

## 运行

Onegit 尚未发布，请按[开发](#开发)从源码构建。运行时需能从 `PATH` 找到 Git。

## 快速开始

1. 按 `⌘,` 打开设置，选择存放项目的父目录。
2. 等待后台扫描把仓库加入侧边栏。
3. 选择项目，再选择 worktree 标签页。
4. 在项目侧边栏中选择“当前更改”或“全部提交”。

## 开发

需要 macOS、Go 1.27.1 或更新版本及 Xcode Command Line Tools。MyGo CLI 是 [go.mod](source/go.mod) 中声明的 Go 工具依赖，无需全局安装。

在 `source/` 下运行：

```sh
cd source
GOWORK=off go mod download
go tool mygo dev
```

`go tool mygo dev` 运行 `Onegit Dev`，代码修改后自动重启。

在 `source/` 下格式检查、测试、检查与构建：

```sh
gofmt -l .
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go tool mygo vet
GOWORK=off go tool mygo build
```

测试均为无界面运行，不会打开窗口或原生对话框。目录选择器、打包与应用图标需人工检查。产物写入 `dist/<os>-<arch>/`，使用临时签名且未经公证。

在 `source/` 下构建全平台发布包：macOS 通用应用与磁盘映像、Windows 安装程序，以及 Linux 归档与 Debian 包。Windows 安装程序需要 NSIS（`brew install makensis`）。

```sh
GOWORK=off go tool mygo build -platform darwin/universal,windows/amd64,windows/arm64,linux/amd64,linux/arm64
```

Neorio 用户可从 [.neorio/config.yaml](.neorio/config.yaml) 获得相同的 Run、Build、Test 与 Vet 任务，其中 Build 构建上述发布包。

修改代码前先阅读 [ARCHITECTURE.md](ARCHITECTURE.md)，协作规则见 [AGENTS.md](AGENTS.md)。用户可感知的变化记录在 [CHANGELOG.md](CHANGELOG.md)。
