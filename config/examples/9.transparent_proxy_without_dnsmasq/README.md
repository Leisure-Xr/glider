
## 9. 透明代理（不含 dnsmasq）

PC 客户端 -> 运行 glider 的网关（Linux）-> 上游转发器 -> 互联网

#### 在此模式下，glider 扮演以下角色：
1. 透明代理服务器
2. DNS 转发服务器
3. ipset 管理器

因此，您的网络中不需要任何其他 DNS 服务器。

#### 手动创建 ipset
```bash
ipset create glider hash:net
```

#### Glider 配置
##### glider.conf
```bash
verbose=True

# 作为透明代理
listen=redir://:1081

# 作为 DNS 转发服务器
dns=:53
dnsserver=8.8.8.8:53
dnsserver=8.8.4.4:53

# 指定规则文件目录
rules-dir=rules.d
```

##### office.rule
```bash
# 添加转发器
forward=http://forwarder1:8080,socks5://forwarder2:1080
forward=http://1.1.1.1:8080
strategy=rr
check=http://www.msftconnecttest.com/connecttest.txt#expect=200

# 指定不同的 DNS 服务器（如需要）
dnsserver=208.67.222.222:53

# 作为 ipset 管理器
ipset=glider

# 指定目标地址
include=office.list

domain=example1.com
domain=example2.com
# 匹配 IP
ip=1.1.1.1
ip=2.2.2.2
# 匹配 IP 段
cidr=192.168.100.0/24
cidr=172.16.100.0/24
```

##### office.list
```bash
# 目标地址列表
domain=mycompany.com
domain=mycompany1.com
ip=4.4.4.4
ip=5.5.5.5
cidr=172.16.101.0/24
cidr=172.16.102.0/24
```

#### 在 Linux 网关上配置 iptables
```bash
iptables -t nat -I PREROUTING -p tcp -m set --match-set glider dst -j REDIRECT --to-ports 1081
iptables -t nat -I OUTPUT -p tcp -m set --match-set glider dst -j REDIRECT --to-ports 1081
```

#### 服务器 DNS 设置
将服务器的 nameserver 指向 glider：
```bash
echo nameserver 127.0.0.1 > /etc/resolv.conf
```

#### 客户端设置
使用 Linux 服务器的 IP 作为网关。
使用 Linux 服务器的 IP 作为 DNS 服务器。

#### 当客户端访问 http://example1.com（位于 office.rule 中）时的完整流程：
DNS 解析：
1. 客户端向 Linux 服务器发送 UDP DNS 请求，glider 接收请求（监听默认 DNS 端口 :53）
2. 上游 DNS 选择：glider 查找规则配置，确定该域名使用的 DNS 服务器（匹配 office.rule 中的 "example1.com"，选择 208.67.222.222:53）
3. glider 使用 office.rule 中的转发器向 208.67.222.222:53 发起 DNS 查询（通过代理的 DNS）
4. glider 更新 office 规则配置，将解析到的 IP 地址添加进去
5. glider 将解析到的 IP 添加到 ipset "glider"，并将 DNS 答复返回给客户端

目标访问：
1. 客户端向 example1.com 解析到的 IP 发起 HTTP 请求
2. Linux 网关服务器收到该请求
3. iptables 匹配 ipset "glider" 中的 IP，将请求重定向到 :1081（glider）
4. glider 在 office 规则中找到该 IP，选择 office.rule 中的转发器完成请求
