package core

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"playfast/internal/api"
	httpclient "playfast/internal/http-client"
	"playfast/internal/node"
	"playfast/internal/path"
	"playfast/utils"
	"sync"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/deprecated"
	"github.com/sagernet/sing-box/include"
	slog "github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	dns "github.com/sagernet/sing-dns"
	"github.com/sagernet/sing/common/json/badoption"
	"github.com/sagernet/sing/service"
)

//go:embed geoip-cn.srs
var geoip []byte

//go:embed geosite-cn.srs
var geosite []byte

//go:embed black-list.json
var black []byte

//go:embed direct-list.json
var direct []byte

//go:embed proxy-list.json
var proxyList []byte

type Box struct {
	box              *box.Box
	ctx              context.Context
	router           bool
	appends          []string
	defaultInterface int
	sync.Mutex
}

func (b *Box) Start(userUUID string, region string, router bool) error {
	b.Lock()
	defer b.Unlock()
	b.router = router
	if b.box == nil {
		err := b.newBox(userUUID, region)
		if err != nil {
			return err
		}
	}
	err := b.box.Start()
	if err != nil {
		return err
	}
	if router {
		var defaultInterface *net.Interface
		defaultInterface, err = utils.GetDefaultInterface()
		if err != nil {
			return err
		}
		b.defaultInterface = defaultInterface.Index
		err = utils.SetIPForwarding(b.defaultInterface, true)
	}
	return route(b.appends)
}
func (b *Box) Stop() error {
	b.Lock()
	defer b.Unlock()
	if b.box == nil {
		return nil
	}
	err := b.box.Close()
	b.box = nil
	if b.router {
		err = utils.SetIPForwarding(b.defaultInterface, false)
		if err != nil {
			return err
		}
	}
	deleteRoute(b.appends)
	return err
}
func New(ctx context.Context) *Box {
	ctx = service.ContextWith(ctx, deprecated.NewStderrManager(slog.StdLogger()))
	ctx = box.Context(ctx, include.InboundRegistry(), include.OutboundRegistry(), include.EndpointRegistry(), include.DNSTransportRegistry(), include.ServiceRegistry())
	_ = os.WriteFile(filepath.Join(path.Path(), "black-list.json"), black, 0644)
	_ = os.WriteFile(filepath.Join(path.Path(), "direct-list.json"), direct, 0644)
	_ = os.WriteFile(filepath.Join(path.Path(), "proxy-list.json"), proxyList, 0644)
	_ = os.WriteFile(filepath.Join(path.Path(), "geoip-cn.srs"), geoip, 0644)
	_ = os.WriteFile(filepath.Join(path.Path(), "geosite-cn.srs"), geosite, 0644)
	b := Box{
		ctx:     ctx,
		appends: []string{},
	}
	go b.update()
	return &b
}
func (b *Box) update() {
	var data []byte
	data, err := httpclient.GET(fmt.Sprintf("%s/black-list.json", api.GetApiDomain()))
	if err != nil {
		data = black
	}
	b.Lock()
	_ = os.WriteFile(filepath.Join(path.Path(), "black-list.json"), data, 0644)
	b.Unlock()
	data, err = httpclient.GET(fmt.Sprintf("%s/direct-list.json", api.GetApiDomain()))
	if err != nil {
		data = direct
	}
	b.Lock()
	_ = os.WriteFile(filepath.Join(path.Path(), "direct-list.json"), data, 0644)
	b.Unlock()
	data, err = httpclient.GET(fmt.Sprintf("%s/proxy-list.json", api.GetApiDomain()))
	if err != nil {
		data = proxyList
	}
	b.Lock()
	_ = os.WriteFile(filepath.Join(path.Path(), "proxy-list.json"), data, 0644)
	b.Unlock()
	data, err = httpclient.GET("https://raw.githubusercontent.com/lyc8503/sing-box-rules/refs/heads/rule-set-geoip/geoip-cn.srs")
	if err != nil {
		data = geoip
	}
	b.Lock()
	_ = os.WriteFile(filepath.Join(path.Path(), "geoip-cn.srs"), data, 0644)
	b.Unlock()
	data, err = httpclient.GET("https://raw.githubusercontent.com/lyc8503/sing-box-rules/refs/heads/rule-set-geosite/geosite-cn.srs")
	if err != nil {
		data = geosite
	}
	b.Lock()
	_ = os.WriteFile(filepath.Join(path.Path(), "geosite-cn.srs"), data, 0644)
	b.Unlock()
}

