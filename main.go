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
	profileName := flag.String("profile", "", "Backup profile name (required)")
	fromDate := flag.String("from-date", "", "Sync only dates >= YYYY-MM-DD")
	limit := flag.Int("limit", 0, "Maximum number of folders to process (0 = no limit)")
	phase := flag.String("phase", "all", "Phase to run: 1, 2, 3, or all")
	flag.Parse()

	if *profileName == "" {
		fmt.Fprintf(os.Stderr, "ERROR: --profile is required\n")
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	profile, err := getProfile(cfg, *profileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	logPrint("=" + strings.Repeat("=", 59))
	logPrint("DGX Synchronization (Go)")
	logPrint(fmt.Sprintf("Profile: %s", *profileName))
	logPrint(fmt.Sprintf("Started: %s", timeNow()))
	logPrint("=" + strings.Repeat("=", 59))
	logPrint(fmt.Sprintf("Config: %s", *configPath))
	logPrint(fmt.Sprintf("  SOURCE: %s", profile.Source))
	logPrint(fmt.Sprintf("  DEST:   %s", profile.Dest))
	logPrint(fmt.Sprintf("  TMP:    %s", profile.Tmp))
	logPrint("")

	if !checkAuth() {
		writeStatus("0", "failed", "Not authenticated", nil, profile.Tmp)
		logPrint("ERROR: Run 'internxt login -x' first")
		os.Exit(1)
	}

	phaseStr := *phase
	switch phaseStr {
	case "1", "all":
		missingDates, err := runPhase1(profile.Source, profile.Dest, profile.Tmp, *fromDate, *limit)
		if err != nil {
			logPrint(fmt.Sprintf("\nERROR: %v", err))
			writeStatus("0", "failed", err.Error(), nil, profile.Tmp)
			os.Exit(1)
		}
		if phaseStr == "1" {
			logPrint(fmt.Sprintf("\nPhase 1 Complete: Found %d items to sync.", len(missingDates)))
			return
		}
	}

	if phaseStr == "2" || phaseStr == "all" {
		if err := runPhase2(profile.Source, profile.Tmp); err != nil {
			logPrint(fmt.Sprintf("\nERROR: %v", err))
			writeStatus("0", "failed", err.Error(), nil, profile.Tmp)
			os.Exit(1)
		}
		if phaseStr == "2" {
			return
		}
	}

	if phaseStr == "3" || phaseStr == "all" {
		if err := runPhase3(profile.Tmp); err != nil {
			logPrint(fmt.Sprintf("\nERROR: %v", err))
			writeStatus("0", "failed", err.Error(), nil, profile.Tmp)
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
