package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	root := NewRootCommand(BuildInfo{Version: "v1.2.3", Commit: "abc123", Date: "2026-01-01"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	got := out.String()
	for _, want := range []string{"v1.2.3", "abc123", "2026-01-01"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q does not contain %q", got, want)
		}
	}
}
