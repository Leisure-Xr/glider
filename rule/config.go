package rule

import (
	"os"
	"strings"

	"github.com/nadoo/conflag"
)

// Config 是规则配置结构体。
type Config struct {
	RulePath string

	Forward  []string
	Strategy Strategy

	DNSServers []string
	IPSet      string

	Domain []string
	IP     []string
	CIDR   []string
}

// Strategy 是策略配置结构体。
type Strategy struct {
	Strategy            string
	Check               string
	CheckInterval       int
	CheckTimeout        int
	CheckTolerance      int
	CheckLatencySamples int
	CheckDisabledOnly   bool
	MaxFailures         int
	DialTimeout         int
	RelayTimeout        int
	IntFace             string
}

// NewConfFromFile 从规则文件返回新的配置。
func NewConfFromFile(ruleFile string) (*Config, error) {
	p := &Config{RulePath: ruleFile}

	f := conflag.NewFromFile("rule", ruleFile)
	f.StringSliceUniqVar(&p.Forward, "forward", nil, "转发地址，格式：SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS[,SCHEME://[USER|METHOD:PASSWORD@][HOST]:PORT?PARAMS]")
	f.StringVar(&p.Strategy.Strategy, "strategy", "rr", "转发策略，默认：rr")
	f.StringVar(&p.Strategy.Check, "check", "http://www.msftconnecttest.com/connecttest.txt#expect=200", "check=tcp[://HOST:PORT]: TCP 端口连接检测\ncheck=http://HOST[:PORT][/URI][#expect=STRING_IN_RESP_LINE]\ncheck=file://SCRIPT_PATH: 运行检测脚本，退出码为 0 表示健康，环境变量：FORWARDER_ADDR\ncheck=disable: 禁用健康检查")
	f.IntVar(&p.Strategy.CheckInterval, "checkinterval", 30, "转发器健康检查间隔（秒）")
	f.IntVar(&p.Strategy.CheckTimeout, "checktimeout", 10, "转发器健康检查超时（秒）")
	f.IntVar(&p.Strategy.CheckLatencySamples, "checklatencysamples", 10, "使用最近 N 次检查的平均延迟")
	f.IntVar(&p.Strategy.CheckTolerance, "checktolerance", 0, "转发器检查容忍值（毫秒），仅在 lha 模式下有效，当 新延迟 < 旧延迟 - 容忍值 时才切换")
	f.BoolVar(&p.Strategy.CheckDisabledOnly, "checkdisabledonly", false, "仅检查已禁用的转发器")
	f.IntVar(&p.Strategy.MaxFailures, "maxfailures", 3, "触发禁用转发器的最大失败次数")
	f.IntVar(&p.Strategy.DialTimeout, "dialtimeout", 3, "连接超时（秒）")
	f.IntVar(&p.Strategy.RelayTimeout, "relaytimeout", 0, "中继超时（秒）")
	f.StringVar(&p.Strategy.IntFace, "interface", "", "来源 IP 或网络接口")

	f.StringSliceUniqVar(&p.DNSServers, "dnsserver", nil, "上游 DNS 服务器")
	f.StringVar(&p.IPSet, "ipset", "", "ipset 名称，将创建两个集合：NAME（IPv4）和 NAME6（IPv6）")

	f.StringSliceVar(&p.Domain, "domain", nil, "域名")
	f.StringSliceVar(&p.IP, "ip", nil, "IP 地址")
	f.StringSliceVar(&p.CIDR, "cidr", nil, "CIDR 地址段")

	err := f.Parse()
	if err != nil {
		return nil, err
	}

	return p, err
}

// ListDir 返回 dirPth 目录中以 suffix 为后缀的文件列表。
func ListDir(dirPth string, suffix string) (files []string, err error) {
	files = make([]string, 0, 10)
	dir, err := os.ReadDir(dirPth)
	if err != nil {
		return nil, err
	}
	PthSep := string(os.PathSeparator)
	suffix = strings.ToLower(suffix)
	for _, fi := range dir {
		if fi.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(fi.Name()), suffix) {
			files = append(files, dirPth+PthSep+fi.Name())
		}
	}
	return files, nil
}
