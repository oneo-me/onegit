# 架构概览

## 当前状态

Onegit 已在 `source/` 建立独立 Go 模块，提供原生侧边栏窗口和欢迎空状态。Git 访问方式、状态刷新机制和持久化方案尚未选定；下文的业务职责划分仍是初步设计。

产品以 worktree 为核心，面向 macOS，支持多仓库状态查看与侧边栏导航。范围以[产品范围](docs/product-scope.md)为准。

## 技术基础

应用使用 Onevra `v0.2026.1` 和其对应的 MyGo 依赖，实际版本记录在 [go.mod](source/go.mod)。[Onevra](https://github.com/oneo-me/onevra) 提供主题控件和窗口壳；应用状态、持久化与异步工作由调用方负责。

Onevra 的 `shells/sidebar` 负责当前窗口、侧边栏折叠与缩放、系统主题。Onegit 在 MyGo 就绪后创建窗口，内容避让浮动标题栏，关闭主窗口即退出。当前界面只呈现空状态，不读取仓库或保存状态；后续 Git 仓库与 worktree 的业务状态由 Onegit 负责。

构建使用同版本 Onevra Packer，通过 `go.mod` 的 `tool` 指令管理项目工具依赖，从 `source/` 使用 `go tool packer` 调用。[packer.yaml](source/packer.yaml) 与应用模块放在 `source/`，产物统一写入根目录 `dist/`，不纳入版本控制。当前配置只打包 macOS arm64 ZIP，不包含自定义图标、分发签名或公证。开发命令见 [README](README.md#开发与构建)。

## 初步职责边界

| 职责 | 范围 |
| --- | --- |
| 应用界面 | 展示多个仓库及 worktree 入口，呈现所选 worktree 的分支状态。 |
| 工作上下文 | 维护仓库与 worktree 的关联，以及当前选中的 worktree。 |
| Git 状态读取与更新 | 获取各 worktree 当前分支的状态，并将变化传递给界面；具体实现待定。 |

这些职责不是已存在的模块，也不规定目录或接口。仓库可包含多个 worktree，分支状态应归属于对应 worktree，避免将一个仓库误当成只有一个当前分支的上下文。

## 目标流程

应用汇集纳入管理的仓库，获得各自的 worktree，在侧边栏提供入口，同时更新它们当前分支的状态。进入某个 worktree 后，内容区域呈现对应上下文。

导航到 worktree 表示切换应用查看的上下文，本身不意味着执行 Git 分支切换。Git 写操作的范围取决于“无用户操作”的后续澄清。

## Subjects

| 文档 | 何时阅读 |
| --- | --- |
| [产品范围](docs/product-scope.md) | 确认产品目标、功能边界和仍待澄清的要求时。 |

当前设计规模较小，尚不拆分独立架构主题；有明确且持久的设计内容后再补充。
