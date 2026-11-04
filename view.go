package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/wux1an/wxapkg/wechat"
)

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		a.header(c)
		ui.Column(c).Grow(1).Padding(16, 20).Gap(12).Children(func() {
			a.summary(c)
			a.tableArea(c)
		})
		ui.Row(c).Height(36).Padding(0, 18).
			BorderWidth(1, 0, 0, 0).BorderColor(t.Border).
			AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, "仅供学习研究使用，请勿用于任何侵权或非法用途").
				FontSize(11).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Link(c, appVersion+"  ↗", githubURL).FontSize(11)
		})
	})

	a.scanDialog(c)
	a.unpackDialog(c)
	a.settingsDialog(c)
	a.flushNotice(c)
}

func (a *app) header(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(74).Padding(0, 20).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).
		AlignItems(ui.Center).Gap(14).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "微信小程序解包工具").FontSize(19).Bold()
				ui.Badge(c, "NATIVE").FontSize(9).TextColor(t.Accent)
			})
			ui.Text(c, "wxapkg · egoist/mygo · 无 WebView").FontSize(11).TextColor(t.TextMuted)
		})
		ui.Spacer(c)
		ui.SearchField(c, &a.search).
			Bind(&a.searchHandle).
			Width(270).
			Placeholder("搜索名称、小程序 ID 或路径").
			Label("搜索小程序")
		if ui.Button(c, "批量解包").
			Tooltip("解包列表中的所有小程序，并按名称、ID 和修改时间保存到默认目录").
			Disabled(len(a.items) == 0 || a.scanning || a.clearing || a.hasRunningUnpack()).
			Clicked() {
			a.startBatchUnpack()
		}
		if ui.Button(c, "设置").Clicked() {
			a.openSettings()
		}
		if ui.Button(c, "清空列表").
			Tooltip("只清空当前列表，不删除微信缓存").
			Disabled(len(a.items) == 0 || a.scanning || a.clearing || a.batchRunning).
			Clicked() {
			a.clearItems()
		}
		clearPackagesLabel := "清空小程序目录"
		if a.clearing {
			clearPackagesLabel = "正在清空…"
		}
		if ui.Button(c, clearPackagesLabel).
			Tooltip("一键删除微信 packages 目录下所有 wx 小程序下载目录").
			Disabled(a.scanning || a.clearing || a.hasRunningUnpack() || !a.hasCleanupTargets()).
			Clicked() {
			a.clearWeChatPackages()
		}
		label := "扫描小程序"
		if a.scanning {
			label = "正在扫描…"
		}
		if ui.PrimaryButton(c, label).Disabled(a.scanning || a.clearing || a.batchRunning).Clicked() {
			a.openScan()
		}
		a.searchHandle.OnShortcut(c, ui.Cmd, ui.KeyF, func() {
			a.searchHandle.Focus(c)
		})
	})
}

func (a *app) summary(c *ui.Context) {
	t := c.Theme()
	running, finished, failed := 0, 0, 0
	named := 0
	for _, item := range a.items {
		if item.Name != "" {
			named++
		}
		switch item.UnpackStatus {
		case wechat.StatusTypeRunning:
			running++
		case wechat.StatusTypeFinished:
			finished++
		case wechat.StatusTypeError:
			failed++
		}
	}

	ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Badge(c, fmt.Sprintf("%d 个小程序", len(a.items)))
		if named > 0 {
			ui.Textf(c, "已识别名称 %d", named).FontSize(12).TextColor(t.TextMuted)
		}
		ui.Textf(c, "已完成 %d", finished).FontSize(12).TextColor(t.TextMuted)
		if running > 0 {
			ui.Spinner(c).Width(14)
			ui.Textf(c, "进行中 %d", running).FontSize(12).TextColor(t.TextMuted)
		}
		if failed > 0 {
			ui.Textf(c, "失败 %d", failed).FontSize(12).TextColor(t.Danger)
		}
		ui.Spacer(c)
		if a.batchRunning {
			ui.Spinner(c).Width(14)
			ui.Textf(c, "批量解包 %d / %d", a.batchIndex, a.batchTotal).
				FontSize(12).TextColor(t.TextMuted)
		} else if a.clearing {
			ui.Spinner(c).Width(14)
			ui.Text(c, "正在清空微信小程序目录…").FontSize(12).TextColor(t.Danger)
		} else if a.detected.NeedsFullDiskAccess {
			ui.Text(c, "需要完全磁盘访问权限").FontSize(12).TextColor(t.Danger)
		} else if a.scanning {
			ui.Spinner(c).Width(14)
			ui.Text(c, "正在检测微信目录并扫描小程序…").FontSize(12).TextColor(t.TextMuted)
		} else if len(a.detected.Paths) > 0 {
			ui.Textf(c, "已识别 %d 个微信目录", len(a.detected.Paths)).
				FontSize(12).TextColor(t.TextMuted)
		}
	})
}

