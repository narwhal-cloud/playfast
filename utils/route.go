package utils

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"os/exec"
	"strings"
	"syscall"

	"github.com/r10v/gowindows"
)

func SetIPForwarding(index int, enable bool) error {
	state := "Disabled"
	if enable {
		state = "Enabled"
	}

	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("Set-NetIPInterface -InterfaceIndex  %d -Forwarding %s", index, state))
	// 隐藏窗口（仅适用于 Windows）
	log.Println(cmd.String())
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true, // 关键设置，阻止窗口闪现
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set IP forwarding: %v, output: %s", err, string(output))
	}

	return nil
}
func SetInterfaceMetric(ifIndex int, metric int) error {
	metricStr := fmt.Sprintf("%d", metric)
	if metric == 0 {
		metricStr = "auto"
	}
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("netsh interface ipv4 set interface %d metric=%s", ifIndex, metricStr))
	// 隐藏窗口（仅适用于 Windows）
	log.Println(cmd.String())
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true, // 关键设置，阻止窗口闪现
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to SetInterfaceMetric: %v, output: %s", err, string(output))
	}
	return nil
}
func UpdateDefaultMetric(gateway string, index, metric int) error {
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`Set-NetRoute -DestinationPrefix "0.0.0.0/0" -NextHop "%s" -InterfaceIndex %d -RouteMetric %d`, gateway, index, metric))
	// 隐藏窗口（仅适用于 Windows）
	log.Println(cmd.String())
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true, // 关键设置，阻止窗口闪现
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to UpdateDefaultMetric: %v, output: %s", err, string(output))
	}

	return nil
}
func SetInterfaceDNS(ifIndex int, servers []string) error {
	quoted := make([]string, len(servers))
	for i, s := range servers {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses (%s)`, ifIndex, strings.Join(quoted, ",")))
	log.Println(cmd.String())
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true, // 关键设置，阻止窗口闪现
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to SetInterfaceDNS: %v, output: %s", err, string(output))
	}
	return nil
}
func ResetInterfaceDNS(ifIndex int) error {
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`Set-DnsClientServerAddress -InterfaceIndex %d -ResetServerAddresses`, ifIndex))
	log.Println(cmd.String())
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true, // 关键设置，阻止窗口闪现
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to ResetInterfaceDNS: %v, output: %s", err, string(output))
	}
	return nil
}
func AddRoute(destination, mask, gateway netip.Addr, metric, ifIndex int) error {
	row := gowindows.MibIpForwardRow{
		ForwardDest:      destination.As4(),
		ForwardMask:      mask.As4(),
		ForwardPolicy:    0,
		ForwardNextHop:   gateway.As4(),
		ForwardIfIndex:   gowindows.DWord(ifIndex),
		ForwardType:      3,
		ForwardProto:     3,
		ForwardAge:       0xFFFFFFFF,
		ForwardNextHopAS: 0,
		ForwardMetric1:   gowindows.DWord(metric),
		ForwardMetric2:   0,
		ForwardMetric3:   0,
		ForwardMetric4:   0,
		ForwardMetric5:   0,
	}
	err := gowindows.CreateIpForwardEntry(&row)
	return err
}
func DeleteRoute(destination, mask, gateway netip.Addr, metric, ifIndex int) error {
	err := gowindows.DeleteIpForwardEntry(&gowindows.MibIpForwardRow{
		ForwardDest:      destination.As4(),
		ForwardMask:      mask.As4(),
		ForwardPolicy:    0,
		ForwardNextHop:   gateway.As4(),
		ForwardIfIndex:   gowindows.DWord(ifIndex),
		ForwardType:      3,
		ForwardProto:     3,
		ForwardAge:       0xFFFFFFFF,
		ForwardNextHopAS: 0,
		ForwardMetric1:   gowindows.DWord(metric),
		ForwardMetric2:   0,
		ForwardMetric3:   0,
		ForwardMetric4:   0,
		ForwardMetric5:   0,
	})
	return err
}
func GetIPsFromString(input string) (string, error) {
	// 首先检查是否是有效的 IP 地址
	if ip := net.ParseIP(input); ip != nil {
		return ip.String(), nil
	}
	// 如果不是 IP，尝试作为域名解析
	addr, err := net.LookupHost(input)
	if err != nil {
		return "", fmt.Errorf("无法解析 %s: %v", input, err)
	}
	return addr[0], nil
}
