package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLog is a helper that writes content to a temp file and returns its path.
func writeLog(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hlops-test.log")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTailLinesRingBufferOrdering(t *testing.T) {
	// 10 lines, ask for the last 3 -> must be lines 8,9,10 in order.
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		b.WriteString("line")
		b.WriteString(fmt.Sprint(i))
		b.WriteString("\n")
	}
	got, truncated, err := tailLines(writeLog(t, b.String()), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("truncated = false, want true")
	}
	want := []string{"line8", "line9", "line10"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestTailLinesNoWrap(t *testing.T) {
	got, truncated, err := tailLines(writeLog(t, "a\nb\nc\n"), 5)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("truncated = true, want false")
	}
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("got %v want [a b c]", got)
	}
}

func TestTailLinesExactFill(t *testing.T) {
	// n == maxLines: no truncation, full contents in order.
	got, truncated, err := tailLines(writeLog(t, "1\n2\n3\n"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("truncated = true, want false")
	}
	if strings.Join(got, ",") != "1,2,3" {
		t.Fatalf("got %v", got)
	}
}

func TestTailLinesEmptyAndBlank(t *testing.T) {
	got, _, err := tailLines(writeLog(t, ""), 10)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("got nil, want non-nil empty slice (JSON array invariant)")
	}
	if len(got) != 0 {
		t.Fatalf("got %v want []", got)
	}
	// Trailing blank lines are trimmed.
	got, _, err = tailLines(writeLog(t, "x\n\n\n"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "x" {
		t.Fatalf("got %v want [x]", got)
	}
}

func TestTailLinesLongLineOverDefault64KB(t *testing.T) {
	// A single line of ~100KB would kill the default bufio.Scanner with
	// ErrTooLong; the 1MB buffer must let it through intact.
	long := strings.Repeat("L", 100*1024)
	got, truncated, err := tailLines(writeLog(t, long+"\nlast\n"), 200)
	if err != nil {
		t.Fatalf("tailLines failed on >64KB line (regression): %v", err)
	}
	if truncated {
		t.Fatal("truncated = true, want false")
	}
	if len(got) != 2 || got[0] != long || got[1] != "last" {
		t.Fatalf("unexpected result: len=%d", len(got))
	}
}

func TestTailLinesMaxLinesNonPositive(t *testing.T) {
	got, truncated, err := tailLines(writeLog(t, "a\nb\n"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(got) != 0 || got == nil {
		t.Fatalf("got %v/%v want []/false", got, truncated)
	}
}
