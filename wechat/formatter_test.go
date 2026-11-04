package wechat

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFormatterExtensions(t *testing.T) {
	got := formatterExtensions(true, true, true)
	for _, extension := range []string{".js", ".html", ".json"} {
		if !got[extension] {
			t.Fatalf("formatterExtensions missing %s: %v", extension, got)
		}
	}
	if got := formatterExtensions(false, false, false); len(got) != 0 {
		t.Fatalf("disabled formatterExtensions = %v, want none", got)
	}
}

func TestFormatterFileBatches(t *testing.T) {
	files := []string{"a.js", "nested/b.js", "nested/目录/c.json"}
	batches := formatterFileBatches(files, 20)
	if len(batches) != 3 {
		t.Fatalf("formatterFileBatches = %q, want 3 batches", batches)
	}
	var flattened []string
	for _, batch := range batches {
		if len(batch) == 0 {
			t.Fatal("formatterFileBatches returned an empty batch")
		}
		flattened = append(flattened, batch...)
	}
	want := []string{"./a.js", "./nested/b.js", "./nested/目录/c.json"}
	if !slices.Equal(flattened, want) {
		t.Fatalf("flattened batches = %q, want %q", flattened, want)
	}
}

func TestOxfmtFormatOutputIntegration(t *testing.T) {
	formatter := strings.TrimSpace(os.Getenv("WXAPKG_OXFMT"))
	if formatter == "" {
		t.Skip("set WXAPKG_OXFMT to run the Oxfmt integration test")
	}

	root := t.TempDir()
	files := map[string]string{
		"app.js":                    "const x={a:1,b:[2,3]};\n",
		"page.html":                 "<html><body><div>hello</div></body></html>\n",
		"config.json":               "{\"a\":1,\"b\":[2,3]}\n",
		"node_modules/pkg/index.js": "const y={c:4};\n",
		"untouched.txt":             "a.txt",
		".gitignore":                "app.js\npage.html\n",
		"oxfmt.json":                "{\"printWidth\":20}\n",
	}
	for name, contents := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	item := WxapkgItem{UnpackSavePath: root}
	unpacker := NewUnpacker(&item, &UnpackOptions{
		EnableJsBeautify:   true,
		EnableHtmlBeautify: true,
		EnableJsonBeautify: true,
		ExternalFormatter:  formatter,
		FormatterThreads:   2,
	})
	if err := unpacker.formatOutput(); err != nil {
		t.Fatal(err)
	}

	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if got := read("app.js"); !strings.Contains(got, "const x = { a: 1, b: [2, 3] };") {
		t.Fatalf("JavaScript was not formatted: %q", got)
	}
	if got := read("page.html"); !strings.Contains(got, "<div>hello</div>") || !strings.Contains(got, "\n") {
		t.Fatalf("HTML was not formatted: %q", got)
	}
	if got := read("config.json"); !strings.Contains(got, `"a": 1`) {
		t.Fatalf("JSON was not formatted: %q", got)
	}
	if got := read("node_modules/pkg/index.js"); !strings.Contains(got, "const y = { c: 4 };") {
		t.Fatalf("node_modules JavaScript was not formatted: %q", got)
	}
	if got := read("untouched.txt"); got != files["untouched.txt"] {
		t.Fatalf("unrelated file changed: %q", got)
	}
}
