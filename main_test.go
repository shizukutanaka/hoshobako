package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	s := &server{store: &ledger.Store{Path: filepath.Join(t.TempDir(), "hoshobako.json")}}
	return s.routes()
}

func doReq(t *testing.T, h http.Handler, method, path, host, origin, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGuardAllowsLocalhostGet(t *testing.T) {
	rec := doReq(t, testHandler(t), "GET", "/api/items", "127.0.0.1:8771", "", "")
	if rec.Code != 200 {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

func TestGuardRejectsForeignHost(t *testing.T) {
	rec := doReq(t, testHandler(t), "GET", "/api/items", "evil.example.com", "", "")
	if rec.Code != 403 {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestGuardRejectsForeignOrigin(t *testing.T) {
	rec := doReq(t, testHandler(t), "POST", "/api/items", "127.0.0.1:8771", "http://evil.example.com", "{}")
	if rec.Code != 403 {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestGuardRejectsMalformedOrigin(t *testing.T) {
	rec := doReq(t, testHandler(t), "POST", "/api/items", "127.0.0.1:8771", "http://[::1", "{}")
	if rec.Code != 403 {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestGuardAllowsMatchingOrigin(t *testing.T) {
	rec := doReq(t, testHandler(t), "POST", "/api/items", "127.0.0.1:8771", "http://127.0.0.1:8771", "{}")
	if rec.Code == 403 {
		t.Fatalf("got 403 for matching origin, want non-403 (got %d)", rec.Code)
	}
}

func TestGuardAllowsMissingOrigin(t *testing.T) {
	rec := doReq(t, testHandler(t), "POST", "/api/items", "127.0.0.1:8771", "", "{}")
	if rec.Code == 403 {
		t.Fatalf("got 403 for missing origin, want non-403 (got %d)", rec.Code)
	}
}
