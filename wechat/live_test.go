package wechat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveWeChatUnpack validates the current machine's real WeChat layout
// when explicitly enabled. It never writes into WeChat's cache directory.
func TestLiveWeChatUnpack(t *testing.T) {
	if os.Getenv("WXAPKG_LIVE_TEST") == "" {
		t.Skip("set WXAPKG_LIVE_TEST=1 to scan and unpack a real local package")
	}

	result := Platform.GetDefaultPaths()
	if len(result.Paths) == 0 {
		t.Fatalf("no WeChat package directories found\n%s", result.Logs)
	}

	var items []WxapkgItem
	for _, path := range result.Paths {
		found, err := ScanWxapkgItem(path, true)
		if err != nil {
			t.Fatalf("scan %s: %v", path, err)
		}
		items = append(items, found...)
	}
	if len(items) == 0 {
		t.Fatal("WeChat directories were found, but they contain no wxapkg packages")
	}

	smallest := items[0]
	for _, item := range items[1:] {
		if item.Size > 0 && (smallest.Size == 0 || item.Size < smallest.Size) {
			smallest = item
		}
	}

	output := filepath.Join(t.TempDir(), smallest.WxId+"_unpacked")
	item := smallest
	options := UnpackOptions{
		EnableDecrypt:      true,
		EnableJsonBeautify: true,
		OutputDir:          t.TempDir(),
		SavePath:           output,
	}
	var final WxapkgItem
	NewUnpacker(&item, &options).UnpackWithStatusCallback(func(current *WxapkgItem) {
		final = *current
	})
	if final.UnpackStatus != StatusTypeFinished {
		t.Fatalf("unpack %s failed: %s", smallest.Location, final.UnpackErrorMessage)
	}
	if final.UnpackTotal == 0 {
		t.Fatal("unpack reported no files")
	}
	if info, err := os.Stat(output); err != nil || !info.IsDir() {
		t.Fatalf("output directory %s: %v", output, err)
	}
	t.Logf("scanned %d items from %d roots; unpacked %d files from %s",
		len(items), len(result.Paths), final.UnpackTotal, smallest.WxId)
}

// TestLiveWeChatNames reports how the local packages are named. It is opt-in
// because package IDs and locally used mini programs are user-specific.
func TestLiveWeChatNames(t *testing.T) {
	if os.Getenv("WXAPKG_LIVE_TEST") == "" {
		t.Skip("set WXAPKG_LIVE_TEST=1 to inspect names of real local packages")
	}

	result := Platform.GetDefaultPaths()
	for _, path := range result.Paths {
		items, err := ScanWxapkgItem(path, true)
		if err != nil {
			t.Fatalf("scan %s: %v", path, err)
		}
		for _, item := range items {
			if item.Name == "" && item.NameProvider != "" {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				name, err := LookupProviderName(ctx, item.NameProvider, item.NameProviderID)
				cancel()
				if err == nil {
					item.Name = name
				} else {
					t.Logf("%s name lookup: %v", item.WxId, err)
				}
			}
			t.Logf("%s name=%q provider=%s providerID=%s",
				item.WxId, item.Name, item.NameProvider, item.NameProviderID)
		}
	}
}
