package main

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/wux1an/wxapkg/wechat"
)

func (a *app) scanDialog(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.scanOpen, func() {
		ui.Column(c).Width(660).MaxHeight(580).Gap(14).Children(func() {
			ui.Text(c, "扫描微信小程序").FontSize(20).Bold()
			ui.Text(c, "自动识别最新微信 4.x 用户目录，也兼容 4.0 与 3.x 的旧目录。").
				FontSize(12).TextColor(t.TextMuted)
			ui.Tabs(c, &a.scanTab, "自动扫描", "手动添加")

			if a.scanTab == 0 {
				a.autoScanPanel(c)
			} else {
				a.manualScanPanel(c)
			}

			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "重新检测").Disabled(a.scanning).Clicked() {
					a.rescan()
				}
				if ui.Button(c, "关闭").Clicked() {
					a.scanOpen = false
				}
			})
		})
	})
}

func (a *app) autoScanPanel(c *ui.Context) {
	t := c.Theme()
	if a.scanning {
		ui.Row(c).Padding(24, 0).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Width(18)
			ui.Text(c, "正在检测微信目录…").FontSize(13).TextColor(t.TextMuted)
		})
		return
	}

	if a.detected.NeedsFullDiskAccess {
		ui.Column(c).Padding(16, 18).Gap(9).Radius(10).
			Background(t.Surface).Children(func() {
			ui.Text(c, "需要完全磁盘访问权限").FontSize(15).Bold().TextColor(t.Danger)
			ui.Text(c, "macOS 会保护其他应用的沙盒数据。请在“系统设置 → 隐私与安全性 → 完全磁盘访问权限”中启用本应用，然后重新检测。").
				FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, fullDiskAccessHint).FontSize(12).TextColor(t.TextMuted)
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
				if ui.Button(c, "打开系统设置").Clicked() {
					a.openFullDiskAccessSettings()
				}
				if ui.Button(c, "手动选择目录").Clicked() {
					a.scanTab = 1
				}
			})
		})
	} else if len(a.detected.Paths) == 0 {
		ui.Column(c).Padding(22).Gap(8).Radius(10).
			Background(t.Surface).Children(func() {
			ui.Text(c, "未找到微信小程序目录").FontSize(15).Bold()
			ui.Text(c, "请先在微信里打开任意一个小程序，让缓存目录出现；然后点“重新检测”。").
				FontSize(12).TextColor(t.TextMuted)
		})
	} else {
		ui.Scroll(c).MaxHeight(260).Children(func() {
			ui.Column(c).Gap(8).Children(func() {
				for _, path := range a.detected.Paths {
					path := path
					ui.Row(c).Padding(10, 12).Radius(9).
						Background(t.Surface).Gap(12).AlignItems(ui.Center).
						Children(func() {
							ui.Column(c).Grow(1).Gap(3).Children(func() {
								ui.Text(c, describeWeChatPath(path)).FontSize(12).Bold()
								ui.Text(c, path).FontSize(11).TextColor(t.TextMuted).SingleLine()
							})
							if ui.PrimaryButton(c, "扫描").Padding(2, 12).Clicked() {
								a.scanPath(path, true)
							}
						})
				}
			})
		})
	}

	if a.detected.Logs != "" {
		ui.Checkbox(c, &a.showDetectLogs, "显示目录检测日志")
		if a.showDetectLogs {
			ui.Scroll(c).MaxHeight(180).Padding(10).Radius(8).
				Background(t.Surface).Children(func() {
				ui.Text(c, a.detected.Logs).FontSize(11).
					TextColor(t.TextMuted).Selectable()
			})
		}
	}
}

