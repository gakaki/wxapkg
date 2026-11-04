package wechat

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	NameProviderWeChatProfile = "wechat-profile"
	NameProviderYouzan        = "youzan"
)

var reHTMLTitle = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

type localAppMetadata struct {
	Name       string
	Provider   string
	ProviderID string
}

var ignoredAppNames = map[string]struct{}{
	"app":                 {},
	"home":                {},
	"index":               {},
	"loading":             {},
	"miniprogram":         {},
	"undefined":           {},
	"wechat":              {},
	"wechat mini program": {},
	"个人中心":                {},
	"会员中心":                {},
	"商品详情":                {},
	"店铺首页":                {},
	"微信小程序":               {},
	"微信支付":                {},
	"微信":                  {},
	"微信公众平台":              {},
	"我的":                  {},
	"搜索":                  {},
	"搜索商品":                {},
	"加载中":                 {},
	"半屏liteapp测试页":        {},
	"小程序":                 {},
	"首页":                  {},
}

// populateLocalAppNames enriches scanned items from package metadata without
// extracting the package to disk. Package metadata is the primary source; an
// AppID or merchant ID is retained for a later, optional public name lookup.
func populateLocalAppNames(items []WxapkgItem) {
	workers := 4
	if len(items) < workers {
		workers = len(items)
	}
	if workers == 0 {
		return
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for index := range jobs {
				meta := inspectLocalAppMetadata(items[index].Location, items[index].EncryptKey)
				items[index].Name = meta.Name
				if meta.Name != "" {
					continue
				}
				if reWxId.MatchString(items[index].WxId) {
					items[index].NameProvider = NameProviderWeChatProfile
					items[index].NameProviderID = items[index].WxId
					items[index].FallbackNameProvider = meta.Provider
					items[index].FallbackNameProviderID = meta.ProviderID
					continue
				}
				items[index].NameProvider = meta.Provider
				items[index].NameProviderID = meta.ProviderID
			}
		}()
	}
	for index := range items {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
}

func inspectLocalAppMetadata(location, key string) localAppMetadata {
	var meta localAppMetadata
	for _, packagePath := range configPackageCandidates(location) {
		data, err := readPackageData(packagePath, key)
		if err != nil {
			continue
		}
		files, err := extractWxapkgConfigs(data)
		if err != nil {
			continue
		}
		for _, name := range []string{"app-config.json", "plugin.json", "app.json", "project.config.json"} {
			content, ok := files[name]
			if !ok {
				continue
			}
			updateMetadataFromJSON(&meta, name, content)
		}
		if meta.Name != "" {
			break
		}
	}
	return meta
}

func configPackageCandidates(location string) []string {
	info, err := os.Stat(location)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		return []string{location}
	}

	paths, err := ListFilesWithExtension(location, ".wxapkg")
	if err != nil {
		return nil
	}
	sort.SliceStable(paths, func(i, j int) bool {
		left, right := packageCandidateRank(location, paths[i]), packageCandidateRank(location, paths[j])
		if left != right {
			return left < right
		}
		return paths[i] < paths[j]
	})
	if len(paths) > 6 {
		paths = paths[:6]
	}
	return paths
}

func packageCandidateRank(root, path string) int {
	switch strings.ToLower(filepath.Base(path)) {
	case "__app__.wxapkg":
		return 0
	case "__pluginCode__.wxapkg":
		return 1
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 4
	}
	return 2 + strings.Count(rel, string(filepath.Separator))
}

func readPackageData(path, key string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if isDecryptedWxapkgData(data) {
		return data, nil
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("package is encrypted")
	}
	return decryptWxapkgFile(key, data)
}

func isDecryptedWxapkgData(data []byte) bool {
	return len(data) >= 14 && data[0] == 0xBE && data[13] == 0xED
}

func extractWxapkgConfigs(data []byte) (map[string][]byte, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("wxapkg header is incomplete")
	}
	if !isDecryptedWxapkgData(data) {
		return nil, fmt.Errorf("wxapkg header is invalid")
	}

	reader := bytes.NewReader(data[14:])
	var fileCount uint32
	if err := binary.Read(reader, binary.BigEndian, &fileCount); err != nil {
		return nil, err
	}
	if fileCount > 102400 {
		return nil, fmt.Errorf("wxapkg file count is too large")
	}

	wanted := map[string]struct{}{
		"app-config.json":     {},
		"plugin.json":         {},
		"app.json":            {},
		"project.config.json": {},
	}
	found := make(map[string][]byte)
	for i := uint32(0); i < fileCount; i++ {
		var nameLength uint32
		if err := binary.Read(reader, binary.BigEndian, &nameLength); err != nil {
			return nil, err
		}
		if nameLength > 1024 {
			return nil, fmt.Errorf("wxapkg file name is too long")
		}
		nameBytes := make([]byte, nameLength)
		if _, err := io.ReadFull(reader, nameBytes); err != nil {
			return nil, err
		}
		var offset, size uint32
		if err := binary.Read(reader, binary.BigEndian, &offset); err != nil {
			return nil, err
		}
		if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
			return nil, err
		}
		if size > 10*1024*1024 || uint64(offset)+uint64(size) > uint64(len(data)) {
			return nil, fmt.Errorf("wxapkg entry is outside the package")
		}
		cleanName, err := cleanWxapkgPath(string(nameBytes))
		if err != nil {
			return nil, err
		}
		if _, ok := wanted[cleanName]; !ok {
			continue
		}
		found[cleanName] = data[offset : offset+size]
	}
	return found, nil
}

