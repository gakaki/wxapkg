package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/wux1an/wxapkg/wechat"
)

const (
	appVersion = "v2.2.0"
	githubURL  = "https://github.com/gakaki/wxapkg"
)

type preferences struct {
	DefaultOutputDir string            `json:"defaultOutputDir"`
	LastOutputDir    string            `json:"lastOutputDir"`
	AppNames         map[string]string `json:"appNames,omitempty"`
}

type app struct {
	win *mygo.Window

	items        []wechat.WxapkgItem
	search       string
	scanning     bool
	clearing     bool
	batchRunning bool
	batchIndex   int
	batchTotal   int
	detected     wechat.PathScanResult

	nameCache   map[string]string
	nameLookups map[string]struct{}

	scanOpen            bool
	scanTab             int
	showDetectLogs      bool
	manualPath          string
	resettingPermission bool

	settingsOpen     bool
	settingsDraft    string
	defaultOutputDir string
	lastOutputDir    string

	unpackOpen       bool
	unpackUUID       string
	unpackDecrypt    bool
	unpackHTML       bool
	unpackJS         bool
	unpackJSON       bool
	unpackOutput     string
	unpackSetDefault bool
	unpackKey        string

	table        ui.ListState
	sort         ui.SortOrder
	searchHandle ui.Handle

	noticeTitle string
	noticeBody  string
	noticeError bool

	initialPath string
}

func newApp() *app {
	a := &app{
		sort:        ui.SortOrder{Column: "modified", Descending: true},
		nameCache:   make(map[string]string),
		nameLookups: make(map[string]struct{}),
	}
	a.loadPreferences()
	changed := false
	if a.defaultOutputDir == "" {
		a.defaultOutputDir = defaultUnpackOutputDir()
		changed = a.defaultOutputDir != ""
	}
	if a.lastOutputDir == "" {
		a.lastOutputDir = a.defaultOutputDir
		changed = changed || a.lastOutputDir != ""
	}
	if changed {
		a.savePreferences()
	}
	return a
}

func (a *app) start(win *mygo.Window) {
	a.win = win
	a.installMenu()
	if a.initialPath != "" {
		a.scanPath(a.initialPath, true)
		return
	}
	a.rescan()
}

func (a *app) installMenu() {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "扫描小程序…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.openScan()
			}},
			{Label: "重新检测微信目录", Accelerator: "CmdOrCtrl+R", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.rescan()
			}},
			{Label: "清空微信小程序目录…", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.clearWeChatPackages()
			}},
			mygo.Separator(),
			{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.openSettings()
			}},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: "帮助", Submenu: []*mygo.MenuItem{
			{Label: "项目主页", Click: func(*mygo.MenuItem, *mygo.Window) {
				_ = mygo.Shell.OpenExternal(githubURL)
			}},
			{Label: "关于 wxapkg", Click: func(*mygo.MenuItem, *mygo.Window) {
				go func() {
					_, _ = mygo.Dialog.Message(mygo.MessageOptions{
						Parent:  a.win,
						Type:    mygo.MessageInfo,
						Title:   "微信小程序解包工具",
						Message: appVersion + " · 纯 Native MyGo",
						Detail:  "基于 egoist/mygo 原生 UI 重写，不使用 WebView。\n仅供学习研究使用。\n\n" + githubURL,
					})
				}()
			}},
		}},
	}))
}

func (a *app) openScan() {
	a.scanOpen = true
	if !a.scanning {
		a.rescan()
	}
}

func (a *app) openSettings() {
	a.settingsDraft = a.defaultOutputDir
	a.settingsOpen = true
}

func (a *app) rescan() {
	if a.scanning || a.clearing || a.batchRunning {
		return
	}
	a.scanning = true
	go a.detectAndScan()
}

