package main

import (
	"fmt"
	"os"
	"path"

	"github.com/nadoo/conflag"

	"github.com/nadoo/glider/dns"
	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/proxy"
	"github.com/nadoo/glider/rule"
)

var flag = conflag.New()

// Config 是全局配置结构体。
type Config struct {
	Verbose    bool
	LogFlags   int
	TCPBufSize int
	UDPBufSize int

	Listens []string

	Forwards []string
	Strategy rule.Strategy

	RuleFiles []string
	RulesDir  string

	DNS       string
	DNSConfig dns.Config

	rules []*rule.Config

	Services []string
}

func parseConfig() *Config {
	conf := &Config{}

	flag.SetOutput(os.Stdout)

	scheme := flag.String("scheme", "", "显示指定代理方案的帮助信息，使用 'all' 查看所有方案")
	example := flag.Bool("example", false, "显示使用示例")

	flag.BoolVar(&conf.Verbose, "verbose", false, "详细模式，打印调试日志")
	flag.IntVar(&conf.LogFlags, "logflags", 19, "日志标志位，不了解请勿修改，参考：https://pkg.go.dev/log#pkg-constants")
	flag.IntVar(&conf.TCPBufSize, "tcpbufsize", 32768, "TCP 缓冲区大小（字节）")
	flag.IntVar(&conf.UDPBufSize, "udpbufsize", 2048, "UDP 缓冲区大小（字节）")
	flag.StringSliceUniqVar(&conf.Listens, "listen", nil, "监听地址，详见下方 URL 说明")

	flag.StringSliceVar(&conf.Forwards, "forward", nil, "转发地址，详见下方 URL 说明")
	flag.StringVar(&conf.Strategy.Strategy, "strategy", "rr", `rr: 轮询模式
ha: 高可用模式
lha: 基于延迟的高可用模式
dh: 目标哈希模式`)
	flag.StringVar(&conf.Strategy.Check, "check", "http://www.msftconnecttest.com/connecttest.txt#expect=200",
		`check=tcp[://HOST:PORT]: TCP 端口连接检测
check=http://HOST[:PORT][/URI][#expect=REGEX_MATCH_IN_RESP_LINE]
check=https://HOST[:PORT][/URI][#expect=REGEX_MATCH_IN_RESP_LINE]
check=file://SCRIPT_PATH: 运行检测脚本，退出码为 0 表示健康，环境变量：FORWARDER_ADDR,FORWARDER_URL
check=disable: 禁用健康检查`)
	flag.IntVar(&conf.Strategy.CheckInterval, "checkinterval", 30, "转发器健康检查间隔（秒）")
	flag.IntVar(&conf.Strategy.CheckTimeout, "checktimeout", 10, "转发器健康检查超时（秒）")
	flag.IntVar(&conf.Strategy.CheckTolerance, "checktolerance", 0, "转发器检查容忍值（毫秒），仅在 lha 模式下有效，当 新延迟 < 旧延迟 - 容忍值 时才切换")
	flag.IntVar(&conf.Strategy.CheckLatencySamples, "checklatencysamples", 10, "使用最近 N 次检查的平均延迟")
	flag.BoolVar(&conf.Strategy.CheckDisabledOnly, "checkdisabledonly", false, "仅检查已禁用的转发器")
	flag.IntVar(&conf.Strategy.MaxFailures, "maxfailures", 3, "触发禁用转发器的最大失败次数")
	flag.IntVar(&conf.Strategy.DialTimeout, "dialtimeout", 3, "连接超时（秒）")
	flag.IntVar(&conf.Strategy.RelayTimeout, "relaytimeout", 0, "中继超时（秒）")
	flag.StringVar(&conf.Strategy.IntFace, "interface", "", "来源 IP 或网络接口")

	flag.StringSliceUniqVar(&conf.RuleFiles, "rulefile", nil, "规则文件路径")
	flag.StringVar(&conf.RulesDir, "rules-dir", "", "规则文件目录")

	// dns 配置
	flag.StringVar(&conf.DNS, "dns", "", "本地 DNS 服务器监听地址")
	flag.StringSliceUniqVar(&conf.DNSConfig.Servers, "dnsserver", []string{"8.8.8.8:53"}, "上游 DNS 服务器地址")
	flag.BoolVar(&conf.DNSConfig.AlwaysTCP, "dnsalwaystcp", false, "始终使用 TCP 查询上游 DNS 服务器（无论是否有转发器）")
	flag.IntVar(&conf.DNSConfig.Timeout, "dnstimeout", 3, "多 DNS 服务器切换超时（秒）")
	flag.IntVar(&conf.DNSConfig.MaxTTL, "dnsmaxttl", 1800, "DNS 缓存最大 TTL（秒）")
	flag.IntVar(&conf.DNSConfig.MinTTL, "dnsminttl", 0, "DNS 缓存最小 TTL（秒）")
	flag.IntVar(&conf.DNSConfig.CacheSize, "dnscachesize", 4096, "DNS 缓存最大条目数")
	flag.BoolVar(&conf.DNSConfig.CacheLog, "dnscachelog", false, "显示 DNS 缓存查询日志")
	flag.BoolVar(&conf.DNSConfig.NoAAAA, "dnsnoaaaa", false, "禁用 AAAA 查询（IPv6 DNS）")
	flag.StringSliceUniqVar(&conf.DNSConfig.Records, "dnsrecord", nil, "自定义 DNS 记录，格式：域名/IP")

	// 服务配置
	flag.StringSliceUniqVar(&conf.Services, "service", nil, "运行指定服务，格式：SERVICE_NAME[,SERVICE_CONFIG]")

	flag.Usage = usage
	if err := flag.Parse(); err != nil {
		// flag.Usage()
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(-1)
	}

	if *scheme != "" {
		fmt.Fprint(flag.Output(), proxy.Usage(*scheme))
		os.Exit(0)
	}

	if *example {
		fmt.Fprint(flag.Output(), examples)
		os.Exit(0)
	}

	// setup logger
	log.Set(conf.Verbose, conf.LogFlags)

	if len(conf.Listens) == 0 && conf.DNS == "" && len(conf.Services) == 0 {
		// flag.Usage()
		fmt.Fprintf(os.Stderr, "ERROR: 必须指定监听地址\n")
		os.Exit(-1)
	}

	// tcpbufsize
	if conf.TCPBufSize > 0 {
		proxy.TCPBufSize = conf.TCPBufSize
	}

	// udpbufsize
	if conf.UDPBufSize > 0 {
		proxy.UDPBufSize = conf.UDPBufSize
	}

	loadRules(conf)
	return conf
}

