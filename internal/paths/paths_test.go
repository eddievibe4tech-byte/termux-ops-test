package paths

import (
	"os"
	"path/filepath"
	"testing"
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
