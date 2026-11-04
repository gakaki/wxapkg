# wxapkg project rules

## Native-only desktop UI

- This application uses `github.com/egoist/mygo` native UI only.
- Every window must be created with `mygo.WindowOptions.Content` and `ui.View`.
- Do not introduce Wails, Chromium, WebKit, WebView2, GTK WebKit, HTML, CSS,
  JavaScript, a frontend build step, or any other webview-based interface.
- Native UI dependencies must remain pure Go. Do not enable cgo for the app.
- Keep the Linux packaging configuration at `"webview": false`.
- Prefer `ui.Table`, `ui.Modal`, `ui.SearchField`, `ui.Progress`, toasts,
  `mygo.Dialog`, `mygo.Clipboard`, and `mygo.Shell` over custom web-style
  substitutes.

## Current WeChat compatibility

- Support the newest WeChat desktop directory layout first:
  `.../radium/users/<user-hash>/applet/packages`.
- Keep fallbacks for WeChat 3.x, WeChat 4.0, and the direct
  `.../radium/Applet/packages` layout.
- On macOS, account for the sandbox container under
  `~/Library/Containers/com.tencent.xinWeChat/Data`.
- On current macOS, WeChat's container is protected by App Data privacy
  (TCC). Detect permission failures and guide the user to grant Full Disk
  Access before reporting that no packages exist.
- Keep a Bundle ID distinct from the old Wails build. Reusing
  `com.wux1an.wxapkg` makes TCC grants and Launch Services resolve against the
  wrong binary.
- On Windows, account for `%APPDATA%\Tencent\xwechat\radium` and the legacy
  `Documents\WeChat Files\Applet` locations.
- Never rely on one hard-coded directory. Use bounded directory discovery for
  small future layout changes, and log every path that was checked.
- Add regression tests for old and new directory layouts whenever path
  detection or scanning changes.

## Mini-program names

- Resolve names from explicit package metadata first, then a specific global
  navigation title, then the official WeChat public profile page by AppID,
  then an allow-listed provider fallback.
- Never use generic page chrome such as `首页`, `我的`, `商品详情`,
  `微信支付`, or `WeChat` mini-program framework labels as the app name.
- Only send a public AppID to WeChat's public profile page, or a public
  merchant ID (currently Youzan `kdtId`) to an allow-listed provider endpoint.
  Never upload WeChat cookies, account identifiers, decryption keys, local
  paths, or chat data.
- Treat the profile page title as the mini-program/plugin nickname only when
  it has a non-generic name and strip the `微信公众平台` page chrome.
- When no reliable name exists, display `未识别` and keep the real `wxid`.
  Do not fabricate or silently substitute the package framework name.
- Cache successful public lookups locally and keep search/sort aware of names.

## Destructive cleanup safety

- Bulk cleanup may only target direct `wx[0-9a-f]{16}` child directories under
  an absolute `.../applet/packages` or `.../.wxapplet/packages` root.
- Never delete the `packages` root itself, non-`wx` directories, files, symlink
  targets, or a root not derived from a scanned WeChat package item.
- Revalidate the root and every direct-child path immediately before deletion.
- Require a native warning dialog with a non-destructive default and a
  destructive confirm button; never clean silently.
- Keep unit tests proving that ordinary directories, legacy/non-`packages`
  roots, malformed names, and parent roots are rejected.

## Platform targets

- Build macOS as a universal app for current Apple silicon and Intel systems.
- Build Windows for both `amd64` and `arm64`.
- Keep the pure native app working on current macOS and Windows releases; do
  not add a webview fallback for older or newer systems.

## Verification

- Run `go test ./...`, `go vet ./...`, and `go tool mygo vet .`.
- Build at least the host native app with `go tool mygo build`; on macOS,
  use `./scripts/build-macos.sh` so the stable Full Disk Access identity is
  applied to the final `.app` and `.dmg`.
- Release workflows must publish only artifacts produced by the current run,
  with SHA-256 checksums.