func loadRules(conf *Config) {
	// rulefiles
	for _, ruleFile := range conf.RuleFiles {
		if !path.IsAbs(ruleFile) {
			ruleFile = path.Join(flag.ConfDir(), ruleFile)
		}

		rule, err := rule.NewConfFromFile(ruleFile)
		if err != nil {
			log.Fatal(err)
		}

		conf.rules = append(conf.rules, rule)
	}

	if conf.RulesDir != "" {
		if !path.IsAbs(conf.RulesDir) {
			conf.RulesDir = path.Join(flag.ConfDir(), conf.RulesDir)
		}

		ruleFolderFiles, _ := rule.ListDir(conf.RulesDir, ".rule")
		for _, ruleFile := range ruleFolderFiles {
			rule, err := rule.NewConfFromFile(ruleFile)
			if err != nil {
				log.Fatal(err)
			}
			conf.rules = append(conf.rules, rule)
		}
	}
}

func usage() {
	fmt.Fprint(flag.Output(), usage1)
	flag.PrintDefaults()
	fmt.Fprintf(flag.Output(), usage2, proxy.ServerSchemes(), proxy.DialerSchemes(), version)
}

var usage1 = `
用法：glider [-listen URL]... [-forward URL]... [选项]...

  示例：glider -config /etc/glider/glider.conf
        glider -listen :8443 -forward socks5://serverA:1080 -forward socks5://serverB:1080 -verbose

选项：
`

var usage2 = `
URL 格式：
   代理：SCHEME://[USER:PASS@][HOST]:PORT
   链式：proxy,proxy[,proxy]...

    示例：-listen socks5://:1080
          -listen tls://:443?cert=crtFilePath&key=keyFilePath,http://    （协议链）

    示例：-forward socks5://server:1080
          -forward tls://server.com:443,http://                          （协议链）
          -forward socks5://serverA:1080,socks5://serverB:1080           （代理链）

协议方案（SCHEME）：
   监听：%s
   转发：%s

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
glider %s, https://github.com/nadoo/glider (glider.proxy@gmail.com)
`

var examples = `
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
`
