package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

func findOxfmtPath() string {
	if configured := strings.TrimSpace(os.Getenv("WXAPKG_OXFMT")); configured != "" {
		if isExecutableFile(configured) {
			return configured
		}
	}

	name := "oxfmt"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{}
	if resources, err := mygo.App.Path(mygo.PathResources); err == nil {
		candidates = append(candidates,
			filepath.Join(resources, name),
			filepath.Join(resources, "bin", name),
		)
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), name))
	}
	for _, candidate := range candidates {
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	if found, err := exec.LookPath(name); err == nil {
		return found
	}
	return ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode()&0o111 != 0
}

func formatterThreads(concurrency int) int {
	cpus := runtime.GOMAXPROCS(0)
	if concurrency < 1 {
		concurrency = 1
	}
	threads := cpus / concurrency
	if threads < 1 {
		threads = 1
	}
	if threads > cpus {
		return cpus
	}
	return threads
}

func batchUnpackConcurrency(count int, hasFormatter bool) int {
	if count <= 1 || !hasFormatter {
		return 1
	}
	limit := runtime.GOMAXPROCS(0) / 2
	if limit > 4 {
		limit = 4
	}
	if limit < 1 {
		limit = 1
	}
	if count < limit {
		return count
	}
	return limit
}