func (b *Box) newBox(userUUID string, proxy string) error {
	proxyOutbound, sshOutbound, proxyOutboundHost, err := node.GetOutbound(userUUID, proxy)
	if err != nil {
		return err
	}
	b.appends = make([]string, 0)
	proxyOutboundIp, err := utils.GetIPsFromString(proxyOutboundHost)
	if err != nil {
		return err
	}
	b.appends = append(b.appends, fmt.Sprintf("%s/32", proxyOutboundIp))
	outbounds := make([]option.Outbound, 0, 3)
	if sshOutbound != nil {
		outbounds = append(outbounds, *sshOutbound)
	}
	outbounds = append(outbounds, *proxyOutbound)
	outbounds = append(outbounds, option.Outbound{Type: constant.TypeDirect, Tag: "direct"})
	options := box.Options{
		Options: option.Options{
			Log: &option.LogOptions{
				Disabled: true,
			},
			DNS: &option.DNSOptions{
				RawDNSOptions: option.RawDNSOptions{
					Servers: []option.DNSServerOptions{
						{
							Type: "https",
							Tag:  "proxyDns",
							Options: &option.RemoteHTTPSDNSServerOptions{
								RemoteTLSDNSServerOptions: option.RemoteTLSDNSServerOptions{
									RemoteDNSServerOptions: option.RemoteDNSServerOptions{
										LocalDNSServerOptions: option.LocalDNSServerOptions{
											DialerOptions: option.DialerOptions{
												Detour: "proxy",
											},
										},
										DNSServerAddressOptions: option.DNSServerAddressOptions{
											Server:     "cloudflare-dns.com",
											ServerPort: 443,
										},
									},
								},
							},
						},
						{
							Type: "https",
							Tag:  "localDns",
							Options: &option.RemoteHTTPSDNSServerOptions{
								RemoteTLSDNSServerOptions: option.RemoteTLSDNSServerOptions{
									RemoteDNSServerOptions: option.RemoteDNSServerOptions{
										DNSServerAddressOptions: option.DNSServerAddressOptions{
											Server:     "223.5.5.5",
											ServerPort: 443,
										},
									},
								},
							},
						},
					},
					Rules: []option.DNSRule{
						{
							Type: constant.RuleTypeDefault,
							DefaultOptions: option.DefaultDNSRule{
								RawDefaultDNSRule: option.RawDefaultDNSRule{
									RuleSet: []string{
										"geosite-cn",
									},
								},
								DNSRuleAction: option.DNSRuleAction{
									Action: constant.RuleActionTypeRoute,
									RouteOptions: option.DNSRouteActionOptions{
										Server: "localDns",
									},
								},
							},
						},
						{
							Type: constant.RuleTypeDefault,
							DefaultOptions: option.DefaultDNSRule{
								RawDefaultDNSRule: option.RawDefaultDNSRule{
									Domain: []string{proxyOutboundHost},
								},
								DNSRuleAction: option.DNSRuleAction{
									Action: constant.RuleActionTypeRoute,
									RouteOptions: option.DNSRouteActionOptions{
										Server: "localDns",
									},
								},
							},
						},
					},
					Final: "proxyDns",
					DNSClientOptions: option.DNSClientOptions{
						Strategy:      option.DomainStrategy(dns.DomainStrategyUseIPv4),
						CacheCapacity: 2048,
					},
				},
			},
			Inbounds: []option.Inbound{
				{
					Type: constant.TypeTun,
					Tag:  "tun-in",
					Options: &option.TunInboundOptions{
						InterfaceName: "utun25",
						MTU:           1500,
						Address: badoption.Listable[netip.Prefix]{
							netip.MustParsePrefix("172.25.0.0/30"),
						},
						//RouteAddress: in(),
						//AutoRoute:    true,
						//StrictRoute:  true,
						UDPTimeout: option.UDPTimeoutCompat(time.Second * 300),
						Stack:      "gvisor",
					},
				},
			},
			Route: &option.RouteOptions{
				RuleSet: []option.RuleSet{
					{
						Type:         constant.RuleSetTypeLocal,
						Tag:          "geosite-cn",
						Format:       constant.RuleSetFormatBinary,
						LocalOptions: option.LocalRuleSet{Path: filepath.Join(path.Path(), "geosite-cn.srs")},
					},
					{
						Type:         constant.RuleSetTypeLocal,
						Tag:          "geoip-cn",
						Format:       constant.RuleSetFormatBinary,
						LocalOptions: option.LocalRuleSet{Path: filepath.Join(path.Path(), "geoip-cn.srs")},
					},
					{
						Type:         constant.RuleSetTypeLocal,
						Tag:          "black-list",
						Format:       constant.RuleSetFormatSource,
						LocalOptions: option.LocalRuleSet{Path: filepath.Join(path.Path(), "black-list.json")},
					},
					{
						Type:         constant.RuleSetTypeLocal,
						Tag:          "direct-list",
						Format:       constant.RuleSetFormatSource,
						LocalOptions: option.LocalRuleSet{Path: filepath.Join(path.Path(), "direct-list.json")},
					},
					{
						Type:         constant.RuleSetTypeLocal,
						Tag:          "proxy-list",
						Format:       constant.RuleSetFormatSource,
						LocalOptions: option.LocalRuleSet{Path: filepath.Join(path.Path(), "proxy-list.json")},
					},
				},
				AutoDetectInterface: true,
				Rules:               []option.Rule{},
			},
			Outbounds: outbounds,
			Experimental: &option.ExperimentalOptions{
				ClashAPI: &option.ClashAPIOptions{
					ExternalController: "127.0.0.1:54713",
				},
			},
		},
		Context: b.ctx,
	}
	options.Options.Route.Rules = append(options.Options.Route.Rules, []option.Rule{
		{
			Type: constant.RuleTypeDefault,
			DefaultOptions: option.DefaultRule{
				RawDefaultRule: option.RawDefaultRule{
					Network: []string{
						"udp",
					},
					Port: []uint16{
						443,
					},
				},
				RuleAction: option.RuleAction{
					Action: constant.RuleActionTypeReject,
					RejectOptions: option.RejectActionOptions{
						Method: "default",
						NoDrop: false,
					},
				},
			},
		}, //禁止http3
		{
			Type: constant.RuleTypeDefault,
			DefaultOptions: option.DefaultRule{
				RawDefaultRule: option.RawDefaultRule{
					Invert: true,
				},
				RuleAction: option.RuleAction{
					Action: constant.RuleActionTypeSniff,
					SniffOptions: option.RouteActionSniff{
						Sniffer: []string{
							"dns", "http", "tls", "quic",
						},
					},
				},
			},
		}, //解析协议域名
		{
			Type: constant.RuleTypeDefault,
			DefaultOptions: option.DefaultRule{
				RawDefaultRule: option.RawDefaultRule{
					RuleSet: []string{"black-list"},
				},
				RuleAction: option.RuleAction{
					Action: constant.RuleActionTypeReject,
					RejectOptions: option.RejectActionOptions{
						Method: constant.RuleActionRejectMethodDefault,
						NoDrop: false,
					},
				},
			},
		}, //过滤黑名单
		{
			Type: constant.RuleTypeDefault,
			DefaultOptions: option.DefaultRule{
				RawDefaultRule: option.RawDefaultRule{
					RuleSet: []string{"direct-list"},
				},
				RuleAction: option.RuleAction{
					Action: constant.RuleActionTypeRoute,
					RouteOptions: option.RouteActionOptions{
						Outbound: "direct",
					},
				},
			},
		}, //直连白名单
		{
			Type: constant.RuleTypeDefault,
			DefaultOptions: option.DefaultRule{
				RawDefaultRule: option.RawDefaultRule{
					Protocol: []string{"dns"},
				},
				RuleAction: option.RuleAction{
					Action: constant.RuleActionTypeHijackDNS,
				},
			},
		}, //dns劫持
		{
			Type: constant.RuleTypeDefault,
			DefaultOptions: option.DefaultRule{
				RawDefaultRule: option.RawDefaultRule{
					RuleSet: []string{"proxy-list"},
				},
				RuleAction: option.RuleAction{
					Action: constant.RuleActionTypeRoute,
					RouteOptions: option.RouteActionOptions{
						Outbound: "proxy",
					},
				},
			},
		}, //强制代理白名单，避免被 geoip-cn 误判直连
		{
			Type: constant.RuleTypeDefault,
			DefaultOptions: option.DefaultRule{
				RawDefaultRule: option.RawDefaultRule{
					RuleSet: []string{
						"geosite-cn", "geoip-cn",
					},
				},
				RuleAction: option.RuleAction{
					Action: constant.RuleActionTypeRoute,
					RouteOptions: option.RouteActionOptions{
						Outbound: "direct",
					},
				},
			},
		}, //中国地区直连
		{
			Type: constant.RuleTypeDefault,
			DefaultOptions: option.DefaultRule{
				RawDefaultRule: option.RawDefaultRule{
					Invert: true,
				},
				RuleAction: option.RuleAction{
					Action: constant.RuleActionTypeRoute,
					RouteOptions: option.RouteActionOptions{
						Outbound: "proxy",
					},
				},
			},
		}, //最终代理
	}...)
	_ = os.Remove(path.Path() + "/run.log")
	options.Log = &option.LogOptions{
		Disabled:     false,
		Level:        slog.FormatLevel(slog.LevelInfo),
		Output:       path.Path() + "/run.log",
		Timestamp:    true,
		DisableColor: true,
	}
	b.box, err = box.New(options)
	return err
}
