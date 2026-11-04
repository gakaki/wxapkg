package wechat

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"
)

func TestLiveUnpackPerformance(t *testing.T) {
	if os.Getenv("WXAPKG_LIVE_TEST") == "" {
		t.Skip("set WXAPKG_LIVE_TEST=1 to benchmark a real local package")
	}

	result := Platform.GetDefaultPaths()
	var items []WxapkgItem
	for _, root := range result.Paths {
		found, err := ScanWxapkgItem(root, true)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, found...)
	}
	if len(items) == 0 {
		t.Fatal("no local packages found")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Size > items[j].Size })

	for _, beautify := range []bool{false, true} {
		name := "raw"
		if beautify {
			name = "beautified"
		}
		t.Run(name, func(t *testing.T) {
			item := items[0]
			outputRoot := t.TempDir()
			if dir := os.Getenv("WXAPKG_BENCH_OUT"); dir != "" {
				outputRoot = dir
			}
			output := filepath.Join(outputRoot, item.WxId+"_"+name)
			options := UnpackOptions{
				EnableDecrypt:      true,
				EnableJsonBeautify: beautify,
				EnableHtmlBeautify: beautify,
				EnableJsBeautify:   beautify,
				OutputDir:          filepath.Dir(output),
				SavePath:           output,
			}
			if beautify {
				options.ExternalFormatter = os.Getenv("WXAPKG_OXFMT")
				if options.ExternalFormatter == "" {
					options.ExternalFormatter, _ = exec.LookPath("oxfmt")
				}
				if options.ExternalFormatter == "" {
					t.Skip("set WXAPKG_OXFMT or install oxfmt to benchmark the optimized beautifier")
				}
				options.FormatterThreads = runtime.GOMAXPROCS(0)
			}
			start := time.Now()
			var final WxapkgItem
			NewUnpacker(&item, &options).UnpackWithStatusCallback(func(current *WxapkgItem) {
				final = *current
			})
			elapsed := time.Since(start)
			if final.UnpackStatus != StatusTypeFinished {
				t.Fatalf("unpack failed: %s", final.UnpackErrorMessage)
			}
			t.Logf("%s: %d files, package %.1f MB, elapsed %s",
				name, final.UnpackTotal, float64(item.Size)/(1024*1024), elapsed)
		})
	}
}
