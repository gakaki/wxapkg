//go:build windows

package wechat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type windowsPlatform struct{}

func newPlatform() platform { return &windowsPlatform{} }

func (m *windowsPlatform) GetDefaultPaths() PathScanResult {
	var b strings.Builder
	log := func(line string) { b.WriteString(line + "\n") }

	seen := make(map[string]struct{})
	var paths []string
	add := func(label, path string) {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok || !isDirectory(path) {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
		log(fmt.Sprintf("【成功】%s\n   路径: %s", label, path))
	}

	// WeChat 4.1.7+ stores packages under the current account hash.
	appData, _ := os.UserConfigDir()
	radium := filepath.Join(appData, "Tencent", "xwechat", "radium")
	for _, path := range discoverAppletPackageDirs(radium, 5) {
		add("微信 4.1.7+ 当前用户小程序目录", path)
	}

	// WeChat 4.0 and transitional layouts.
	for _, path := range []string{
		filepath.Join(radium, "Applet", "packages"),
		filepath.Join(radium, "applet", "packages"),
	} {
		add("微信 4.0 小程序目录", path)
	}

	// WeChat 3.x stores its data under the configured Documents folder.
	weChatFiles := []string{}
	if key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Tencent\WeChat`, registry.QUERY_VALUE); err == nil {
		value, _, valueErr := key.GetStringValue("FileSavePath")
		_ = key.Close()
		if valueErr == nil {
			if value == "MyDocument:" {
				if home, homeErr := os.UserHomeDir(); homeErr == nil {
					value = filepath.Join(home, "Documents")
				}
			}
			weChatFiles = append(weChatFiles,
				filepath.Join(value, "WeChat Files"),
				filepath.Join(value, "WeChatFiles"),
			)
			log("【成功】读取微信 3.x 文件保存位置\n   路径: " + value)
		} else {
			log("【未找到】微信 3.x 注册表值 HKCU\\Software\\Tencent\\WeChat\\FileSavePath")
		}
	} else {
		log("【未找到】微信 3.x 注册表项 HKCU\\Software\\Tencent\\WeChat")
	}
	if home, err := os.UserHomeDir(); err == nil {
		weChatFiles = append(weChatFiles,
			filepath.Join(home, "Documents", "WeChat Files"),
			filepath.Join(home, "Documents", "WeChatFiles"),
		)
	}
	for _, base := range dedupePaths(weChatFiles) {
		if isDirectory(filepath.Join(base, "Applet")) {
			add("微信 3.x Applet 目录", filepath.Join(base, "Applet"))
		}
		if isDirectory(base) {
			add("微信 3.x 数据目录", base)
		}
	}

	if len(paths) == 0 {
		log("【提示】请先在微信中打开任意小程序，再重新检测；也可以手动选择 .wxapkg 文件或目录。")
	}
	return PathScanResult{Paths: dedupePaths(paths), Logs: b.String()}
}