func (a *app) detectAndScan() {
	result := wechat.Platform.GetDefaultPaths()
	logs := result.Logs
	var found []wechat.WxapkgItem

	for _, path := range result.Paths {
		items, err := wechat.ScanWxapkgItem(path, true)
		if err != nil {
			if denied := wechat.PermissionBlockedPath(err); denied != "" {
				result.NeedsFullDiskAccess = true
				if result.PermissionBlockedPath == "" {
					result.PermissionBlockedPath = denied
				}
			}
			logs += fmt.Sprintf("扫描目录失败: %s\n错误: %v\n", path, err)
			continue
		}
		found = append(found, items...)
	}

	if a.win == nil {
		return
	}
	debugLog("scan finished: roots=%d items=%d blocked=%v", len(result.Paths), len(found), result.NeedsFullDiskAccess)
	a.win.Update(func() {
		result.Logs = logs
		a.detected = result
		a.mergeItems(found)
		a.scanning = false
		a.enrichAppNames()
		switch {
		case result.NeedsFullDiskAccess:
			a.scanOpen = true
			a.showDetectLogs = true
			a.queueNotice("需要完全磁盘访问权限", "若授权开关已打开仍无权限，请清除旧授权并重新添加当前应用。", true)
		case len(result.Paths) == 0:
			a.queueNotice("未找到小程序目录", "请先在微信中打开任意小程序，或使用“手动添加”选择目录。", true)
		case len(found) == 0:
			a.queueNotice("目录已找到，但没有小程序包", "请先在微信中打开任意小程序，再重新检测。", true)
		default:
			a.queueNotice("扫描完成", fmt.Sprintf("共发现 %d 个小程序", len(found)), false)
		}
	})
}

func (a *app) scanPath(path string, scan bool) {
	if a.scanning || a.batchRunning {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	a.scanOpen = false
	a.scanning = true
	go func() {
		items, err := wechat.ScanWxapkgItem(path, scan)
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			a.scanning = false
			if err != nil {
				if denied := wechat.PermissionBlockedPath(err); denied != "" {
					a.detected = wechat.PathScanResult{
						NeedsFullDiskAccess:   true,
						PermissionBlockedPath: denied,
						Logs:                  err.Error(),
					}
					a.scanOpen = true
					a.showDetectLogs = true
					a.queueNotice("需要完全磁盘访问权限", "请重新授权当前应用后重试。", true)
					return
				}
				a.queueNotice("扫描失败", err.Error(), true)
				return
			}
			a.mergeItems(items)
			a.enrichAppNames()
			a.queueNotice("扫描完成", fmt.Sprintf("共发现 %d 个小程序", len(items)), false)
		})
	}()
}

func (a *app) mergeItems(incoming []wechat.WxapkgItem) {
	if len(incoming) == 0 {
		return
	}
	items := append([]wechat.WxapkgItem(nil), a.items...)
	index := make(map[string]int, len(items))
	for i := range items {
		index[items[i].Location] = i
	}
	for _, next := range incoming {
		if i, ok := index[next.Location]; ok {
			old := items[i]
			next.UUID = old.UUID
			if strings.TrimSpace(next.Name) == "" {
				next.Name = old.Name
			}
			if next.NameProvider == "" {
				next.NameProvider = old.NameProvider
			}
			if next.NameProviderID == "" {
				next.NameProviderID = old.NameProviderID
			}
			if next.FallbackNameProvider == "" {
				next.FallbackNameProvider = old.FallbackNameProvider
			}
			if next.FallbackNameProviderID == "" {
				next.FallbackNameProviderID = old.FallbackNameProviderID
			}
			next.EncryptKey = firstNonEmpty(old.EncryptKey, next.EncryptKey)
			next.UnpackStatus = old.UnpackStatus
			next.UnpackCurrent = old.UnpackCurrent
			next.UnpackTotal = old.UnpackTotal
			next.UnpackProgress = old.UnpackProgress
			next.UnpackCurrentFile = old.UnpackCurrentFile
			next.UnpackSavePath = old.UnpackSavePath
			next.UnpackErrorMessage = old.UnpackErrorMessage
			items[i] = next
			continue
		}
		index[next.Location] = len(items)
		items = append(items, next)
	}
	a.items = items
}

