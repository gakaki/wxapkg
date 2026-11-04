package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wux1an/wxapkg/wechat"
)

func TestPackageCleanupOnlyTargetsWxDirectories(t *testing.T) {
	root := t.TempDir()
	packages := filepath.Join(root, "users", "hash", "applet", "packages")
	wx1 := filepath.Join(packages, "wx1111111111111111")
	wx2 := filepath.Join(packages, "wx2222222222222222")
	other := filepath.Join(packages, "normal-directory")
	writeCleanupTestFile(t, filepath.Join(wx1, "1", "__APP__.wxapkg"), []byte("one"))
	writeCleanupTestFile(t, filepath.Join(wx2, "2", "__APP__.wxapkg"), []byte("two"))
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}

	items := []wechat.WxapkgItem{{Location: wx1, IsDir: true}}
	targets, err := collectPackageCleanupTargets(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || len(targets[0].Paths) != 2 {
		t.Fatalf("targets = %+v, want both wx directories", targets)
	}
	if targets[0].Size == 0 {
		t.Fatal("cleanup size was not calculated")
	}

	result := deletePackageCleanupTargets(targets)
	if len(result.Failed) != 0 || result.Deleted != 2 {
		t.Fatalf("cleanup result = %+v", result)
	}
	for _, path := range []string{wx1, wx2} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s still exists: %v", path, err)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("non-wx directory was removed: %v", err)
	}
	if _, err := os.Stat(packages); err != nil {
		t.Fatalf("packages root was removed: %v", err)
	}
}

func TestPackageCleanupRejectsUnsafeRoots(t *testing.T) {
	root := t.TempDir()
	applet := filepath.Join(root, "applet")
	wxid := filepath.Join(applet, "wx1111111111111111")
	if err := os.MkdirAll(wxid, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := cleanablePackageRoots([]wechat.WxapkgItem{{Location: wxid, IsDir: true}}); len(got) != 0 {
		t.Fatalf("legacy applet root was accepted: %v", got)
	}

	arbitrary := filepath.Join(root, "packages")
	arbitraryWxID := filepath.Join(arbitrary, "wx2222222222222222")
	if err := os.MkdirAll(arbitraryWxID, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := cleanablePackageRoots([]wechat.WxapkgItem{{Location: arbitraryWxID, IsDir: true}}); len(got) != 0 {
		t.Fatalf("arbitrary packages root was accepted: %v", got)
	}
}

func TestPackageCleanupSkipsMalformedTargets(t *testing.T) {
	root := t.TempDir()
	packages := filepath.Join(root, "applet", "packages")
	good := filepath.Join(packages, "wx1111111111111111")
	bad := filepath.Join(packages, "not-a-wxid")
	writeCleanupTestFile(t, filepath.Join(good, "__APP__.wxapkg"), []byte("x"))
	writeCleanupTestFile(t, filepath.Join(bad, "__APP__.wxapkg"), []byte("y"))

	target := packageCleanupTarget{
		Root:  packages,
		Paths: []string{good, bad, filepath.Join(root, "wx2222222222222222")},
	}
	result := deletePackageCleanupTargets([]packageCleanupTarget{target})
	if result.Deleted != 1 || len(result.Failed) != 2 {
		t.Fatalf("cleanup result = %+v", result)
	}
	if _, err := os.Stat(bad); err != nil {
		t.Fatalf("malformed target was removed: %v", err)
	}
}

func TestPackageCleanupRejectsSymlinkedTargets(t *testing.T) {
	root := t.TempDir()
	packages := filepath.Join(root, "applet", "packages")
	outside := filepath.Join(root, "outside")
	writeCleanupTestFile(t, filepath.Join(outside, "keep.txt"), []byte("keep"))
	if err := os.MkdirAll(packages, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(packages, "wx1111111111111111")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	items := []wechat.WxapkgItem{{Location: link, IsDir: true}}
	if _, err := collectPackageCleanupTargets(items); err == nil {
		t.Fatal("symlinked target was accepted for cleanup")
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.txt")); err != nil {
		t.Fatalf("symlink target was modified: %v", err)
	}
}

func TestItemsOutsideRoots(t *testing.T) {
	root := filepath.Join(t.TempDir(), "applet", "packages")
	items := []wechat.WxapkgItem{
		{Location: filepath.Join(root, "wx1111111111111111")},
		{Location: filepath.Join(t.TempDir(), "keep.wxapkg")},
	}
	got := itemsOutsideRoots(items, []string{root})
	if len(got) != 1 || got[0].Location != items[1].Location {
		t.Fatalf("itemsOutsideRoots = %+v", got)
	}
}

func writeCleanupTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
