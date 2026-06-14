# 📂 bcknxt — Internxt Backup Synchronization Tool

[![Go Version](https://img.shields.io/badge/Go-%E2%89%A5%201.21-00ADD8?style=for-the-badge&logo=go)](https://golang.org)
[![Platform Compatibility](https://img.shields.io/badge/Platform-Cross--Platform-4B32C3?style=for-the-badge)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](LICENSE)

A high-performance, cross-platform Go tool to automatically synchronize date-based backup folders from your local storage to Internxt Drive. It packages your folders into compressed `.tgz` archives using Go's native archiver and uploads them securely via the Internxt CLI.

With support for **profiles** in a single configuration file, you can manage multiple independent backup scopes with ease.

---

## 🧭 Table of Contents

- [🚀 Key Features](#-key-features)
- [📋 Requirements](#-requirements)
- [📦 Installation](#-installation)
- [⚙️ Configuration](#%EF%B8%8F-configuration)
- [💻 CLI Usage](#-cli-usage)
- [📂 Single Directory Upload (`--dir`)](#-single-directory-upload-dir)
- [👤 Default Profile](#-default-profile)
- [🔄 Sync Phases](#-sync-phases)
  - [Phase 1: Discovery](#phase-1-discovery)
  - [Phase 2: Archive & Upload](#phase-2-archive--upload)
  - [Phase 3: Verification](#phase-3-verification)
- [💡 Usage Examples](#-usage-examples)
- [🛠️ Building from Source](#%EF%B8%8F-building-from-source)
- [🔍 Troubleshooting](#-troubleshooting)
- [📊 Status File Format](#-status-file-format)

---

## 🚀 Key Features

* **Native Compression**: Built-in `tar/gzip` archiving (no dependency on external `tar` executables).
* **Multi-Profile Configurations**: Define multiple backup sources, destinations, and temporary paths.
* **Smart Synchronization**: Scans and uploads only what is missing on the remote side, starting from a custom date or defaulting to newer than the latest remote backup.
* **Automatic Retries**: Failed uploads are retried automatically with configured delays.
* **State & Tracking**: Generates machine-readable `sync_status.json` states after each phase for integrations.
* **Clean, Styled Output**: Color-coded, aligned console output for clear status tracking.

---

## 📋 Requirements

* **Go** $\ge$ 1.21 (only if building from source)
* **Internxt CLI** installed and logged in (`internxt login -x`)
* **Internxt CLI** executable must be in your system's `PATH`
* Write access to the local source, temporary (`tmp`), and destination paths

> [!IMPORTANT]
> You must run `internxt login -x` and verify your session with `internxt whoami -x` before running `bcknxt`.

---

## 📦 Installation

To build `bcknxt` from source:

```bash
git clone <repo> && cd bcknxt
make build-all
# Binaries are compiled and placed in bin/
```

Add the compiled binary for your platform to your `PATH` or copy it to a directory in your `PATH`.

---

## ⚙️ Configuration

Create a `config.json` file in your working directory. You can define multiple independent backup profiles:

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

### Profile Fields

| Field | Type | Description |
| :--- | :--- | :--- |
| `source` | `string` | Local directory containing date-based (`YYYY-MM-DD`) sub-folders. |
| `dest` | `string` | Remote path on Internxt Drive (folders separated by `/`). |
| `tmp` | `string` | Temporary directory used for staging archive files and tracking `sync_status.json`. |

> [!NOTE]
> All fields (`source`, `dest`, `tmp`) are **required** for each profile.
> By default, `bcknxt` looks for `config.json` in the current working directory. You can override this using the `--config <path>` flag.

---

## 💻 CLI Usage

```bash
bcknxt --config <path> --profile <name> [--from-date YYYY-MM-DD] [--limit N] [--phase 1|2|3|all]
bcknxt --config <path> --profile <name> --dir <path>
```

### Flags

| Flag | Required | Default | Description |
| :--- | :--- | :--- | :--- |
| `--config` | No | `config.json` | Path to JSON configuration file |
| `--profile` | No | `default` (if exists) | Name of the backup profile to run |
| `--from-date`| No | — | Only sync folders starting from this date (`YYYY-MM-DD`) |
| `--limit` | No | `0` (no limit) | Max number of local folders to process in a single run |
| `--phase` | No | `all` | Specific phase to execute: `1`, `2`, `3`, or `all` |
| `--dir` | No | — | Upload a specific directory directly (bypasses discovery phases) |

---

## 📂 Single Directory Upload (`--dir`)

Use `--dir` to upload a specific directory directly, bypassing discovery entirely. 

```bash
bcknxt --profile myprofile --dir /path/to/specific/folder
```

### Execution Flow:
1. Archives the target directory into a `.tgz` file in your profile's `tmp` path.
2. Uploads the archive to the profile's `dest` directory on Internxt.
3. Automatically deletes the temporary archive file on success.

> [!TIP]
> The folder name is used as the archive name (e.g., `/path/to/specific/my_photos` $\rightarrow$ `my_photos.tgz`). This is great for one-off backups.

---

## 👤 Default Profile

If you omit the `--profile` flag, the tool will look for a profile named `"default"` in your `config.json`:

```json
{
  "profiles": {
    "default": {
      "source": "/data/backup",
      "dest": "bck/default",
      "tmp": "/tmp/bcknxt"
    }
  }
}
```

> [!WARNING]
> If no `"default"` profile is configured and you do not supply the `--profile` flag, `bcknxt` will exit with an error.

---

## 🔄 Sync Phases

When running a full synchronization (`--phase all`), the tool runs three sequential phases:

| Phase | Name | Description |
| :---: | :--- | :--- |
| **1** | **Discovery** | Checks which local date-folders are missing on the remote Internxt directory. |
| **2** | **Archive & Upload** | Creates `.tgz` archives of missing folders and uploads them to Internxt. |
| **3** | **Verification** | Re-scans the remote Internxt directory to verify every uploaded folder exists. |

---

### Phase 1: Discovery

* **Goal**: Detect local backups that have not yet been uploaded.
* **Process**:
  1. Scans `source` for sub-directories matching `YYYY-MM-DD`.
  2. Resolves `dest` to an Internxt remote folder ID.
  3. Lists existing remote files and parses their backup dates.
  4. Identifies which dates exist locally but are missing on the remote.
* **Artifacts**: Writes `local_dates.txt`, `remote_dates.txt`, `destination_id.txt`, and `missing_dates.txt` to the `tmp` folder.
* **Status**: Updates `sync_status.json` with phase `"1"`, status `"discovered"`.

#### Discovery Date Logic:
* If `--from-date` is set: Syncs local dates $\ge$ that date that are absent remotely.
* If no `--from-date` and no remote backups exist: Syncs all local dates.
* If no `--from-date` and remote backups exist: Syncs only local dates *newer* than the latest remote backup date.

---

### Phase 2: Archive & Upload

* **Goal**: Generate compressed archives and upload them.
* **Process**:
  1. Reads `missing_dates.txt` and `destination_id.txt` from Phase 1.
  2. For each date:
     * Packs the folder into a `.tgz` archive inside the `tmp` directory.
     * Uploads the archive to Internxt.
     * Deletes the temporary archive file upon success.
* **Artifacts**: Temporary `.tgz` files (cleaned up automatically).
* **Status**: Updates `sync_status.json` with phase `"2"`, status `"synced"` or `"failed"`.

> [!NOTE]
> Upload failures are retried once after a 5-second delay. Failed items are reported in the summary but do not halt the synchronization of remaining folders.

---

### Phase 3: Verification

* **Goal**: Ensure the integrity of the upload process.
* **Process**:
  1. Re-fetches the remote Internxt directory contents.
  2. Confirms every date folder listed in `missing_dates.txt` is present on the remote.
* **Status**: Updates `sync_status.json` with phase `"3"`, status `"verified"` or `"failed"`.

> [!WARNING]
> If any backup is missing, the phase returns an error detailing the missing items and exits with code `1`.

---

## 💡 Usage Examples

```bash
# Perform a full sync of the "dgxcomfy_p2" profile
bcknxt --profile dgxcomfy_p2

# Sync only folders from 2026-06-01 onwards
bcknxt --profile dgxcomfy_p2 --from-date 2026-06-01

# Sync only the first 3 missing folders
bcknxt --profile dgxcomfy_p2 --limit 3

# Run only discovery (Phase 1)
bcknxt --profile dgxcomfy_p2 --phase 1

# Run only archive & upload (Phase 2)
bcknxt --profile dgxcomfy_p2 --phase 2

# Run only verification (Phase 3)
bcknxt --profile dgxcomfy_p2 --phase 3

# Specify a custom config file path
bcknxt --config /etc/bcknxt/config.json --profile dgxcomfy_p2

# Upload a specific directory directly, bypassing discovery
bcknxt --profile dgxcomfy_p2 --dir /wdblack/ARS/dgxcomfy/2026-06-14

# Run with the default profile
bcknxt
```

---

## 🛠️ Building from Source

### Makefile Targets

| Target | Description |
| :--- | :--- |
| `make build` | Builds for the current system architecture $\rightarrow$ `bin/bcknxt` |
| `make build-osx-arm64` | Cross-compiles for macOS Apple Silicon $\rightarrow$ `bin/bcknxt-darwin-arm64` |
| `make build-linux-arm64` | Cross-compiles for Linux ARM64 $\rightarrow$ `bin/bcknxt-linux-arm64` |
| `make build-linux-amd64` | Cross-compiles for Linux x86_64 $\rightarrow$ `bin/bcknxt-linux-amd64` |
| `make build-all` | Builds all cross-compilation targets |
| `make run` | Runs `go run .` passing any trailing command arguments |
| `make clean` | Cleans up the `bin/` output directory |

### Manual Cross-Compilation

```bash
GOOS=darwin GOARCH=arm64 go build -o bcknxt-darwin-arm64 .
GOOS=linux GOARCH=arm64 go build -o bcknxt-linux-arm64 .
GOOS=linux GOARCH=amd64 go build -o bcknxt-linux-amd64 .
```

---

## 🔍 Troubleshooting

### 🛑 "Not authenticated"
Ensure your Internxt account is logged in:
```bash
internxt login -x
internxt whoami -x
```

### 📂 "Folder 'X' NOT FOUND in path"
The remote path defined in the profile's `dest` does not exist or is mistyped. Create the parent folders in Internxt manually or verify the `dest` path.

### 🧩 "Run phase 1 first"
Phase 2 and Phase 3 require internal metadata files (`missing_dates.txt` and `destination_id.txt`) created during Phase 1. Run `--phase 1` first or use `--phase all`.

### 📦 Archive Creation Fails
Ensure your `source` directory is accessible and contains folders in the format `YYYY-MM-DD`. Also check that you have sufficient disk space in your `tmp` directory.

---

## 📊 Status File Format

A status file is written to `<tmp>/sync_status.json` at the end of each phase. This makes it easy to monitor sync runs programmatically:

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
