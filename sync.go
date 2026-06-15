package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runPhase1(sourceDir, destPath, tmpDir, fromDate string, limit int) ([]string, error) {
	logHeader("PHASE 1: Discovery")

	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("source directory %s not found", sourceDir)
	}
	os.MkdirAll(tmpDir, 0755)

	logPrint(fmt.Sprintf("Scanning: %s%s%s", colorCyan, sourceDir, colorReset))
	localDates := scanLocalDates(sourceDir)
	logPrint(fmt.Sprintf("  Found %s%d%s local folders", colorBold, len(localDates), colorReset))
	if len(localDates) > 0 {
		logPrint(fmt.Sprintf("  Range: %s%s%s - %s%s%s", colorCyan, localDates[0], colorReset, colorCyan, localDates[len(localDates)-1], colorReset))
	}

	logPrint(fmt.Sprintf("Resolving destination: %s%s%s", colorCyan, destPath, colorReset))
	destID, err := resolvePathToID(destPath)
	if err != nil {
		return nil, fmt.Errorf("resolve destination: %w", err)
	}
	logPrint(fmt.Sprintf("  Destination ID: %s%s%s", colorCyan, destID, colorReset))

	logPrint("Fetching remote backups...")
	contents, err := listFolderContents(destID)
	if err != nil {
		return nil, fmt.Errorf("list destination: %w", err)
	}
	remoteDates := parseRemoteDates(contents)
	logPrint(fmt.Sprintf("  Found %s%d%s remote backups", colorBold, len(remoteDates), colorReset))
	if len(remoteDates) > 0 {
		logPrint(fmt.Sprintf("  Latest: %s%s%s", colorCyan, remoteDates[len(remoteDates)-1], colorReset))
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

	logDivider()
	if len(missingDates) > 0 {
		logPrint(fmt.Sprintf("%sTO SYNC (%d):%s", colorYellow+colorBold, len(missingDates), colorReset))
		for _, d := range missingDates {
			sz := folderSize(filepath.Join(sourceDir, d))
			logPrint(fmt.Sprintf("  - %s%s%s (%s)", colorCyan, d, colorReset, formatSize(sz)))
		}
	} else {
		logPrint(colorGreen + "Nothing to sync" + colorReset)
	}
	logDivider()
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
	logHeader("PHASE 2: Archive & Upload - DETAILED PROGRESS")

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
		logPrint(colorGreen + "Nothing to sync" + colorReset)
		writeStatus("2", "synced", "Nothing to sync", map[string]interface{}{"total_folders": 0}, tmpDir)
		return nil
	}

	logPrint(fmt.Sprintf("Destination ID: %s%s%s", colorCyan, destID, colorReset))
	logPrint(fmt.Sprintf("Total folders to sync: %s%d%s", colorBold, total, colorReset))
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
		logHeader(fmt.Sprintf("PROCESSING ITEM [%d/%d]: %s", idx+1, total, date))

		sourcePath := filepath.Join(sourceDir, date)
		infoPrint("Source path: " + sourcePath)
		infoPrint("Archive path: " + archivePath)

		err := func() error {
			sz := folderSize(sourcePath)
			infoPrint(fmt.Sprintf("Folder size: %s (%d bytes)", formatSize(sz), sz))
			detailPrint("Starting archive creation...")

			logPrint(fmt.Sprintf("  %s[1/3] Creating tar archive...%s", colorYellow, colorReset))
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

			logPrint(fmt.Sprintf("  %s[2/3] Uploading to Internxt...%s", colorYellow, colorReset))
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

			logPrint(fmt.Sprintf("  %s[3/3] Cleaning up temporary archive...%s", colorYellow, colorReset))
			detailPrint("Removing: " + archivePath)
			os.Remove(archivePath)
			detailPrint("Cleanup complete")
			return nil
		}()

		elapsed := time.Since(itemStart)

		if err != nil {
			logPrint("")
			logPrint(fmt.Sprintf("%s✗ ITEM [%d/%d] %s FAILED%s", colorRed+colorBold, idx+1, total, date, colorReset))
			logPrint(fmt.Sprintf("  Error: %s%v%s", colorRed, err, colorReset))
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
			logPrint(fmt.Sprintf("%s✓ ITEM [%d/%d] %s COMPLETED SUCCESSFULLY%s", colorGreen+colorBold, idx+1, total, date, colorReset))
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
	logHeader("PHASE 2 COMPLETE - SUMMARY")
	logPrint(fmt.Sprintf("Total folders processed: %d/%d", processed, total))
	logPrint(fmt.Sprintf("Successful: %s%d%s", colorGreen, processed, colorReset))
	if len(failedDates) > 0 {
		logPrint(fmt.Sprintf("Failed: %s%d%s", colorRed+colorBold, len(failedDates), colorReset))
		logPrint(fmt.Sprintf("Failed folders: %s%v%s", colorRed, failedDates, colorReset))
	} else {
		logPrint(fmt.Sprintf("Failed: %d", len(failedDates)))
	}
	logPrint(fmt.Sprintf("Total time: %.1fs (%.1f minutes)", totalTime.Seconds(), totalTime.Minutes()))
	logDivider()

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
	logHeader("PHASE 3: Verification")

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
		logPrint(colorGreen + "Nothing to verify" + colorReset)
		writeStatus("3", "verified", "Nothing to verify",
			map[string]interface{}{"verified_folders": 0}, tmpDir)
		return nil
	}

	logPrint(fmt.Sprintf("Verifying %s%d%s uploads...", colorBold, len(intendedDates), colorReset))
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
		logDivider()
		logPrint(colorRed + colorBold + "FAILED: Missing on remote:" + colorReset)
		for _, d := range failed {
			logPrint(fmt.Sprintf("  - %s%s%s", colorRed, d, colorReset))
		}
		logDivider()
		return fmt.Errorf("verification failed: %d dates missing remotely", len(failed))
	}

	writeStatus("3", "verified", fmt.Sprintf("Verified %d", len(intendedDates)),
		map[string]interface{}{"verified_folders": len(intendedDates)}, tmpDir)
	logDivider()
	logPrint(fmt.Sprintf("%sOK: All %d verified%s", colorGreen+colorBold, len(intendedDates), colorReset))
	logDivider()
	return nil
}

