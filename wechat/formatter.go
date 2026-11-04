package wechat

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	formatterTimeout       = 2 * time.Minute
	formatterCommandBudget = 20 * 1024
)

func (u *Unpacker) usesExternalFormatter() bool {
	return strings.TrimSpace(u.options.ExternalFormatter) != "" &&
		(u.options.EnableJsBeautify || u.options.EnableHtmlBeautify || u.options.EnableJsonBeautify)
}

func (u *Unpacker) formatOutput() error {
	extensions := formatterExtensions(
		u.options.EnableJsBeautify,
		u.options.EnableHtmlBeautify,
		u.options.EnableJsonBeautify,
	)
	if len(extensions) == 0 {
		return nil
	}

	files, err := collectFormatterFiles(u.item.UnpackSavePath, extensions)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	config, err := os.CreateTemp("", "wxapkg-oxfmt-*.json")
	if err != nil {
		return err
	}
	configPath := config.Name()
	defer os.Remove(configPath)
	if _, err := config.WriteString("{\"printWidth\":120,\"tabWidth\":2,\"useTabs\":false}\n"); err != nil {
		config.Close()
		return err
	}
	if err := config.Close(); err != nil {
		return err
	}

	threads := u.options.FormatterThreads
	if threads <= 0 {
		threads = runtime.GOMAXPROCS(0)
	}
	if threads < 1 {
		threads = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), formatterTimeout)
	defer cancel()
	for _, batch := range formatterFileBatches(files, formatterCommandBudget) {
		args := []string{
			"--write",
			fmt.Sprintf("--threads=%d", threads),
			"--config=" + configPath,
			"--disable-nested-config",
			"--with-node-modules",
			"--no-error-on-unmatched-pattern",
		}
		args = append(args, batch...)
		cmd := exec.CommandContext(ctx, u.options.ExternalFormatter, args...)
		cmd.Dir = u.item.UnpackSavePath
		cmd.Env = append(os.Environ(), "NO_COLOR=1")

		err = cmd.Run()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			continue
		}
		// Oxfmt exits non-zero when some individual files cannot be parsed,
		// while still formatting every valid file. The raw extraction is
		// already on disk, so these diagnostics must not turn a successful
		// unpack into a failure.
		if _, ok := err.(*exec.ExitError); !ok {
			return err
		}
	}
	return nil
}

func formatterExtensions(js, html, json bool) map[string]bool {
	extensions := make(map[string]bool, 3)
	add := func(extension string) {
		extensions["."+extension] = true
	}
	if js {
		add("js")
	}
	if html {
		add("html")
	}
	if json {
		add("json")
	}
	return extensions
}

func collectFormatterFiles(root string, extensions map[string]bool) ([]string, error) {
	files := make([]string, 0, 1024)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		if !extensions[strings.ToLower(filepath.Ext(entry.Name()))] {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, relative)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func formatterFileBatches(files []string, maxBytes int) [][]string {
	if maxBytes <= 0 {
		maxBytes = formatterCommandBudget
	}
	batches := make([][]string, 0, 1)
	batch := make([]string, 0, 512)
	used := 0
	for _, file := range files {
		argument := "./" + filepath.ToSlash(file)
		size := len(argument) + 1
		if len(batch) > 0 && used+size > maxBytes {
			batches = append(batches, batch)
			batch = make([]string, 0, 512)
			used = 0
		}
		batch = append(batch, argument)
		used += size
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches
}
