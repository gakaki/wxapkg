package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/wux1an/wxapkg/wechat"
)

var reWxMiniProgramID = regexp.MustCompile(`^wx[0-9a-f]{16}$`)

type packageCleanupTarget struct {
	Root  string
	Paths []string
	Size  int64
}

type packageCleanupResult struct {
	Deleted int
	Bytes   int64
	Failed  []string
}

func cleanablePackageRoots(items []wechat.WxapkgItem) []string {
	roots := make(map[string]struct{})
	for _, item := range items {
		if !item.IsDir || !reWxMiniProgramID.MatchString(filepath.Base(item.Location)) {
			continue
		}
		root := filepath.Clean(filepath.Dir(item.Location))
		if isSafePackageRoot(root) {
			roots[root] = struct{}{}
		}
	}
	result := make([]string, 0, len(roots))
	for root := range roots {
		result = append(result, root)
	}
	sort.Strings(result)
	return result
}

func isSafePackageRoot(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." || !filepath.IsAbs(root) {
		return false
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if !strings.EqualFold(filepath.Base(root), "packages") {
		return false
	}
	parent := filepath.Base(filepath.Dir(root))
	return strings.EqualFold(parent, "applet") || strings.EqualFold(parent, ".wxapplet")
}

func collectPackageCleanupTargets(items []wechat.WxapkgItem) ([]packageCleanupTarget, error) {
	roots := cleanablePackageRoots(items)
	if len(roots) == 0 {
		return nil, fmt.Errorf("没有可安全清理的微信 packages 目录")
	}

	targets := make([]packageCleanupTarget, 0, len(roots))
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		target := packageCleanupTarget{Root: root}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !reWxMiniProgramID.MatchString(entry.Name()) {
				continue
			}
			path := filepath.Join(root, entry.Name())
			target.Paths = append(target.Paths, path)
			target.Size += directorySize(path)
		}
		if len(target.Paths) > 0 {
			sort.Strings(target.Paths)
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("packages 目录中没有匹配 wx 前缀的小程序目录")
	}
	return targets, nil
}

func deletePackageCleanupTargets(targets []packageCleanupTarget) packageCleanupResult {
	var result packageCleanupResult
	for _, target := range targets {
		if !isSafePackageRoot(target.Root) {
			result.Failed = append(result.Failed, target.Root+": 不是可清理的微信 packages 目录")
			continue
		}
		for _, path := range target.Paths {
			if filepath.Clean(filepath.Dir(path)) != target.Root ||
				!reWxMiniProgramID.MatchString(filepath.Base(path)) {
				result.Failed = append(result.Failed, path+": 路径校验失败")
				continue
			}
			info, err := os.Lstat(path)
			if err != nil {
				result.Failed = append(result.Failed, fmt.Sprintf("%s: %v", path, err))
				continue
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				result.Failed = append(result.Failed, path+": 不是普通小程序目录")
				continue
			}
			size := directorySize(path)
			if err := os.RemoveAll(path); err != nil {
				result.Failed = append(result.Failed, fmt.Sprintf("%s: %v", path, err))
				continue
			}
			result.Deleted++
			result.Bytes += size
		}
	}
	return result
}

func directorySize(root string) int64 {
	var size int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func cleanupSummary(targets []packageCleanupTarget) (count int, size int64, roots []string) {
	for _, target := range targets {
		count += len(target.Paths)
		size += target.Size
		roots = append(roots, target.Root)
	}
	return count, size, roots
}
