// Package cleanup implements native cleanup actions (temp files, old
// backups) and provider cleanups (pkg/apt caches) behind a common Action
// model with risk levels and enable/disable state.
package cleanup

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eddievibe4tech-byte/termux-ops/internal/config"
	"github.com/eddievibe4tech-byte/termux-ops/internal/paths"
	"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

// Risk classifies how dangerous an action is.
type Risk string

const (
	RiskSafe     Risk = "safe"
	RiskModerate Risk = "moderate"
	RiskDangerous Risk = "dangerous"
)

// Result status values.
const (
	StatusDone                 = "done"
	StatusDryRun               = "dry-run"
	StatusSkippedDisabled      = "skipped_disabled"
	StatusSkippedDependencyMissing = "skipped_dependency_missing"
	StatusFailed               = "failed"
)

// Action is one cleanup step.
type Action struct {
	ID          string
	Title       string
	Risk        Risk
	Enabled     bool
	NeedsTermux bool
	Run         func(a *Action, dryRun bool) Result
}

// Result is the outcome of an action.
type Result struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Risk          string   `json:"risk"`
	Status        string   `json:"status"`
	FreedBytes    int64    `json:"freed_bytes"`
	FilesRemoved  []string `json:"files_removed,omitempty"`
	CommandShown  string   `json:"command,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// Report aggregates action results.
type Report struct {
	Started  string   `json:"started"`
	Finished string   `json:"finished"`
	DryRun   bool     `json:"dry_run"`
	Results  []Result `json:"results"`
}

// Planner builds the action list from config + environment.
type Planner struct {
	Cfg    *config.Config
	DryRun bool
	AllowDangerous bool
}

// Actions returns all known cleanup actions in order.
func (p *Planner) Actions() []Action {
	info := termux.Detect()
	yes := true
	native := p.Cfg.Cleanup.Native
	if native.TmpEnabled == nil {
		native.TmpEnabled = &yes
	}
	if native.OldBackupsEnabled == nil {
		native.OldBackupsEnabled = &yes
	}

	acts := []Action{
		{
			ID: "tmp", Title: "Remove temp files older than threshold", Risk: RiskSafe,
			Enabled: *native.TmpEnabled, Run: func(a *Action, dry bool) Result {
				return p.cleanTmp(dry, native.TmpOlderThan)
			},
		},
		{
			ID: "old-backups", Title: "Prune old hlops backups beyond retention", Risk: RiskModerate,
			Enabled: *native.OldBackupsEnabled, Run: func(a *Action, dry bool) Result {
				return p.pruneOldBackups(dry)
			},
		},
		{
			ID: "pkg-cache", Title: "Clean Termux pkg cache (pkg clean / apt-get clean)", Risk: RiskSafe,
			Enabled: p.Cfg.Cleanup.Providers.Pkg, NeedsTermux: false,
			Run: func(a *Action, dry bool) Result { return p.providerClean("pkg", dry) },
		},
		{
			ID: "apt-cache", Title: "Clean apt cache (apt-get clean)", Risk: RiskSafe,
			Enabled: p.Cfg.Cleanup.Providers.Apt,
			Run: func(a *Action, dry bool) Result { return p.providerClean("apt", dry) },
		},
	}
	_ = info
	return acts
}

// RunAll executes enabled actions; dangerous ones unless explicitly allowed.
func (p *Planner) RunAll() *Report {
	rep := &Report{Started: time.Now().UTC().Format(time.RFC3339), DryRun: p.DryRun}
	for _, a := range p.Actions() {
		var res Result
		switch {
		case !a.Enabled:
			res = Result{ID: a.ID, Title: a.Title, Risk: string(a.Risk), Status: StatusSkippedDisabled}
		case a.Risk == RiskDangerous && !p.AllowDangerous:
			res = Result{ID: a.ID, Title: a.Title, Risk: string(a.Risk), Status: StatusSkippedDisabled,
				Error: "dangerous action skipped (use --allow-dangerous)"}
		default:
			res = a.Run(&a, p.DryRun)
			res.ID, res.Title, res.Risk = a.ID, a.Title, string(a.Risk)
		}
		rep.Results = append(rep.Results, res)
	}
	rep.Finished = time.Now().UTC().Format(time.RFC3339)
	return rep
}

// cleanTmp removes stale files from $PREFIX/tmp and $TMPDIR based on mtime.
// Files that are busy/in use are skipped (EBUSY/EACCES handling).
func (p *Planner) cleanTmp(dry bool, olderThanDays int) Result {
	if olderThanDays <= 0 {
		olderThanDays = 7
	}
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	dirs := tmpDirs()
	res := Result{Status: StatusDone}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // missing dir is fine
		}
		for _, e := range entries {
			full := filepath.Join(dir, e.Name())
			if full == dir {
				continue
			}
			fi, err := e.Info()
			if err != nil || fi.ModTime().After(cutoff) {
				continue
			}
			size := fileSize(full)
			if dry {
				res.FilesRemoved = append(res.FilesRemoved, full+" (would delete)")
				res.FreedBytes += size
				continue
			}
			var rmErr error
			if fi.IsDir() {
				rmErr = os.RemoveAll(full)
			} else {
				rmErr = os.Remove(full)
			}
			if rmErr != nil {
				// busy or permission problems: skip and log, never fail the run
				slog.Warn("cleanup skip (in use or protected)", "path", full, "err", rmErr)
				continue
			}
			res.FilesRemoved = append(res.FilesRemoved, full)
			res.FreedBytes += size
		}
	}
	if dry {
		res.Status = StatusDryRun
	}
	return res
}

func tmpDirs() []string {
	seen := map[string]bool{}
	var out []string
	add := func(d string) {
		if d == "" {
			return
		}
		d = filepath.Clean(d)
		if seen[d] {
			return
		}
		seen[d] = true
		out = append(out, d)
	}
	info := termux.Detect()
	if info.IsTermux && info.Prefix != "" {
		add(filepath.Join(info.Prefix, "tmp"))
	}
	add(os.Getenv("TMPDIR"))
	return out
}

func fileSize(p string) int64 {
	fi, err := os.Stat(p)
	if err != nil {
		return 0
	}
	if !fi.IsDir() {
		return fi.Size()
	}
	var total int64
	_ = filepath.Walk(p, func(_ string, i os.FileInfo, err error) error {
		if err == nil && !i.IsDir() {
			total += i.Size()
		}
		return nil
	})
	return total
}

// pruneOldBackups deletes archives that violate each task's retention but
// keeps the newest Keep-per-task files. Only hlops-created names match.
func (p *Planner) pruneOldBackups(dry bool) Result {
	res := Result{Status: StatusDone}
	seen := map[string]bool{}
	for _, t := range p.Cfg.Backup.Tasks {
		dest := paths.Resolve(t.Destination)
		if dest == "" || seen[dest+":"+t.Name] {
			continue
		}
		seen[dest+":"+t.Name] = true
		ret := p.Cfg.EffectiveRetention(t)
		if ret.Keep <= 0 && ret.RetentionDays <= 0 {
			continue
		}
		prefix := sanitizeName(t.Name) + "-"
		cands := listHlopsArchives(dest, prefix)
		now := time.Now()
		for i, c := range cands {
			dropAge := ret.RetentionDays > 0 && now.Sub(c.info.ModTime()) > time.Duration(ret.RetentionDays)*24*time.Hour
			dropKeep := ret.Keep > 0 && i >= ret.Keep
			if !(dropAge || dropKeep) {
				continue
			}
			sz := c.info.Size()
			if dry {
				res.FilesRemoved = append(res.FilesRemoved, c.path+" (would delete)")
				res.FreedBytes += sz
				continue
			}
			if err := os.Remove(c.path); err == nil {
				_ = os.Remove(c.path + ".sha256")
				res.FilesRemoved = append(res.FilesRemoved, c.path)
				res.FreedBytes += sz
			}
		}
	}
	if dry {
		res.Status = StatusDryRun
	}
	return res
}

func sanitizeName(s string) string {
	return strings.NewReplacer("/", "_", " ", "_").Replace(s)
}

type arch struct {
	path string
	info os.FileInfo
}

func listHlopsArchives(dest, prefix string) []arch {
	entries, err := os.ReadDir(dest)
	if err != nil {
		return nil
	}
	var out []arch
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, arch{filepath.Join(dest, e.Name()), fi})
	}
	// newest first
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].info.ModTime().After(out[i].info.ModTime()) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// providerClean probes dependency availability then runs the cache clean
// command. Missing dependencies yield skipped_dependency_missing, never a
// hard failure.
func (p *Planner) providerClean(which string, dry bool) Result {
	res := Result{Status: StatusDone}
	info := termux.Detect()
	var cmdName string
	var args []string

	switch which {
	case "pkg":
		if binPath("pkg") != "" {
			cmdName = "pkg"
			args = []string{"clean", "-y"}
		} else if binPath("apt-get") != "" {
			cmdName = "apt-get"
			args = []string{"clean"}
		} else {
			res.Status = StatusSkippedDependencyMissing
			res.Error = "pkg/apt-get not found"
			return res
		}
	case "apt":
		if binPath("apt-get") != "" {
			cmdName = "apt-get"
			args = []string{"clean"}
		} else {
			res.Status = StatusSkippedDependencyMissing
			res.Error = "apt-get not found"
			return res
		}
	}
	res.CommandShown = cmdName + " " + strings.Join(args, " ")
	if dry {
		res.Status = StatusDryRun
		return res
	}
	cmd := exec.Command(cmdName, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if info.IsTermux {
		cmd.Env = append(os.Environ(), "APT_NO_LOCK=1")
	}
	if err := cmd.Run(); err != nil {
		// treat failures as warnings, not fatal errors
		res.Status = StatusFailed
		res.Error = err.Error()
		slog.Warn("provider clean failed", "cmd", res.CommandShown, "err", err)
	}
	return res
}

func binPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// Probe describes availability of a package manager provider.
type Probe struct {
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
}

// ProbeProviders detects pkg/apt presence without failing when absent.
func ProbeProviders() []Probe {
	out := []Probe{}
	for _, name := range []string{"pkg", "apt", "apt-get"} {
		p := binPath(name)
		pr := Probe{Name: name, Path: p, Available: p != ""}
		if pr.Available {
			if v, err := exec.Command(name, "--version").Output(); err == nil {
				lines := strings.SplitN(strings.TrimSpace(string(v)), "\n", 2)
				if len(lines) > 0 {
					pr.Version = lines[0]
				}
			}
		}
		out = append(out, pr)
	}
	return out
}

// isBusy reports whether err looks like a "resource busy / in use" or
// permission problem, in which case cleanup skips the entry instead of
// failing. Implemented via error strings so it works without importing
// syscall (keeps the package cross-platform buildable).
func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, s := range []string{"device or resource busy", "permission denied", "operation not permitted"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
