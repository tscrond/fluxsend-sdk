package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadCLIRoutesPrefixesAPIPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.go")
	source := `
r.Handle("/files/upload", handler)
r.Get("/files/{checksum}/note", handler)
r.Delete("/folders", handler)
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	routes, err := readCLIRoutes(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(routes, ",")
	want := "/api/files/upload,/api/files/{checksum}/note,/api/folders"
	if got != want {
		t.Fatalf("routes = %q, want %q", got, want)
	}
}

func TestCompareRoutesReportsCountMismatch(t *testing.T) {
	err := compareRoutes([]string{"/api/a"}, []string{"/api/a", "/api/b"})
	if err == nil || !strings.Contains(err.Error(), "route count differs") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateNamesRejectsDuplicates(t *testing.T) {
	document := swagger{Paths: map[string]map[string]json.RawMessage{
		"/api/a": {"get": json.RawMessage(`{}`)},
		"/api/b": {"get": json.RawMessage(`{}`)},
	}}
	names := map[string]string{
		"GET /api/a": "Same",
		"GET /api/b": "Same",
	}
	err := validateNames(document, names)
	if err == nil || !strings.Contains(err.Error(), "duplicate SDK operation name") {
		t.Fatalf("error = %v", err)
	}
}
