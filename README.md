# [glider](https://github.com/nadoo/glider)

[![Go Version](https://img.shields.io/github/go-mod/go-version/nadoo/glider?style=flat-square)](https://go.dev/dl/)
[![Go Report Card](https://goreportcard.com/badge/github.com/nadoo/glider?style=flat-square)](https://goreportcard.com/report/github.com/nadoo/glider)
[![GitHub release](https://img.shields.io/github/v/release/nadoo/glider.svg?style=flat-square&include_prereleases)](https://github.com/nadoo/glider/releases)
[![Actions Status](https://img.shields.io/github/actions/workflow/status/nadoo/glider/build.yml?branch=dev&style=flat-square)](https://github.com/nadoo/glider/actions)
[![DockerHub](https://img.shields.io/docker/image-size/nadoo/glider?color=blue&label=docker&style=flat-square)](https://hub.docker.com/r/nadoo/glider)

glider 是一个支持多协议的转发代理，同时也是一个具备 ipset 管理功能的 DNS/DHCP 服务器（类似 dnsmasq）。

我们可以将本地监听器设置为代理服务器，并通过转发器将请求转发到互联网。

```bash
                |转发器 ----------------->|
   监听器 --> |                            | 互联网
                |转发器 --> 转发器->...|
```

## 功能特性
- 同时作为代理客户端和代理服务器（协议转换器）
- 灵活的代理链和协议链
- 支持以下负载均衡调度算法：
  - rr：轮询
  - ha：高可用
  - lha：基于延迟的高可用
  - dh：目标哈希
- 基于规则和优先级的转发器选择：[配置示例](config/examples)
- DNS 转发服务器：
  - 通过代理转发 DNS
  - 强制使用 TCP 查询上游 DNS
  - DNS 与转发器选择的关联规则
  - DNS 与 ipset 的关联规则
  - DNS 缓存支持
  - 自定义 DNS 记录
- IPSet 管理（Linux 内核版本 >= 2.6.32）：
  - 启动时从规则文件添加 IP/CIDR
  - 通过 DNS 转发服务器为规则文件中的域名添加解析 IP
- 在同一端口同时支持 HTTP 和 SOCKS5
- 定期检测转发器可用性
- 从指定本地 IP/接口发送请求
- 服务：
  - dhcpd：简单的 DHCP 服务器，支持故障转移模式

## 支持的协议

<details>
<summary>点击展开详情</summary>

|协议           | 监听/TCP |  监听/UDP | 转发/TCP | 转发/UDP | 说明
|:-:            |:-:|:-:|:-:|:-:|:-
|Mixed          |√|√| | |HTTP+SOCKS5 服务器
|HTTP           |√| |√| |客户端 & 服务端
|SOCKS5         |√|√|√|√|客户端 & 服务端
|SS             |√|√|√|√|客户端 & 服务端
|Trojan         |√|√|√|√|客户端 & 服务端
|Trojanc        |√|√|√|√|Trojan 明文（不含 TLS）
|VLESS          |√|√|√|√|客户端 & 服务端
|VMess          | | |√|√|仅客户端
|SSR            | | |√| |仅客户端
|SSH            | | |√| |仅客户端
|SOCKS4         | | |√| |仅客户端
|SOCKS4A        | | |√| |仅客户端
|TCP            |√| |√| |TCP 隧道客户端 & 服务端
|UDP            | |√| |√|UDP 隧道客户端 & 服务端
|TLS            |√| |√| |传输层客户端 & 服务端
|KCP            | |√|√| |传输层客户端 & 服务端
|Unix           |√|√|√|√|传输层客户端 & 服务端
|VSOCK          |√| |√| |传输层客户端 & 服务端
|Smux           |√| |√| |传输层客户端 & 服务端
|Websocket(WS)  |√| |√| |传输层客户端 & 服务端
|WS Secure      |√| |√| |WebSocket 加密（wss）
|Proxy Protocol |√| | | |仅版本 1 服务端
|Simple-Obfs    | | |√| |仅传输层客户端
|Redir          |√| | | |Linux 透明代理
|Redir6         |√| | | |Linux 透明代理（IPv6）
|TProxy         | |√| | |Linux tproxy（仅 UDP）
|Reject         | | |√|√|拒绝所有请求

</details>

## 安装

- 二进制：[https://github.com/nadoo/glider/releases](https://github.com/nadoo/glider/releases)
- Docker：`docker pull nadoo/glider`
- Manjaro：`pamac install glider`
- ArchLinux：`sudo pacman -S glider`
- Homebrew：`brew install glider`
- MacPorts：`sudo port install glider`
- 源码：`go install github.com/nadoo/glider@latest`

## 使用方法

#### 运行

```bash
glider -verbose -listen :8443
# docker run --rm -it nadoo/glider -verbose -listen :8443
```

#### 帮助

<details>
<summary><code>glider -help</code></summary>

```bash
用法：glider [-listen URL]... [-forward URL]... [选项]...

  示例：glider -config /etc/glider/glider.conf
        glider -listen :8443 -forward socks5://serverA:1080 -forward socks5://serverB:1080 -verbose

选项：
  -check string
        check=tcp[://HOST:PORT]: TCP 端口连接检测
        check=http://HOST[:PORT][/URI][#expect=REGEX_MATCH_IN_RESP_LINE]
        check=https://HOST[:PORT][/URI][#expect=REGEX_MATCH_IN_RESP_LINE]
        check=file://SCRIPT_PATH: 运行检测脚本，退出码为 0 表示健康，环境变量：FORWARDER_ADDR,FORWARDER_URL
        check=disable: 禁用健康检查（默认："http://www.msftconnecttest.com/connecttest.txt#expect=200"）
  -checkdisabledonly
        仅检查已禁用的转发器
  -checkinterval int
        转发器健康检查间隔（秒）（默认 30）
  -checklatencysamples int
        使用最近 N 次检查的平均延迟（默认 10）
  -checktimeout int
        转发器健康检查超时（秒）（默认 10）
  -checktolerance int
        转发器检查容忍值（毫秒），仅在 lha 模式下有效
  -config string
        配置文件路径
  -dialtimeout int
        连接超时（秒）（默认 3）
  -dns string
        本地 DNS 服务器监听地址
  -dnsalwaystcp
        始终使用 TCP 查询上游 DNS 服务器
  -dnscachelog
        显示 DNS 缓存查询日志
  -dnscachesize int
        DNS 缓存最大条目数（默认 4096）
  -dnsmaxttl int
        DNS 缓存最大 TTL（秒）（默认 1800）
  -dnsminttl int
        DNS 缓存最小 TTL（秒）
  -dnsnoaaaa
        禁用 AAAA 查询（IPv6 DNS）
  -dnsrecord value
        自定义 DNS 记录，格式：域名/IP
  -dnsserver value
        上游 DNS 服务器地址
  -dnstimeout int
        多 DNS 服务器切换超时（秒）（默认 3）
  -example
        显示使用示例
  -forward value
        转发地址，详见下方 URL 说明
  -include value
        包含文件
  -interface string
        来源 IP 或网络接口
  -listen value
        监听地址，详见下方 URL 说明
  -logflags int
        日志标志位，不了解请勿修改（默认 19）
  -maxfailures int
        触发禁用转发器的最大失败次数（默认 3）
  -relaytimeout int
        中继超时（秒）
  -rulefile value
        规则文件路径
  -rules-dir string
        规则文件目录
  -scheme string
        显示指定代理方案的帮助信息，使用 'all' 查看所有方案
  -service value
        运行指定服务，格式：SERVICE_NAME[,SERVICE_CONFIG]
  -strategy string
        rr: 轮询模式
        ha: 高可用模式
        lha: 基于延迟的高可用模式
        dh: 目标哈希模式（默认 "rr"）
  -tcpbufsize int
        TCP 缓冲区大小（字节）（默认 32768）
  -udpbufsize int
        UDP 缓冲区大小（字节）（默认 2048）
  -verbose
        详细模式，打印调试日志

URL 格式：
   代理：SCHEME://[USER:PASS@][HOST]:PORT
   链式：proxy,proxy[,proxy]...

    示例：-listen socks5://:1080
          -listen tls://:443?cert=crtFilePath&key=keyFilePath,http://    （协议链）

    示例：-forward socks5://server:1080
          -forward tls://server.com:443,http://                          （协议链）
          -forward socks5://serverA:1080,socks5://serverB:1080           （代理链）

协议方案（SCHEME）：
   监听：http kcp mixed pxyproto redir redir6 smux sni socks5 ss tcp tls tproxy trojan trojanc udp unix vless vsock ws wss
   转发：direct http kcp reject simple-obfs smux socks4 socks4a socks5 ss ssh ssr tcp tls trojan trojanc udp unix vless vmess vsock ws wss

   提示：使用 'glider -scheme all' 或 'glider -scheme SCHEME' 查看具体方案帮助。

--
转发器选项：FORWARD_URL#OPTIONS
   priority ：转发器优先级，数值越大优先级越高，默认：0
   interface：连接远程服务器时使用的本地接口或 IP 地址。

   示例：-forward socks5://server:1080#priority=100
         -forward socks5://server:1080#interface=eth0
         -forward socks5://server:1080#priority=100&interface=192.168.1.99

服务：
   dhcpd: service=dhcpd,网卡,起始IP,结束IP,租约分钟数[,MAC=IP,MAC=IP...]
          service=dhcpd-failover,网卡,起始IP,结束IP,租约分钟数[,MAC=IP,MAC=IP...]
     示例：service=dhcpd,eth1,192.168.1.100,192.168.1.199,720

--
帮助：
   glider -help
   glider -scheme all
   glider -example

详见 README.md 和 glider.conf.example。
--
glider 0.16.4, https://github.com/nadoo/glider (glider.proxy@gmail.com)
```

</details>

#### 协议方案

<details>
<summary><code>glider -scheme all</code></summary>

```bash
Direct 方案：
  direct://

仅在需要指定出口网络接口时使用：
  glider -verbose -listen :8443 -forward direct://#interface=eth0

或直接对多个接口进行负载均衡：
  glider -verbose -listen :8443 -forward direct://#interface=eth0 -forward direct://#interface=eth1 -strategy rr

或使用高可用模式：
  glider -verbose -listen :8443 -forward direct://#interface=eth0&priority=100 -forward direct://#interface=eth1&priority=200 -strategy ha

--
Http 方案：
  http://[user:pass@]host:port

--
KCP 方案：
  kcp://CRYPT:KEY@host:port[?dataShards=NUM&parityShards=NUM&mode=MODE]

KCP 可用加密类型：
  none, sm4, tea, xor, aes, aes-128, aes-192, blowfish, twofish, cast5, 3des, xtea, salsa20

KCP 可用模式：
  fast, fast2, fast3, normal，默认：fast

--
Simple-Obfs 方案：
  simple-obfs://host:port[?type=TYPE&host=HOST&uri=URI&ua=UA]

Simple-Obfs 可用类型：
  http, tls

--
Reject 方案：
  reject://

--
Smux 方案：
  smux://host:port

--
Socks4 方案：
  socks4://host:port

--
Socks5 方案：
  socks5://[user:pass@]host:port

--
SS 方案：
  ss://method:pass@host:port

  SS 可用加密方法：
    AEAD 加密：
      AEAD_AES_128_GCM AEAD_AES_192_GCM AEAD_AES_256_GCM AEAD_CHACHA20_POLY1305 AEAD_XCHACHA20_POLY1305
    流式加密：
      AES-128-CFB AES-128-CTR AES-192-CFB AES-192-CTR AES-256-CFB AES-256-CTR CHACHA20-IETF XCHACHA20 CHACHA20 RC4-MD5
    别名：
          chacha20-ietf-poly1305 = AEAD_CHACHA20_POLY1305, xchacha20-ietf-poly1305 = AEAD_XCHACHA20_POLY1305
    明文：NONE

--
SSH 方案：
  ssh://user[:pass]@host:port[?key=keypath&timeout=SECONDS]
    timeout：SSH 握手和通道操作的超时时间，默认：5 秒

--
SSR 方案：
  ssr://method:pass@host:port?protocol=xxx&protocol_param=yyy&obfs=zzz&obfs_param=xyz

--
TLS 客户端方案：
  tls://host:port[?serverName=SERVERNAME][&skipVerify=true][&cert=PATH][&alpn=proto1][&alpn=proto2]

TLS 客户端代理链：
  tls://host:port[?skipVerify=true][&serverName=SERVERNAME],scheme://
  tls://host:port[?skipVerify=true],http://[user:pass@]
  tls://host:port[?skipVerify=true],socks5://[user:pass@]
  tls://host:port[?skipVerify=true],vmess://[security:]uuid@?alterID=num

TLS 服务端方案：
  tls://host:port?cert=PATH&key=PATH[&alpn=proto1][&alpn=proto2]

TLS 服务端代理链：
  tls://host:port?cert=PATH&key=PATH,scheme://
  tls://host:port?cert=PATH&key=PATH,http://
  tls://host:port?cert=PATH&key=PATH,socks5://
  tls://host:port?cert=PATH&key=PATH,ss://method:pass@

--
Trojan 客户端方案：
  trojan://pass@host:port[?serverName=SERVERNAME][&skipVerify=true][&cert=PATH]
  trojanc://pass@host:port     （明文，不含 TLS）

Trojan 服务端方案：
  trojan://pass@host:port?cert=PATH&key=PATH[&fallback=127.0.0.1]
  trojanc://pass@host:port[?fallback=127.0.0.1]     （明文，不含 TLS）

--
Unix 域套接字方案：
  unix://path

--
VLESS 方案：
  vless://uuid@host:port[?fallback=127.0.0.1:80]

--
VMess 方案：
  vmess://[security:]uuid@host:port[?alterID=num]
    若 alterID=0 或未设置，将启用 VMessAEAD

  VMess 可用加密类型：
    zero, none, aes-128-gcm, chacha20-poly1305

--
Websocket 客户端方案：
  ws://host:port[/path][?host=HOST][&origin=ORIGIN]
  wss://host:port[/path][?serverName=SERVERNAME][&skipVerify=true][&cert=PATH][&host=HOST][&origin=ORIGIN]

Websocket 服务端方案：
  ws://:port[/path][?host=HOST]
  wss://:port[/path]?cert=PATH&key=PATH[?host=HOST]

Websocket 指定代理协议：
  ws://host:port[/path][?host=HOST],scheme://
  ws://host:port[/path][?host=HOST],http://[user:pass@]
  ws://host:port[/path][?host=HOST],socks5://[user:pass@]

TLS + Websocket 指定代理协议：
  tls://host:port[?skipVerify=true][&serverName=SERVERNAME],ws://[@/path[?host=HOST]],scheme://
  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],http://[user:pass@]
  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],socks5://[user:pass@]
  tls://host:port[?skipVerify=true],ws://[@/path[?host=HOST]],vmess://[security:]uuid@?alterID=num

--
VM Socket 方案（仅 Linux）：
  vsock://[CID]:port

  若要监听所有地址，将 CID 设为 4294967295。
```

</details>

#### 使用示例

<details>
<summary><code>glider -example</code></summary>

```bash
使用示例：
  glider -config glider.conf
    -使用指定的配置文件启动 glider。

  glider -listen :8443 -verbose
    -监听 :8443，同端口同时支持 http/socks5 代理，开启详细日志模式。

  glider -listen socks5://:1080 -listen http://:8080 -verbose
    -多监听器：在 :1080 监听 socks5 代理，在 :8080 监听 http 代理。

  glider -listen :8443 -forward direct://#interface=eth0 -forward direct://#interface=eth1
    -多转发器：监听 :8443，通过 eth0 和 eth1 接口以轮询方式转发请求。

  glider -listen tls://:443?cert=crtFilePath&key=keyFilePath,http:// -verbose
    -协议链：在 :443 监听 HTTPS 代理服务器（http over tls）。

  glider -listen http://:8080 -forward socks5://serverA:1080,socks5://serverB:1080
    -代理链：在 :8080 监听 http 代理，所有请求经代理链转发。

  glider -listen :8443 -forward socks5://serverA:1080 -forward socks5://serverB:1080#priority=10 -forward socks5://serverC:1080#priority=10
    -转发器优先级：serverB 和 serverC 不可用时才使用 serverA。

  glider -listen tcp://:80 -forward tcp://serverA:80
    -TCP 隧道：监听 :80，将所有请求转发到 serverA:80。

  glider -listen udp://:53 -forward socks5://serverA:1080,udp://8.8.8.8:53
    -UDP 隧道：监听 :53，经 socks5 服务器将 UDP 请求转发到 8.8.8.8:53。

  glider -verbose -dns=:53 -dnsserver=8.8.8.8:53 -forward socks5://serverA:1080 -dnsrecord=abc.com/1.2.3.4
    -DNS 代理：在 :53 监听 DNS，经 socks5 服务器转发到 8.8.8.8:53。
```

</details>


## 配置

```bash
glider -config CONFIG_PATH
```

- [配置文件](config)
  - [glider.conf.example](config/glider.conf.example)
  - [office.rule.example](config/rules.d/office.rule.example)
- [配置示例](config/examples)
  - [透明代理（含 dnsmasq）](config/examples/8.transparent_proxy_with_dnsmasq)
  - [透明代理（不含 dnsmasq）](config/examples/9.transparent_proxy_without_dnsmasq)

## 服务

- dhcpd / dhcpd-failover（DHCP 服务 / 故障转移模式）：
  - service=dhcpd,网卡,起始IP,结束IP,租约分钟数[,MAC=IP,MAC=IP...]
    - service=dhcpd,eth1,192.168.1.100,192.168.1.199,720,fc:23:34:9e:25:01=192.168.1.101
    - service=dhcpd-failover,eth2,192.168.2.100,192.168.2.199,720
  - 注意：`dhcpd-failover` 仅在局域网内没有其他 DHCP 服务器时响应请求
    - 检测间隔：1 分钟

## Linux 守护进程

- systemd：[https://github.com/nadoo/glider/tree/main/systemd](https://github.com/nadoo/glider/tree/main/systemd)

- <details> <summary>docker：点击展开详情</summary>

  - 运行 glider（配置文件路径：/etc/glider/glider.conf）
    ```
    docker run -d --name glider --net host --restart=always \
      -v /etc/glider:/etc/glider \
      -v /etc/localtime:/etc/localtime:ro \
      nadoo/glider -config=/etc/glider/glider.conf
    ```
  - 若需自动更新，运行 watchtower
    ```
    docker run -d --name watchtower --restart=always \
      -v /var/run/docker.sock:/var/run/docker.sock \
      containrrr/watchtower --interval 21600 --cleanup \
      glider
    ```
  - 若需 UDP NAT 全锥形，开放 UDP 端口
    ```
    iptables -I INPUT -p udp -m udp --dport 1024:65535 -j ACCEPT
    ```

  </details>


## 自定义构建

<details><summary>若需要更小的二进制文件，可以自定义构建（点击展开详情）</summary>


1. 克隆源码：
  ```bash
  git clone https://github.com/nadoo/glider && cd glider
  ```
2. 自定义功能：

  ```bash
  打开 `feature.go` 和 `feature_linux.go`，注释掉不需要的包
  // _ "github.com/nadoo/glider/proxy/kcp"
  ```

3. 构建：
  ```bash
  go build -v -ldflags "-s -w"
  ```

</details>

## 代理链与协议链
<details><summary>在 glider 中，可以轻松地将多个代理服务器或协议串联（点击展开详情）</summary>

- 串联代理服务器：

  ```bash
  forward=http://1.1.1.1:80,socks5://2.2.2.2:1080,ss://method:pass@3.3.3.3:8443@
  ```

- 串联协议：HTTPS 代理（HTTP over TLS）

  ```bash
  forward=tls://server.com:443,http://
  ```

- 串联协议：VMess over WS over TLS

  ```bash
  forward=tls://server.com:443,ws://,vmess://5a146038-0b56-4e95-b1dc-5c6f5a32cd98@?alterID=2
  ```

- 串联协议和服务器：

  ``` bash
  forward=socks5://1.1.1.1:1080,tls://server.com:443,vmess://5a146038-0b56-4e95-b1dc-5c6f5a32cd98@?alterID=2
  ```

- 在监听器中串联协议：HTTPS 代理服务器

  ``` bash
  listen=tls://:443?cert=crtFilePath&key=keyFilePath,http://
  ```

- 在监听器中串联协议：HTTP over SMUX over WebSocket 代理服务器

  ``` bash
  listen=ws://:10000,smux://,http://
  ```

</details>

## 相关链接

- [ipset](https://github.com/nadoo/ipset)：Go 语言的 netlink ipset 包。
- [conflag](https://github.com/nadoo/conflag)：支持配置文件的 Go 标准 flag 包替代品。
- [ArchLinux](https://archlinux.org/packages/extra/x86_64/glider)：预置 glider 包的优秀 Linux 发行版。
- [urlencode](https://www.w3schools.com/tags/ref_urlencode.asp)：在 scheme URL 中应对特殊字符进行编码，例如 `@`→`%40`
