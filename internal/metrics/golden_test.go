package metrics

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/4ster-light/go-pane/internal/opencode"
)

var update = flag.Bool("update", false, "update golden files")

// TestGoldenSnapshot pins the computed snapshot for the recorded API sample.
// Regenerate with: go test ./internal/metrics -run TestGoldenSnapshot -update
func TestGoldenSnapshot(t *testing.T) {
	body, err := os.ReadFile("../../testdata/usage.sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Usage opencode.Usage `json:"usage"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	now, err := time.Parse(time.RFC3339, "2026-10-06T08:40:48Z")
	if err != nil {
		t.Fatal(err)
	}

	snap := Compute(now, env.Usage, DefaultLengths())
	got, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	const golden = "../../testdata/snapshot.golden.json"
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run `go test ./internal/metrics -run TestGoldenSnapshot -update`): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("snapshot differs from %s:\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
	}
}
