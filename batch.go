package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/wux1an/wxapkg/wechat"
)

const (
	defaultUnpackFolderName = "微信小程序解包目录"
	maxBatchBaseBytes       = 180
)

type batchUnpackJob struct {
	item      wechat.WxapkgItem
	root      string
	base      string
	finalPath string
	key       string
	decrypt   bool
}

func defaultUnpackOutputDir() string {
	desktop, err := mygo.App.Path(mygo.PathDesktop)
	if err != nil || strings.TrimSpace(desktop) == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return ""
		}
		desktop = filepath.Join(home, "Desktop")
	}
	return filepath.Join(desktop, defaultUnpackFolderName)
}

func batchOutputBase(item wechat.WxapkgItem) string {
	name := safeOutputName(item.Name)
	if name == "" {
		name = "未识别"
	}
	id := safeOutputName(item.WxId)
	if id == "" {
		id = "unknown"
	}
	id = truncateUTF8Bytes(id, 80)
	budget := maxBatchBaseBytes - len(id) - 1
	if budget < 24 {
		budget = 24
	}
	name = truncateUTF8Bytes(name, budget)
	if name == "" {
		name = "未识别"
	}
	return name + "-" + id
}

func batchOutputModified(timestamp int64) string {
	if timestamp <= 0 {
		return "00000000-000000"
	}
	return time.Unix(timestamp, 0).Format("20060102-150405")
}

func batchOutputDirectoryName(item wechat.WxapkgItem) string {
	return batchOutputBase(item) + "-" + batchOutputModified(item.LastModifyTime)
}

func (a *app) batchOutputRoot() string {
	root := firstNonEmpty(a.defaultOutputDir, defaultUnpackOutputDir())
	if abs, err := filepath.Abs(strings.TrimSpace(root)); err == nil {
		return abs
	}
	return strings.TrimSpace(root)
}

func (a *app) startBatchUnpack() {
	if a.scanning || a.clearing || a.hasRunningUnpack() {
		a.queueNotice("无法开始批量解包", "请等待当前扫描、清理或解包任务完成。", true)
		return
	}
	if len(a.items) == 0 {
		a.queueNotice("列表为空", "请先扫描小程序。", true)
		return
	}

	root := a.batchOutputRoot()
	if root == "" {
		a.queueNotice("无法确定输出目录", "找不到桌面目录，请先在设置中指定输出目录。", true)
		return
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		a.queueNotice("无法创建输出目录", err.Error(), true)
		return
	}

	jobs := make([]batchUnpackJob, 0, len(a.items))
	for _, item := range a.items {
		key := firstNonEmpty(item.EncryptKey, item.WxId)
		decrypt := key != "" && key != "unknown"
		if !decrypt {
			key = ""
		}
		base := batchOutputBase(item)
		jobs = append(jobs, batchUnpackJob{
			item:      item,
			root:      root,
			base:      base,
			finalPath: filepath.Join(root, base+"-"+batchOutputModified(item.LastModifyTime)),
			key:       key,
			decrypt:   decrypt,
		})
	}

	a.batchRunning = true
	a.batchIndex = 0
	a.batchTotal = len(jobs)
	go a.confirmBatchUnpack(jobs, root)
}

func (a *app) confirmBatchUnpack(jobs []batchUnpackJob, root string) {
	result, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:  a.win,
		Type:    mygo.MessageQuestion,
		Title:   "批量解包",
		Message: fmt.Sprintf("将依次解包列表中的 %d 个小程序。", len(jobs)),
		Detail: "输出目录：\n" + root + "\n\n" +
			"目录格式：小程序名称-小程序ID-修改时间\n" +
			"同名且同 ID 的旧解包目录会自动删除并替换，其他目录不受影响。",
		Buttons:            []string{"取消", "开始批量解包"},
		DefaultButton:      0,
		CancelButton:       0,
		DestructiveButtons: []int{1},
	})
	if a.win == nil {
		return
	}
	if err != nil {
		a.win.Update(func() {
			a.batchRunning = false
			a.queueNotice("批量解包失败", err.Error(), true)
		})
		return
	}
	if result.Button != 1 {
		a.win.Update(func() {
			a.batchRunning = false
		})
		return
	}
	a.runBatchUnpack(jobs)
}

