package wechat

import "testing"

func TestCleanWxapkgPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "package root slash", in: "/static/loading.gif", want: "static/loading.gif"},
		{name: "package root backslash", in: `\static\app.js`, want: "static/app.js"},
		{name: "nested", in: "pages/index/index.js", want: "pages/index/index.js"},
		{name: "redundant segments", in: "/pages/./index/../index.js", want: "pages/index.js"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cleanWxapkgPath(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("cleanWxapkgPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanWxapkgPathRejectsEscape(t *testing.T) {
	for _, name := range []string{
		"../evil.js",
		"/../evil.js",
		`..\evil.js`,
		"pages/../../evil.js",
		`C:\Windows\evil.js`,
		"C:/Windows/evil.js",
		"/",
		"//",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := cleanWxapkgPath(name); err == nil {
				t.Fatalf("cleanWxapkgPath(%q) = %q, want an error", name, got)
			}
		})
	}
}
