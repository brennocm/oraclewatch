package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	tableStart = "<!-- checks:start -->"
	tableEnd   = "<!-- checks:end -->"
)

// CatalogTable renders the README checks table from Catalog.
func CatalogTable() string {
	var b strings.Builder
	b.WriteString("| ID | Check | Max severity | Detects | Status |\n|---|---|---|---|---|\n")
	for _, m := range Catalog {
		status := "stable"
		if !m.Stable {
			status = "experimental (off by default)"
		}
		b.WriteString("| [`" + m.ID + "`](docs/checks/" + m.ID + ".md) | " + m.Title + " | " + m.Max.String() + " | " + m.Detects + " | " + status + " |\n")
	}
	return b.String()
}

// TestReadmeTableMatchesCatalog keeps the README table generated from code.
// On failure it prints the expected table to paste between the markers.
func TestReadmeTableMatchesCatalog(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i, j := strings.Index(s, tableStart), strings.Index(s, tableEnd)
	if i < 0 || j < i {
		t.Fatalf("README.md has no %s ... %s block", tableStart, tableEnd)
	}
	got := strings.TrimSpace(s[i+len(tableStart) : j])
	want := strings.TrimSpace(CatalogTable())
	if got != want {
		t.Fatalf("README checks table is out of date; replace it with:\n\n%s", want)
	}
}
