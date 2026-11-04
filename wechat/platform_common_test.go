package wechat

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverAppletPackageDirsLatestLayout(t *testing.T) {
	root := t.TempDir()
	packages := filepath.Join(root, "users", "908e4395bf85f9f053f25a63288bcf20", "applet", "packages")
	writeTestFile(t, filepath.Join(packages, "wx50d13a67c1b59969", "45", "__APP__.wxapkg"), []byte("x"))

	got := discoverAppletPackageDirs(root, 5)
	if !slices.Equal(got, []string{packages}) {
		t.Fatalf("discoverAppletPackageDirs = %v, want %v", got, []string{packages})
	}
}

func TestDiscoverAppletPackageDirsLegacyLayouts(t *testing.T) {
	root := t.TempDir()
	v3 := filepath.Join(root, ".wxapplet", "packages")
	v4 := filepath.Join(root, "radium", "Applet", "packages")
	writeTestFile(t, filepath.Join(v3, "wx1111111111111111", "1", "__APP__.wxapkg"), []byte("x"))
	writeTestFile(t, filepath.Join(v4, "wx2222222222222222", "2", "__APP__.wxapkg"), []byte("y"))
	writeTestFile(t, filepath.Join(root, "web", "profiles", "applet", "packages", "ignored.wxapkg"), []byte("z"))

	got := discoverAppletPackageDirs(root, 5)
	want := []string{v3, v4}
	if !slices.Equal(got, want) {
		t.Fatalf("discoverAppletPackageDirs = %v, want %v", got, want)
	}
}

func TestPackageRootsForScanFindsNestedLatestRoot(t *testing.T) {
	root := t.TempDir()
	packages := filepath.Join(root, "users", "user-hash", "applet", "packages")
	writeTestFile(t, filepath.Join(packages, "wx3333333333333333", "7", "__APP__.wxapkg"), []byte("x"))

	got := packageRootsForScan(root)
	if !slices.Contains(got, packages) {
		t.Fatalf("packageRootsForScan(%q) = %v, missing %q", root, got, packages)
	}
}

func TestContainsWxapkgRespectsDepth(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "one", "two", "file.wxapkg"), []byte("x"))
	if !containsWxapkg(root, 2) {
		t.Fatal("containsWxapkg missed a file within the depth")
	}
	if containsWxapkg(root, 1) {
		t.Fatal("containsWxapkg found a file beyond the depth")
	}
}

func TestIsPermissionError(t *testing.T) {
	if !isPermissionError(&os.PathError{Op: "open", Path: "test", Err: fs.ErrPermission}) {
		t.Fatal("isPermissionError missed fs.ErrPermission")
	}
	if isPermissionError(os.ErrNotExist) {
		t.Fatal("isPermissionError treated a missing path as a permission error")
	}
}

func TestDiscoverAppletPackageDirsReportsDeniedChild(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX directory permissions as a regular user")
	}
	root := t.TempDir()
	denied := filepath.Join(root, "users", "908e4395bf85f9f053f25a63288bcf20")
	writeTestFile(t, filepath.Join(denied, "applet", "packages", "wx4444444444444444", "8", "__APP__.wxapkg"), []byte("x"))
	if err := os.Chmod(denied, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(denied, 0o755) })

	_, blocked := discoverAppletPackageDirsWithAccess(root, 5)
	if blocked != denied {
		t.Fatalf("blocked path = %q, want %q", blocked, denied)
	}
}

func TestPermissionBlockedPathUnwraps(t *testing.T) {
	denied := &PermissionError{Path: "/protected/packages", Err: fs.ErrPermission}
	if got := PermissionBlockedPath(fmt.Errorf("scan: %w", denied)); got != denied.Path {
		t.Fatalf("PermissionBlockedPath = %q, want %q", got, denied.Path)
	}
	if got := PermissionBlockedPath(os.ErrNotExist); got != "" {
		t.Fatalf("PermissionBlockedPath(non-permission) = %q, want empty", got)
	}
}
