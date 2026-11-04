# wxapkg

> **免责声明**：此工具仅限于学习和研究软件内含的设计思想和原理，用户承担因使用此工具而导致的所有法律和相关责任。作者不承担任何法律责任。

使用 [egoist/mygo](https://github.com/egoist/mygo) 原生 UI 重写的微信小程序（`.wxapkg`）解包工具，支持当前 Windows 与 macOS。界面、表格、弹窗和文件选择器全部由 Go 原生组件绘制，不使用 Wails、HTML、JavaScript 或 WebView。

![原生界面](assets/native-main.png)

## 功能

- 自动扫描当前微信 4.x 用户目录，并兼容微信 4.0 和 3.x 的旧目录
- 支持选择安装目录、单个小程序目录或单个 `.wxapkg` 文件
- 支持粘贴路径、命令行传入路径和系统文件选择器
- 使用小程序 ID 解密新版加密包
- 还原小程序源码目录结构
- JSON、HTML、JavaScript 代码美化
- 原生表格排序、搜索、后台解包、进度与完成提示
- 一键批量解包列表中的全部小程序，按“小程序名称-小程序 ID-修改时间”保存并自动替换同名同 ID 的旧结果
- 从包内配置和微信官方公开资料页识别中文名称，失败时显示“未识别”
- 每行可打开小程序缓存的上级目录（通常为 `packages`）
- 一键清空微信 `packages` 下的小程序下载目录，执行前显示原生危险操作确认
- 默认批量输出到 `桌面/微信小程序解包目录/`，也可以在设置中修改默认输出目录

## 最新系统与微信目录

已在 macOS 27.2、微信 4.2.0 上验证当前实际目录：

```text
~/Library/Containers/com.tencent.xinWeChat/Data/Documents/app_data/radium/users/<用户哈希>/applet/packages
```

同时支持：

| 版本 | macOS | Windows |
| --- | --- | --- |
| 微信 4.1.7+ | `.../radium/users/<hash>/applet/packages` | `%APPDATA%\Tencent\xwechat\radium\users\<hash>\applet\packages` |
| 微信 4.0 | `.../radium/Applet/packages` | `%APPDATA%\Tencent\xwechat\radium\Applet\packages` |
| 微信 3.x | `.../Data/.wxapplet/packages`、`Documents/WeChat Files/Applet` | `Documents\WeChat Files\Applet` |

程序会在有限深度内查找 `applet/packages` 和 `.wxapplet/packages`，并跳过 `web`、`cache`、`xworker`、`ilink` 等无关的大目录。旧版本与未来小范围目录调整不会被单一硬编码路径限制。

## 名称识别与清理

名称按以下顺序识别：

1. 读取包内 `plugin.json`、`app-config.json` 等元数据中的明确应用名称。
2. 使用包内全局导航标题，但过滤“首页”“商品详情”“微信支付”等通用页面标题。
3. 使用小程序 ID 查询微信官方公开资料页，读取小程序或插件昵称。当前实测可识别“德罗心DELORSIM”“腾讯视频”“中国移动余量查询”等名称。
4. 官方资料页没有结果时，对有赞店铺小程序使用包内公开 `kdtId` 查询公开店铺名称。
5. 没有可靠来源时显示“未识别”，不会伪造名称；小程序 ID 仍可正常用于解密和解包。

公开名称查询只向微信官方资料页发送公开 AppID，或在备用流程中发送有赞公开商家 ID；不读取或上传微信 Cookie、账号信息、聊天记录、本机路径或解密密钥。成功结果会缓存在本机，后续扫描优先使用缓存；离线、未公开或资料页没有昵称时会回退到包内名称或“未识别”。

“清空小程序目录”只会删除已识别 `applet/packages`（或 `.wxapplet/packages`）根目录下、名称严格匹配 `^wx[0-9a-f]{16}$` 的直接子目录。它不会删除 `packages` 根目录、普通文件、其他目录、聊天记录或账号数据，并且默认按钮是“取消”。建议先退出微信再清理；下次打开小程序时微信会重新下载。

### macOS 完全磁盘访问权限

macOS 会保护其他应用的沙盒数据。未授权时，系统会阻止程序读取微信容器，即使目录确实存在。首次自动扫描时：

1. 点击程序提示中的“打开系统设置”。
2. 在“系统设置 → 隐私与安全性 → 完全磁盘访问权限”中启用“微信小程序解包工具”。
3. 退出并重新打开程序，然后点击“重新检测”。

![macOS 权限提示](assets/macos-full-disk-access.png)

如果开关已经是打开的，程序仍提示需要权限，说明系统里保留的是旧构建的授权记录。点击程序里的“清除旧授权”，再在系统设置中点击 `+` 添加当前 `微信小程序解包工具.app`，重启程序即可。也可以在终端手动清除：

```bash
tccutil reset SystemPolicyAllFiles com.wux1an.wxapkg.native
```

原生版使用独立 Bundle ID `com.wux1an.wxapkg.native`，不会与旧 Wails 版 `com.wux1an.wxapkg` 混用授权。若你仍安装旧版，建议先退出或移除旧版，避免误把权限授予旧应用。

从网络下载的未签名版本如果被 Gatekeeper 拦截，可在 Finder 中右键应用并选择“打开”。本项目生成的 macOS 包使用固定的代码要求，重新授权一次后，后续未签名构建不会再因为代码哈希变化而丢失权限。

## 安装

从 [Releases](https://github.com/gakaki/wxapkg/releases/latest) 下载：

- macOS：`.dmg`
- Windows：`x64` 或 `ARM64` 安装包
- 每个安装包旁的 `SHA256SUMS-*.txt` 可用于校验

## 自行编译

要求 Go 1.27.3 或兼容版本。项目没有前端依赖，不需要 Node.js、Bun、npm、Wails/CGO，也不需要 WebView SDK。

```bash
go tool mygo dev
./scripts/build-macos.sh
```

`scripts/build-macos.sh` 会生成通用 macOS `.app` 和 `.dmg`，并写入稳定的完全磁盘访问代码要求。Windows 安装包：

```bash
go tool mygo build -platform windows/amd64,windows/arm64
```

产物位于 `build/`：

- `build/darwin-universal/`：通用 macOS `.app` 与 `.dmg`
- `build/windows-amd64/`：Windows x64 安装包
- `build/windows-arm64/`：Windows ARM64 安装包

Windows 安装包使用 NSIS。在 Linux 上交叉构建时先安装 `nsis`；在 macOS 上构建 Windows 安装包时同样需要 `makensis`。

也可以直接从命令行打开路径：

```bash
open -a "微信小程序解包工具" --args "/path/to/packages"
```

## 验证

```bash
go test ./...
go vet ./...
go tool mygo vet .
./scripts/build-macos.sh
```

本机真实微信包测试（不会写入微信缓存目录）：

```bash
WXAPKG_LIVE_TEST=1 go test ./wechat -run TestLiveWeChatUnpack -v
```

## 技术说明

- 窗口使用 `mygo.WindowOptions.Content` 和 `ui.View` 构建。
- 所有交互均由 `github.com/egoist/mygo/ui` 原生组件完成。
- 全局禁用 WebView、Chromium、WebKit、WebView2、GTK WebKit 和前端构建步骤。
- 项目规则见 [AGENTS.md](AGENTS.md)。

## 来源与灵感

- 本项目的产品方向、微信小程序扫描与解包思路来自 [wux1an/wxapkg](https://github.com/wux1an/wxapkg)。感谢原作者和贡献者提供的基础与灵感。
- 当前版本在此基础上完全使用 [egoist/mygo](https://github.com/egoist/mygo) 的 MyGo Native UI 重写：窗口、表格、弹窗、文件选择器和全部交互均由 Go 原生组件实现，不使用 Wails、WebView、HTML、CSS 或 JavaScript 前端。
- 微信小程序解密、解包格式和原理还参考了下方“参考”中列出的公开项目与文章。

## 参考

- 微信小程序插件公开资料页：<https://mp.weixin.qq.com/wxopen/pluginbasicprofile>
- 小程序解密：<https://github.com/BlackTrace/pc_wxapkg_decrypt>
- 小程序解包：<https://gist.github.com/Integ/bcac5c21de5ea35b63b3db2c725f07ad>
- 原理介绍：<https://misakikata.github.io/2021/03/%E5%BE%AE%E4%BF%A1%E5%B0%8F%E7%A8%8B%E5%BA%8F%E8%A7%A3%E5%8C%85/>