func (a *app) tableArea(c *ui.Context) {
	items := a.visibleItems()
	if len(items) == 0 {
		if a.scanning {
			a.scanningState(c)
		} else if a.search != "" && len(a.items) > 0 {
			a.noResultsState(c)
		} else {
			a.emptyState(c)
		}
		return
	}

	columns := []ui.TableColumn{
		{Title: "小程序名称", ID: "name", Width: 180, MinWidth: 130, Sortable: true},
		{Title: "小程序 ID", ID: "id", Width: 168, MinWidth: 150, Sortable: true},
		{Title: "修改时间", ID: "modified", Width: 136, MinWidth: 124, Sortable: true},
		{Title: "大小", ID: "size", Width: 78, MinWidth: 68, Align: ui.End, Sortable: true},
		{Title: "路径", ID: "path", MinWidth: 180, Sortable: true},
		{Title: "操作", ID: "status", Width: 180, MinWidth: 164, Sortable: true},
	}

	a.table.Sort = &a.sort
	a.table.Key = func(row int) any { return items[row].UUID }
	a.table.Label = func(row int) string {
		return firstNonEmpty(items[row].Name, items[row].WxId)
	}
	a.table.CellMenu = func(m *ui.Menu, row, _ int) {
		if row < 0 || row >= len(items) {
			return
		}
		uuid := items[row].UUID
		if m.Item("复制小程序 ID").Chosen() {
			if item := a.itemByUUID(uuid); item != nil {
				a.copyPath(item.WxId)
			}
		}
		if m.Item("复制路径").Chosen() {
			if item := a.itemByUUID(uuid); item != nil {
				a.copyPath(item.Location)
			}
		}
		if m.Item("打开上级目录").Chosen() {
			if item := a.itemByUUID(uuid); item != nil {
				a.openParentPath(item.Location)
			}
		}
		if m.Item("解包 / 重试").Chosen() {
			a.openUnpack(uuid)
		}
	}

	ui.Table(c, &a.table, columns, len(items), func(row, col int) {
		item := items[row]
		switch col {
		case 0:
			if item.Name != "" {
				ui.Text(c, item.Name).FontSize(12).SingleLine().
					Tooltip("小程序名称：" + item.Name)
			} else {
				ui.Text(c, "未识别").FontSize(12).SingleLine().
					TextColor(c.Theme().TextMuted).
					Tooltip("包内没有可直接使用的名称；小程序 ID 仍可正常解包")
			}
		case 1:
			ui.Text(c, item.WxId).FontSize(12).SingleLine()
		case 2:
			ui.Text(c, formatTime(item.LastModifyTime)).FontSize(12).SingleLine()
		case 3:
			ui.Text(c, formatSize(item.Size)).FontSize(12).SingleLine().TextAlign(ui.End)
		case 4:
			ui.Text(c, item.Location).FontSize(11).SingleLine().
				TextColor(c.Theme().TextMuted).
				Cursor(ui.CursorPointer).
				Tooltip("点击复制路径\n" + item.Location).
				OnClick(func() { a.copyPath(item.Location) })
		case 5:
			a.statusCell(c, item)
		}
	}).Grow(1).Label("微信小程序列表")
}

