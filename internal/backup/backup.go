// Package backup implements multi-task tar.gz backups with atomic writes,
// SHA256 sidecars, permission manifests for storage that drops Unix modes,
// retention pruning and JSON reports.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/eddievibe4tech-byte/termux-ops/internal/config"
	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
	"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

// Status of a single task run.
type Status string

const (
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
	StatusDryRun  Status = "dry-run"
)

// TaskResult is the outcome for one task.
type TaskResult struct {
	Task      string    `json:"task"`
	Type      string    `json:"type"`
	Status    Status    `json:"status"`
	Archive   string    `json:"archive,omitempty"`
	SHA256    string    `json:"sha256,omitempty"`
	Manifest  string    `json:"manifest,omitempty"`
	SizeBytes int64     `json:"size_bytes,omitempty"`
	Error     string    `json:"error,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	Removed   []string  `json:"removed,omitempty"`
	DurationS float64   `json:"duration_seconds"`
}

// Report is the whole run report persisted as JSON.
type Report struct {
 Started   string       `json:"started"`
 Finished  string       `json:"finished"`
 Config    string       `json:"config"`
 DryRun    bool         `json:"dry_run"`
 Success   int          `json:"success"`
 Failed    int          `json:"failed"`
 Skipped   int          `json:"skipped"`
 Results   []TaskResult `json:"results"`
}

// Runner executes backup tasks.
type Runner struct {
	Cfg    *config.Config
	DryRun bool
	Only   string // task name filter, "" = all
}

// RunAll runs every enabled task (or Only) and returns the report.
func (r *Runner) RunAll() *Report {
	rep := &Report{Started: time.Now().UTC().Format(time.RFC3339), Config: r.Cfg.Path, DryRun: r.DryRun}
	for _, t := range r.Cfg.Backup.Tasks {
		if r.Only != "" && t.Name != r.Only {
			continue
		}
		res := r.RunTask(t)
		switch res.Status {
		case StatusSuccess, StatusDryRun:
			rep.Success++
		case StatusSkipped:
			rep.Skipped++
		default:
			rep.Failed++
		}
		rep.Results = append(rep.Results, res)
	}
	rep.Finished = time.Now().UTC().Format(time.RFC3339)
	return rep
}

// RunTask executes a single task.
func (r *Runner) RunTask(t config.Task) TaskResult {
	start := time.Now()
	res := TaskResult{Task: t.Name, Type: t.Type}
	defer func() { res.DurationS = time.Since(start).Seconds() }()

	if !t.IsEnabled() {
		res.Status = StatusSkipped
		res.Reason = "disabled"
		return res
	}

	dest := paths.Resolve(t.Destination)
	source := paths.Resolve(t.Source)

	switch t.Type {
	case config.TypeTermuxEnv:
		return r.finish(t, dest, r.runTermuxEnv(t, dest, source, &res))
	case config.TypeFiles, "":
		if source == "" {
			res.Status = StatusSkipped
			res.Reason = "missing source"
			return res
		}
		if _, err := os.Stat(source); err != nil {
			res.Status = StatusSkipped
			res.Reason = fmt.Sprintf("source not found: %v", err)
			slog.Warn("backup source missing", "task", t.Name, "source", source)
			return res
		}
		if r.DryRun {
			res.Status = StatusDryRun
			res.Archive = filepath.Join(dest, archiveName(t.Name, start))
			res.Reason = "dry-run"
			return res
		}
		if err := os.MkdirAll(dest, 0o755); err != nil {
			res.Status = StatusFailed
			res.Error = err.Error()
			return res
		}
		arch, sum, manifest, err := r.createArchive(t, source, dest, start)
		if err != nil {
			res.Status = StatusFailed
			res.Error = err.Error()
			return res
		}
		res.Status = StatusSuccess
		res.Archive = arch
		res.SHA256 = sum
		res.Manifest = manifest
		if fi, e := os.Stat(arch); e == nil {
			res.SizeBytes = fi.Size()
		}
		res.Removed = r.applyRetention(t, dest)
		return res
	default:
		res.Status = StatusFailed
		res.Error = fmt.Sprintf("unknown task type %q", t.Type)
		return res
	}
}

func (r *Runner) finish(t config.Task, dest string, res TaskResult) TaskResult {
	if res.Status == StatusSuccess && !r.DryRun {
		res.Removed = r.applyRetention(t, dest)
	}
	return res
}

// runTermuxEnv collects package list, TERMUX env vars and ~/.termux into
// a staging dir, then archives it as termux-env.tar.gz.
func (r *Runner) runTermuxEnv(t config.Task, dest, sourceOverride string, res *TaskResult) TaskResult {
	info := termux.Detect()
	if !info.IsTermux {
		res.Status = StatusSkipped
		res.Reason = "not running on Termux"
		return *res
	}
	if r.DryRun {
		res.Status = StatusDryRun
		res.Archive = filepath.Join(dest, "termux-env.tar.gz")
		res.Reason = "dry-run"
		return *res
	}
	stage, err := os.MkdirTemp("", "hlops-termux-env-*")
	if err != nil {
		res.Status = StatusFailed
		res.Error = err.Error()
		return *res
	}
	defer os.RemoveAll(stage)

	// 1. package lists via dpkg status (works without pkg wrapper)
	if err := collectPackages(stage, info.Prefix); err != nil {
		slog.Warn("collect packages failed", "err", err)
	}
	// 2. env | grep TERMUX equivalent
	collectTermuxEnv(stage)
	// 3. ~/.termux directory if present
	tcfg := filepath.Join(paths.Home(), ".termux")
	if sourceOverride != "" {
		tcfg = sourceOverride
	}
	if st, e := os.Stat(tcfg); e == nil && st.IsDir() {
		out := filepath.Join(stage, ".termux")
		if e := copyTree(tcfg, out); e != nil {
			slog.Warn("copy ~/.termux failed", "err", e)
		}
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		res.Status = StatusFailed
		res.Error = err.Error()
		return *res
	}
	now := time.Now()
	arch := filepath.Join(dest, fmt.Sprintf("%s-%s.tar.gz", "termux-env", now.UTC().Format("20060102-150405")))
	part := arch + ".part"
	files, ferr := walkFiles(stage, nil)
	written, modeErr := writeTarGz(part, stage, files, false)
	if ferr != nil || modeErr != nil {
		os.Remove(part)
		res.Status = StatusFailed
		res.Error = fmt.Sprint(ferr, modeErr)
		return *res
	}
	if err := os.Rename(part, arch); err != nil {
		os.Remove(part)
		res.Status = StatusFailed
		res.Error = err.Error()
		return *res
	}
	sum, err := sha256File(arch)
	if err == nil {
		_ = os.WriteFile(arch+".sha256", []byte(fmt.Sprintf("%s  %s\n", sum, filepath.Base(arch))), 0o644)
	}
	res.Status = StatusSuccess
	res.Archive = arch
	res.SHA256 = sum
	res.SizeBytes = written
	return *res
}

func collectPackages(stage, prefix string) error {
	out, err := os.ReadFile(filepath.Join(prefix, "var", "lib", "dpkg", "status"))
	if err != nil {
		// fallback: pkg list --manual
		cmd := exec.Command("pkg", "list", "--manual")
		if data, e := cmd.Output(); e == nil {
			return os.WriteFile(filepath.Join(stage, "packages-manual.txt"), data, 0o644)
		}
		return err
	}
	var names []string
	for _, block := range strings.Split(string(out), "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "Package: ") {
				names = append(names, strings.TrimPrefix(line, "Package: "))
			}
		}
	}
	header := "# Termux installed packages (from dpkg status)\n"
	body := strings.Join(names, "\n") + "\n"
	return os.WriteFile(filepath.Join(stage, "packages.txt"), []byte(header+body), 0o644)
}

func collectTermuxEnv(stage string) {
	var b strings.Builder
	for _, kv := range os.Environ() {
		if strings.Contains(strings.ToUpper(kv), "TERMUX") {
			b.WriteString(kv + "\n")
		}
	}
	if b.Len() == 0 {
		b.WriteString("# no TERMUX_* variables in environment\n")
	}
	_ = os.WriteFile(filepath.Join(stage, "termux-env-vars.txt"), []byte(b.String()), 0o644)
}

// createArchive tars source (file or dir) into dest/<name>-<ts>.tar.gz atomically.
// Returns archive path, sha256, optional manifest path.
func (r *Runner) createArchive(t config.Task, source, dest string, now time.Time) (string, string, string, error) {
	name := archiveName(t.Name, now)
	arch := filepath.Join(dest, name)
	part := arch + ".part" // .part lives in destination dir (same filesystem)

	files, err := walkFiles(source, t.Exclude)
	if err != nil {
		return "", "", "", err
	}

	needManifest := !supportsUnixPerms(dest)
	var manifestPath string
	var mf *os.File
	if needManifest {
		manifestPath = filepath.Join(dest, strings.TrimSuffix(name, ".tar.gz")+".manifest.json")
		mf, err = os.Create(manifestPath + ".part")
		if err != nil {
			return "", "", "", err
		}
	}

	_, err = writeTarGzWithManifest(part, source, files, needManifest, mf)
	if mf != nil {
		mf.Close()
	}
	if err != nil {
		os.Remove(part)
		if mf != nil {
			os.Remove(manifestPath + ".part")
		}
		return "", "", "", err
	}
	if err := os.Rename(part, arch); err != nil {
		os.Remove(part)
		return "", "", "", err
	}
	if needManifest {
		if err := os.Rename(manifestPath+".part", manifestPath); err != nil {
			slog.Warn("manifest rename failed", "err", err)
			manifestPath = ""
		}
	} else {
		manifestPath = ""
	}
	sum, err := sha256File(arch)
	if err != nil {
		return arch, "", manifestPath, nil
	}
	if err := os.WriteFile(arch+".sha256", []byte(fmt.Sprintf("%s  %s\n", sum, name)), 0o644); err != nil {
		slog.Warn("write sha256 failed", "err", err)
	}
	return arch, sum, manifestPath, nil
}

func archiveName(task string, now time.Time) string {
	return fmt.Sprintf("%s-%s.tar.gz", sanitize(task), now.UTC().Format("20060102-150405"))
}

func sanitize(s string) string {
	return strings.NewReplacer("/", "_", " ", "_", string(os.PathSeparator), "_").Replace(s)
}

// supportsUnixPerms probes whether a directory preserves file modes
// (Android shared storage / sdcard typically does not).
func supportsUnixPerms(dir string) bool {
	if err := paths.EnsureDir(dir); err != nil {
		return true
	}
	probe := filepath.Join(dir, ".hlops-perm-probe")
	if err := os.WriteFile(probe, []byte("x"), 0o600); err != nil {
		return true
	}
	defer os.Remove(probe)
	fi, err := os.Stat(probe)
	if err != nil {
		return true
	}
	return fi.Mode().Perm()&0o077 == 0 // 0600 preserved => unix perms supported
}

func walkFiles(root string, excludes []string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			slog.Warn("walk skip", "path", p, "err", err)
			if fi != nil && fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if matchesAny(excludes, p) {
			if fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil // never follow symlinks
		}
		if !fi.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out, err
}

func matchesAny(patterns []string, p string) bool {
	base := filepath.Base(p)
	for _, pat := range patterns {
		if ok, _ := path.Match(pat, base); ok {
			return true
		}
		if strings.Contains(p, pat) {
			return true
		}
	}
	return false
}

func writeTarGz(part, root string, files []string, withMode bool) (int64, error) {
	return writeTarGzWithManifest(part, root, files, false, nil)
}

func writeTarGzWithManifest(part, root string, files []string, wantManifest bool, mf *os.File) (int64, error) {
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	type mEntry struct {
		Path string `json:"path"`
		Mode uint32 `json:"mode"`
		UID  int    `json:"uid"`
		GID  int    `json:"gid"`
		Size int64  `json:"size"`
	}
	var manifest []mEntry

	for _, p := range files {
		fi, err := os.Lstat(p)
		if err != nil {
			continue
		}
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = filepath.Base(p)
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			f.Close()
			return 0, err
		}
		src, err := os.Open(p)
		if err != nil {
			slog.Warn("skip unreadable file", "path", p, "err", err)
			continue
		}
		if _, err := io.Copy(tw, src); err != nil {
			src.Close()
			tw.Close()
			gz.Close()
			return 0, err
		}
		src.Close()
		if wantManifest {
			uid, gid := -1, -1
			if st, ok := fi.Sys().(*syscall.Stat_t); ok {
					uid, gid = int(st.Uid), int(st.Gid)
			}
			manifest = append(manifest, mEntry{Path: rel, Mode: uint32(fi.Mode().Perm()), UID: uid, GID: gid, Size: fi.Size()})
		}
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		return 0, err
	}
	if err := gz.Close(); err != nil {
		return 0, err
	}
	if mf != nil {
		doc := map[string]any{
			"generated":  time.Now().UTC().Format(time.RFC3339),
			"note":       "destination volume does not support Unix permissions; restore with tar --no-same-owner then apply modes",
			"files":      manifest,
		}
		enc := json.NewEncoder(mf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			return 0, err
		}
	}
	fi, err := f.Stat()
	if err != nil {
		return 0, nil
	}
	return fi.Size(), nil
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

// applyRetention prunes old archives according to effective retention.
// Only files created by hlops (matching <task>-*.tar.gz) are removed.
func (r *Runner) applyRetention(t config.Task, dest string) []string {
	ret := r.Cfg.EffectiveRetention(t)
	if ret.Keep <= 0 && ret.RetentionDays <= 0 {
		return nil
	}
	prefix := sanitize(t.Name) + "-"
	candidates := listArchives(dest, prefix)
	var removed []string

	now := time.Now()
	for i, c := range candidates {
		dropByAge := false
		if ret.RetentionDays > 0 {
			if age := now.Sub(c.info.ModTime()); age > time.Duration(ret.RetentionDays)*24*time.Hour {
				dropByAge = true
			}
		}
		dropByKeep := ret.Keep > 0 && i >= ret.Keep
		if !(dropByAge || dropByKeep) {
			continue
		}
		if r.DryRun {
			removed = append(removed, c.path+" (would delete)")
			continue
		}
		if err := os.Remove(c.path); err == nil {
			removed = append(removed, c.path)
			_ = os.Remove(c.path + ".sha256")
			slog.Info("pruned old backup", "task", t.Name, "path", c.path)
		}
	}
	return removed
}

type archiveInfo struct {
	path string
	info os.FileInfo
}

func listArchives(dest, prefix string) []archiveInfo {
	entries, err := os.ReadDir(dest)
	if err != nil {
		return nil
	}
	var out []archiveInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}
		if !strings.HasPrefix(e.Name(), prefix) {
			continue // only prune hlops-created archives for this task
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, archiveInfo{filepath.Join(dest, e.Name()), fi})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].info.ModTime().After(out[j].info.ModTime()) })
	return out
}

// WriteReport persists the run report as JSON under the state dir.
func WriteReport(rep *Report) (string, error) {
	dir := paths.ReportDir()
	if err := paths.EnsureDir(dir); err != nil {
		return "", err
	}
	file := filepath.Join(dir, fmt.Sprintf("backup-%s.json", time.Now().UTC().Format("20060102-150405")))
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return "", err
	}
	return file, nil
}