func (a *app) visibleItems() []wechat.WxapkgItem {
	query := strings.ToLower(strings.TrimSpace(a.search))
	items := make([]wechat.WxapkgItem, 0, len(a.items))
	for _, item := range a.items {
		if query != "" &&
			!strings.Contains(strings.ToLower(item.WxId), query) &&
			!strings.Contains(strings.ToLower(item.Name), query) &&
			!strings.Contains(strings.ToLower(item.Location), query) {
			continue
		}
		items = append(items, item)
	}

	col, desc := a.sort.Column, a.sort.Descending
	if col == "" {
		col, desc = "modified", true
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		var less, equal bool
		switch col {
		case "name":
			leftName, rightName := strings.ToLower(left.Name), strings.ToLower(right.Name)
			less, equal = leftName < rightName, leftName == rightName
		case "id":
			less, equal = left.WxId < right.WxId, left.WxId == right.WxId
		case "size":
			less, equal = left.Size < right.Size, left.Size == right.Size
		case "path":
			less, equal = left.Location < right.Location, left.Location == right.Location
		case "status":
			leftStatus, rightStatus := statusRank(left.UnpackStatus), statusRank(right.UnpackStatus)
			less, equal = leftStatus < rightStatus, leftStatus == rightStatus
		default:
			less, equal = left.LastModifyTime < right.LastModifyTime, left.LastModifyTime == right.LastModifyTime
		}
		if equal {
			return left.Location < right.Location
		}
		if desc {
			return !less
		}
		return less
	})
	return items
}

func statusRank(status wechat.StatusType) int {
	switch status {
	case wechat.StatusTypeRunning:
		return 1
	case wechat.StatusTypeFinished:
		return 2
	case wechat.StatusTypeError:
		return 3
	default:
		return 0
	}
}

func (a *app) itemByUUID(uuid string) *wechat.WxapkgItem {
	for i := range a.items {
		if a.items[i].UUID == uuid {
			return &a.items[i]
		}
	}
	return nil
}

func (a *app) openUnpack(uuid string) {
	if a.batchRunning {
		a.queueNotice("无法打开解包配置", "请等待批量解包完成。", true)
		return
	}
	item := a.itemByUUID(uuid)
	if item == nil {
		return
	}
	a.unpackUUID = uuid
	a.unpackOpen = true
	a.unpackDecrypt = true
	a.unpackHTML = true
	a.unpackJS = true
	a.unpackJSON = true
	a.unpackSetDefault = false
	a.unpackKey = item.EncryptKey
	if a.unpackKey == "" && item.WxId != "unknown" {
		a.unpackKey = item.WxId
	}
	a.unpackOutput = firstNonEmpty(a.defaultOutputDir, a.lastOutputDir)
}

func (a *app) startUnpack() {
	item := a.itemByUUID(a.unpackUUID)
	if item == nil {
		return
	}
	outputDir := strings.TrimSpace(a.unpackOutput)
	if outputDir == "" {
		a.queueNotice("无法开始解包", "请选择输出目录。", true)
		return
	}
	key := strings.TrimSpace(a.unpackKey)
	if a.unpackDecrypt && key == "" {
		a.queueNotice("无法开始解包", "启用解密时必须填写小程序 ID。", true)
		return
	}

	item.EncryptKey = key
	item.UnpackStatus = wechat.StatusTypeRunning
	item.UnpackCurrent = 0
	item.UnpackTotal = 0
	item.UnpackProgress = 0
	item.UnpackCurrentFile = ""
	item.UnpackSavePath = computeSavePath(outputDir, item.Location, item.Name)
	item.UnpackErrorMessage = ""

	a.lastOutputDir = outputDir
	if a.unpackSetDefault {
		a.defaultOutputDir = outputDir
	}
	a.savePreferences()

	workerItem := *item
	options := wechat.UnpackOptions{
		EnableDecrypt:      a.unpackDecrypt,
		EnableHtmlBeautify: a.unpackHTML,
		EnableJsBeautify:   a.unpackJS,
		EnableJsonBeautify: a.unpackJSON,
		OutputDir:          outputDir,
		SavePath:           item.UnpackSavePath,
	}

	go func() {
		var last time.Time
		unpacker := wechat.NewUnpacker(&workerItem, &options)
		unpacker.UnpackWithStatusCallback(func(current *wechat.WxapkgItem) {
			snapshot := *current
			final := snapshot.UnpackStatus != wechat.StatusTypeRunning
			if !final && time.Since(last) < 60*time.Millisecond {
				return
			}
			last = time.Now()
			if a.win == nil {
				return
			}
			a.win.Update(func() {
				a.updateItem(snapshot)
			})
		})
	}()
}

