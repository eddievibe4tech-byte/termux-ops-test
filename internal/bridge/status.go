// Package bridge provides the machine-readable status contract between the
// hlops CLI and the future Android Companion APK (issue #37 / M10-1 #38).
//
// The Companion APK never parses human output or log files. Instead it reads
// the newest JSON report written under $HOME/.local/state/hlops/reports/ by
// `hlops backup` / `hlops run backup`. This package is the single source of
// truth for that schema: it defines the normalized BridgeStatus document the
// APK consumes, plus helpers to locate and parse the on-disk reports.
//
// Contract rules (see docs/companion-app-bridge.md):
//   - All timestamps are RFC3339 UTC strings as written by the CLI.
//   - Unknown fields in report JSON are ignored (forward compatible).
//   - A missing reports directory is not an error: LatestReport returns
//     an error wrapping os.ErrNotExist.
package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
)

// SchemaVersion is bumped whenever the BridgeStatus JSON shape changes in a
// way the Companion APK must be aware of. The APK checks this field first.
const SchemaVersion = 1

// Job kinds surfaced to the Companion APK.
const (
	JobBackup  = "backup"
	JobCleanup = "cleanup"
)

// Aggregated outcome states. Exactly one Status value appears per document;
// the APK maps them to UI colours without inspecting per-task details.
const (
	StatusOK       = "ok"        // everything succeeded
	StatusPartial  = "partial"   // mixed success/skipped or dry-run, nothing failed
	StatusFailed   = "failed"    // at least one failure
	StatusNoReport = "no_report" // no report found yet (fresh install)
)

// TaskState is a normalised per-item result shared by backup and cleanup jobs.
type TaskState struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // success|failed|skipped|dry-run|done
	Archive    string `json:"archive,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	FreedBytes int64  `json:"freed_bytes,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BridgeStatus is the normalized status document consumed by the Companion
// APK. It is produced from a raw job report via ParseStatus and rendered by
// `hlops bridge status --json`.
type BridgeStatus struct {
	Schema       int         `json:"schema"`
	GeneratedAt  string      `json:"generated_at"`
	Job          string      `json:"job"`
	SourceFile   string      `json:"source_file,omitempty"`
	Status       string      `json:"status"`
	Started      string      `json:"started,omitempty"`
	Finished     string      `json:"finished,omitempty"`
	DryRun       bool        `json:"dry_run"`
	Success      int         `json:"success"`
	Failed       int         `json:"failed"`
	Skipped      int         `json:"skipped"`
	Tasks        []TaskState `json:"tasks"`
	Error        string      `json:"error,omitempty"` // set when status=failed before any report was written
	HlopsVersion string      `json:"hlops_version,omitempty"`
}

// rawBackup mirrors internal/backup.Report without importing it, keeping the
// bridge package dependency-light (and easy to mirror as Kotlin data classes).
type rawBackup struct {
	Started  string `json:"started"`
	Finished string `json:"finished"`
	DryRun   bool   `json:"dry_run"`
	Success  int    `json:"success"`
	Failed   int    `json:"failed"`
	Skipped  int    `json:"skipped"`
	Results  []struct {
		Task      string `json:"task"`
		Status    string `json:"status"`
		Archive   string `json:"archive"`
		SHA256    string `json:"sha256"`
		SizeBytes int64  `json:"size_bytes"`
		Error     string `json:"error"`
		Reason    string `json:"reason"`
	} `json:"results"`
}

type rawCleanup struct {
	Started  string `json:"started"`
	Finished string `json:"finished"`
	DryRun   bool   `json:"dry_run"`
	Results  []struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Status     string `json:"status"`
		FreedBytes int64  `json:"freed_bytes"`
		Error      string `json:"error"`
	} `json:"results"`
}

// ReportInfo describes one report file on disk.
type ReportInfo struct {
	Path    string
	Job     string
	ModTime time.Time
}

