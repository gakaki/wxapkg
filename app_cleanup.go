package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/wux1an/wxapkg/wechat"
)

func (a *app) hasCleanupTargets() bool {
	return len(cleanablePackageRoots(a.items)) > 0
}

func (a *app) hasRunningUnpack() bool {
	if a.batchRunning {
		return true
	}
	for _, item := range a.items {
		if item.UnpackStatus == wechat.StatusTypeRunning {
			return true
		}
	}
	return false
}

func (a *app) clearWeChatPackages() {
	if a.scanning || a.clearing {
		return
	}
	if a.hasRunningUnpack() {
		a.queueNotice("无法清空", "请等待当前解包任务完成后再清理微信小程序目录。", true)
		return
	}

	targets, err := collectPackageCleanupTargets(a.items)
	if err != nil {
		a.queueNotice("无法清空", err.Error(), true)
		return
	}
	count, size, roots := cleanupSummary(targets)
	a.clearing = true
	go a.runPackageCleanup(targets, count, size, roots)
}

func (a *app) runPackageCleanup(
	targets []packageCleanupTarget,
	count int,
	size int64,
	roots []string,
) {
	message := fmt.Sprintf("将永久删除 %d 个微信小程序下载目录（%s）。", count, formatSize(size))
	detail := "只会删除微信 packages 目录下名称严格匹配 wx + 16 位十六进制字符的直接子目录，" +
		"不会删除 packages 根目录、微信聊天记录或账号数据。\n\n" +
		"清理后再次打开小程序时，微信会重新下载。建议先退出微信再执行。\n\n目录：\n" +
		strings.Join(roots, "\n")
	result, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:             a.win,
		Type:               mygo.MessageWarning,
		Title:              "清空微信小程序目录",
		Message:            message,
		Detail:             detail,
		Buttons:            []string{"取消", "清空缓存"},
		DefaultButton:      0,
		CancelButton:       0,
		DestructiveButtons: []int{1},
	})
	if a.win == nil {
		return
	}
	if err != nil {
		a.win.Update(func() {
			a.clearing = false
			a.queueNotice("清空失败", err.Error(), true)
		})
		return
	}
	if result.Button != 1 {
		a.win.Update(func() {
			a.clearing = false
		})
		return
	}

	cleanup := deletePackageCleanupTargets(targets)
	a.win.Update(func() {
		a.clearing = false
		a.items = itemsOutsideRoots(a.items, roots)
		if len(cleanup.Failed) > 0 {
			detail := strings.Join(cleanup.Failed, "\n")
			if len(cleanup.Failed) > 4 {
				detail = strings.Join(cleanup.Failed[:4], "\n") +
					fmt.Sprintf("\n另有 %d 项失败", len(cleanup.Failed)-4)
			}
			a.queueNotice(
				"部分小程序目录清理失败",
				fmt.Sprintf("已删除 %d 个，释放 %s。\n%s", cleanup.Deleted, formatSize(cleanup.Bytes), detail),
				true,
			)
		} else {
			a.queueNotice(
				"微信小程序缓存已清空",
				fmt.Sprintf("已删除 %d 个下载目录，释放 %s。", cleanup.Deleted, formatSize(cleanup.Bytes)),
				false,
			)
		}
		a.rescan()
	})
}

func itemsOutsideRoots(items []wechat.WxapkgItem, roots []string) []wechat.WxapkgItem {
	if len(roots) == 0 {
		return items
	}
	result := make([]wechat.WxapkgItem, 0, len(items))
	for _, item := range items {
		if pathInsideAnyRoot(item.Location, roots) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func pathInsideAnyRoot(path string, roots []string) bool {
	path = filepath.Clean(path)
	for _, root := range roots {
		rel, err := filepath.Rel(filepath.Clean(root), path)
		if err != nil || rel == "." || filepath.IsAbs(rel) ||
			rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return true
	}
	return false
}
