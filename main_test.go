package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"hoshobako/internal/ledger"
)

func buildHoshobako(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "hoshobako")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return exe
}

func TestCLIExportDefaultsToCSV(t *testing.T) {
	dataDir := t.TempDir()
	st := &ledger.Store{Path: filepath.Join(dataDir, "hoshobako.json")}
	if err := os.MkdirAll(st.PhotosDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	entries := []ledger.Item{
		{Name: "Blender", Category: ledger.CategoryWarranty, Store: "Denkiya", Purchased: "2025-03-10", Months: 24},
		{Name: "Milk, fresh", Category: ledger.CategoryFood, Store: "Corner \"Mart\"", Expires: "2026-11-25"},
		{Name: "Passport", Category: ledger.CategoryDocument, Expires: "2030-06-30"},
	}
	for _, it := range entries {
		if _, err := st.Add(it); err != nil {
			t.Fatal(err)
		}
	}

	exe := buildHoshobako(t)
	run := func(args ...string) (string, string, error) {
		cmd := exec.Command(exe, args...)
		cmd.Env = append(os.Environ(), "HOSHOBAKO_DATA_DIR="+dataDir)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}

	stdout, stderr, err := run("export")
	if err != nil {
		t.Fatalf("export: %v\n%s", err, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != len(entries)+1 {
		t.Fatalf("stdout has %d lines, want header plus %d entries:\n%s", len(lines), len(entries), stdout)
	}
	if lines[0] != "name,category,store,model,price,purchased,warranty_months,expires,notes" {
		t.Fatalf("CSV header = %q", lines[0])
	}
	for _, want := range []string{"\"Milk, fresh\"", "\"Corner \"\"Mart\"\"\"", "Blender", "Passport"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}

	outPath := filepath.Join(t.TempDir(), "ledger.csv")
	fileStdout, stderr, err := run("export", outPath)
	if err != nil {
		t.Fatalf("export to file: %v\n%s", err, stderr)
	}
	if fileStdout != "" {
		t.Fatalf("export to file wrote to stdout: %q", fileStdout)
	}
	fileRaw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(fileRaw) != stdout {
		t.Fatalf("file output does not match stdout:\nfile:\n%s\nstdout:\n%s", fileRaw, stdout)
	}

	explicit, stderr, err := run("export", "csv")
	if err != nil {
		t.Fatalf("export csv: %v\n%s", err, stderr)
	}
	if explicit != stdout {
		t.Fatalf("explicit CSV output differs from default:\nexplicit:\n%s\ndefault:\n%s", explicit, stdout)
	}
}
