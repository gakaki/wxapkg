//go:build darwin

package wechat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type macosPlatform struct{}

func newPlatform() platform { return &macosPlatform{} }

func (m *macosPlatform) GetDefaultPaths() PathScanResult {
	var b strings.Builder
	log := func(line string) { b.WriteString(line + "\n") }

	home, err := os.UserHomeDir()
	if err != nil {
		log("获取用户目录失败: " + err.Error())
		return PathScanResult{Logs: b.String()}
	}

	container := filepath.Join(home, "Library", "Containers", "com.tencent.xinWeChat", "Data")
	radium := filepath.Join(container, "Documents", "app_data", "radium")
	var blockedPath string
	v3 := filepath.Join(container, ".wxapplet", "packages")
	for _, candidate := range []string{
		container,
		radium,
		filepath.Join(container, ".wxapplet"),
		v3,
	} {
		if _, err := os.ReadDir(candidate); isPermissionError(err) {
			blockedPath = candidate
			break
		}
	}
	if blockedPath != "" {
		log("【需要权限】macOS 已阻止读取微信沙盒目录")
		log("   路径: " + blockedPath)
		log("   请在“系统设置 → 隐私与安全性 → 完全磁盘访问权限”中添加并启用本应用。")
	}

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

	// WeChat 3.x
	add("微信 3.x 小程序目录", v3)
	if !isDirectory(v3) {
		log("【未找到】微信 3.x 小程序目录\n   路径: " + v3)
	}

	if blockedPath == "" {
		// WeChat 4.1.7+ stores packages under the current account hash.
		discovered, denied := discoverAppletPackageDirsWithAccess(radium, 5)
		for _, path := range discovered {
			add("微信 4.1.7+ 当前用户小程序目录", path)
		}
		if denied != "" {
			blockedPath = denied
			log("【需要权限】macOS 已阻止读取微信小程序目录")
			log("   路径: " + blockedPath)
			log("   请在“系统设置 → 隐私与安全性 → 完全磁盘访问权限”中添加并启用本应用。")
		}
		if len(paths) == 0 {
			log("【未找到】微信 4.1.7+ 用户目录\n   根路径: " + radium)
		}

		// WeChat 4.0 and transitional layouts.
		for _, candidate := range []string{
			filepath.Join(radium, "Applet", "packages"),
			filepath.Join(radium, "applet", "packages"),
		} {
			add("微信 4.0 小程序目录", candidate)
		}
	}

	if len(paths) == 0 && blockedPath == "" {
		log("【提示】请先在微信中打开任意小程序，再重新检测。")
	}
	return PathScanResult{
		Paths:                 dedupePaths(paths),
		Logs:                  b.String(),
		NeedsFullDiskAccess:   blockedPath != "",
		PermissionBlockedPath: blockedPath,
	}
}
