package termux

import "testing"

func TestDetectPlainLinuxNotTermux(t *testing.T) {
	ResetCache()
	t.Setenv("TERMUX_VERSION", "")
	t.Setenv("PREFIX", "/usr")
	t.Setenv("HOME", "/home/user")
	info := Detect()
	if info.IsTermux {
		t.Fatalf("plain linux misdetected as termux")
	}
}

func TestDetectViaVersionEnv(t *testing.T) {
	ResetCache()
	t.Setenv("TERMUX_VERSION", "0.118.0")
	t.Setenv("HOME", "/data/data/com.termux/files/home")
	t.Setenv("PREFIX", "/data/data/com.termux/files/usr")
	info := Detect()
	if !info.IsTermux {
		t.Fatalf("expected termux")
	}
	if info.Version != "0.118.0" {
		t.Fatalf("version = %q", info.Version)
	}
	if info.Prefix != "/data/data/com.termux/files/usr" {
		t.Fatalf("prefix = %q", info.Prefix)
	}
}

func TestDetectViaHomePath(t *testing.T) {
	ResetCache()
	t.Setenv("TERMUX_VERSION", "")
	t.Setenv("PREFIX", "")
	t.Setenv("HOME", "/data/data/com.termux/files/home")
	info := Detect()
	if !info.IsTermux {
		t.Fatalf("expected termux via home path")
	}
	if info.Prefix != constTermuxPrefix {
		t.Fatalf("fallback prefix = %q", info.Prefix)
	}
}
