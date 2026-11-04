package wechat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanWxapkgItemLatestLayout(t *testing.T) {
	root := t.TempDir()
	packages := filepath.Join(root, "users", "user-hash", "applet", "packages")
	writeTestFile(t, filepath.Join(packages, "wx50d13a67c1b59969", "45", "__APP__.wxapkg"), []byte("app"))
	writeTestFile(t, filepath.Join(packages, "wx50d13a67c1b59969", "45", "_sub_.wxapkg"), []byte("sub"))
	writeTestFile(t, filepath.Join(packages, "wx75ad38701ee080d6", "393", "__APP__.wxapkg"), []byte("other"))

	items, err := ScanWxapkgItem(packages, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("ScanWxapkgItem returned %d items, want 2", len(items))
	}
	if items[0].WxId != "wx50d13a67c1b59969" || items[0].EncryptKey != items[0].WxId {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	if items[0].Size != int64(len("app")+len("sub")) {
		t.Errorf("size = %d, want %d", items[0].Size, len("app")+len("sub"))
	}
}

func TestScanWxapkgItemLegacyAppletLayout(t *testing.T) {
	root := t.TempDir()
	applet := filepath.Join(root, "Documents", "WeChat Files", "Applet")
	writeTestFile(t, filepath.Join(applet, "wx1111111111111111", "1", "__APP__.wxapkg"), []byte("x"))

	items, err := ScanWxapkgItem(filepath.Join(root, "Documents", "WeChat Files"), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].WxId != "wx1111111111111111" {
		t.Fatalf("ScanWxapkgItem = %+v, want one wx1111111111111111 item", items)
	}
}

func TestScanWxapkgItemSingleWxIDDirectory(t *testing.T) {
	root := t.TempDir()
	wxid := filepath.Join(root, "wx1111111111111111")
	writeTestFile(t, filepath.Join(wxid, "2", "__APP__.wxapkg"), []byte("x"))

	items, err := ScanWxapkgItem(wxid, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Location != wxid {
		t.Fatalf("ScanWxapkgItem = %+v, want the wxid directory", items)
	}
}

func TestScanWxapkgItemFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.wxapkg")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := ScanWxapkgItem(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].IsDir || items[0].Location != path {
		t.Fatalf("ScanWxapkgItem = %+v, want one file item", items)
	}
}
