// Package analyze walks directory trees safely and produces size reports.
// It never follows symlinks (so cycles cannot hang it), skips virtual
// filesystems (/proc, /sys, /dev), supports a wall-clock timeout, and
// records permission-denied entries instead of failing.
package analyze

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Entry is one file or aggregated directory.
type Entry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Mtime string `json:"mtime,omitempty"`
	IsDir bool   `json:"is_dir"`
}

// Volume describes free/total space of a mount point.
type Volume struct {
	Path       string `json:"path"`
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	Available  uint64 `json:"available_bytes"`
	Error      string `json:"error,omitempty"`
}

// Skipped records an entry that could not be read.
type Skipped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Result is the report for one scanned root.
type Result struct {
	Root          string    `json:"root"`
	TotalSize     int64     `json:"total_size"`
	FileCount     int       `json:"file_count"`
	DirCount      int       `json:"dir_count"`
	Directories   []Entry   `json:"directories"`
	Files         []Entry   `json:"files"`
	Volumes       []Volume  `json:"volumes"`
	Skipped       []Skipped `json:"skipped,omitempty"`
	TimedOut      bool      `json:"timed_out"`
	ScanSeconds   float64   `json:"scan_seconds"`
}

// Options controls a scan.
type Options struct {
	TopN    int
	Timeout time.Duration
}

var deniedPrefixes = []string{"/proc", "/sys", "/dev", "/data/adb"}

// Scan walks root with safety limits. Symlinks are recorded but never
// followed, so symlink loops cannot cause infinite recursion.
func Scan(ctx context.Context, root string, opt Options) (*Result, error) {
	if opt.TopN <= 0 {
		opt.TopN = 15
	}
	if opt.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.Timeout)
		defer cancel()
	}
	res := &Result{Root: root}
	start := time.Now()

	dirSizes := map[string]int64{}
	var files []Entry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			var perr *fs.PathError
			if errors.As(err, &perr) {
				res.Skipped = append(res.Skipped, Skipped{Path: perr.Path, Reason: perr.Err.Error()})
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return err
		}
		// Never descend into virtual filesystems if someone points at them.
		for _, p := range deniedPrefixes {
			if path == p || (len(path) > len(p) && path[:len(p)+1] == p+string(os.PathSeparator)) {
				if d.IsDir() {
					res.Skipped = append(res.Skipped, Skipped{Path: path, Reason: "virtual filesystem skipped"})
					return filepath.SkipDir
				}
				return nil
			}
		}
		info, ierr := d.Info()
		if ierr != nil {
			res.Skipped = append(res.Skipped, Skipped{Path: path, Reason: ierr.Error()})
			return nil
		}
		switch {
		case d.Type()&os.ModeSymlink != 0:
			// record but do not follow
			return nil
		case d.IsDir():
			res.DirCount++
			if _, ok := dirSizes[path]; !ok {
				dirSizes[path] = 0
			}
		default:
			res.FileCount++
			size := info.Size()
			res.TotalSize += size
			files = append(files, Entry{Path: path, Size: size, Mtime: info.ModTime().Format(time.RFC3339)})
			// add size to all ancestor dirs inside root
			for p := filepath.Dir(path); p != "" && p != filepath.Dir(p); p = filepath.Dir(p) {
				cur := dirSizes[p] + size
				dirSizes[p] = cur
				if p == root {
					break
				}
			}
		}
		return nil
	})

	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		res.TimedOut = true
	} else if err != nil {
		return res, err
	}

	var dirs []Entry
	for p, sz := range dirSizes {
		if p == root {
			continue
		}
		dirs = append(dirs, Entry{Path: p, Size: sz, IsDir: true})
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Size > dirs[j].Size })
	sort.Slice(files, func(i, j int) bool { return files[i].Size > files[j].Size })
	if len(dirs) > opt.TopN {
		dirs = dirs[:opt.TopN]
	}
	if len(files) > opt.TopN {
		files = files[:opt.TopN]
	}
	res.Directories = dirs
	res.Files = files
	res.Volumes = []Volume{statfs(root)}
	res.ScanSeconds = time.Since(start).Seconds()
	return res, nil
}

