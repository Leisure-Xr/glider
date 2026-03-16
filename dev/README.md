# 节点代理池快速启动指南

本目录提供了将订阅节点转换为 glider 代理池配置并启动服务的完整流程。

## 目录结构

```
dev/
├── node.txt                  # 节点列表（每行一个节点链接）
├── gen_glider_pool_conf.py   # 配置生成脚本
├── glider.pool.conf          # 生成的代理池配置文件
└── README.md                 # 本文档
```

## 第一步：准备节点列表

将你的代理节点链接写入 `node.txt`，每行一个，支持以下协议：

- `ss://` — Shadowsocks（SIP002 格式，支持 obfs 插件）
- `vmess://` — VMess（v2rayN base64 JSON 格式）
- `trojan://` — Trojan
- `vless://` — VLESS
- `ssr://` — ShadowsocksR

以 `#` 开头的行会被忽略，空行也会被跳过。

## 第二步：生成代理池配置

在项目根目录下运行：

```bash
python3 dev/gen_glider_pool_conf.py
```

默认读取 `dev/node.txt`，输出到 `dev/glider.pool.conf`。

### 可选参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--nodes` | `dev/node.txt` | 节点列表文件路径 |
| `--out` | `dev/glider.pool.conf` | 输出配置文件路径 |
| `--listen-socks` | `127.0.0.1:10810` | SOCKS5 监听地址 |
| `--listen-http` | `127.0.0.1:10811` | HTTP 监听地址 |
| `--strategy` | `rr` | 负载均衡策略（rr/ha/lha/dh） |
| `--verbose / --no-verbose` | `--verbose` | 是否开启详细日志 |
| `--dialtimeout` | `3` | 连接超时（秒） |
| `--checkinterval` | `30` | 健康检查间隔（秒） |
| `--checktimeout` | `10` | 健康检查超时（秒） |
| `--maxfailures` | `3` | 最大失败次数，超过则禁用该节点 |
| `--checkdisabledonly` | 关闭 | 仅检查已禁用的转发器 |

示例：自定义监听端口和策略

```bash
python3 dev/gen_glider_pool_conf.py \
  --listen-socks 0.0.0.0:1080 \
  --listen-http 0.0.0.0:8080 \
  --strategy lha \
  --checkinterval 60
```

## 第三步：启动 glider 代理池

### 方式一：使用预编译二进制文件

1. 从 [GitHub Releases](https://github.com/nadoo/glider/releases) 下载对应平台的二进制文件
2. 运行：

```bash
./glider -config dev/glider.pool.conf
```

### 方式二：从 Go 源码编译运行

确保已安装 Go（1.21+），然后：

```bash
# 编译
go build -v -ldflags "-s -w" -o glider .

# 运行
./glider -config dev/glider.pool.conf
```

或直接用 `go run`：

```bash
go run . -config dev/glider.pool.conf
```

## 验证代理是否工作

启动后，可以通过以下方式测试：

```bash
# 测试 SOCKS5 代理
curl -x socks5://127.0.0.1:10810 https://www.google.com

# 测试 HTTP 代理
curl -x http://127.0.0.1:10811 https://www.google.com
```

如果开启了 `verbose` 模式，终端会输出健康检查结果和每个连接的转发日志，方便排查节点可用性。

## 负载均衡策略说明

| 策略 | 说明 |
|------|------|
| `rr` | 轮询 — 依次使用每个可用节点 |
| `ha` | 高可用 — 优先使用第一个可用节点 |
| `lha` | 基于延迟的高可用 — 优先使用延迟最低的节点 |
| `dh` | 目标哈希 — 相同目标地址始终使用同一节点 |
