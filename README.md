<div align="center">

# freexnats

**跑在终端里的 NATS JetStream 管理器**

一屏搞定 Stream / Consumer / KV / Messages —— 全键盘 · 全鼠标 · 实时 Tail

[![Release](https://img.shields.io/github/v/release/CooDdk/freexnats?style=flat-square)](https://github.com/CooDdk/freexnats/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/CooDdk/freexnats/ci.yml?branch=master&label=CI&style=flat-square)](https://github.com/CooDdk/freexnats/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/CooDdk/freexnats?style=flat-square)](https://goreportcard.com/report/github.com/CooDdk/freexnats)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/CooDdk/freexnats?style=flat-square)](go.mod)

![freexnats preview](screenshots/preview.png)

</div>

## ✨ Features

- 🌊 **Streams** — 列表 · 详情 · 创建 · 编辑 · Purge · 删除
- 👥 **Consumers** — 跨 Stream 聚合视图，Lag 严重度指示，重置游标
- 💬 **Messages** — 顶部 Stream 选择器 + History / Live Tail 分段控件
  - History: 按 seq 双向浏览 (`h`/`l`)、全屏详情 (`v` / `Enter`)、一键复制 (`y`)、按 seq 删除
  - Live Tail: 按 subject / payload 过滤实时订阅
- 🗄️ **KV Store** — 桶管理 · Key 查看 / 编辑 / 删除
- 📤 **Publish** — 任意 Stream 目标发布，含 Header 编辑
- ⌨️ **全键盘导航** — 完整焦点管理，无需鼠标即可操作所有功能
- 🖱️ **全鼠标支持** — 表格、按钮、下拉、滚轮均可点击
- 🎨 **原生终端 UI** — 基于 Bubble Tea · Lipgloss，零依赖运行

## 📦 安装

**macOS / Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/CooDdk/freexnats/master/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/CooDdk/freexnats/master/install.ps1 | iex
```

**手动下载**

从 [Releases](https://github.com/CooDdk/freexnats/releases) 下载对应平台的二进制，放到 `$PATH` 中即可。

## 🚀 使用

```sh
freexnats                                       # 连接 nats://localhost:4222
freexnats --url nats://remote:4222              # 指定服务端
freexnats --url nats://remote:4222 --user admin --pass secret
freexnats --token <TOKEN>                       # Token 认证
freexnats --no-splash                           # 跳过启动动画
freexnats -v                                    # 打印版本
```

在应用内切换连接：进入 **Settings** 标签页，编辑 URL / User / Pass / Token 后点击 **Connect**。配置会保存到本地：

| 平台    | 路径                                              |
| ------- | ------------------------------------------------- |
| macOS   | `~/Library/Application Support/freexnats/`        |
| Linux   | `~/.config/freexnats/`                            |
| Windows | `%AppData%\freexnats\`                            |

## ⌨️ 快捷键

**全局**

| 按键                       | 功能                          |
| -------------------------- | ----------------------------- |
| `Tab` / `Shift+Tab`        | 切换标签页                    |
| `↑` `↓` `←` `→`           | 焦点在页面部件间移动          |
| `Enter`                    | 激活当前焦点部件              |
| `Esc`                      | 返回 / 关闭浮层               |
| `Ctrl+C`                   | 退出                          |

**表格 / 列表**

| 按键              | 功能                       |
| ----------------- | -------------------------- |
| `j` / `k`         | 上下移动光标               |
| `g` / `G`         | 跳到首/末行                |
| `PgUp` / `PgDn`   | 翻页                       |
| 鼠标滚轮 / 点击   | 滚动 / 触发行内动作        |

**Messages · History**

| 按键                | 功能                                   |
| ------------------- | -------------------------------------- |
| `h` / `l`           | 上一条 / 下一条消息（按 seq）         |
| `j` / `k`           | Payload 滚动                           |
| `y`                 | 复制 Payload 到系统剪贴板             |
| `v` / `Enter`       | 打开全屏详情视图                      |
| `Shift + 鼠标拖拽`  | 终端原生文本选择                      |
| `P`                 | 打开 Publish 表单                     |
| `d`                 | 删除当前消息                          |
| `t`                 | 切换到 Live Tail                       |
| `r`                 | 刷新                                   |

**Messages · Detail (`v`)**

| 按键                | 功能                     |
| ------------------- | ------------------------ |
| `j` / `k`, `PgUp/PgDn`, `g` / `G` | Payload 滚动 |
| `h` / `l`           | 相邻消息切换             |
| `y`                 | 复制 Payload             |
| `Esc` / `v` / `q`   | 返回                     |

## 🛠️ Development

需要 **Go 1.25+** 与一个可访问的 NATS JetStream 服务。

```sh
# 克隆
git clone https://github.com/CooDdk/freexnats.git
cd freexnats

# 快速运行（连接本地 NATS）
go run . --url nats://localhost:4222

# 构建
go build -o freexnats .

# 测试
go test ./...
```

本地起 NATS（Docker）：

```sh
docker run --rm -p 4222:4222 nats:latest -js
```

**项目结构**

```
freexnats/
├── main.go                    # 入口 + CLI 参数
├── internal/
│   ├── app/                   # 顶层 tea.Model，Tab 布局，全局键鼠派发
│   ├── nats/                  # NATS JetStream 客户端封装
│   ├── config/                # 版本 / 配置持久化
│   └── ui/
│       ├── components/        # Tabs · Toolbar · Form · Dialog · StreamSelector ...
│       ├── focus/             # 键盘焦点管理器
│       ├── pages/             # Streams · Consumers · Messages · KV · Settings 页
│       └── styles.go          # 主题色板
└── pkg/utils/                 # 通用格式化工具
```

## 🤝 Contributing

Issues 与 Pull Requests 都欢迎。

- 报 bug / 提需求：[开个 Issue](https://github.com/CooDdk/freexnats/issues/new)
- 提交代码前请确保 `go build ./...` 与 `go vet ./...` 通过
- 遵循已有的代码风格（见 `internal/ui/components/` 中的组件模式）

## 📄 License

[MIT](LICENSE)