func (a *app) runBatchUnpack(jobs []batchUnpackJob) {
	successes, failures := 0, 0
	failureDetails := make([]string, 0, 4)
	for index, job := range jobs {
		current := index + 1
		a.win.Update(func() {
			a.batchIndex = current
			a.markBatchItemRunning(job)
		})

		snapshot, err := a.runBatchJob(job)
		if err == nil && snapshot.UnpackStatus != wechat.StatusTypeFinished {
			err = fmt.Errorf("解包未完成")
		}
		if err != nil {
			failures++
			if len(failureDetails) < 4 {
				label := firstNonEmpty(job.item.Name, job.item.WxId, filepath.Base(job.item.Location))
				failureDetails = append(failureDetails, fmt.Sprintf("%s：%v", label, err))
			}
		} else {
			successes++
		}
		a.win.Update(func() {
			a.updateItem(snapshot)
		})
	}

	a.win.Update(func() {
		a.batchRunning = false
		a.batchIndex = len(jobs)
		title := "批量解包完成"
		body := fmt.Sprintf("成功 %d 个，失败 %d 个。\n%s", successes, failures, jobs[0].root)
		if failures > 0 {
			title = "批量解包完成，部分失败"
			body = fmt.Sprintf("成功 %d 个，失败 %d 个。\n%s", successes, failures, strings.Join(failureDetails, "\n"))
		}
		a.queueNotice(title, body, failures > 0)
	})
}

func (a *app) markBatchItemRunning(job batchUnpackJob) {
	item := a.itemByUUID(job.item.UUID)
	if item == nil {
		return
	}
	item.UnpackStatus = wechat.StatusTypeRunning
	item.UnpackCurrent = 0
	item.UnpackTotal = 0
	item.UnpackProgress = 0
	item.UnpackCurrentFile = ""
	item.UnpackSavePath = job.finalPath
	item.UnpackErrorMessage = ""
}

func (a *app) runBatchJob(job batchUnpackJob) (wechat.WxapkgItem, error) {
	worker := job.item
	worker.EncryptKey = job.key
	worker.UnpackStatus = wechat.StatusTypeRunning
	worker.UnpackCurrent = 0
	worker.UnpackTotal = 0
	worker.UnpackProgress = 0
	worker.UnpackCurrentFile = ""
	worker.UnpackSavePath = job.finalPath
	worker.UnpackErrorMessage = ""

	stage, err := os.MkdirTemp(job.root, ".wxapkg-batch-*")
	if err != nil {
		err = fmt.Errorf("创建临时解包目录失败：%v", err)
		worker.SetErrorState(err.Error())
		worker.UnpackSavePath = job.finalPath
		return worker, err
	}
	stageExists := true
	defer func() {
		if stageExists {
			_ = os.RemoveAll(stage)
		}
	}()

	options := wechat.UnpackOptions{
		EnableDecrypt:      job.decrypt,
		EnableHtmlBeautify: true,
		EnableJsBeautify:   true,
		EnableJsonBeautify: true,
		OutputDir:          job.root,
		SavePath:           stage,
	}
	var last time.Time
	unpacker := wechat.NewUnpacker(&worker, &options)
	unpacker.UnpackWithStatusCallback(func(current *wechat.WxapkgItem) {
		snapshot := *current
		snapshot.UnpackSavePath = job.finalPath
		if snapshot.UnpackStatus == wechat.StatusTypeFinished {
			return
		}
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

	if worker.UnpackStatus != wechat.StatusTypeFinished {
		if worker.UnpackStatus != wechat.StatusTypeError {
			worker.SetErrorState("解包未完成")
		}
		worker.UnpackSavePath = job.finalPath
		return worker, fmt.Errorf("%s", worker.UnpackErrorMessage)
	}

	if _, err := removeMatchingBatchOutputs(job.root, job.base+"-"); err != nil {
		err = fmt.Errorf("覆盖旧解包目录失败：%v", err)
		worker.SetErrorState(err.Error())
		worker.UnpackSavePath = job.finalPath
		return worker, err
	}
	if err := os.Rename(stage, job.finalPath); err != nil {
		err = fmt.Errorf("保存解包目录失败：%v", err)
		worker.SetErrorState(err.Error())
		worker.UnpackSavePath = job.finalPath
		return worker, err
	}
	stageExists = false
	worker.UnpackSavePath = job.finalPath
	worker.UnpackStatus = wechat.StatusTypeFinished
	worker.UnpackProgress = 100
	return worker, nil
}

func removeMatchingBatchOutputs(root, prefix string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	removed := make([]string, 0, 1)
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !strings.HasPrefix(name, prefix) || len(name) <= len(prefix) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
			return removed, fmt.Errorf("%s：%v", name, err)
		}
		removed = append(removed, name)
	}
	return removed, nil
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	var b strings.Builder
	used := 0
	for _, r := range value {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = 1
		}
		if used+size > maxBytes {
			break
		}
		b.WriteRune(r)
		used += size
	}
	return b.String()
}
