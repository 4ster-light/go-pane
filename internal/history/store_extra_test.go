package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/4ster-light/go-pane/internal/metrics"
	"github.com/4ster-light/go-pane/internal/opencode"
)

func TestLearnedLengthsPersistAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	mustRecord(t, s, base, opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 1, ResetsAt: base.Add(5 * time.Hour)},
	})
	mustRecord(t, s, base.Add(time.Hour), opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 2, ResetsAt: base.Add(10 * time.Hour)},
	})

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	l := reopened.Lengths()
	if l.Estimated[metrics.Rolling] {
		t.Error("learned rolling length should persist across Open")
	}
	if got := l.Durations[metrics.Rolling]; got != 5*time.Hour {
		t.Errorf("rolling length = %v, want 5h", got)
	}
}

func TestLoadLastSampleSkipsCorruptTrailingLine(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	mustRecord(t, s, base, opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 1, ResetsAt: base.Add(5 * time.Hour)},
	})

	f, err := os.OpenFile(filepath.Join(dir, "history.jsonl"), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not json}\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Advancing the reset by 5h should still be learned from the valid line.
	mustRecord(t, reopened, base.Add(time.Hour), opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 2, ResetsAt: base.Add(10 * time.Hour)},
	})
	if got := reopened.Lengths().Durations[metrics.Rolling]; got != 5*time.Hour {
		t.Errorf("rolling length = %v, want 5h learned via the valid line", got)
	}
}

func TestLoadLengthsMalformedFallsBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "windows.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Lengths().Estimated[metrics.Rolling] {
		t.Error("malformed windows.json should fall back to estimated defaults")
	}
}

func TestLoadLengthsValid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "windows.json"), []byte(`{"durationsSeconds":{"rolling":9000}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	l := s.Lengths()
	if l.Durations[metrics.Rolling] != 9000*time.Second || l.Estimated[metrics.Rolling] {
		t.Errorf("learned length = %v estimated=%v", l.Durations[metrics.Rolling], l.Estimated[metrics.Rolling])
	}
}

func TestPlausibleWindowTable(t *testing.T) {
	cases := []struct {
		kind metrics.Kind
		d    time.Duration
		want bool
	}{
		{metrics.Rolling, 5 * time.Hour, true},
		{metrics.Rolling, 10 * time.Hour, false},
		{metrics.Rolling, 3 * time.Hour, false},
		{metrics.Weekly, 7 * 24 * time.Hour, true},
		{metrics.Weekly, 5 * 24 * time.Hour, false},
		{metrics.Monthly, 30 * 24 * time.Hour, true},
		{metrics.Monthly, 20 * 24 * time.Hour, false},
		{metrics.Kind("bogus"), 5 * time.Hour, false},
	}
	for _, c := range cases {
		if got := plausibleWindow(c.kind, c.d); got != c.want {
			t.Errorf("plausibleWindow(%q, %v) = %v, want %v", c.kind, c.d, got, c.want)
		}
	}
}

func TestRecentLimitZeroReturnsAll(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		mustRecord(t, s, base.Add(time.Duration(i)*time.Minute), opencode.Usage{})
	}
	if got := len(s.Recent(0)); got != 3 {
		t.Errorf("Recent(0) = %d, want 3", got)
	}
}

func TestReadLinesMissingFile(t *testing.T) {
	lines, err := readLines(filepath.Join(t.TempDir(), "absent"))
	if err != nil || lines != nil {
		t.Errorf("readLines(missing) = %v, %v; want nil, nil", lines, err)
	}
}

func TestTrimFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(path, []byte("1\n2\n3\n4\n5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := trimFile(path, 0, 2); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "4\n5\n" {
		t.Errorf("trimmed file = %q, want %q", got, "4\n5\n")
	}

	// Below threshold is a no-op.
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := trimFile(path, 1<<20, 1); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "a\nb\nc\n" {
		t.Errorf("below-threshold file changed: %q", got)
	}

	// Missing file is a no-op.
	if err := trimFile(filepath.Join(t.TempDir(), "absent"), 0, 1); err != nil {
		t.Errorf("trimFile(missing) = %v, want nil", err)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := writeFileAtomic(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "one" {
		t.Errorf("content = %q, want one", got)
	}
	if err := writeFileAtomic(path, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "two" {
		t.Errorf("content = %q, want two", got)
	}
}

func TestRecordFailsOnMissingDir(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := s.Record(time.Now(), opencode.Usage{}); err == nil {
		t.Error("Record should fail when the state dir is gone")
	}
}
