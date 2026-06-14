package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var rootFolderRe = regexp.MustCompile(`Root folder ID\s*[│:]\s*([a-fA-F0-9\-]+)`)

func checkAuth() bool {
	cmd := exec.Command("internxt", "whoami", "-x")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(out))
	return strings.Contains(lower, "logged in")
}

func getRootFolderID() (string, error) {
	// Try JSON mode first
	cmd := exec.Command("internxt", "config", "-x", "--json")
	out, err := cmd.Output()
	if err == nil {
		js, err := extractJSON(string(out))
		if err == nil {
			var data map[string]interface{}
			if json.Unmarshal([]byte(js), &data) == nil {
				for _, k := range []string{"rootFolderId", "root_folder_id", "Root folder ID"} {
					if v, ok := data[k]; ok {
						if s, ok := v.(string); ok && s != "" {
							return s, nil
						}
					}
				}
				// Some keys may be nested; check values for list
				for _, v := range data {
					if arr, ok := v.([]interface{}); ok {
						for _, item := range arr {
							if m, ok := item.(map[string]interface{}); ok {
								for _, k := range []string{"rootFolderId", "root_folder_id"} {
									if s, ok := m[k].(string); ok && s != "" {
										return s, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Fallback: parse text output
	cmd = exec.Command("internxt", "config")
	out, err = cmd.Output()
	if err != nil {
		return "", fmt.Errorf("could not get root folder ID: %w", err)
	}
	m := rootFolderRe.FindStringSubmatch(string(out))
	if m == nil {
		return "", fmt.Errorf("root folder ID not found in config output")
	}
	return m[1], nil
}

func listFolderContents(folderID string) ([]interface{}, error) {
	cmd := exec.Command("internxt", "list", "-x", "--json", "-i", folderID)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list folder %s: %w", folderID, err)
	}
	js, err := extractJSON(string(out))
	if err != nil {
		return nil, fmt.Errorf("extract JSON from list: %w", err)
	}
	var data map[string]interface{}
	if json.Unmarshal([]byte(js), &data) == nil {
		if list, ok := data["list"]; ok {
			if m, ok := list.(map[string]interface{}); ok {
				var combined []interface{}
				if f, ok := m["folders"]; ok {
					if arr, ok := f.([]interface{}); ok {
						combined = append(combined, arr...)
					}
				}
				if f, ok := m["files"]; ok {
					if arr, ok := f.([]interface{}); ok {
						combined = append(combined, arr...)
					}
				}
				return combined, nil
			}
		}
	}
	// Maybe output is a flat list
	var arr []interface{}
	if json.Unmarshal([]byte(js), &arr) == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("unexpected list output format")
}

func resolvePathToID(pathStr string) (string, error) {
	rootID, err := getRootFolderID()
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(pathStr)
	if trimmed == "" || trimmed == "/" {
		return rootID, nil
	}
	parts := strings.Split(trimmed, "/")
	parts = filterEmpty(parts)
	currentID := rootID
	for _, part := range parts {
		contents, err := listFolderContents(currentID)
		if err != nil {
			return "", fmt.Errorf("list contents for '%s': %w", part, err)
		}
		found := false
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
			itemType := ""
			if v, ok := m["type"]; ok {
				itemType, _ = v.(string)
			}
			if strings.EqualFold(name, part) && (strings.EqualFold(itemType, "directory") || strings.EqualFold(itemType, "folder")) {
				id := ""
				if v, ok := m["uuid"]; ok {
					id, _ = v.(string)
				}
				if id == "" {
					if v, ok := m["id"]; ok {
						id, _ = v.(string)
					}
				}
				if id != "" {
					currentID = id
					found = true
					break
				}
			}
		}
		if !found {
			return "", fmt.Errorf("folder '%s' not found in path '%s'", part, pathStr)
		}
	}
	return currentID, nil
}

func uploadFile(localPath, destID string) error {
	cmd := exec.Command("internxt", "upload-file", "-x", "--json", "-f", localPath, "-i", destID)
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("upload failed: %w\nstderr: %s", err, stderrBuf.String())
	}
	_ = out
	return nil
}

func parseRemoteDates(contents []interface{}) []string {
	var dates []string
	re := regexp.MustCompile(`^20\d{2}-\d{2}-\d{2}$`)
	tgzRe := regexp.MustCompile(`^(20\d{2}-\d{2}-\d{2})\.tgz$`)
	seen := map[string]bool{}
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
		itemType := ""
		if v, ok := m["type"]; ok {
			itemType, _ = v.(string)
		}
		var date string
		if strings.EqualFold(itemType, "file") || strings.EqualFold(itemType, "tgz") || strings.EqualFold(itemType, "archive") {
			if m := tgzRe.FindStringSubmatch(name); len(m) > 1 {
				date = m[1]
			} else if re.MatchString(name) {
				date = name
			}
		} else if strings.EqualFold(itemType, "directory") || strings.EqualFold(itemType, "folder") {
			if re.MatchString(name) {
				date = name
			}
		}
		if date != "" && re.MatchString(date) && !seen[date] {
			dates = append(dates, date)
			seen[date] = true
		}
	}
	sortStrings(dates)
	return dates
}

func filterEmpty(s []string) []string {
	var out []string
	for _, v := range s {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func writeStatus(phase, status, message string, extra map[string]interface{}, tmpDir string) {
	if extra == nil {
		extra = map[string]interface{}{}
	}
	data := map[string]interface{}{
		"phase":     phase,
		"status":    status,
		"message":   message,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range extra {
		data[k] = v
	}
	statusPath := tmpDir + "/sync_status.json"
	osMkdirAll(tmpDir, 0755)
	f, err := osCreate(statusPath)
	if err != nil {
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(data)
}

// osMkdirAll and osCreate are wrappers for testing.
var osMkdirAll = func(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}

var osCreate = func(path string) (*os.File, error) {
	return os.Create(path)
}
