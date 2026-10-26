package termination

import (
	"strings"
	"syscall"
	"testing"
)

func TestClassifyExitInterrupted(t *testing.T) {
	// Signal name is derived from the exit code (no globals involved). Go's
	// syscall.Signal.String() returns lowercase names ("terminated",
	// "interrupt"); what matters is they come from the code, not stale state.
	got := ClassifyExit(ExitSignalBase+int(syscall.SIGTERM), true, false)
	if !strings.Contains(got, "forcibly terminated") || !strings.Contains(got, "terminated") ||
		!strings.Contains(got, "OEM battery settings") {
		t.Errorf("generic interrupted msg = %q", got)
	}
	gotInt := ClassifyExit(ExitSignalBase+int(syscall.SIGINT), true, false)
	if !strings.Contains(gotInt, "interrupt") {
		t.Errorf("SIGINT interrupted msg = %q, want signal name derived from code", gotInt)
	}
	gotAgg := ClassifyExit(ExitSignalBase+int(syscall.SIGTERM), true, true)
	if !strings.Contains(gotAgg, "aggressive OEM power manager detected") {
		t.Errorf("aggressive interrupted msg = %q", gotAgg)
	}
	// Guard: an out-of-range code must not panic and should say "unknown".
	if got := ClassifyExit(1, true, false); !strings.Contains(got, "exit 1") {
		t.Errorf("weird interrupted code msg = %q", got)
	}
}

func TestClassifyExitKnownCodes(t *testing.T) {
	cases := []struct {
		code  int
		wantS string
	}{
		{137, "SIGKILLed"},
		{2, "configuration syntax error"},
		{5, "holds the lock"},
		{6, "holds the lock"},
		{1, "returned error"},
	}
	for _, tc := range cases {
		got := ClassifyExit(tc.code, false, false)
		if !strings.Contains(got, tc.wantS) {
			t.Errorf("ClassifyExit(%d) = %q, want substring %q", tc.code, got, tc.wantS)
		}
		if got == "" {
			t.Errorf("ClassifyExit(%d) returned empty string", tc.code)
		}
	}
	// SIGKILL + aggressive ROM adds checklist pointer.
	if got := ClassifyExit(137, false, true); !strings.Contains(got, "survival checklist") {
		t.Errorf("aggressive SIGKILL msg = %q", got)
	}
}

func TestDiagnoseMissingReport(t *testing.T) {
	base := DiagnoseMissingReport("backup", false)
	if !strings.Contains(base, `"backup"`) || !strings.Contains(base, "crond") {
		t.Errorf("non-aggressive diagnosis = %q", base)
	}
	agg := DiagnoseMissingReport("backup", true)
	if !strings.Contains(agg, "Aggressive OEM power management") || !strings.Contains(agg, "dontkillmyapp.com") {
		t.Errorf("aggressive diagnosis = %q", agg)
	}
}

func TestExitSignalBaseConvention(t *testing.T) {
	// Shell convention: 128 + signum. Guard against accidental changes that
	// would break Companion APK / cron exit-code parsing.
	if ExitSignalBase != 128 {
		t.Errorf("ExitSignalBase = %d, want 128", ExitSignalBase)
	}
	if ExitSignalBase+int(syscall.SIGKILL) != 137 {
		t.Error("SIGKILL exit code must be 137")
	}
}
