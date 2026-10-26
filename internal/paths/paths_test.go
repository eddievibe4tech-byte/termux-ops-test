package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eddievibe4tech-byte/termux-ops/internal/termux"
)

func TestResolveTilde(t *testing.T) {
	t.Setenv("HOME", "/fake/home")
	got := Resolve("~/storage/shared")
	want := filepath.Join("/fake/home", "storage/shared")
	if got != want {
		t.Fatalf("Resolve tilde = %q want %q", got, want)
	}
}

func TestResolveEnvHome(t *testing.T) {
	t.Setenv("HOME", "/fake/home")
	if got := Resolve("$HOME/x"); got != "/fake/home/x" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAbsolutePassthrough(t *testing.T) {
	if got := Resolve("/data/data/com.termux/files/home/y"); got != "/data/data/com.termux/files/home/y" {
		t.Fatalf("got %q", got)
	}
}

func TestWritable(t *testing.T) {
	dir := t.TempDir()
	if !Writable(dir) {
		t.Fatal("expected writable")
	}
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	if Writable(ro) && os.Geteuid() != 0 {
		t.Fatal("expected non-writable")
	}
}

func TestHomeDisplayTildeWhenEnvMatchesUserHome(t *testing.T) {
	uh, err := os.UserHomeDir()
	if err != nil || uh == "" {
		t.Skip("no OS user home available in this environment")
	}
	SetHomeForTest(uh)
	if got := HomeDisplay(); got != "~" {
		t.Fatalf("HomeDisplay = %q want ~", got)
	}
}

func TestHomeDisplayAbsoluteForForeignHome(t *testing.T) {
	fake := filepath.Join(string(filepath.Separator), "fake", "termux", "home")
	SetHomeForTest(fake)
	if got := HomeDisplay(); got != fake {
		t.Fatalf("HomeDisplay = %q want %q (foreign HOME must stay absolute)", got, fake)
	}
}

func TestHomeDisplayTildeInsideTermux(t *testing.T) {
	// Inside Termux the shell owns $HOME, so "~" is unambiguous even though
	// it differs from the Android system account home.
	t.Setenv("PREFIX", "/data/data/com.termux/files/usr")
	termux.ResetCache()
	t.Cleanup(termux.ResetCache)
	SetHomeForTest("/data/data/com.termux/files/home")
	if got := HomeDisplay(); got != "~" {
		t.Fatalf("HomeDisplay = %q want ~ inside Termux", got)
	}
}

func TestDisplayPath(t *testing.T) {
	fake := filepath.Join(string(filepath.Separator), "fake", "home")
	SetHomeForTest(fake)
	for _, tc := range []struct{ in, want string }{
		{filepath.Join(fake, ".local", "state", "hlops", "logs"), HomeDisplay() + string(filepath.Separator) + filepath.Join(".local", "state", "hlops", "logs")},
		{fake, HomeDisplay()},
		{"/totally/other/path", "/totally/other/path"},
		{"", ""},
	} {
		if got := DisplayPath(tc.in); got != tc.want {
			t.Fatalf("DisplayPath(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestTermuxHomeVariants(t *testing.T) {
	vs := TermuxHomeVariants()
	if len(vs) < 2 {
		t.Fatalf("expected multiple variants, got %v", vs)
	}
	if vs[0] != TermuxHome {
		t.Fatalf("first variant must be the regular build, got %q", vs[0])
	}
	seen := map[string]bool{}
	for _, v := range vs {
		if seen[v] {
			t.Fatalf("duplicate variant %q", v)
		}
		seen[v] = true
	}
}

func TestHomeRel(t *testing.T) {
	got := HomeRel("/home/u", ".local", "state", "hlops", "reports")
	want := filepath.Join("/home/u", ".local", "state", "hlops", "reports")
	if got != want {
		t.Fatalf("HomeRel = %q want %q", got, want)
	}
}
