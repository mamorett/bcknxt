package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to JSON configuration file")
	profileName := flag.String("profile", "", "Backup profile name (default: 'default' if exists in config)")
	fromDate := flag.String("from-date", "", "Sync only dates >= YYYY-MM-DD")
	limit := flag.Int("limit", 0, "Maximum number of folders to process (0 = no limit)")
	phase := flag.String("phase", "all", "Phase to run: 1, 2, 3, or all")
	dirPath := flag.String("dir", "", "Upload a specific directory directly (bypasses discovery)")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage of %s:\n", os.Args[0])
		flag.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(out, "  --%s %s\n", f.Name, f.Usage)
			if f.DefValue != "" {
				fmt.Fprintf(out, "    (default %q)\n", f.DefValue)
			}
		})
	}
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	profile := *profileName
	if profile == "" {
		if _, ok := cfg.Profiles["default"]; ok {
			profile = "default"
			logPrint("No --profile specified, using 'default' profile")
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: --profile is required (or add a 'default' profile to config)\n")
			flag.Usage()
			os.Exit(1)
		}
	}

	prof, err := getProfile(cfg, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	logPrint("=" + strings.Repeat("=", 59))
	logPrint("DGX Synchronization (Go)")
	logPrint(fmt.Sprintf("Profile: %s", profile))
	logPrint(fmt.Sprintf("Started: %s", timeNow()))
	logPrint("=" + strings.Repeat("=", 59))
	logPrint(fmt.Sprintf("Config: %s", *configPath))
	logPrint(fmt.Sprintf("  SOURCE: %s", prof.Source))
	logPrint(fmt.Sprintf("  DEST:   %s", prof.Dest))
	logPrint(fmt.Sprintf("  TMP:    %s", prof.Tmp))
	logPrint("")

	if !checkAuth() {
		writeStatus("0", "failed", "Not authenticated", nil, prof.Tmp)
		logPrint("ERROR: Run 'internxt login -x' first")
		os.Exit(1)
	}

	if *dirPath != "" {
		if err := runSingleDirUpload(*dirPath, prof.Dest, prof.Tmp); err != nil {
			logPrint(fmt.Sprintf("\nERROR: %v", err))
			writeStatus("0", "failed", err.Error(), nil, prof.Tmp)
			os.Exit(1)
		}
		return
	}

	phaseStr := *phase
	switch phaseStr {
	case "1", "all":
		missingDates, err := runPhase1(prof.Source, prof.Dest, prof.Tmp, *fromDate, *limit)
		if err != nil {
			logPrint(fmt.Sprintf("\nERROR: %v", err))
			writeStatus("0", "failed", err.Error(), nil, prof.Tmp)
			os.Exit(1)
		}
		if phaseStr == "1" {
			logPrint(fmt.Sprintf("\nPhase 1 Complete: Found %d items to sync.", len(missingDates)))
			return
		}
	}

	if phaseStr == "2" || phaseStr == "all" {
		if err := runPhase2(prof.Source, prof.Tmp); err != nil {
			logPrint(fmt.Sprintf("\nERROR: %v", err))
			writeStatus("0", "failed", err.Error(), nil, prof.Tmp)
			os.Exit(1)
		}
		if phaseStr == "2" {
			return
		}
	}

	if phaseStr == "3" || phaseStr == "all" {
		if err := runPhase3(prof.Tmp); err != nil {
			logPrint(fmt.Sprintf("\nERROR: %v", err))
			writeStatus("0", "failed", err.Error(), nil, prof.Tmp)
			os.Exit(1)
		}
	}

	logPrint("\n" + "=" + strings.Repeat("=", 59))
	logPrint("SYNC COMPLETE")
	logPrint("=" + strings.Repeat("=", 59))
}

func timeNow() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
