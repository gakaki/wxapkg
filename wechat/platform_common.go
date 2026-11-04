package wechat

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var errFoundWxapkg = errors.New("found wxapkg")

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isPermissionError(err error) bool {
	return errors.Is(err, fs.ErrPermission) || os.IsPermission(err)
}

// PermissionError reports a path whose contents macOS refused to let the
// app read. The path is the one to guide the user to grant Full Disk Access
// for; it may be deeper than the directory the scan started from.
type PermissionError struct {
	Path string
	Err  error
}

func (e *PermissionError) Error() string {
	return fmt.Sprintf("macOS 已阻止访问 %s: %v", e.Path, e.Err)
}

func (e *PermissionError) Unwrap() error { return e.Err }

// PermissionBlockedPath returns the blocked path in err, or "" when err is
// not a permission error.
func PermissionBlockedPath(err error) string {
	var permission *PermissionError
	if errors.As(err, &permission) {
		return permission.Path
	}
	return ""
}

// discoverAppletPackageDirs finds current and future applet package roots
// under a bounded WeChat data tree. It deliberately skips large unrelated
// directories so scanning remains fast.
func discoverAppletPackageDirs(root string, maxDepth int) []string {
	paths, _ := discoverAppletPackageDirsWithAccess(root, maxDepth)
	return paths
}

// discoverAppletPackageDirsWithAccess is discoverAppletPackageDirs, and
// reports the first path TCC refused to let the scan read.
func discoverAppletPackageDirsWithAccess(root string, maxDepth int) ([]string, string) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		if isPermissionError(err) {
			return nil, root
		}
		return nil, ""
	}
	if !info.IsDir() {
		return nil, ""
	}
	found := make(map[string]struct{})
	var blocked string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if blocked == "" && isPermissionError(err) {
				blocked = path
			}
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root {
			if shouldSkipWeChatDir(entry.Name()) {
				return fs.SkipDir
			}
			if directoryDepth(root, path) > maxDepth {
				return fs.SkipDir
			}
		}
		if isAppletPackagesDir(path) {
			found[path] = struct{}{}
			return fs.SkipDir
		}
		return nil
	})
	return sortedPaths(found), blocked
}

func isAppletPackagesDir(path string) bool {
	if !strings.EqualFold(filepath.Base(path), "packages") {
		return false
	}
	parent := filepath.Base(filepath.Dir(path))
	return strings.EqualFold(parent, "applet") || strings.EqualFold(parent, ".wxapplet")
}

func shouldSkipWeChatDir(name string) bool {
	switch strings.ToLower(name) {
	case "web", "cache", "crashpad", "mmkv", "xworker", "ilink", "log", "logs",
		"temp", "tmp", "gpucache", "code cache", "blob_storage", "indexeddb",
		"service worker", "local storage", "session storage", "sessions",
		"dawnwebgpucache", "dawngraphitecache", "shared dictionary",
		"subresource filter", "feature engagement tracker", "webstorage":
		return true
	default:
		return false
	}
}

func directoryDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(rel, string(filepath.Separator)))
}

func sortedPaths(set map[string]struct{}) []string {
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func dedupePaths(paths ...[]string) []string {
	set := make(map[string]struct{})
	for _, group := range paths {
		for _, path := range group {
			path = filepath.Clean(strings.TrimSpace(path))
			if path == "" || path == "." {
				continue
			}
			set[path] = struct{}{}
		}
	}
	return sortedPaths(set)
}

func packageRootsForScan(path string) []string {
	path = filepath.Clean(path)
	if reWxId.MatchString(filepath.Base(path)) {
		return []string{path}
	}

	set := make(map[string]struct{})
	if entries, err := os.ReadDir(path); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && reWxId.MatchString(entry.Name()) {
				set[path] = struct{}{}
				break
			}
		}
	}

	candidates := []string{
		filepath.Join(path, "packages"),
		filepath.Join(path, "applet", "packages"),
		filepath.Join(path, "Applet", "packages"),
		filepath.Join(path, "users"),
		path,
	}
	for _, candidate := range candidates {
		if isAppletPackagesDir(candidate) && isDirectory(candidate) {
			set[candidate] = struct{}{}
		}
	}
	for _, discovered := range discoverAppletPackageDirs(path, 5) {
		set[discovered] = struct{}{}
	}
	if len(set) > 0 {
		return sortedPaths(set)
	}

	// WeChat 3.x keeps wxid directories directly under Applet, without a
	// packages directory. Find those by their name and content.
	for _, dir := range findWxIDDirs(path, 4) {
		set[dir] = struct{}{}
	}
	return sortedPaths(set)
}

func findWxIDDirs(root string, maxDepth int) []string {
	root = filepath.Clean(root)
	if !isDirectory(root) {
		return nil
	}
	found := make(map[string]struct{})
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root {
			if directoryDepth(root, path) > maxDepth {
				return fs.SkipDir
			}
			if shouldSkipWeChatDir(entry.Name()) {
				return fs.SkipDir
			}
		}
		if reWxId.MatchString(entry.Name()) && containsWxapkg(path, 3) {
			found[path] = struct{}{}
			return fs.SkipDir
		}
		return nil
	})
	return sortedPaths(found)
}

func containsWxapkg(root string, maxDepth int) bool {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			if strings.EqualFold(filepath.Ext(entry.Name()), ".wxapkg") {
				return errFoundWxapkg
			}
			return nil
		}
		if path != root && directoryDepth(root, path) > maxDepth {
			return fs.SkipDir
		}
		return nil
	})
	return errors.Is(err, errFoundWxapkg)
}
