package wechat

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"testing"
)

func TestScanResolvesLocalAppNameAndProvider(t *testing.T) {
	root := t.TempDir()
	wxid := "wx50d13a67c1b59969"
	packageDir := filepath.Join(root, "users", "hash", "applet", "packages", wxid, "45")
	writeTestFile(t, filepath.Join(packageDir, "__APP__.wxapkg"), makeTestWxapkg(map[string]string{
		"app-config.json": `{
			"ext": {"appId": "` + wxid + `", "kdtId": 146833520},
			"global": {"window": {"navigationBarTitleText": " "}}
		}`,
	}))

	items, err := ScanWxapkgItem(filepath.Dir(filepath.Dir(packageDir)), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Name != "" {
		t.Fatalf("generic navigation title was used as name: %q", items[0].Name)
	}
	if items[0].NameProvider != NameProviderWeChatProfile ||
		items[0].NameProviderID != wxid ||
		items[0].FallbackNameProvider != NameProviderYouzan ||
		items[0].FallbackNameProviderID != "146833520" {
		t.Fatalf("provider metadata = %+v", items[0])
	}
}

func TestScanUsesSpecificNavigationTitle(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "packages", "wx1111111111111111", "1")
	writeTestFile(t, filepath.Join(packageDir, "__APP__.wxapkg"), makeTestWxapkg(map[string]string{
		"app-config.json": `{"global":{"window":{"navigationBarTitleText":"手机充值"}}}`,
	}))

	items, err := ScanWxapkgItem(filepath.Join(root, "packages"), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "手机充值" {
		t.Fatalf("items = %+v, want name 手机充值", items)
	}
}

func TestUpdateMetadataRejectsGenericNames(t *testing.T) {
	var meta localAppMetadata
	updateMetadataFromJSON(&meta, "app-config.json", []byte(
		`{"ext":{"shopName":"首页"},"global":{"window":{"navigationBarTitleText":"商品详情"}}}`,
	))
	if meta.Name != "" {
		t.Fatalf("generic app name accepted: %q", meta.Name)
	}

	meta = localAppMetadata{}
	updateMetadataFromJSON(&meta, "app-config.json", []byte(
		`{"ext":{"appName":"WeChat"},"global":{"window":{"navigationBarTitleText":"微信"}}}`,
	))
	if meta.Name != "" {
		t.Fatalf("WeChat framework label accepted: %q", meta.Name)
	}
}

func TestUsefulAppNameRejectsWxID(t *testing.T) {
	if usefulAppName("wx50d13a67c1b59969", "") {
		t.Fatal("wxid was accepted as a display name")
	}
}

func TestLookupYouzanName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("kdt_id") != "146833520" {
			t.Errorf("kdt_id = %q", r.URL.Query().Get("kdt_id"))
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"shopName":"德罗心DELORSIM"}}`))
	}))
	defer server.Close()

	name, err := lookupYouzanName(context.Background(), server.Client(), server.URL, "146833520")
	if err != nil {
		t.Fatal(err)
	}
	if name != "德罗心DELORSIM" {
		t.Fatalf("name = %q", name)
	}
}

func TestLookupWeChatProfileName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("appid") != "wxa75efa648b60994b" {
			t.Errorf("appid = %q", r.URL.Query().Get("appid"))
		}
		if r.URL.Query().Get("action") != "intro" {
			t.Errorf("action = %q", r.URL.Query().Get("action"))
		}
		_, _ = w.Write([]byte(
			`<html><head><title>腾讯视频 | 小程序插件 | 微信公众平台</title></head></html>`,
		))
	}))
	defer server.Close()

	name, err := lookupWeChatProfileName(
		context.Background(),
		server.Client(),
		server.URL,
		"wxa75efa648b60994b",
	)
	if err != nil {
		t.Fatal(err)
	}
	if name != "腾讯视频" {
		t.Fatalf("name = %q", name)
	}
}

func TestParseWeChatProfileName(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "Chinese title",
			body: `<title>中国移动余量查询 | 小程序插件 | 微信公众平台</title>`,
			want: "中国移动余量查询",
		},
		{
			name: "HTML entities",
			body: `<TITLE>A&amp;B 工具 - 微信公众平台</TITLE>`,
			want: "A&B 工具",
		},
		{
			name: "missing profile",
			body: `<title>小程序</title>`,
			want: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseWeChatProfileName([]byte(test.body))
			if test.want == "" {
				if err == nil {
					t.Fatalf("parseWeChatProfileName = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("parseWeChatProfileName = %q, want %q", got, test.want)
			}
		})
	}
}

func makeTestWxapkg(files map[string]string) []byte {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var index bytes.Buffer
	_ = binary.Write(&index, binary.BigEndian, uint32(len(names)))
	var body bytes.Buffer
	for _, name := range names {
		content := []byte(files[name])
		_ = binary.Write(&index, binary.BigEndian, uint32(len(name)))
		index.WriteString(name)
		dataOffset := 14 + index.Len() + 4 + 4 + body.Len()
		_ = binary.Write(&index, binary.BigEndian, uint32(dataOffset))
		_ = binary.Write(&index, binary.BigEndian, uint32(len(content)))
		body.Write(content)
	}

	var packageData bytes.Buffer
	packageData.WriteByte(0xBE)
	_ = binary.Write(&packageData, binary.BigEndian, uint32(0))
	_ = binary.Write(&packageData, binary.BigEndian, uint32(index.Len()))
	_ = binary.Write(&packageData, binary.BigEndian, uint32(body.Len()))
	packageData.WriteByte(0xED)
	packageData.Write(index.Bytes())
	packageData.Write(body.Bytes())
	return packageData.Bytes()
}