func updateMetadataFromJSON(meta *localAppMetadata, filename string, data []byte) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root any
	if decoder.Decode(&root) != nil {
		return
	}
	object, ok := root.(map[string]any)
	if !ok {
		return
	}

	if meta.ProviderID == "" {
		if ext, ok := object["ext"].(map[string]any); ok {
			if kdtID := stringValue(lookupFold(ext, "kdtId")); kdtID != "" {
				meta.Provider = NameProviderYouzan
				meta.ProviderID = kdtID
			}
		}
	}
	if meta.Name != "" {
		return
	}

	var candidates []any
	switch filename {
	case "app-config.json":
		if ext, ok := object["ext"].(map[string]any); ok {
			for _, key := range []string{"shopName", "appName", "nickName", "nickname"} {
				candidates = append(candidates, lookupFold(ext, key))
			}
		}
		for _, key := range []string{"appName", "nickName", "nickname", "name"} {
			candidates = append(candidates, lookupFold(object, key))
		}
		if global, ok := object["global"].(map[string]any); ok {
			if window, ok := global["window"].(map[string]any); ok {
				candidates = append(candidates, lookupFold(window, "navigationBarTitleText"))
			}
		}
	case "plugin.json":
		for _, key := range []string{"name", "appName", "nickName", "nickname"} {
			candidates = append(candidates, lookupFold(object, key))
		}
	case "project.config.json":
		candidates = append(candidates, lookupFold(object, "projectname"))
	case "app.json":
		for _, key := range []string{"appName", "nickName", "nickname", "name"} {
			candidates = append(candidates, lookupFold(object, key))
		}
	}
	for _, candidate := range candidates {
		name := strings.TrimSpace(stringValue(candidate))
		if usefulAppName(name, "") {
			meta.Name = name
			return
		}
	}
}

func lookupFold(object map[string]any, key string) any {
	for candidate, value := range object {
		if strings.EqualFold(candidate, key) {
			return value
		}
	}
	return nil
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return ""
	}
}

func usefulAppName(name, wxid string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == wxid || reWxId.MatchString(name) {
		return false
	}
	count := utf8.RuneCountInString(name)
	if count < 2 || count > 60 {
		return false
	}
	lower := strings.ToLower(name)
	if _, ignored := ignoredAppNames[lower]; ignored {
		return false
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.ContainsAny(name, "{}%$<>") {
		return false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

// LookupProviderName resolves a display name from an allow-listed public page.
// It intentionally sends only the public AppID or merchant ID; no WeChat
// cookies, account data, or decryption keys leave the machine.
func LookupProviderName(ctx context.Context, provider, providerID string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case NameProviderWeChatProfile:
		endpoint := "https://mp.weixin.qq.com/wxopen/pluginbasicprofile"
		return lookupWeChatProfileName(ctx, http.DefaultClient, endpoint, providerID)
	case NameProviderYouzan:
		endpoint := "https://h5.youzan.com/wscdeco/shop/get-shop-meta-info-by-kdtId.json"
		return lookupYouzanName(ctx, http.DefaultClient, endpoint, providerID)
	default:
		return "", fmt.Errorf("unsupported app name provider %q", provider)
	}
}

// lookupWeChatProfileName reads the public profile page for a mini program or
// plugin. The request carries only the public AppID; no WeChat session cookie
// or account data is used.
func lookupWeChatProfileName(
	ctx context.Context,
	client *http.Client,
	endpoint, appID string,
) (string, error) {
	appID = strings.TrimSpace(appID)
	if !reWxId.MatchString(appID) {
		return "", fmt.Errorf("invalid mini program AppID")
	}
	query := url.Values{
		"action": []string{"intro"},
		"appid":  []string{appID},
		"token":  []string{""},
		"lang":   []string{"zh_CN"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	request.Header.Set("User-Agent", "wxapkg-native/2.2")

	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("public profile returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return parseWeChatProfileName(body)
}

func parseWeChatProfileName(body []byte) (string, error) {
	match := reHTMLTitle.FindSubmatch(body)
	if len(match) != 2 {
		return "", fmt.Errorf("public profile has no page title")
	}
	title := strings.TrimSpace(html.UnescapeString(string(match[1])))
	if strings.Contains(title, "微信公众平台") {
		if index := strings.IndexAny(title, "|｜"); index >= 0 {
			title = title[:index]
		}
	}
	title = strings.TrimSpace(strings.TrimSuffix(title, "- 微信公众平台"))
	if !usefulAppName(title, "") {
		return "", fmt.Errorf("public profile has no usable mini program name")
	}
	return title, nil
}

func lookupYouzanName(ctx context.Context, client *http.Client, endpoint, providerID string) (string, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return "", fmt.Errorf("empty Youzan merchant ID")
	}
	query := url.Values{"kdt_id": []string{providerID}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "wxapkg-native/2.2")

	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata service returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			ShopName string `json:"shopName"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	name := strings.TrimSpace(payload.Data.ShopName)
	if payload.Code != 0 || !usefulAppName(name, "") {
		return "", fmt.Errorf("metadata service returned no usable shop name")
	}
	return name, nil
}