func (a *app) manualScanPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(10).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c, &a.manualPath).
				Placeholder("粘贴微信目录、小程序目录或 .wxapkg 文件路径").
				Label("路径").
				Grow(1)
			if ui.PrimaryButton(c, "扫描路径").
				Disabled(a.scanning || strings.TrimSpace(a.manualPath) == "").
				Clicked() {
				a.scanPath(a.manualPath, true)
			}
		})
		ui.Row(c).Padding(12).Radius(9).Background(t.Surface).Gap(12).
			AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "小程序安装目录").FontSize(13).Bold()
				ui.Text(c, "自动识别目录下的小程序，并用小程序 ID 作为解密密钥。").
					FontSize(11).TextColor(t.TextMuted)
			})
			if ui.Button(c, "选择目录").Clicked() {
				a.browseScanDirectory("选择微信小程序安装目录", true)
			}
		})
		ui.Row(c).Padding(12).Radius(9).Background(t.Surface).Gap(12).
			AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "单个 .wxapkg 文件").FontSize(13).Bold()
				ui.Text(c, "选择加密包时，需要在解包配置中手动填写小程序 ID。").
					FontSize(11).TextColor(t.TextMuted)
			})
			if ui.Button(c, "选择文件").Clicked() {
				a.browseScanFile()
			}
		})
		ui.Row(c).Padding(12).Radius(9).Background(t.Surface).Gap(12).
			AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "单个小程序目录").FontSize(13).Bold()
				ui.Text(c, "递归查找目录中的 .wxapkg 文件。").
					FontSize(11).TextColor(t.TextMuted)
			})
			if ui.Button(c, "选择目录").Clicked() {
				a.browseScanDirectory("选择单个小程序目录", false)
			}
		})
	})
}

func (a *app) unpackDialog(c *ui.Context) {
	item := a.itemByUUID(a.unpackUUID)
	if item == nil {
		a.unpackOpen = false
		return
	}
	if !a.unpackOpen {
		return
	}

	switch item.UnpackStatus {
	case wechat.StatusTypeRunning:
		a.unpackProgressDialog(c, *item)
	case wechat.StatusTypeFinished:
		a.unpackFinishedDialog(c, *item)
	default:
		a.unpackConfigDialog(c, *item)
	}
}

func (a *app) unpackConfigDialog(c *ui.Context, item wechat.WxapkgItem) {
	t := c.Theme()
	ui.Modal(c, &a.unpackOpen, func() {
		ui.Column(c).Width(540).Gap(14).Children(func() {
			ui.Text(c, "解包配置").FontSize(20).Bold()
			ui.Text(c, item.WxId).FontSize(12).TextColor(t.TextMuted).Selectable()

			if item.UnpackStatus == wechat.StatusTypeError {
				ui.Column(c).Padding(10, 12).Radius(8).
					Background(t.Surface).Children(func() {
					ui.Text(c, "上次解包失败").FontSize(12).Bold().TextColor(t.Danger)
					ui.Text(c, item.UnpackErrorMessage).FontSize(11).
						TextColor(t.TextMuted).Selectable()
				})
			}

			ui.Checkbox(c, &a.unpackDecrypt, "启用解密")
			if a.unpackDecrypt {
				ui.TextInput(c, &a.unpackKey).
					Placeholder("小程序 ID，如 wxabcdef1234567890").
					Label("小程序 ID")
				ui.Text(c, "密钥就是小程序 ID：wx 开头，后接 16 位字符。").
					FontSize(11).TextColor(t.TextMuted)
			}

			ui.Divider(c)
			ui.Text(c, "代码美化").FontSize(12).FontWeight(600)
			ui.Row(c).Gap(18).Children(func() {
				ui.Checkbox(c, &a.unpackJSON, "JSON")
				ui.Checkbox(c, &a.unpackHTML, "HTML")
				ui.Checkbox(c, &a.unpackJS, "JavaScript")
			})

			ui.Divider(c)
			ui.Text(c, "输出目录").FontSize(12).FontWeight(600)
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &a.unpackOutput).
					Placeholder("选择输出目录").
					Label("输出目录").
					Grow(1)
				if ui.Button(c, "浏览…").Clicked() {
					a.browseOutputDirectory(&a.unpackOutput, "选择输出目录", false)
				}
			})
			if a.unpackOutput != "" {
				ui.Text(c, "保存到："+computeSavePath(a.unpackOutput, item.Location, item.Name)).
					FontSize(11).TextColor(t.TextMuted).Selectable()
			}
			ui.Checkbox(c, &a.unpackSetDefault, "将当前目录设为默认输出目录")

			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.unpackOpen = false
				}
				canStart := a.unpackOutput != "" && (!a.unpackDecrypt || a.unpackKey != "")
				if ui.PrimaryButton(c, "开始解包").Disabled(!canStart).Clicked() {
					a.startUnpack()
				}
			})
		})
	})
}

