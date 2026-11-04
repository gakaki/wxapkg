package main

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/wux1an/wxapkg/wechat"
)

func TestVisibleItemsFilterAndSort(t *testing.T) {
	a := &app{
		items: []wechat.WxapkgItem{
			{UUID: "1", WxId: "wxbbbbbbbbbbbbbbbb", Name: "咖啡店", Location: "/a", Size: 20, LastModifyTime: 10},
			{UUID: "2", WxId: "wxaaaaaaaaaaaaaaaa", Name: "书店", Location: "/b", Size: 10, LastModifyTime: 30},
			{UUID: "3", WxId: "wxcccccccccccccccc", Name: "面包房", Location: "/c", Size: 30, LastModifyTime: 20},
		},
		search: "咖啡",
		sort:   uiSort("modified", true),
	}
	got := a.visibleItems()
	if len(got) != 1 || got[0].UUID != "1" {
		t.Fatalf("filtered items = %+v, want UUID 1", got)
	}

	a.search = ""
	a.sort = uiSort("size", false)
	got = a.visibleItems()
	if len(got) != 3 || got[0].UUID != "2" || got[2].UUID != "3" {
		t.Fatalf("sorted items = %+v, want 2,1,3 by size", got)
	}

	a.sort = uiSort("name", false)
	got = a.visibleItems()
	if len(got) != 3 || got[0].UUID != "2" || got[2].UUID != "3" {
		t.Fatalf("sorted items = %+v, want 2,1,3 by name", got)
	}
}

func TestMergeItemsPreservesUnpackState(t *testing.T) {
	a := &app{items: []wechat.WxapkgItem{{
		UUID:                   "old",
		WxId:                   "wx1111111111111111",
		Location:               "/same",
		UnpackStatus:           wechat.StatusTypeFinished,
		UnpackSavePath:         "/out/same_unpacked",
		UnpackTotal:            12,
		UnpackProgress:         100,
		UnpackCurrent:          12,
		UnpackCurrentFile:      "app.js",
		Name:                   "德罗心DELORSIM",
		NameProvider:           wechat.NameProviderYouzan,
		NameProviderID:         "146833520",
		FallbackNameProvider:   wechat.NameProviderWeChatProfile,
		FallbackNameProviderID: "wx1111111111111111",
	}}}
	a.mergeItems([]wechat.WxapkgItem{{
		UUID:     "new",
		WxId:     "wx1111111111111111",
		Location: "/same",
	}})
	if len(a.items) != 1 {
		t.Fatalf("mergeItems added a duplicate: %+v", a.items)
	}
	got := a.items[0]
	if got.UUID != "old" || got.Name != "德罗心DELORSIM" ||
		got.FallbackNameProvider != wechat.NameProviderWeChatProfile ||
		got.UnpackStatus != wechat.StatusTypeFinished || got.UnpackTotal != 12 ||
		got.UnpackSavePath != "/out/same_unpacked" {
		t.Fatalf("mergeItems lost unpack state: %+v", got)
	}
}

func TestComputeSavePath(t *testing.T) {
	output := t.TempDir()
	got := computeSavePath(output, "/tmp/mini/wx1111111111111111", "企业专属服务")
	want := filepath.Join(output, "wx1111111111111111_unpacked_企业专属服务")
	if got != want {
		t.Fatalf("computeSavePath = %q, want %q", got, want)
	}
	got = computeSavePath(output, "/tmp/mini/sample.wxapkg", "未识别")
	want = filepath.Join(output, "sample_unpacked")
	if got != want {
		t.Fatalf("computeSavePath = %q, want %q", got, want)
	}
	got = computeSavePath(output, "/tmp/mini/sample.wxapkg", `A/B\C:D`)
	want = filepath.Join(output, "sample_unpacked_A_B_C_D")
	if got != want {
		t.Fatalf("computeSavePath sanitized = %q, want %q", got, want)
	}
}

func TestOpenUnpackDefaultsToAllBeautifiers(t *testing.T) {
	a := &app{
		items: []wechat.WxapkgItem{{
			UUID:            "item",
			WxId:            "wx1111111111111111",
			Name:            "企业专属服务",
			WxapkgFilePaths: []string{},
		}},
		lastOutputDir: "/tmp/output",
	}
	a.openUnpack("item")
	if !a.unpackJSON || !a.unpackHTML || !a.unpackJS {
		t.Fatalf("default beautifiers = JSON %v, HTML %v, JavaScript %v; want all true",
			a.unpackJSON, a.unpackHTML, a.unpackJS)
	}
}

func TestFormatHelpers(t *testing.T) {
	if got := formatSize(1536); got != "1.5 KB" {
		t.Errorf("formatSize = %q", got)
	}
	if got := formatTime(0); got != "—" {
		t.Errorf("formatTime(0) = %q", got)
	}
}

func uiSort(column string, descending bool) ui.SortOrder {
	return ui.SortOrder{Column: column, Descending: descending}
}

func TestFormatterConcurrencyLimits(t *testing.T) {
	previous := runtime.GOMAXPROCS(8)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })

	if got := formatterThreads(4); got != 2 {
		t.Fatalf("formatterThreads(4) = %d, want 2", got)
	}
	if got := formatterThreads(1); got != 8 {
		t.Fatalf("formatterThreads(1) = %d, want 8", got)
	}
	if got := batchUnpackConcurrency(19, true); got != 4 {
		t.Fatalf("batchUnpackConcurrency = %d, want 4", got)
	}
	if got := batchUnpackConcurrency(19, false); got != 1 {
		t.Fatalf("batchUnpackConcurrency without formatter = %d, want 1", got)
	}
}
