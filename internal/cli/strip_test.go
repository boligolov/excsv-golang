package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataPrintSidecarNotice(t *testing.T) {
	root := repoRoot(t)
	bin := ensureExcsvBinary(t, root)
	sidecar := filepath.Join(root, "test", "fixtures", "plain", "valid", "037_sidecar_csv_sibling.excsv")
	if _, err := os.Stat(sidecar); err != nil {
		t.Skip("fixtures not synced")
	}
	cmd := exec.Command(bin, sidecar, "data", "print")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("data print sidecar: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "sidecar") {
		t.Fatalf("expected sidecar notice, stderr=%q", stderr.String())
	}
}
