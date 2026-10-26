package bridge

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
)

// writeReport drops a report JSON file into the state reports dir. Every test
// that uses it must first isolate HOME (t.TempDir + t.Setenv): the tests share
// one package binary and would otherwise observe each other's seeded files.
func writeReport(t *testing.T, name, body string) string {
	t.Helper()
	dir := paths.ReportDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const sampleBackup = `{
  "started": "2026-10-26T03:00:00Z",
  "finished": "2026-10-26T03:00:05Z",
  "config": "/home/u/.config/hlops/config.toml",
  "dry_run": false,
  "success": 2,
  "failed": 1,
  "skipped": 0,
  "results": [
    {"task":"dotfiles","type":"files","status":"success","archive":"/b/dotfiles.tar.gz","sha256":"abc","size_bytes":123},
    {"task":"termux-env","type":"termux-env","status":"success","archive":"/b/env.tar.gz"},
    {"task":"scripts","type":"files","status":"failed","error":"open /missing: permission denied"}
  ]
}`

const sampleCleanup = `{
  "started": "2026-10-25T04:00:00Z",
  "finished": "2026-10-25T04:00:01Z",
  "dry_run": false,
  "results": [
    {"id":"native.tmp","title":"Termux tmp","status":"done","freed_bytes":4096},
    {"id":"provider.pkg-cache","title":"pkg cache","status":"skipped_dependency_missing","error":"apk not found"}
  ],
  "unknown_future_field": {"ignored": true}
}`

func TestParseBackupStatus(t *testing.T) {
	bs, err := ParseStatus([]byte(sampleBackup), "x.json", JobBackup, "test")
	if err != nil {
		t.Fatal(err)
	}
	if bs.Schema != SchemaVersion {
		t.Fatalf("schema = %d, want %d", bs.Schema, SchemaVersion)
	}
	if bs.Status != StatusFailed || bs.Failed != 1 || bs.Success != 2 {
		t.Fatalf("unexpected aggregate: %+v", bs)
	}
	if len(bs.Tasks) != 3 {
		t.Fatalf("tasks = %d, want 3", len(bs.Tasks))
	}
	if bs.Tasks[2].Error == "" {
		t.Fatal("failed task should carry error message for the APK UI")
	}
	if bs.Tasks[0].SHA256 != "abc" || bs.Tasks[0].SizeBytes != 123 {
		t.Fatalf("archive metadata not propagated: %+v", bs.Tasks[0])
	}
}

func TestParseCleanupStatusAndUnknownFields(t *testing.T) {
	bs, err := ParseStatus([]byte(sampleCleanup), "c.json", JobCleanup, "test")
	if err != nil {
		t.Fatal(err) // unknown_future_field must be ignored
	}
	if bs.Success != 1 || bs.Skipped != 1 || bs.Failed != 0 {
		t.Fatalf("counts wrong: %+v", bs)
	}
	if bs.Status != StatusPartial {
		t.Fatalf("status = %s, want partial", bs.Status)
	}
	if bs.Tasks[1].FreedBytes != 0 || bs.Tasks[0].FreedBytes != 4096 {
		t.Fatalf("freed bytes wrong: %+v", bs.Tasks)
	}
}

func TestLatestReportPrefersNewest(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate reports dir from sibling tests
	old := writeReport(t, "backup-20200101-000000.json", sampleBackup)
	newer := writeReport(t, "backup-20240101-000000.json", sampleBackup)
	info, err := LatestReport(JobBackup)
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != newer || info.Path == old {
		t.Fatalf("got %q, want newest %q", info.Path, newer)
	}
	if info.Job != JobBackup {
		t.Fatalf("job = %q", info.Job)
	}
}

func TestNoReportIsNotFatal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	info, err := LatestReport("")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist-wrapping error, got %v", err)
	}
	if info.Path != "" {
		t.Fatal("no report expected")
	}
	nr := NoReport("test")
	if nr.Status != StatusNoReport || nr.Schema != SchemaVersion {
		t.Fatalf("NoReport placeholder wrong: %+v", nr)
	}
}

func TestDryRunCountsAsPartial(t *testing.T) {
	body := `{"started":"s","finished":"f","dry_run":true,"success":1,"failed":0,"skipped":0,"results":[{"task":"t","status":"dry-run"}]}`
	bs, err := ParseStatus([]byte(body), "d.json", JobBackup, "test")
	if err != nil {
		t.Fatal(err)
	}
	if bs.Status != StatusPartial {
		t.Fatalf("dry-run should map to partial, got %s", bs.Status)
	}
}

// Review regression #3: an existing report with zero tasks (user ran a job
// while no tasks are enabled) is a real, successful run — it must map to
// "ok", never "no_report", so the APK does not wrongly show onboarding.
func TestEmptyReportAggregatesToOK(t *testing.T) {
	body := `{"started":"s","finished":"f","dry_run":false,"success":0,"failed":0,"skipped":0,"results":[]}`
	bs, err := ParseStatus([]byte(body), "empty.json", JobBackup, "test")
	if err != nil {
		t.Fatal(err)
	}
	if bs.Status != StatusOK {
		t.Fatalf("empty report should be ok, got %s", bs.Status)
	}
}

// Review regression #2 (amended, CI fix): the primary ordering key is the
// fixed-width run stamp parsed from the *tail* of the file name, so task names
// containing dashes still sort correctly and copy/restore of the reports dir
// (which flattens every ModTime to "now") keeps the true run order. The test
// intentionally writes the newer-stamped file first: with pure-ModTime
// ordering this passed only on coarse-granularity filesystems via the path
// tie-break and failed deterministically on nanosecond-granularity ext4 in CI.
func TestListReportsSortsByStampWithDashedNames(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate reports dir from sibling tests
	newer := writeReport(t, "backup-website-blog-20240102-000000.json", sampleBackup)
	writeReport(t, "backup-website-blog-20240101-000000.json", sampleBackup)
	info, err := LatestReport(JobBackup)
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != newer {
		t.Fatalf("got %q, want newest %q", info.Path, newer)
	}
}

// Names without a parseable stamp must still order newest-first by ModTime.
func TestListReportsFallsBackToModTime(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp) // isolate: earlier tests seeded the reports dir
	a := writeReport(t, "backup-legacy-a.json", sampleBackup)
	b := writeReport(t, "backup-legacy-b.json", sampleBackup)
	now := time.Now()
	if err := os.Chtimes(a, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(b, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	info, err := LatestReport(JobBackup)
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != b {
		t.Fatalf("got %q, want %q", info.Path, b)
	}
}

// Review regression #4: hard failures synthesise a parseable failed doc.
func TestNewFailedStatusShape(t *testing.T) {
	fs := NewFailedStatus(JobBackup, "test", "another run holds the lock")
	if fs.Status != StatusFailed || fs.Error == "" || fs.Schema != SchemaVersion {
		t.Fatalf("bad failed status: %+v", fs)
	}
	if fs.Tasks == nil {
		t.Fatal("tasks must be an array, never null")
	}
	if fs.Job != JobBackup {
		t.Fatalf("job = %q", fs.Job)
	}
}
