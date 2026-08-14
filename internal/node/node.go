package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"playfast/internal/api"
	"playfast/internal/echo"
	"playfast/internal/http-client"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
)

type SSHTunnel struct {
	Host     string `json:"host"`
	Port     uint16 `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type Proxy struct {
	Name      string     `json:"name"`
	Method    string     `json:"method"`
	Password  string     `json:"password"`
	Host      string     `json:"host"`
	Port      uint16     `json:"port"`
	Protocol  string     `json:"protocol"`
	SSHTunnel *SSHTunnel `json:"ssh_tunnel,omitempty"`
}

func Get(userUUID string) []Proxy {
	data := make([]Proxy, 0)
	var url string
	if userUUID == "" {
		url = fmt.Sprintf("%s/proxy.json", api.GetApiDomain())
	} else {
		url = fmt.Sprintf("%s/user/%s/proxy.json", api.GetApiDomain(), userUUID)
	}
	all, err := http_client.GET(url)
	if err != nil {
		return data
	}
	_ = json.Unmarshal(all, &data)
	return data
}
func GetOutbound(userUUID string, proxy string) (*option.Outbound, *option.Outbound, string, error) {
	data := Get(userUUID)
	var lastErr string
	for i, p := range data {
		if p.Name != proxy {
			continue
		}
		var out option.Outbound
		var sshOut *option.Outbound
		switch p.Protocol {
		case "shadowsocks":
			out = option.Outbound{
				Type: constant.TypeShadowsocks,
				Tag:  "proxy",
				Options: &option.ShadowsocksOutboundOptions{
					ServerOptions: option.ServerOptions{
						Server:     p.Host,
						ServerPort: p.Port,
					},
					Method:   p.Method,
					Password: p.Password,
					UDPOverTCP: &option.UDPOverTCPOptions{
						Enabled: true,
						Version: 2,
					},
				},
			}
		case "vless":
			out = option.Outbound{
				Type: constant.TypeVLESS,
				Tag:  "proxy",
				Options: &option.VLESSOutboundOptions{
					ServerOptions: option.ServerOptions{
						Server:     p.Host,
						ServerPort: p.Port,
					},
					UUID:                        p.Password,
					OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{},
					Multiplex: &option.OutboundMultiplexOptions{
						Enabled:        true,
						Protocol:       "h2mux",
						MaxConnections: 8,
						MinStreams:     16,
						Padding:        false,
					},
				},
			}
		case "socks":
			out = option.Outbound{
				Type: constant.TypeSOCKS,
				Tag:  "proxy",
				Options: &option.SOCKSOutboundOptions{
					ServerOptions: option.ServerOptions{
						Server:     p.Host,
						ServerPort: p.Port,
					},
					Version:  "5",
					Username: "playfast",
					Password: p.Password,
					UDPOverTCP: &option.UDPOverTCPOptions{
						Enabled: true,
						Version: 2,
					},
				},
			}
		default:
			continue
		}
		if p.SSHTunnel != nil {
			user := p.SSHTunnel.User
			if user == "" {
				user = "root"
			}
			sshOut = &option.Outbound{
				Type: constant.TypeSSH,
				Tag:  "ssh-detour",
				Options: &option.SSHOutboundOptions{
					ServerOptions: option.ServerOptions{
						Server:     p.SSHTunnel.Host,
						ServerPort: p.SSHTunnel.Port,
					},
					User:     user,
					Password: p.SSHTunnel.Password,
				},
			}
			switch opts := out.Options.(type) {
			case *option.ShadowsocksOutboundOptions:
				opts.Detour = "ssh-detour"
			case *option.VLESSOutboundOptions:
				opts.Detour = "ssh-detour"
			case *option.SOCKSOutboundOptions:
				opts.Detour = "ssh-detour"
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		ctx = box.Context(ctx, include.InboundRegistry(), include.OutboundRegistry(), include.EndpointRegistry(), include.DNSTransportRegistry(), include.ServiceRegistry())

		outbounds := make([]option.Outbound, 0, 2)
		if sshOut != nil {
			outbounds = append(outbounds, *sshOut)
		}
		outbounds = append(outbounds, out)
		testBoxOptions := option.Options{
			Outbounds: outbounds,
		}

		instance, err := box.New(box.Options{
			Options: testBoxOptions,
			Context: ctx,
		})
		if err != nil {
			lastErr = fmt.Sprintf("创建测试 Box 失败: %v", err)
			cancel()
			continue
		}
		err = instance.Start()
		if err != nil {
			lastErr = fmt.Sprintf("启动 SSH 隧道/代理失败: %v", err)
			cancel()
			continue
		}

		outboundService, ok := instance.Outbound().Outbound("proxy")
		if !ok {
			lastErr = "未找到 proxy Outbound"
			_ = instance.Close()
			cancel()
			continue
		}

		client := echo.NewClient("1.1.1.1:80", echo.WithTimeout(3*time.Second), echo.WithDialer(outboundService.DialContext))
		err = client.Connect(context.Background())
		if err != nil {
			lastErr = fmt.Sprintf("节点测试连接失败: %v", err)
			_ = instance.Close()
			cancel()
			continue
		}
		var ms int64
		result := client.Test(context.Background(), []byte("GET / HTTP/1.1\r\nHost: 1.1.1.1\r\nAccept: *\r\n\r\n\r\n"))
		ms = result.Latency.Milliseconds()
		log.Println(fmt.Sprintf("节点选择:ID:%d 节点:%s 延迟=%dms\n", i, p.Name, ms))

		_ = instance.Close()
		cancel()

		if ms <= 0 {
			return nil, nil, "", errors.New("节点超时")
		}
		realHost := p.Host
		if p.SSHTunnel != nil {
			realHost = p.SSHTunnel.Host
		}
		return &out, sshOut, realHost, nil
	}
	if len(lastErr) > 0 {
		return nil, nil, "", fmt.Errorf("%s", lastErr)
	}
	return nil, nil, "", errors.New("not fount Outbound")
}