func (a *app) unpackProgressDialog(c *ui.Context, item wechat.WxapkgItem) {
	t := c.Theme()
	ui.Modal(c, &a.unpackOpen, func() {
		ui.Column(c).Width(540).Gap(14).Children(func() {
			ui.Text(c, "正在解包").FontSize(20).Bold()
			ui.Text(c, item.WxId).FontSize(12).TextColor(t.TextMuted).Selectable()
			value := math.Max(0, math.Min(1, item.UnpackProgress/100))
			ui.Progress(c, value).Height(8)
			ui.Row(c).Children(func() {
				ui.Textf(c, "%.0f%%", item.UnpackProgress).FontSize(12)
				ui.Spacer(c)
				ui.Textf(c, "%d / %d 个文件", item.UnpackCurrent, item.UnpackTotal).
					FontSize(11).TextColor(t.TextMuted)
			})
			if item.UnpackCurrentFile != "" {
				ui.Column(c).Padding(10, 12).Radius(8).
					Background(t.Surface).Gap(3).Children(func() {
					ui.Text(c, "当前文件").FontSize(10).TextColor(t.TextMuted)
					ui.Text(c, item.UnpackCurrentFile).FontSize(11).SingleLine().Selectable()
				})
			}
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.Button(c, "后台运行").Clicked() {
					a.unpackOpen = false
				}
			})
		})
	})
}

func (a *app) unpackFinishedDialog(c *ui.Context, item wechat.WxapkgItem) {
	t := c.Theme()
	ui.Modal(c, &a.unpackOpen, func() {
		ui.Column(c).Width(540).Gap(14).Children(func() {
			ui.Text(c, "解包完成").FontSize(20).Bold().TextColor(t.Success)
			ui.Textf(c, "已还原 %d 个文件", item.UnpackTotal).FontSize(12).TextColor(t.TextMuted)
			ui.Column(c).Padding(10, 12).Radius(8).
				Background(t.Surface).Gap(3).Children(func() {
				ui.Text(c, "输出目录").FontSize(10).TextColor(t.TextMuted)
				ui.Text(c, item.UnpackSavePath).FontSize(11).Selectable()
			})
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "关闭").Clicked() {
					a.unpackOpen = false
				}
				if ui.PrimaryButton(c, "打开目录").Clicked() {
					a.openPath(item.UnpackSavePath)
				}
			})
		})
	})
}

func (a *app) settingsDialog(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.settingsOpen, func() {
		ui.Column(c).Width(540).Gap(14).Children(func() {
			ui.Text(c, "设置").FontSize(20).Bold()
			ui.Text(c, "默认输出目录").FontSize(12).FontWeight(600)
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &a.settingsDraft).
					Placeholder("留空则使用上一次输出目录").
					Label("默认输出目录").
					Grow(1)
				if ui.Button(c, "浏览…").Clicked() {
					a.browseOutputDirectory(&a.settingsDraft, "选择默认输出目录", true)
				}
			})
			ui.Text(c, "解包时会优先填入这里；留空时沿用上一次的输出目录。").
				FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, "默认目录："+defaultUnpackOutputDir()).
				FontSize(11).TextColor(t.TextMuted).Selectable()
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "清空默认目录").Clicked() {
					a.settingsDraft = ""
					a.saveSettings()
				}
				if ui.Button(c, "取消").Clicked() {
					a.settingsOpen = false
				}
				if ui.PrimaryButton(c, "保存").Clicked() {
					a.saveSettings()
				}
			})
		})
	})
}

func describeWeChatPath(path string) string {
	switch {
	case pathHasSegment(path, "packages") && pathHasSegment(path, "users"):
		return "微信 4.1.7+ 当前用户目录"
	case containsPath(path, "radium") && containsPath(path, "Applet"):
		return "微信 4.0 目录"
	case containsPath(path, ".wxapplet"):
		return "微信 3.x 目录"
	case containsPath(path, "WeChat Files"), containsPath(path, "WeChatFiles"):
		return "微信 3.x 旧版目录"
	default:
		return "微信小程序目录"
	}
}

func containsPath(path, part string) bool {
	return pathHasSegment(path, part)
}

func pathHasSegment(path, part string) bool {
	for _, segment := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if strings.EqualFold(segment, part) {
			return true
		}
	}
	return false
}
