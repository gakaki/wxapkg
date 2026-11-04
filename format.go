package main

import (
	"fmt"
	"log"
	"os"
	"time"
)

func formatSize(bytes int64) string {
	if bytes < 0 {
		return "—"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func formatTime(timestamp int64) string {
	if timestamp <= 0 {
		return "—"
	}
	return time.Unix(timestamp, 0).Format("2006/01/02 15:04")
}

func debugLog(format string, args ...any) {
	if os.Getenv("WXAPKG_DEBUG") != "" {
		log.Printf(format, args...)
	}
}
