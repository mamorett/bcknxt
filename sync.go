package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runPhase1(sourceDir, destPath, tmpDir, fromDate string, limit int) ([]string, error) {
	logPrint("=" + strings.Repeat("=", 59))
	logPrint("PHASE 1: Discovery")
	logPrint("=" + strings.Repeat("=", 59))

	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("source directory %s not found", sourceDir)
	}
	os.MkdirAll(tmpDir, 0755)

	logPrint("Scanning: " + sourceDir)
	localDates := scanLocalDates(sourceDir)
	logPrint(fmt.Sprintf("  Found %d local folders", len(localDates)))
	if len(localDates) > 0 {
		logPrint(fmt.Sprintf("  Range: %s - %s", localDates[0], localDates[len(localDates)-1]))
	}

	logPrint("Resolving destination: " + destPath)
	destID, err := resolvePathToID(destPath)
	if err != nil {
		return nil, fmt.Errorf("resolve destination: %w", err)
	}
	logPrint("  Destination ID: " + destID)

	logPrint("Fetching remote backups...")
	contents, err := listFolderContents(destID)
	if err != nil {
		return nil, fmt.Errorf("list destination: %w", err)
	}
	remoteDates := parseRemoteDates(contents)
	logPrint(fmt.Sprintf("  Found %d remote backups", len(remoteDates)))
	if len(remoteDates) > 0 {
		logPrint("  Latest: " + remoteDates[len(remoteDates)-1])
	}

	// Write discovery artifacts
	writeLines(filepath.Join(tmpDir, "local_dates.txt"), localDates)
	writeLines(filepath.Join(tmpDir, "remote_dates.txt"), remoteDates)
	writeString(filepath.Join(tmpDir, "destination_id.txt"), destID)

	missingDates := determineMissing(localDates, remoteDates, fromDate, limit)

	writeLines(filepath.Join(tmpDir, "missing_dates.txt"), missingDates)

	extra := map[string]interface{}{
		"total_folders": len(missingDates),
		"missing_list":  missingDates,
	}
	writeStatus("1", "discovered", fmt.Sprintf("Found %d items", len(missingDates)), extra, tmpDir)

	logPrint("=" + strings.Repeat("=", 59))
	if len(missingDates) > 0 {
		logPrint(fmt.Sprintf("TO SYNC (%d):", len(missingDates)))
		for _, d := range missingDates {
			sz := folderSize(filepath.Join(sourceDir, d))
			logPrint(fmt.Sprintf("  - %s (%s)", d, formatSize(sz)))
		}
	} else {
		logPrint("Nothing to sync")
	}
	logPrint("=" + strings.Repeat("=", 59))
	return missingDates, nil
}

func determineMissing(localDates, remoteDates []string, fromDate string, limit int) []string {
	remoteSet := make(map[string]bool, len(remoteDates))
	for _, d := range remoteDates {
		remoteSet[d] = true
	}

	var missing []string
	if fromDate != "" {
		for _, d := range localDates {
			if d >= fromDate && !remoteSet[d] {
				missing = append(missing, d)
			}
		}
	} else if len(remoteDates) == 0 {
		missing = append([]string{}, localDates...)
	} else {
		latest := remoteDates[len(remoteDates)-1]
		for _, d := range localDates {
			if d > latest && !remoteSet[d] {
				missing = append(missing, d)
			}
		}
	}

	if limit > 0 && len(missing) > limit {
		missing = missing[:limit]
	}
	return missing
}

