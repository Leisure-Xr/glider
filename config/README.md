
## 配置文件
命令：
```bash
glider -config glider.conf
```
配置文件，**直接使用命令行参数名作为键名**：
```bash
  # 注释行
  KEY=VALUE
  KEY=VALUE
  # KEY 与命令行参数名相同：listen forward strategy...
```

示例：
```bash
### glider 配置文件

# 详细模式，打印日志
verbose

# 监听 8443，同端口同时支持 http/socks5 代理
listen=:8443

# 上游转发代理
forward=socks5://192.168.1.10:1080

# 上游转发代理
forward=ss://method:pass@1.1.1.1:8443

# 上游转发代理（代理链）
forward=http://1.1.1.1:8080,socks5://2.2.2.2:1080

# 多上游代理转发策略
strategy=rr

# 转发器健康检查
check=http://www.msftconnecttest.com/connecttest.txt#expect=200

# 检查间隔
checkinterval=30


# 设置 DNS 转发服务器
dns=:53
# 全局上游 DNS 服务器（可在规则文件中指定不同的 DNS 服务器）
dnsserver=8.8.8.8:53

# 规则文件
rules-dir=rules.d
#rulefile=office.rule
#rulefile=home.rule

# 包含更多配置文件
#include=dnsrecord.inc.conf
#include=more.inc.conf
```
参考：
- [glider.conf.example](glider.conf.example)
- [配置示例](examples)

## 规则文件
规则文件，**格式与配置文件相同，但基于目标地址指定转发器**：
```bash
# 可以使用全局配置文件中的所有键，除了 "listen" 和 "rulefile"
forward=socks5://192.168.1.10:1080
forward=ss://method:pass@1.1.1.1:8443
forward=http://192.168.2.1:8080,socks5://192.168.2.2:1080
strategy=rr
check=http://www.msftconnecttest.com/connecttest.txt#expect=200
checkinterval=30

# 本规则文件中域名使用的 DNS 服务器
dnsserver=208.67.222.222:53

# IPSET 管理
# ----------------
# 在 Linux 上基于规则文件中的目标地址创建和管理 ipset
#   - 启动时添加规则文件中的 IP/CIDR
#   - 通过 DNS 转发服务器为规则文件中的域名添加解析 IP
# 通常用于 Linux 上的透明代理模式
ipset=glider

# 可以指定目标地址以使用上面的转发器
# 匹配 abc.com 和 *.abc.com
domain=abc.com

# 匹配 1.1.1.1
ip=1.1.1.1

# 匹配 192.168.100.0/24
cidr=192.168.100.0/24

# 可以包含一个只含目标地址设置的列表文件
include=office.list.example

```
参考：
- [office.rule.example](rules.d/office.rule.example)
- [配置示例](examples)
