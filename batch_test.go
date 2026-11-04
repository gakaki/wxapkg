package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wux1an/wxapkg/wechat"
)

func TestBatchOutputDirectoryName(t *testing.T) {
	timestamp := time.Date(2026, 11, 4, 11, 4, 0, 0, time.Local).Unix()
	item := wechat.WxapkgItem{
		WxId:           "wx1111111111111111",
		Name:           "企业专属服务",
		LastModifyTime: timestamp,
	}
	want := "企业专属服务-wx1111111111111111-20261104-110400"
	if got := batchOutputDirectoryName(item); got != want {
		t.Fatalf("batchOutputDirectoryName = %q, want %q", got, want)
	}

	item.Name = ""
	item.WxId = "unknown"
	item.LastModifyTime = 0
	want = "未识别-unknown-00000000-000000"
	if got := batchOutputDirectoryName(item); got != want {
		t.Fatalf("unknown batchOutputDirectoryName = %q, want %q", got, want)
	}
}

func TestBatchOutputBaseLimitsLength(t *testing.T) {
	item := wechat.WxapkgItem{
		WxId: "wx1111111111111111",
		Name: strings.Repeat("超长名称", 40),
	}
	base := batchOutputBase(item)
	if len(base) > maxBatchBaseBytes {
		t.Fatalf("batchOutputBase length = %d, want <= %d", len(base), maxBatchBaseBytes)
	}
	if !strings.HasSuffix(base, "-wx1111111111111111") {
		t.Fatalf("batchOutputBase = %q, want it to preserve the app ID", base)
	}
}

func TestRemoveMatchingBatchOutputs(t *testing.T) {
	root := t.TempDir()
	prefix := "企业专属服务-wx1111111111111111-"
	keep := []string{
		"企业专属服务-wx2222222222222222-20261104-110400",
		"其他小程序-wx1111111111111111-20261104-110400",
	}
	keepFile := "企业专属服务-wx1111111111111111-notes.txt"
	remove := []string{
		prefix + "20261103-100000",
		prefix + "20261104-110400",
	}
	for _, name := range append(append([]string{}, keep...), remove...) {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, keepFile), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	removed, err := removeMatchingBatchOutputs(root, prefix)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != len(remove) {
		t.Fatalf("removed = %v, want %v", removed, remove)
	}
	for _, name := range remove {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists, err = %v", name, err)
		}
	}
	for _, name := range keep {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s should be kept: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, keepFile)); err != nil {
		t.Fatalf("%s should be kept: %v", keepFile, err)
	}
}

func TestRunBatchJobUnpacksAndReplacesMatchingOutput(t *testing.T) {
	root := t.TempDir()
	packagePath := filepath.Join(t.TempDir(), "sample.wxapkg")
	if err := os.WriteFile(packagePath, buildTestWxapkg("app.js", []byte("console.log('ok')")), 0o600); err != nil {
		t.Fatal(err)
	}

	item := wechat.WxapkgItem{
		UUID:           "item",
		WxId:           "wx1111111111111111",
		Name:           "企业专属服务",
		Location:       packagePath,
		LastModifyTime: time.Date(2026, 11, 4, 11, 4, 0, 0, time.Local).Unix(),
	}
	base := batchOutputBase(item)
	finalPath := filepath.Join(root, base+"-"+batchOutputModified(item.LastModifyTime))
	oldPath := filepath.Join(root, base+"-20261103-100000")
	otherPath := filepath.Join(root, "其他小程序-wx2222222222222222-20261103-100000")
	for _, path := range []string{oldPath, otherPath} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(oldPath, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	a := &app{}
	snapshot, err := a.runBatchJob(batchUnpackJob{
		item:      item,
		root:      root,
		base:      base,
		finalPath: finalPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.UnpackStatus != wechat.StatusTypeFinished {
		t.Fatalf("status = %q, error = %q", snapshot.UnpackStatus, snapshot.UnpackErrorMessage)
	}
	data, err := os.ReadFile(filepath.Join(finalPath, "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte("console.log('ok')")) {
		t.Fatalf("app.js = %q", data)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old batch output should be removed, err = %v", err)
	}
	if _, err := os.Stat(otherPath); err != nil {
		t.Fatalf("unrelated output should be kept: %v", err)
	}
}

func buildTestWxapkg(name string, content []byte) []byte {
	nameBytes := []byte(name)
	indexLength := 4 + len(nameBytes) + 4 + 4
	offset := 14 + 4 + indexLength

	var buf bytes.Buffer
	buf.WriteByte(0xBE)
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))
	_ = binary.Write(&buf, binary.BigEndian, uint32(indexLength))
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(content)))
	buf.WriteByte(0xED)
	_ = binary.Write(&buf, binary.BigEndian, uint32(1))
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(nameBytes)))
	buf.Write(nameBytes)
	_ = binary.Write(&buf, binary.BigEndian, uint32(offset))
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(content)))
	buf.Write(content)
	return buf.Bytes()
}
