package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderOperationUsesTypedJSONBodyAndIntegerPath(t *testing.T) {
	operation := generatedOperation{
		Method: "POST",
		Path:   "/api/workspaces/{workspace_id}/files/uploads",
		Name:   "InitializeWorkspaceUpload",
		Details: operation{
			Summary: "Initialize workspace upload",
			Parameters: []parameter{
				{In: "path", Name: "workspace_id", Type: "string"},
				{In: "body", Name: "request", Schema: map[string]any{"$ref": "#/definitions/api.CreateUploadRequest"}},
			},
		},
	}

	var out bytes.Buffer
	renderOperation(&out, operation)
	source := out.String()
	if !strings.Contains(source, "workspaceID string") {
		t.Fatalf("workspace path parameter is not a string:\n%s", source)
	}
	if !strings.Contains(source, "body CreateUploadRequest") {
		t.Fatalf("JSON body is not typed:\n%s", source)
	}
	if strings.Contains(source, "body any") {
		t.Fatalf("JSON body uses any:\n%s", source)
	}
}

func TestRenderOperationUsesBinaryBodyAndIntegerPath(t *testing.T) {
	operation := generatedOperation{
		Method: "POST",
		Path:   "/api/files/uploads/{upload_id}/parts/{part_id}",
		Name:   "UploadPersonalFileMultipartPart",
		Details: operation{
			Summary:  "Upload personal file multipart part",
			Consumes: []string{"application/octet-stream"},
			Parameters: []parameter{
				{In: "path", Name: "upload_id", Type: "string"},
				{In: "path", Name: "part_id", Type: "integer"},
				{In: "body", Name: "part", Type: "string"},
			},
		},
	}

	var out bytes.Buffer
	renderOperation(&out, operation)
	source := out.String()
	for _, expected := range []string{
		"partID int",
		"contents io.Reader",
		"contentLength int64",
		`ContentType: "application/octet-stream"`,
		"strconv.Itoa(partID)",
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("generated operation missing %q:\n%s", expected, source)
		}
	}
}

func TestRenderRequestTypesPreservesRequiredAndOptionalFields(t *testing.T) {
	definitions := map[string]schema{
		"api.MoveRequest": {
			Type:     "object",
			Required: []string{"source"},
			Properties: map[string]schema{
				"destination": {Type: "string"},
				"source":      {Type: "string"},
			},
		},
	}

	source, err := renderRequestTypes("test", []string{"api.MoveRequest"}, definitions)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(source)
	for _, expected := range []string{
		"type MoveRequest struct",
		`json:"source"`,
		`json:"destination,omitempty"`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("generated types missing %q:\n%s", expected, rendered)
		}
	}
}

func TestCollectRequestTypesIncludesNestedDefinitions(t *testing.T) {
	document := spec{
		Definitions: map[string]schema{
			"api.Outer": {
				Type: "object",
				Properties: map[string]schema{
					"inner": {Ref: "#/definitions/api.Inner"},
				},
			},
			"api.Inner": {Type: "object"},
		},
	}
	operations := []generatedOperation{{
		Details: operation{Parameters: []parameter{{
			In:     "body",
			Name:   "request",
			Schema: map[string]any{"$ref": "#/definitions/api.Outer"},
		}}},
	}}

	got := collectRequestTypes(document, operations)
	if strings.Join(got, ",") != "api.Inner,api.Outer" {
		t.Fatalf("types = %v", got)
	}
}

func TestStableOperationNamesSurviveSummaryChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "names.json")
	if err := os.WriteFile(path, []byte(`{"POST /api/workspaces/create":"CreateWorkspace"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	operations := []generatedOperation{{
		Method: "POST",
		Path:   "/api/workspaces/create",
		Name:   "RenamedFromSummary",
	}}

	names, err := loadOperationNames(path, operations)
	if err != nil {
		t.Fatal(err)
	}
	if got := names[operationKey(operations[0])]; got != "CreateWorkspace" {
		t.Fatalf("name = %q, want CreateWorkspace", got)
	}
}