func runPhase2(sourceDir, tmpDir string) error {
	logPrint("=" + strings.Repeat("=", 79))
	logPrint("PHASE 2: Archive & Upload - DETAILED PROGRESS")
	logPrint("=" + strings.Repeat("=", 79))

	missingPath := filepath.Join(tmpDir, "missing_dates.txt")
	destIDPath := filepath.Join(tmpDir, "destination_id.txt")

	if _, err := os.Stat(missingPath); os.IsNotExist(err) {
		return fmt.Errorf("run phase 1 first — %s not found", missingPath)
	}
	if _, err := os.Stat(destIDPath); os.IsNotExist(err) {
		return fmt.Errorf("run phase 1 first — %s not found", destIDPath)
	}

	missingDates := readLines(missingPath)
	destID := readString(destIDPath)

	total := len(missingDates)
	if total == 0 {
		logPrint("Nothing to sync")
		writeStatus("2", "synced", "Nothing to sync", map[string]interface{}{"total_folders": 0}, tmpDir)
		return nil
	}

	logPrint("Destination ID: " + destID)
	logPrint(fmt.Sprintf("Total folders to sync: %d", total))
	logPrint("")
	writeStatus("2", "syncing", fmt.Sprintf("Starting %d folders", total),
		map[string]interface{}{"total_folders": total, "processed_folders": 0}, tmpDir)

	processed := 0
	var failedDates []string
	totalStart := time.Now()

	for idx, date := range missingDates {
		archivePath := filepath.Join(tmpDir, date+".tgz")
		itemStart := time.Now()

		logPrint("")
		logPrint("=" + strings.Repeat("=", 79))
		logPrint(fmt.Sprintf("PROCESSING ITEM [%d/%d]: %s", idx+1, total, date))
		logPrint("=" + strings.Repeat("=", 79))

		sourcePath := filepath.Join(sourceDir, date)
		infoPrint("Source path: " + sourcePath)
		infoPrint("Archive path: " + archivePath)

		err := func() error {
			sz := folderSize(sourcePath)
			infoPrint(fmt.Sprintf("Folder size: %s (%d bytes)", formatSize(sz), sz))
			detailPrint("Starting archive creation...")

			logPrint("  [1/3] Creating tar archive...")
			detailPrint(fmt.Sprintf("Archiving %s from %s", date, sourceDir))
			if err := createTgz(archivePath, sourceDir, date); err != nil {
				return fmt.Errorf("tar failed: %w", err)
			}
			detailPrint("Archive created successfully")

			stat, _ := os.Stat(archivePath)
			infoPrint(fmt.Sprintf("Archive size: %s", formatSize(stat.Size())))

			writeStatus("2", "processing",
				fmt.Sprintf("[%d/%d] %s: Uploading", idx+1, total, date),
				map[string]interface{}{
					"total_folders":    total,
					"current_folder":   date,
					"processed_folders": idx,
					"current_size":     formatSize(sz),
				}, tmpDir)

			logPrint("  [2/3] Uploading to Internxt...")
			detailPrint(fmt.Sprintf("Destination folder ID: %s", destID))

			var uploadErr error
			for attempt := 0; attempt < 2; attempt++ {
				detailPrint(fmt.Sprintf("Upload attempt #%d", attempt+1))
				uploadStart := time.Now()
				uploadErr = uploadFile(archivePath, destID)
				elapsed := time.Since(uploadStart)

				if uploadErr != nil {
					detailPrint(fmt.Sprintf("Upload failed (attempt %d): %v", attempt+1, uploadErr))
					if attempt == 0 {
						detailPrint("Waiting 5 seconds before retry...")
						time.Sleep(5 * time.Second)
					}
				} else {
					detailPrint(fmt.Sprintf("Upload completed successfully in %.1fs", elapsed.Seconds()))
					break
				}
			}
			if uploadErr != nil {
				return fmt.Errorf("upload failed after all retries: %w", uploadErr)
			}

			logPrint("  [3/3] Cleaning up temporary archive...")
			detailPrint("Removing: " + archivePath)
			os.Remove(archivePath)
			detailPrint("Cleanup complete")
			return nil
		}()

		elapsed := time.Since(itemStart)

		if err != nil {
			logPrint("")
			logPrint(fmt.Sprintf("✗ ITEM [%d/%d] %s FAILED", idx+1, total, date))
			logPrint(fmt.Sprintf("  Error: %v", err))
			logPrint(fmt.Sprintf("  Time elapsed: %.1fs", elapsed.Seconds()))
			writeStatus("2", "failed",
				fmt.Sprintf("[%d/%d] %s: Failed", idx+1, total, date),
				map[string]interface{}{
					"total_folders":    total,
					"current_folder":   date,
					"processed_folders": processed,
					"error":            err.Error(),
					"item_duration":    elapsed.Seconds(),
				}, tmpDir)
			failedDates = append(failedDates, date)
			logPrint("  Note: Archive kept at " + archivePath + " for debugging")
			logPrint("")
		} else {
			processed++
			writeStatus("2", "completed",
				fmt.Sprintf("[%d/%d] %s: Done in %.1fs", idx+1, total, date, elapsed.Seconds()),
				map[string]interface{}{
					"total_folders":    total,
					"current_folder":   date,
					"processed_folders": processed,
					"item_duration":    elapsed.Seconds(),
				}, tmpDir)
			logPrint("")
			logPrint(fmt.Sprintf("✓ ITEM [%d/%d] %s COMPLETED SUCCESSFULLY", idx+1, total, date))
			logPrint(fmt.Sprintf("  Total time: %.1fs", elapsed.Seconds()))
			logPrint(fmt.Sprintf("  Progress: %d/%d folders processed", processed, total))
			logPrint("")
		}

		// Clean up archive on success (already done), but also ensure no leftover
		if _, statErr := os.Stat(archivePath); statErr == nil {
			os.Remove(archivePath)
		}
	}

	totalTime := time.Since(totalStart)

	logPrint("")
	logPrint("=" + strings.Repeat("=", 79))
	logPrint("PHASE 2 COMPLETE - SUMMARY")
	logPrint("=" + strings.Repeat("=", 79))
	logPrint(fmt.Sprintf("Total folders processed: %d/%d", processed, total))
	logPrint(fmt.Sprintf("Successful: %d", processed))
	logPrint(fmt.Sprintf("Failed: %d", len(failedDates)))
	logPrint(fmt.Sprintf("Total time: %.1fs (%.1f minutes)", totalTime.Seconds(), totalTime.Minutes()))
	if len(failedDates) > 0 {
		logPrint(fmt.Sprintf("Failed folders: %v", failedDates))
	}
	logPrint("=" + strings.Repeat("=", 79))

	if len(failedDates) > 0 {
		writeStatus("2", "failed", fmt.Sprintf("%d failures", len(failedDates)),
			map[string]interface{}{
				"total_folders":    total,
				"processed_folders": processed,
				"failed_folders":   len(failedDates),
				"failed_list":      failedDates,
			}, tmpDir)
	} else {
		writeStatus("2", "synced", "Success",
			map[string]interface{}{
				"total_folders":    total,
				"processed_folders": processed,
			}, tmpDir)
	}
	return nil
}