func runSingleDirUpload(dirPath, destPath, tmpDir string, dryRun bool) error {
	logHeader("SINGLE DIRECTORY UPLOAD")

	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return fmt.Errorf("directory %s not found", dirPath)
	}
	os.MkdirAll(tmpDir, 0755)

	folderName := filepath.Base(dirPath)
	sourceDir := filepath.Dir(dirPath)
	archivePath := filepath.Join(tmpDir, folderName+".tgz")

	// Phase 1: Discovery
	logPrint(fmt.Sprintf("Resolving destination: %s%s%s", colorCyan, destPath, colorReset))
	destID, err := resolvePathToID(destPath)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	logPrint(fmt.Sprintf("  Destination ID: %s%s%s", colorCyan, destID, colorReset))
	writeString(filepath.Join(tmpDir, "destination_id.txt"), destID)

	logPrint("Checking remote backups...")
	contents, err := listFolderContents(destID)
	if err != nil {
		return fmt.Errorf("list destination: %w", err)
	}

	existsRemotely := false
	targetFilename := folderName + ".tgz"
	for _, item := range contents {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := ""
		if v, ok := m["plainName"]; ok {
			name, _ = v.(string)
		}
		if name == "" {
			if v, ok := m["name"]; ok {
				name, _ = v.(string)
			}
		}
		if strings.EqualFold(name, targetFilename) || strings.EqualFold(name, folderName) {
			existsRemotely = true
			break
		}
	}

	logDivider()
	if existsRemotely {
		logPrint(colorGreen + "Status: Already uploaded (exists on remote)" + colorReset)
	} else {
		sz := folderSize(dirPath)
		logPrint(fmt.Sprintf("%sTO UPLOAD:%s %s%s%s (%s)", colorYellow+colorBold, colorReset, colorCyan, folderName, colorReset, formatSize(sz)))
	}
	logDivider()

	writeStatus("1", "discovered", fmt.Sprintf("Checked %s", folderName),
		map[string]interface{}{"dir": folderName, "exists_remotely": existsRemotely}, tmpDir)

	if dryRun {
		return nil
	}

	// Phase 2: Archive & Upload
	destIDPath := filepath.Join(tmpDir, "destination_id.txt")
	if _, err := os.Stat(destIDPath); err == nil {
		destID = readString(destIDPath)
	} else {
		var err error
		destID, err = resolvePathToID(destPath)
		if err != nil {
			return fmt.Errorf("resolve destination: %w", err)
		}
		writeString(destIDPath, destID)
	}

	sz := folderSize(dirPath)
	infoPrint(fmt.Sprintf("Folder size: %s (%d bytes)", formatSize(sz), sz))
	detailPrint("Starting archive creation...")

	logPrint(fmt.Sprintf("  %s[1/2] Creating tar archive...%s", colorYellow, colorReset))
	detailPrint(fmt.Sprintf("Archiving %s", dirPath))
	if err := createTgz(archivePath, sourceDir, folderName); err != nil {
		return fmt.Errorf("tar failed: %w", err)
	}
	detailPrint("Archive created successfully")

	stat, _ := os.Stat(archivePath)
	infoPrint(fmt.Sprintf("Archive size: %s", formatSize(stat.Size())))

	writeStatus("2", "processing", fmt.Sprintf("Uploading %s", folderName),
		map[string]interface{}{"dir": folderName, "size": formatSize(sz)}, tmpDir)

	logPrint(fmt.Sprintf("  %s[2/2] Uploading to Internxt...%s", colorYellow, colorReset))
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

	logPrint(fmt.Sprintf("  %sCleaning up temporary archive...%s", colorYellow, colorReset))
	detailPrint("Removing: " + archivePath)
	os.Remove(archivePath)
	detailPrint("Cleanup complete")

	writeStatus("2", "completed", fmt.Sprintf("Uploaded %s", folderName),
		map[string]interface{}{"dir": folderName}, tmpDir)

	// Phase 3: Verification
	destIDPath = filepath.Join(tmpDir, "destination_id.txt")
	if _, err := os.Stat(destIDPath); err == nil {
		destID = readString(destIDPath)
	} else {
		var err error
		destID, err = resolvePathToID(destPath)
		if err != nil {
			return fmt.Errorf("resolve destination: %w", err)
		}
	}

	logPrint("Verifying upload...")
	contents, err = listFolderContents(destID)
	if err != nil {
		return fmt.Errorf("list destination for verification: %w", err)
	}

	existsRemotely = false
	for _, item := range contents {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := ""
		if v, ok := m["plainName"]; ok {
			name, _ = v.(string)
		}
		if name == "" {
			if v, ok := m["name"]; ok {
				name, _ = v.(string)
			}
		}
		if strings.EqualFold(name, targetFilename) || strings.EqualFold(name, folderName) {
			existsRemotely = true
			break
		}
	}

	logDivider()
	if existsRemotely {
		logPrint(colorGreen + colorBold + "UPLOAD COMPLETE & VERIFIED" + colorReset)
		logDivider()
		writeStatus("3", "verified", fmt.Sprintf("Verified %s", folderName),
			map[string]interface{}{"dir": folderName}, tmpDir)
	} else {
		logPrint(colorRed + colorBold + "VERIFICATION FAILED: File not found on remote" + colorReset)
		logDivider()
		writeStatus("3", "failed", fmt.Sprintf("Verification failed for %s", folderName),
			map[string]interface{}{"dir": folderName}, tmpDir)
		return fmt.Errorf("verification failed: uploaded file not found on remote")
	}

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
