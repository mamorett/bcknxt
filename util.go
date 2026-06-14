package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var ansiRegexp = regexp.MustCompile(`\x1B[@-Z\\-_]|\x1B\[[0-?]*[ -/]*[@-~]`)

func stripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

func extractJSON(out string) (string, error) {
	clean := strings.TrimSpace(stripANSI(out))
	// Check for object first ({}), then array ([]).
	// Object check must come first because JSON objects may contain
	// inner arrays whose '[' appears before the outer '{'.
	if start := strings.Index(clean, "{"); start != -1 {
		if end := strings.LastIndex(clean, "}"); end != -1 && end > start {
			return clean[start : end+1], nil
		}
	}
	if start := strings.Index(clean, "["); start != -1 {
		if end := strings.LastIndex(clean, "]"); end != -1 && end > start {
			return clean[start : end+1], nil
		}
	}
	return "", fmt.Errorf("no JSON found in output: %.200s", clean)
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"

	dividerWidth = 60
)

func logPrint(msg string) {
	fmt.Printf("%s[%s]%s %s\n", colorGray, time.Now().Format("15:04:05"), colorReset, msg)
}

func infoPrint(msg string) {
	fmt.Printf("  %s%s%s\n", colorGray, msg, colorReset)
}

func detailPrint(msg string) {
	fmt.Printf("    %s%s%s\n", colorGray, msg, colorReset)
}

func logDivider() {
	logPrint(colorGray + strings.Repeat("━", dividerWidth) + colorReset)
}

func logHeader(lines ...string) {
	logDivider()
	for _, line := range lines {
		logPrint(colorBold + colorCyan + line + colorReset)
	}
	logDivider()
}

func formatSize(bytes int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	val := float64(bytes)
	for _, u := range units {
		if val < 1024 {
			return fmt.Sprintf("%.1f%s", val, u)
		}
		val /= 1024
	}
	return fmt.Sprintf("%.1fPB", val)
}

func folderSize(path string) int64 {
	var total int64
	filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func scanLocalDates(sourceDir string) []string {
	var dates []string
	re := regexp.MustCompile(`^20\d{2}-\d{2}-\d{2}$`)
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return dates
	}
	for _, e := range entries {
		if e.IsDir() && re.MatchString(e.Name()) {
			dates = append(dates, e.Name())
		}
	}
	sortStrings(dates)
	return dates
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