func (a *app) updateItem(snapshot wechat.WxapkgItem) {
	for i := range a.items {
		if a.items[i].UUID != snapshot.UUID {
			continue
		}
		current := a.items[i]
		snapshot.Name = firstNonEmpty(current.Name, snapshot.Name)
		snapshot.NameProvider = firstNonEmpty(current.NameProvider, snapshot.NameProvider)
		snapshot.NameProviderID = firstNonEmpty(current.NameProviderID, snapshot.NameProviderID)
		snapshot.FallbackNameProvider = firstNonEmpty(current.FallbackNameProvider, snapshot.FallbackNameProvider)
		snapshot.FallbackNameProviderID = firstNonEmpty(current.FallbackNameProviderID, snapshot.FallbackNameProviderID)
		snapshot.EncryptKey = firstNonEmpty(current.EncryptKey, snapshot.EncryptKey)
		snapshot.WxId = firstNonEmpty(current.WxId, snapshot.WxId)
		snapshot.Location = firstNonEmpty(current.Location, snapshot.Location)
		snapshot.Size = current.Size
		snapshot.IsDir = current.IsDir
		snapshot.LastModifyTime = current.LastModifyTime
		previous := a.items[i].UnpackStatus
		a.items[i] = snapshot
		if snapshot.UnpackStatus == wechat.StatusTypeFinished && previous != wechat.StatusTypeFinished {
			a.queueNotice("解包完成", snapshot.UnpackSavePath, false)
		}
		if snapshot.UnpackStatus == wechat.StatusTypeError && previous != wechat.StatusTypeError {
			a.queueNotice("解包失败", snapshot.UnpackErrorMessage, true)
		}
		return
	}
}

func (a *app) queueNotice(title, body string, isError bool) {
	a.noticeTitle = title
	a.noticeBody = body
	a.noticeError = isError
}

func (a *app) flushNotice(c *ui.Context) {
	if a.noticeTitle == "" {
		return
	}
	toast := ui.Toast{ID: "app-notice", Title: a.noticeTitle, Description: a.noticeBody}
	if a.noticeError {
		toast.Type = "error"
	}
	c.AddToast(toast)
	a.noticeTitle, a.noticeBody, a.noticeError = "", "", false
}

func (a *app) clearItems() {
	if a.scanning || a.clearing || a.batchRunning {
		return
	}
	a.items = nil
	a.queueNotice("已清空", "小程序列表已清空。", false)
}

func (a *app) copyPath(path string) {
	if path == "" {
		return
	}
	mygo.Clipboard.WriteText(path)
	a.queueNotice("已复制", path, false)
}

func (a *app) openPath(path string) {
	if path == "" {
		return
	}
	go func() {
		if err := mygo.Shell.OpenPath(path); err != nil {
			if a.win != nil {
				a.win.Update(func() { a.queueNotice("打开目录失败", err.Error(), true) })
			}
		}
	}()
}

func (a *app) openParentPath(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	a.openPath(filepath.Dir(filepath.Clean(path)))
}

func (a *app) openFullDiskAccessSettings() {
	if runtime.GOOS != "darwin" {
		return
	}
	go func() {
		url := "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"
		if err := mygo.Shell.OpenExternal(url); err != nil && a.win != nil {
			a.win.Update(func() {
				a.queueNotice("打开系统设置失败", "请手动打开“系统设置 → 隐私与安全性 → 完全磁盘访问权限”。", true)
			})
		}
	}()
}

