
## 8. 透明代理（含 dnsmasq）

#### 用 glider 设置透明代理和 DNS 服务器
glider.conf
```bash
verbose=True
listen=redir://:1081
forward=http://forwarder1:8080,socks5://forwarder2:1080
forward=http://1.1.1.1:8080
dns=:5353
dnsserver=8.8.8.8:53
strategy=rr
checkinterval=30
```

#### 手动创建 ipset
```bash
ipset create myset hash:net
```

#### 配置 dnsmasq
```bash
server=/example1.com/127.0.0.1#5353
ipset=/example1.com/myset
server=/example2.com/127.0.0.1#5353
ipset=/example2.com/myset
server=/example3.com/127.0.0.1#5353
ipset=/example4.com/myset
```

#### 在 Linux 网关上配置 iptables
```bash
iptables -t nat -I PREROUTING -p tcp -m set --match-set myset dst -j REDIRECT --to-ports 1081
#iptables -t nat -I OUTPUT -p tcp -m set --match-set myset dst -j REDIRECT --to-ports 1081
```

#### 客户端请求网络时的完整流程：
1. dnsmasq 将 example1.com 的所有 DNS 请求转发到 glider(:5353)
2. glider 通过转发器以 TCP 方式将 DNS 请求转发到 8.8.8.8:53
3. dnsmasq 将解析到的 IP 地址添加到 ipset "myset"
4. iptables 将所有到 example1.com 的 TCP 请求重定向到 glider(:1081)
5. glider 通过转发器将请求转发到 example1.com
