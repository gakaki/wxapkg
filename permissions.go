package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	nativeBundleID           = "com.wux1an.wxapkg.native"
	fullDiskAccessTCCService = "SystemPolicyAllFiles"
	fullDiskAccessHint       = "如果系统设置中的开关已经打开，仍提示需要权限，通常是旧版构建留下的授权记录。请点“清除旧授权”，再用 + 添加当前应用，然后重启本应用。"
)

// currentAppBundlePath returns the .app that holds the running program, or
// "" when the program runs outside a macOS app bundle.
func currentAppBundlePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return ""
	}
	macOSDir := filepath.Dir(exe)
	contentsDir := filepath.Dir(macOSDir)
	if filepath.Base(macOSDir) != "MacOS" || filepath.Base(contentsDir) != "Contents" {
		return ""
	}
	bundle := filepath.Dir(contentsDir)
	if !strings.EqualFold(filepath.Ext(bundle), ".app") {
		return ""
	}
	return bundle
}

// resetFullDiskAccessAuthorization removes the TCC record of this bundle.
// It is needed when an unsigned build changed its code hash: macOS then
// keeps an old, enabled-looking record that no longer matches the app.
func resetFullDiskAccessAuthorization() error {
	if runtime.GOOS != "darwin" {
		return errors.New("完全磁盘访问权限只适用于 macOS")
	}
	out, err := exec.Command("/usr/bin/tccutil", "reset", fullDiskAccessTCCService, nativeBundleID).CombinedOutput()
	if err != nil {
		return fmt.Errorf("清除旧授权失败: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