func (a *app) resetFullDiskAccess() {
	if runtime.GOOS != "darwin" || a.resettingPermission {
		return
	}
	a.resettingPermission = true
	go func() {
		err := resetFullDiskAccessAuthorization()
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			a.resettingPermission = false
			if err != nil {
				a.queueNotice("清除旧授权失败", err.Error(), true)
				return
			}
			a.queueNotice("旧授权已清除", "请在系统设置中点击 + 添加当前应用，然后重启本应用再重新检测。", false)
			a.openFullDiskAccessSettings()
		})
	}()
}

func (a *app) revealCurrentApp() {
	bundle := currentAppBundlePath()
	if bundle == "" {
		a.queueNotice("找不到应用位置", "请从访达中手动打开当前应用所在的 .app。", true)
		return
	}
	mygo.Shell.ShowItemInFolder(bundle)
}

func (a *app) saveSettings() {
	a.defaultOutputDir = strings.TrimSpace(a.settingsDraft)
	a.savePreferences()
	a.settingsOpen = false
	a.queueNotice("设置已保存", defaultOutputMessage(a.defaultOutputDir), false)
}

func (a *app) browseOutputDirectory(draft *string, title string, forSettings bool) {
	current := strings.TrimSpace(*draft)
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            a.win,
			Title:             title,
			DefaultPath:       current,
			Directory:         true,
			CreateDirectories: true,
		})
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			if err != nil {
				a.queueNotice("选择目录失败", err.Error(), true)
				return
			}
			if len(paths) > 0 {
				if forSettings {
					a.settingsDraft = paths[0]
				} else {
					a.unpackOutput = paths[0]
				}
			}
		})
	}()
}

func (a *app) browseScanFile() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win,
			Title:  "选择微信小程序文件",
			Filters: []mygo.FileFilter{
				{Name: "微信小程序文件", Extensions: []string{"wxapkg"}},
			},
		})
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			if err != nil {
				a.queueNotice("选择文件失败", err.Error(), true)
				return
			}
			if len(paths) > 0 {
				a.scanPath(paths[0], false)
			}
		})
	}()
}

func (a *app) browseScanDirectory(title string, scan bool) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    a.win,
			Title:     title,
			Directory: true,
		})
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			if err != nil {
				a.queueNotice("选择目录失败", err.Error(), true)
				return
			}
			if len(paths) > 0 {
				a.scanPath(paths[0], scan)
			}
		})
	}()
}

func (a *app) preferencesPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "preferences.json"), nil
}

func (a *app) loadPreferences() {
	path, err := a.preferencesPath()
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var prefs preferences
	if json.Unmarshal(data, &prefs) != nil {
		return
	}
	a.defaultOutputDir = strings.TrimSpace(prefs.DefaultOutputDir)
	a.lastOutputDir = strings.TrimSpace(prefs.LastOutputDir)
	if prefs.AppNames != nil {
		a.nameCache = prefs.AppNames
	}
	if a.nameLookups == nil {
		a.nameLookups = make(map[string]struct{})
	}
}

func (a *app) savePreferences() {
	path, err := a.preferencesPath()
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(preferences{
		DefaultOutputDir: a.defaultOutputDir,
		LastOutputDir:    a.lastOutputDir,
		AppNames:         a.nameCache,
	}, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func computeSavePath(outputDir, wxapkgPath, appName string) string {
	absDir, err := filepath.Abs(strings.TrimSpace(outputDir))
	if err != nil {
		absDir = strings.TrimSpace(outputDir)
	}
	base := filepath.Base(wxapkgPath)
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "wxapkg"
	}
	saveName := base + "_unpacked"
	if name := safeOutputName(appName); name != "" && name != base && name != "未识别" {
		saveName += "_" + name
	}
	return filepath.Join(absDir, saveName)
}

func safeOutputName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '/', '\\', ':', 0:
			b.WriteRune('_')
		default:
			if r < 0x20 || r == 0x7f {
				continue
			}
			b.WriteRune(r)
		}
	}
	name = strings.Trim(b.String(), " .")
	runes := []rune(name)
	if len(runes) > 80 {
		name = string(runes[:80])
	}
	return name
}

func defaultOutputMessage(path string) string {
	if path == "" {
		return "已清空默认输出目录。"
	}
	return path
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
