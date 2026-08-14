package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"playfast/internal/api"
	"playfast/internal/core"
	"playfast/internal/dialog"
	"playfast/internal/http-client"
	"playfast/internal/node"
	"playfast/internal/systray"
	"playfast/utils"
	"sync/atomic"
	"time"

	goRuntime "runtime"

	"github.com/hashicorp/go-version"
	"github.com/minio/selfupdate"
	"github.com/sagernet/sing/common/json"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx  context.Context
	box  *core.Box
	init atomic.Bool
}

func NewApp() *App {
	return &App{}
}
func (a *App) startup(ctx context.Context) {
	defer a.init.Store(true)
	a.ctx = ctx
	go systray.Run(a.systemTray, func() {})
	a.checkUpdate(false)
	a.box = core.New(a.ctx)
}
func (a *App) checkUpdate(tip bool) {
	data := make(map[string]string)
	all, err := http_client.GET(fmt.Sprintf("%s/version.json", api.GetApiDomain()))
	if err != nil {
		dialog.Error(a.ctx, "获取最新版本失败", fmt.Sprintln("Error:", err))
		return
	}
	err = json.Unmarshal(all, &data)
	if err != nil {
		dialog.Error(a.ctx, "获取最新版本失败", fmt.Sprintln("Error:", err))
		return
	}
	v, err := version.NewVersion(data["version"])
	if err != nil {
		dialog.Error(a.ctx, "获取最新版本失败", fmt.Sprintln("Error:", err))
		return
	}
	current, _ := version.NewVersion(Version)
	if v.LessThanOrEqual(current) {
		if tip {
			_, _ = runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
				Type:    runtime.InfoDialog,
				Title:   "更新提示",
				Message: "已经是最新版本了",
			})
		}
		return
	}
	res, _ := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "更新提示",
		Message:       "发现新版本是否需要更新?",
		Buttons:       []string{"是", "否"},
		DefaultButton: "是",
		CancelButton:  "否",
	})
	if res == "No" {
		return
	}
	all, err = http_client.GET(data[fmt.Sprintf("url_%s", goRuntime.GOOS)] + "?version=" + data["version"])
	if err != nil {
		dialog.Error(a.ctx, "下载新版本失败", fmt.Sprintln("Error:", err))
		return
	}
	decodeString, _ := hex.DecodeString(data[fmt.Sprintf("sha256_%s", goRuntime.GOOS)])
	err = selfupdate.Apply(bytes.NewBuffer(all), selfupdate.Options{
		Checksum: decodeString,
	})
	if err != nil {
		dialog.Error(a.ctx, "更新失败", fmt.Sprintln("Error:", err))
	} else {
		_, _ = runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.InfoDialog,
			Title:   "更新完成",
			Message: "请重启应用",
		})
		a.exit()
	}
}
func (a *App) exit() {
	_ = a.box.Stop()
	runtime.Quit(a.ctx)
	systray.Quit()
	time.Sleep(time.Second)
	os.Exit(0)
}
func (a *App) systemTray() {
	systray.SetIcon(logo)
	systray.SetTitle("PlayFast")
	systray.SetTooltip("PlayFast")
	show := systray.AddMenuItem("显示窗口", "")
	systray.AddSeparator()
	check := systray.AddMenuItem(fmt.Sprintln("版本:", Version), "")
	check.Click(func() {
		go a.checkUpdate(true)
	})
	systray.AddSeparator()
	exit := systray.AddMenuItem("退出程序", "")
	show.Click(func() { runtime.WindowShow(a.ctx) })
	exit.Click(func() {
		a.exit()
	})
	systray.SetOnClick(func(menu systray.IMenu) { runtime.WindowShow(a.ctx) })
	go func() {
		listener, err := net.Listen("tcp", "127.0.0.1:54712")
		if err != nil {
			dialog.Error(a.ctx, "监听错误", fmt.Sprintln("Error:", err))
			return
		}
		var conn net.Conn
		for {
			conn, err = listener.Accept()
			if err != nil {
				return
			}
			// 读取指令
			buffer := make([]byte, 1024)
			n, err2 := conn.Read(buffer)
			if err2 != nil {
				continue
			}
			command := string(buffer[:n])
			// 如果收到显示窗口的命令，则显示窗口
			if command == "SHOW_WINDOW" {
				// 展示窗口的代码
				runtime.WindowShow(a.ctx)
			}
			_ = conn.Close()
		}
	}()
}
func (a *App) Switch(userUUID string, status bool, proxy string, route bool) string {
	for i := 0; i < 300; i++ {
		if a.init.Load() == false {
			time.Sleep(time.Millisecond * 100)
		} else {
			break
		}
	}
	var err error
	if status {
		if route {
			ip, gateway, mask, err2 := utils.RandIP()
			if err2 != nil {
				dialog.Error(a.ctx, "获取本地IP失败", err2.Error())
			}
			defer func() {
				go func() {
					dialog.Info(a.ctx, "网关配置", fmt.Sprintf("请务必将主机PS/XBOX插上网线，并确认本电脑和主机的网线都已经插在同一个路由器上后，主机PS/XBOX按照如下进行配置：IP:%s子网掩码：%s，网关：%s，DNS：1.1.1.1", ip, mask, gateway))
				}()
			}()
		}
		if err = a.box.Start(userUUID, proxy, route); err != nil {
			dialog.Error(a.ctx, "加速失败", err.Error())
			return err.Error()
		}

	} else {
		if err = a.box.Stop(); err != nil {
			dialog.Error(a.ctx, "停止失败", err.Error())
			return err.Error()
		}
	}
	return ""
}
func (a *App) GetAnnouncement() string {
	all, err := http_client.GET(fmt.Sprintf("%s/announcement", api.GetApiDomain()))
	if err != nil {
		return ""
	}
	return string(all)
}
func (a *App) Version() string {
	return Version
}
func (a *App) Open(path string) {
	_ = exec.Command(path).Start()
}
func (a *App) ProxyList(userUUID string) []string {
	get := node.Get(userUUID)
	strings := make([]string, 0)
	for _, proxy := range get {
		strings = append(strings, proxy.Name)
	}
	return strings
}
