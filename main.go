package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to JSON configuration file")
	profileName := flag.String("profile", "", "Backup profile name (default: 'default' if exists in config)")
	fromDate := flag.String("from-date", "", "Sync only dates >= YYYY-MM-DD")
	limit := flag.Int("limit", 0, "Maximum number of folders to process (0 = no limit)")
	dryRun := flag.Bool("dry-run", false, "Show what would be synced without executing the sync")
	dirPath := flag.String("dir", "", "Upload a specific directory directly (bypasses discovery)")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "\n%sbcknxt Synchronization (Go) — Usage Instructions%s\n\n", colorBold+colorCyan, colorReset)
		fmt.Fprintf(out, "%sUsage:%s\n", colorBold, colorReset)
		fmt.Fprintf(out, "  %s%s%s [options]\n", colorGreen, os.Args[0], colorReset)
		fmt.Fprintf(out, "  %s%s%s --profile <name> [--from-date YYYY-MM-DD] [--limit N] [--dry-run]\n", colorGreen, os.Args[0], colorReset)
		fmt.Fprintf(out, "  %s%s%s --profile <name> --dir <path> [--dry-run]\n\n", colorGreen, os.Args[0], colorReset)

		fmt.Fprintf(out, "%sOptions:%s\n", colorBold, colorReset)
		flag.VisitAll(func(f *flag.Flag) {
			def := ""
			if f.DefValue != "" {
				def = fmt.Sprintf(" %s(default %q)%s", colorGray, f.DefValue, colorReset)
			}
			fmt.Fprintf(out, "  %s--%-14s%s %s%s\n", colorYellow, f.Name, colorReset, f.Usage, def)
		})
		fmt.Fprintf(out, "\n")
	}
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sERROR: %v%s\n", colorRed+colorBold, err, colorReset)
		flag.Usage()
		fmt.Fprintf(os.Stderr, "%sTIP: Create a '%s' file in this directory or specify one via --config.%s\n\n", colorYellow, *configPath, colorReset)
		os.Exit(1)
	}

	profile := *profileName
	if profile == "" {
		if _, ok := cfg.Profiles["default"]; ok {
			profile = "default"
			logPrint("No --profile specified, using 'default' profile")
		} else {
			fmt.Fprintf(os.Stderr, "%sERROR: --profile is required (or add a 'default' profile to config)%s\n", colorRed+colorBold, colorReset)
			flag.Usage()
			os.Exit(1)
		}
	}

	prof, err := getProfile(cfg, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sERROR: %v%s\n", colorRed+colorBold, err, colorReset)
		os.Exit(1)
	}

	logHeader(
		"bcknxt Synchronization (Go)",
		fmt.Sprintf("Profile: %s", profile),
		fmt.Sprintf("Started: %s", timeNow()),
	)
	logPrint(fmt.Sprintf("Config: %s%s%s", colorCyan, *configPath, colorReset))
	logPrint(fmt.Sprintf("  %sSOURCE:%s %s", colorBold, colorReset, prof.Source))
	logPrint(fmt.Sprintf("  %sDEST:%s   %s", colorBold, colorReset, prof.Dest))
	logPrint(fmt.Sprintf("  %sTMP:%s    %s", colorBold, colorReset, prof.Tmp))
	if *dryRun {
		logPrint(fmt.Sprintf("  %sDRY RUN:%s  %strue%s", colorBold, colorReset, colorYellow, colorReset))
	}
	logPrint("")

	if !checkAuth() {
		writeStatus("0", "failed", "Not authenticated", nil, prof.Tmp)
		logPrint(colorRed + "ERROR: Run 'internxt login -x' first" + colorReset)
		os.Exit(1)
	}

	if *dirPath != "" {
		resolvedPath := filepath.Clean(*dirPath)
		// If the path doesn't exist as-is, check relative to the profile's source directory
		if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
			testPath := filepath.Join(prof.Source, *dirPath)
			if _, errSub := os.Stat(testPath); errSub == nil {
				resolvedPath = testPath
			}
		}

		// Ensure we resolve to an absolute path so that Base and Dir are parsed cleanly
		if absPath, err := filepath.Abs(resolvedPath); err == nil {
			resolvedPath = absPath
		}

		if err := runSingleDirUpload(resolvedPath, prof.Dest, prof.Tmp, *dryRun); err != nil {
			logPrint(fmt.Sprintf("\n%sERROR: %v%s", colorRed+colorBold, err, colorReset))
			writeStatus("0", "failed", err.Error(), nil, prof.Tmp)
			os.Exit(1)
		}
		return
	}

	missingDates, err := runPhase1(prof.Source, prof.Dest, prof.Tmp, *fromDate, *limit)
	if err != nil {
		logPrint(fmt.Sprintf("\n%sERROR: %v%s", colorRed+colorBold, err, colorReset))
		writeStatus("0", "failed", err.Error(), nil, prof.Tmp)
		os.Exit(1)
	}
	if *dryRun {
		logPrint(fmt.Sprintf("\n%sDry Run Complete: Found %d items to sync.%s", colorGreen, len(missingDates), colorReset))
		return
	}

	if err := runPhase2(prof.Source, prof.Tmp); err != nil {
		logPrint(fmt.Sprintf("\n%sERROR: %v%s", colorRed+colorBold, err, colorReset))
		writeStatus("0", "failed", err.Error(), nil, prof.Tmp)
		os.Exit(1)
	}

	if err := runPhase3(prof.Tmp); err != nil {
		logPrint(fmt.Sprintf("\n%sERROR: %v%s", colorRed+colorBold, err, colorReset))
		writeStatus("0", "failed", err.Error(), nil, prof.Tmp)
		os.Exit(1)
	}

	logPrint("")
	logHeader("SYNC COMPLETE")
}

func timeNow() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