// ListReports returns every known report file under the state report dir,
// sorted newest-first by modification time. Ordering deliberately relies on
// the filesystem ModTime instead of parsing the timestamp out of the file
// name: name-based parsing breaks as soon as task names contain dashes or the
// stamp format evolves, while ModTime is always available and matches how
// reports are actually written (one fresh file per run). Files whose prefix
// is unrecognised are ignored.
func ListReports() ([]ReportInfo, error) {
	dir := paths.ReportDir()
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []ReportInfo
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		job := ""
		switch {
		case strings.HasPrefix(e.Name(), "backup-"):
			job = JobBackup
		case strings.HasPrefix(e.Name(), "cleanup-"):
			job = JobCleanup
		default:
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		out = append(out, ReportInfo{
			Path:    filepath.Join(dir, e.Name()),
			Job:     job,
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.ModTime.Equal(b.ModTime) {
			return a.ModTime.After(b.ModTime)
		}
		return a.Path > b.Path
	})
	return out, nil
}

// NewFailedStatus builds the bridge document emitted when a run failed so
// hard that no report was written at all (lock contention, invalid config,
// ...). The Companion APK can then always parse stdout as one BridgeStatus
// instead of special-casing "non-zero exit + empty stdout".
func NewFailedStatus(job, version, errMsg string) *BridgeStatus {
	return &BridgeStatus{
		Schema:       SchemaVersion,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Job:          job,
		Status:       StatusFailed,
		Tasks:        []TaskState{},
		Error:        errMsg,
		HlopsVersion: version,
	}
}

// LatestReport returns the newest report for the given job ("" = any job).
// When nothing matches it returns an error wrapping os.ErrNotExist.
func LatestReport(job string) (ReportInfo, error) {
	all, err := ListReports()
	if err != nil {
		return ReportInfo{}, err
	}
	for _, r := range all {
		if job == "" || r.Job == job {
			return r, nil
		}
	}
	want := job
	if want == "" {
		want = "any"
	}
	return ReportInfo{}, fmt.Errorf("no %s report found in %s: %w", want, paths.ReportDir(), os.ErrNotExist)
}

// LoadStatus reads a report file and converts it into a BridgeStatus.
func LoadStatus(info ReportInfo, version string) (*BridgeStatus, error) {
	data, err := os.ReadFile(info.Path)
	if err != nil {
		return nil, fmt.Errorf("read report: %w", err)
	}
	return ParseStatus(data, info.Path, info.Job, version)
}

// ParseStatus converts raw report JSON bytes into the normalized document.
func ParseStatus(data []byte, srcPath, job, version string) (*BridgeStatus, error) {
	bs := &BridgeStatus{
		Schema:       SchemaVersion,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Job:          job,
		SourceFile:   srcPath,
		HlopsVersion: version,
		Tasks:        []TaskState{},
	}
	switch job {
	case JobBackup:
		var rep rawBackup
		if err := json.Unmarshal(data, &rep); err != nil {
			return nil, fmt.Errorf("parse backup report: %w", err)
		}
		bs.Started, bs.Finished, bs.DryRun = rep.Started, rep.Finished, rep.DryRun
		bs.Success, bs.Failed, bs.Skipped = rep.Success, rep.Failed, rep.Skipped
		for _, r := range rep.Results {
			errMsg := r.Error
			if errMsg == "" {
				errMsg = r.Reason
			}
			bs.Tasks = append(bs.Tasks, TaskState{
				Name: r.Task, Status: r.Status, Archive: r.Archive,
				SHA256: r.SHA256, SizeBytes: r.SizeBytes, Error: errMsg,
			})
		}
	case JobCleanup:
		var rep rawCleanup
		if err := json.Unmarshal(data, &rep); err != nil {
			return nil, fmt.Errorf("parse cleanup report: %w", err)
		}
		bs.Started, bs.Finished, bs.DryRun = rep.Started, rep.Finished, rep.DryRun
		for _, r := range rep.Results {
			st := r.Status
			switch st {
			case "done":
				bs.Success++
			case "failed":
				bs.Failed++
			default:
				bs.Skipped++ // skipped_* and dry-run variants
			}
			name := r.ID
			if r.Title != "" {
				name = r.ID + " - " + r.Title
			}
			bs.Tasks = append(bs.Tasks, TaskState{
				Name: name, Status: st, FreedBytes: r.FreedBytes, Error: r.Error,
			})
		}
	default:
		return nil, fmt.Errorf("unknown job kind %q", job)
	}
	bs.Status = aggregate(bs)
	return bs, nil
}

// aggregate derives the outcome of an *existing* report. It must never
// return StatusNoReport: that state is reserved for NoReport(), which the
// CLI builds only when no report file exists at all. A successful run with
// zero enabled tasks is a real (empty) report and maps to ok, so the APK
// shows "done" instead of wrongly falling back to the onboarding screen.
func aggregate(bs *BridgeStatus) string {
	switch {
	case bs.Failed > 0:
		return StatusFailed
	case bs.Skipped > 0 || bs.DryRun:
		return StatusPartial
	default:
		return StatusOK
	}
}

// NoReport builds the placeholder document returned when a fresh install has
// not produced any report yet (the APK shows the onboarding screen instead).
func NoReport(version string) *BridgeStatus {
	return &BridgeStatus{
		Schema:       SchemaVersion,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Status:       StatusNoReport,
		Tasks:        []TaskState{},
		HlopsVersion: version,
	}
}
