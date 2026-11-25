package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"hoshobako/internal/ledger"
)

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