func runPhase3(tmpDir string) error {
	logPrint("=" + strings.Repeat("=", 59))
	logPrint("PHASE 3: Verification")
	logPrint("=" + strings.Repeat("=", 59))

	missingPath := filepath.Join(tmpDir, "missing_dates.txt")
	destIDPath := filepath.Join(tmpDir, "destination_id.txt")

	if _, err := os.Stat(missingPath); os.IsNotExist(err) {
		return fmt.Errorf("run phase 1 first — %s not found", missingPath)
	}
	if _, err := os.Stat(destIDPath); os.IsNotExist(err) {
		return fmt.Errorf("run phase 1 first — %s not found", destIDPath)
	}

	intendedDates := readLines(missingPath)
	destID := readString(destIDPath)

	if len(intendedDates) == 0 {
		logPrint("Nothing to verify")
		writeStatus("3", "verified", "Nothing to verify",
			map[string]interface{}{"verified_folders": 0}, tmpDir)
		return nil
	}

	logPrint(fmt.Sprintf("Verifying %d uploads...", len(intendedDates)))
	contents, err := listFolderContents(destID)
	if err != nil {
		return fmt.Errorf("list destination for verification: %w", err)
	}
	remoteDates := parseRemoteDates(contents)
	remoteSet := make(map[string]bool, len(remoteDates))
	for _, d := range remoteDates {
		remoteSet[d] = true
	}

	var failed []string
	for _, d := range intendedDates {
		if !remoteSet[d] {
			failed = append(failed, d)
		}
	}

	if len(failed) > 0 {
		writeStatus("3", "failed", "Verification failed",
			map[string]interface{}{"failed_verification": failed}, tmpDir)
		logPrint("=" + strings.Repeat("=", 59))
		logPrint("FAILED: Missing on remote:")
		for _, d := range failed {
			logPrint("  - " + d)
		}
		logPrint("=" + strings.Repeat("=", 59))
		return fmt.Errorf("verification failed: %d dates missing remotely", len(failed))
	}

	writeStatus("3", "verified", fmt.Sprintf("Verified %d", len(intendedDates)),
		map[string]interface{}{"verified_folders": len(intendedDates)}, tmpDir)
	logPrint("=" + strings.Repeat("=", 59))
	logPrint(fmt.Sprintf("OK: All %d verified", len(intendedDates)))
	logPrint("=" + strings.Repeat("=", 59))
	return nil
}

func writeLines(path string, lines []string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	for _, l := range lines {
		f.WriteString(l + "\n")
	}
}

func readLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func writeString(path, content string) {
	os.WriteFile(path, []byte(content), 0644)
}

func readString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
