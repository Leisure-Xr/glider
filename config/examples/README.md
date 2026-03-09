
# Glider 配置示例

## 1. 简单代理服务
在 8443 端口同时监听 HTTP/SOCKS5 代理，所有请求直接转发。

```
   客户端 --> 监听器 --> 互联网
```

- [simple_proxy_service](1.simple_proxy_service)

## 2. 单个上游代理

```
   客户端 --> 监听器 --> 转发器 -->  互联网
```

- [one_forwarder](2.one_forwarder)

## 3. 单个上游代理链

```
   客户端 -->  监听器 --> 转发器1 --> 转发器2 -->  互联网
```

- [forward_chain](3.forward_chain)

## 4. 多个上游代理

```
                            |转发器 ----------------->|
   客户端 --> 监听器 --> |                            | 互联网
                            |转发器 --> 转发器->...|
```

- [multiple_forwarders](4.multiple_forwarders)


## 5. 使用规则文件：默认直连，规则文件使用转发器

默认：
```
   客户端 --> 监听器 --> 互联网
```
规则文件中指定的目标：
```
                             |转发器 ----------------->|
   客户端 -->  监听器 --> |                            | 互联网
                             |转发器 --> 转发器->...|
```

- [rule_default_direct](5.rule_default_direct)


## 6. 使用规则文件：默认使用转发器，规则文件直连

默认：
```
                             |转发器 ----------------->|
   客户端 -->  监听器 --> |                            | 互联网
                             |转发器 --> 转发器->...|
```

规则文件中指定的目标：
```
   客户端 --> 监听器 --> 互联网
```

- [rule_default_forwarder](6.rule_default_forwarder)


## 7. 使用规则文件：多规则文件

默认：
```
   客户端 --> 监听器 --> 互联网
```
规则文件1指定的目标：
```
                             |转发器1 ----------------->|
   客户端 -->  监听器 --> |                            | 互联网
                             |转发器2 --> 转发器3->...|
```
规则文件2指定的目标：
```
                             |转发器4 ----------------->|
   客户端 -->  监听器 --> |                            | 互联网
                             |转发器5 --> 转发器6->...|
```

- [rule_multiple_rule_files](7.rule_multiple_rule_files)

## 8. 透明代理（含 Dnsmasq）
- [transparent_proxy_with_dnsmasq](8.transparent_proxy_with_dnsmasq)

## 9. 透明代理（不含 Dnsmasq）
- [transparent_proxy_without_dnsmasq](9.transparent_proxy_without_dnsmasq)
