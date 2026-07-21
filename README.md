# freexnats

一个跑在终端里的 NATS JetStream 管理器。查看 Stream / Consumer / KV、发布与浏览消息、实时 Tail 订阅，全键盘 + 全鼠标交互。

## 安装

**macOS / Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/CooDdk/freexnats/master/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/CooDdk/freexnats/master/install.ps1 | iex
```

**手动下载**

从 [Releases](https://github.com/CooDdk/freexnats/releases) 下载对应平台二进制，放到 `$PATH` 中即可。

## 使用

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

## 功能

- **Streams** — 列表、详情、创建、编辑、Purge、删除。
- **Consumers** — 跨 Stream 聚合视图、Lag 严重度指示、创建、删除、重置游标。
- **Messages** — 顶部 Stream 选择器 + History / Live Tail 分段控件。
  - History：按 seq 双向浏览（`h` / `l`），载荷全屏详情（`v`），一键复制（`y`），按 seq 删除（`d`）。
  - Live Tail：按 subject / payload 过滤实时订阅。
- **KV Store** — 桶创建/删除、Key 查看/编辑/删除。
- **Publish** — 任意 Stream 目标发布，含 header 编辑。

## 快捷键

**全局**

| 按键                       | 功能                          |
| -------------------------- | ----------------------------- |
| `Tab` / `Shift+Tab`        | 切换标签页                    |
| `↑` `↓` `←` `→`           | 焦点在页面部件间移动          |
| `Enter`                    | 激活当前焦点部件              |
| `Esc`                      | 返回 / 关闭浮层               |
| `Ctrl+C`                   | 退出                          |

**表格 / 列表**

| 按键          | 功能                       |
| ------------- | -------------------------- |
| `j` / `k`     | 上下移动光标               |
| `g` / `G`     | 跳到首/末行                |
| `PgUp` / `PgDn` | 翻页                     |
| 鼠标滚轮      | 滚动                       |
| 鼠标点击      | 选中行 / 触发行内动作      |

**Messages · History**

| 按键                | 功能                                   |
| ------------------- | -------------------------------------- |
| `h` / `l`           | 上一条 / 下一条消息（按 seq）         |
| `j` / `k`           | Payload 滚动                           |
| `y`                 | 复制 Payload 到系统剪贴板             |
| `v`                 | 打开全屏详情视图                      |
| `Shift + 鼠标拖拽`  | 终端原生文本选择（避开 Bubble Tea 鼠标捕获） |
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

## 从源码构建

需要 Go 1.25 或更高版本：

```sh
git clone https://github.com/CooDdk/freexnats.git
cd freexnats
go build -o freexnats .
```

## License

MIT
