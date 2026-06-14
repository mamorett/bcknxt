# bcknxt — Internxt Backup Synchronization Tool (Go)

A cross-platform Go implementation of `sync.py` for synchronising date-based folders from local storage to Internxt backup. Uses a JSON configuration file with named **profiles** so you can define multiple backup scopes and run them independently.

---

## Table of Contents

- [Requirements](#requirements)
- [Installation](#installation)
- [Configuration](#configuration)
- [CLI Usage](#cli-usage)
- [Phases](#phases)
  - [Phase 1: Discovery](#phase-1-discovery)
  - [Phase 2: Archive & Upload](#phase-2-archive--upload)
  - [Phase 3: Verification](#phase-3-verification)
- [Examples](#examples)
- [Building from Source](#building-from-source)
- [Differences from sync.py](#differences-from-syncpy)
- [Troubleshooting](#troubleshooting)
- [Status File Format](#status-file-format)

---

## Requirements

- **Go** ≥ 1.21 (for building from source)
- **Internxt CLI** installed and authenticated (`internxt login -x`)
- Write access to the source, temp, and destination paths
- `internxt` CLI must be in `PATH`

---

## Installation

### Pre-built binaries

If a pre-built binary is provided for your platform:

```bash
chmod +x bcknxt-<platform>
./bcknxt-<platform> --profile dgxcomfy_p2
```

### Build from source

```bash
git clone <repo> && cd bcknxt
make build-all
# Binaries are placed in bin/
```

Add `bin/` to your `PATH` or copy the binary for your platform.

---

## Configuration

Create a `config.json` file with one or more backup profiles:

```json
{
  "profiles": {
    "dgxcomfy_p2": {
      "source": "/wdblack/ARS/dgxcomfy",
      "dest": "bck/dgxcomfy_p2",
      "tmp": "/wdblack/tmp"
    },
    "another_backup": {
      "source": "/data/my_project",
      "dest": "bck/my_project_backup",
      "tmp": "/tmp/bcknxt"
    }
  }
}
```

| Field | Description |
|---|---|
| `source` | Local directory containing `YYYY-MM-DD` sub-folders |
| `dest`  | Remote path on Internxt Drive (folders separated by `/`) |
| `tmp`   | Temporary directory for archive files and status tracking |

All fields are **required** for each profile.

### Config location

By default the tool looks for `config.json` in the current working directory. Override with `--config <path>`.

---

## CLI Usage

```
bcknxt --config <path> --profile <name> [--from-date YYYY-MM-DD] [--limit N] [--phase 1|2|3|all]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--config` | no | `config.json` | Path to JSON configuration |
| `--profile` | **yes** | — | Backup profile name from config |
| `--from-date` | no | — | Only sync folders ≥ this date (inclusive) |
| `--limit` | no | `0` (no limit) | Maximum number of folders to process |
| `--phase` | no | `all` | Phase to run: `1`, `2`, `3`, or `all` |

---

## Phases

The tool runs three distinct phases in sequence. Each can be executed independently via `--phase <N>`.

| Phase | Name | Description |
|---|---|---|
| **1** | **Discovery** | Determine which date folders are missing remotely and need to be backed up |
| **2** | **Archive & Upload** | Create `.tgz` archives of missing date folders and upload them to Internxt |
| **3** | **Verification** | Re-fetch remote contents to confirm every uploaded date is present |

### Phase 1: Discovery

| Aspect | Definition |
|---|---|
| **Purpose** | Determine which date folders need to be backed up |
| **Inputs** | `source` directory, `dest` remote path, optional `--from-date`, optional `--limit` |
| **Process** | 1. Scan `source` for `YYYY-MM-DD` folders<br>2. Resolve `dest` path to a remote Internxt folder ID<br>3. Fetch existing remote backup listing<br>4. Compute the set of dates present locally but absent remotely |
| **Artifacts** | `local_dates.txt`, `remote_dates.txt`, `destination_id.txt`, `missing_dates.txt` |
| **Status** | `sync_status.json` → phase `"1"`, status `"discovered"` |

**Missing date logic:**
- If `--from-date` is set: sync local dates ≥ that date that are absent remotely
- If no `--from-date` and no remote backups exist: sync all local dates
- If no `--from-date` and remote backups exist: sync only local dates *newer* than the latest remote backup

### Phase 2: Archive & Upload

| Aspect | Definition |
|---|---|
| **Purpose** | Create compressed archives of missing date folders and upload them to Internxt |
| **Inputs** | `missing_dates.txt`, `destination_id.txt` (from Phase 1), `source` directory |
| **Process** | 1. Read missing dates list<br>2. For each date: create `.tgz` archive (Go-native `archive/tar` + `compress/gzip`)<br>3. Upload archive via `internxt upload-file` to the resolved remote folder<br>4. Delete temporary archive on success |
| **Artifacts** | Temporary `.tgz` files (removed after upload) |
| **Status** | `sync_status.json` → phase `"2"`, status `"synced"` or `"failed"` |

Each upload retries once after a 5-second wait if it fails. Failed dates are reported in the summary but do not halt processing of remaining items.

### Phase 3: Verification

| Aspect | Definition |
|---|---|
| **Purpose** | Confirm every intended backup date exists in the remote folder |
| **Inputs** | `missing_dates.txt`, `destination_id.txt` (from Phase 1) |
| **Process** | 1. Re-fetch remote folder contents<br>2. Build set of remote date folders<br>3. Check each date from `missing_dates.txt` against the remote set |
| **Artifacts** | None (read-only verification) |
| **Status** | `sync_status.json` → phase `"3"`, status `"verified"` or `"failed"` |

If any dates are missing remotely, the phase returns an error listing the failures and exits with code 1.

---

## Examples

```bash
# Full sync of dgxcomfy_p2 profile
bcknxt --profile dgxcomfy_p2

# Sync only from 2026-06-01 onwards
bcknxt --profile dgxcomfy_p2 --from-date 2026-06-01

# Sync only the first 3 missing folders
bcknxt --profile dgxcomfy_p2 --limit 3

# Run only discovery
bcknxt --profile dgxcomfy_p2 --phase 1

# Run only upload (requires phase 1 artifacts)
bcknxt --profile dgxcomfy_p2 --phase 2

# Run only verification (requires phase 1+2 artifacts)
bcknxt --profile dgxcomfy_p2 --phase 3

# Use a custom config path
bcknxt --config /etc/bcknxt/config.json --profile dgxcomfy_p2
```

---

## Building from Source

### Makefile targets

| Target | Description |
|---|---|
| `build` | Build for the current platform → `bin/bcknxt` |
| `build:osx-arm64` | macOS Apple Silicon → `bin/bcknxt-darwin-arm64` |
| `build:linux-arm64` | Linux ARM64 → `bin/bcknxt-linux-arm64` |
| `build:linux-amd64` | Linux x86_64 → `bin/bcknxt-linux-amd64` |
| `build-all` | Build all three cross-compilation targets |
| `run` | `go run .` with passthrough arguments |
| `clean` | Remove `bin/` directory |

### Manual cross-compilation

```bash
GOOS=darwin GOARCH=arm64 go build -o bcknxt-darwin-arm64 .
GOOS=linux GOARCH=arm64 go build -o bcknxt-linux-arm64 .
GOOS=linux GOARCH=amd64 go build -o bcknxt-linux-amd64 .
```

---

## Differences from sync.py

| Feature | sync.py | bcknxt |
|---|---|---|
| **Configuration** | Markdown file with `# SOURCE:`, `# DEST:`, `# TMP:` | JSON file with named profiles |
| **Multiple backups** | No — single config only | Yes — any number of profiles |
| **Archive creation** | Shells out to `tar -czf` | Go-native `archive/tar` + `compress/gzip` |
| **JSON parsing** | Regex-based ANSI stripping + JSON extraction | Same approach (stdlib `encoding/json`) |
| **Output format** | Same timestamped logging | Identical logging style |
| **Status file** | Same `sync_status.json` format | Identical format |
| **CLI flags** | `--source`, `--dest`, `--tmp` | `--config`, `--profile` (source/dest/tmp come from config) |
| **Cross-platform** | Python (requires Python 3) | Standalone binary — no runtime dependency |
| **Retry logic** | One retry with 5s wait | Same |

---

## Troubleshooting

### "Not authenticated"

Run `internxt login -x` and verify with `internxt whoami -x`.

### "Folder 'X' NOT FOUND in path"

The remote path defined in `dest` does not exist or is incomplete. Create the folders manually or verify the path.

### "Run phase 1 first"

Phase 2 and 3 require artifact files (`missing_dates.txt`, `destination_id.txt`) produced by phase 1. Run `--phase 1` first or `--phase all`.

### Archive creation fails

Ensure `source` path exists and contains the expected date folders. The tool uses Go's native archiver — no external `tar` needed.

### Upload fails

Verify Internet connectivity and Internxt authentication. The tool retries once automatically. If it continues failing, inspect the archive in `tmp/` for debugging.

---

## Status File Format

Written to `<tmp>/sync_status.json` after each phase:

```json
{
  "phase": "2",
  "status": "completed",
  "message": "[3/5] 2026-06-14: Done in 45.2s",
  "timestamp": "2026-06-14T12:34:56Z",
  "total_folders": 5,
  "current_folder": "2026-06-14",
  "processed_folders": 3,
  "item_duration": 45.2
}
```