func (a *app) statusCell(c *ui.Context, item wechat.WxapkgItem) {
	t := c.Theme()
	uuid := item.UUID
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		switch item.UnpackStatus {
		case wechat.StatusTypeRunning:
			if ui.Button(c, "上级").Padding(2, 7).
				Tooltip("打开所在文件夹").
				Clicked() {
				a.openParentPath(item.Location)
			}
			value := math.Max(0, math.Min(1, item.UnpackProgress/100))
			ui.Progress(c, value).Width(54)
			ui.Textf(c, "%.0f%%", item.UnpackProgress).FontSize(11).TextColor(t.TextMuted)
		case wechat.StatusTypeFinished:
			if ui.Button(c, "上级").Padding(2, 7).
				Tooltip("打开所在文件夹").
				Clicked() {
				a.openParentPath(item.Location)
			}
			if ui.Button(c, "打开").Padding(2, 7).Clicked() {
				a.openPath(item.UnpackSavePath)
			}
		case wechat.StatusTypeError:
			if ui.Button(c, "上级").Padding(2, 7).
				Tooltip("打开所在文件夹").
				Clicked() {
				a.openParentPath(item.Location)
			}
			if ui.Button(c, "重试").Padding(2, 7).
				Tooltip(firstNonEmpty(item.UnpackErrorMessage, "解包失败")).
				Clicked() {
				a.openUnpack(uuid)
			}
		default:
			if ui.Button(c, "上级").Padding(2, 7).
				Tooltip("打开所在文件夹").
				Clicked() {
				a.openParentPath(item.Location)
			}
			if ui.Button(c, "解包").Padding(2, 9).Clicked() {
				a.openUnpack(uuid)
			}
		}
	})
}

func (a *app) emptyState(c *ui.Context) {
	t := c.Theme()
	if a.detected.NeedsFullDiskAccess {
		ui.Column(c).Fill().Center().Gap(12).Children(func() {
			ui.Text(c, "macOS 需要完全磁盘访问权限").FontSize(18).Bold()
			ui.Text(c, "系统已阻止读取微信沙盒目录。授权本应用后，才能自动识别并解包微信小程序。").
				FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center)
			ui.Text(c, fullDiskAccessHint).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center)
			ui.Text(c, a.detected.PermissionBlockedPath).FontSize(11).
				TextColor(t.TextMuted).Selectable()
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "清除旧授权").
					Disabled(a.resettingPermission).Clicked() {
					a.resetFullDiskAccess()
				}
				if ui.Button(c, "在 Finder 中显示应用").Clicked() {
					a.revealCurrentApp()
				}
			})
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "打开完全磁盘访问设置").Clicked() {
					a.openFullDiskAccessSettings()
				}
				if ui.Button(c, "手动添加").Clicked() {
					a.openScan()
					a.scanTab = 1
				}
			})
		})
		return
	}
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, "还没有扫描到小程序").FontSize(18).Bold()
		ui.Text(c, "打开任意微信小程序后重新检测，或手动选择 .wxapkg 文件与目录。").
			FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "手动添加").Clicked() {
				a.openScan()
			}
			if ui.PrimaryButton(c, "扫描微信目录").Clicked() {
				a.rescan()
			}
		})
	})
}

func (a *app) noResultsState(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(10).Children(func() {
		ui.Text(c, "没有匹配的小程序").FontSize(17).Bold()
		ui.Textf(c, "没有找到与“%s”相关的名称、ID 或路径", a.search).
			FontSize(12).TextColor(t.TextMuted)
		if ui.Button(c, "清空搜索").Clicked() {
			a.search = ""
		}
	})
}

func (a *app) scanningState(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Spinner(c).Width(22)
		ui.Text(c, "正在扫描微信小程序…").FontSize(14)
		ui.Text(c, "首次扫描会统计每个小程序的体积，可能需要几秒钟。").
			FontSize(12).TextColor(t.TextMuted)
	})
}
