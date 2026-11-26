// Command genref generates reference/operations.go from the backend Swagger 2
// document. It deliberately uses only the standard library so the snapshot is
// easy to regenerate during early SDK design.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type spec struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	Paths       map[string]map[string]operation `json:"paths"`
	Definitions map[string]schema               `json:"definitions"`
}

type operation struct {
	Consumes   []string            `json:"consumes"`
	Summary    string              `json:"summary"`
	Parameters []parameter         `json:"parameters"`
	Responses  map[string]response `json:"responses"`
}

type response struct {
	Schema schema `json:"schema"`
}

type parameter struct {
	In       string         `json:"in"`
	Name     string         `json:"name"`
	Required bool           `json:"required"`
	Type     string         `json:"type"`
	Schema   map[string]any `json:"schema"`
}

type schema struct {
	Type       string            `json:"type"`
	Ref        string            `json:"$ref"`
	Format     string            `json:"format"`
	Properties map[string]schema `json:"properties"`
	Required   []string          `json:"required"`
	Items      *schema           `json:"items"`
}

type generatedOperation struct {
	Method  string
	Path    string
	Details operation
	Name    string
}

var pathParameter = regexp.MustCompile(`\{([^}]+)\}`)

func main() {
	specPath := flag.String("spec", "../fluxsend-backend/docs/swagger.json", "path to the backend Swagger document")
	outputPath := flag.String("out", "reference/operations.go", "generated Go output")
	typesOutputPath := flag.String("types-out", "reference/types.go", "generated request types output")
	namesPath := flag.String("names", "internal/genref/names.json", "stable operation name map")
	publicOutputPath := flag.String("public-out", "operations.go", "generated public operations output")
	publicTypesOutputPath := flag.String("public-types-out", "types.go", "generated public types output")
	flag.Parse()

	raw, err := os.ReadFile(*specPath)
	if err != nil {
		log.Fatal(err)
	}
	var document spec
	if err := json.Unmarshal(raw, &document); err != nil {
		log.Fatal(err)
	}

	operations := collectOperations(document)
	names, err := loadOperationNames(*namesPath, operations)
	if err != nil {
		log.Fatal(err)
	}
	for i := range operations {
		operations[i].Name = names[operationKey(operations[i])]
	}
	requestTypes := collectRequestTypes(document, operations)
	source, err := render(document.Info.Version, operations)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*outputPath, source, 0o644); err != nil {
		log.Fatal(err)
	}
	typesSource, err := renderRequestTypes(document.Info.Version, requestTypes, document.Definitions)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*typesOutputPath, typesSource, 0o644); err != nil {
		log.Fatal(err)
	}
	publicTypes, err := collectPublicTypes(document, operations)
	if err != nil {
		log.Fatal(err)
	}
	publicTypesSource, err := renderRequestTypesForPackage(document.Info.Version, "fluxsend", publicTypes, document.Definitions)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*publicTypesOutputPath, publicTypesSource, 0o644); err != nil {
		log.Fatal(err)
	}
	publicOperationsSource, err := renderPublicOperations(document.Info.Version, document, operations)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*publicOutputPath, publicOperationsSource, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("generated %d public and reference operations from Swagger %s\n", len(operations), document.Info.Version)
}

func collectRequestTypes(document spec, operations []generatedOperation) []string {
	seen := make(map[string]bool)
	var pending []string
	for _, operation := range operations {
		for _, parameter := range operation.Details.Parameters {
			if parameter.In != "body" {
				continue
			}
			reference, _ := parameter.Schema["$ref"].(string)
			if key := definitionKey(reference); key != "" {
				pending = append(pending, key)
			}
		}
	}

	for len(pending) > 0 {
		key := pending[0]
		pending = pending[1:]
		if seen[key] {
			continue
		}
		definition, ok := document.Definitions[key]
		if !ok {
			log.Fatalf("body schema references missing definition %q", key)
		}
		seen[key] = true
		for _, field := range definition.Properties {
			for _, reference := range schemaReferences(field) {
				if nested := definitionKey(reference); nested != "" {
					pending = append(pending, nested)
				}
			}
		}
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func collectOperations(document spec) []generatedOperation {
	methods := map[string]bool{
		"delete":  true,
		"get":     true,
		"head":    true,
		"options": true,
		"patch":   true,
		"post":    true,
		"put":     true,
	}
	var operations []generatedOperation
	for path, item := range document.Paths {
		for method, details := range item {
			if !methods[method] {
				continue
			}
			operations = append(operations, generatedOperation{
				Method:  strings.ToUpper(method),
				Path:    path,
				Details: details,
				Name:    exportedName(details.Summary),
			})
		}
	}
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Path == operations[j].Path {
			return operations[i].Method < operations[j].Method
		}
		return operations[i].Path < operations[j].Path
	})
	return operations
}

func loadOperationNames(path string, operations []generatedOperation) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		names := make(map[string]string, len(operations))
		for _, operation := range operations {
			names[operationKey(operation)] = operation.Name
		}
		raw, err := json.MarshalIndent(names, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode operation names: %w", err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
			return nil, fmt.Errorf("write operation names: %w", err)
		}
		return names, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read operation names: %w", err)
	}

	var names map[string]string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, fmt.Errorf("decode operation names: %w", err)
	}
	seenNames := make(map[string]string, len(names))
	for _, operation := range operations {
		key := operationKey(operation)
		name := names[key]
		if name == "" {
			return nil, fmt.Errorf("missing stable operation name for %s", key)
		}
		if previous, ok := seenNames[name]; ok {
			return nil, fmt.Errorf("duplicate SDK operation name %q for %s and %s", name, previous, key)
		}
		seenNames[name] = key
	}
	return names, nil
}

func operationKey(operation generatedOperation) string {
	return operation.Method + " " + operation.Path
}

func render(version string, operations []generatedOperation) ([]byte, error) {
	var out bytes.Buffer
	fmt.Fprintln(&out, "// Code generated by internal/genref; DO NOT EDIT.")
	fmt.Fprintf(&out, "// Source: fluxsend-backend docs/swagger.json, API version %s.\n\n", version)
	fmt.Fprintln(&out, "package reference")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "import (")
	fmt.Fprintln(&out, "\t\"context\"")
	fmt.Fprintln(&out, "\t\"fmt\"")
	fmt.Fprintln(&out, "\t\"io\"")
	fmt.Fprintln(&out, "\t\"net/http\"")
	fmt.Fprintln(&out, "\t\"net/url\"")
	fmt.Fprintln(&out, "\t\"strconv\"")
	fmt.Fprintln(&out, ")")
	fmt.Fprintln(&out)

	for _, operation := range operations {
		renderOperation(&out, operation)
	}
	return format.Source(out.Bytes())
}

func renderOperation(out *bytes.Buffer, operation generatedOperation) {
	pathArgs := parametersByLocation(operation.Details.Parameters, "path")
	queryArgs := parametersByLocation(operation.Details.Parameters, "query")
	bodyArgs := parametersByLocation(operation.Details.Parameters, "body")
	formArgs := parametersByLocation(operation.Details.Parameters, "formData")
	binaryBody := len(bodyArgs) > 0 && contains(operation.Details.Consumes, "application/octet-stream")
	bodyType := "any"
	if len(bodyArgs) > 0 {
		if reference, ok := bodyArgs[0].Schema["$ref"].(string); ok {
			bodyType = sdkTypeName(definitionKey(reference))
		}
	}

	var args []string
	args = append(args, "ctx context.Context", "c *Client")
	if len(formArgs) > 0 {
		for _, parameter := range pathArgs {
			args = append(args, parameterName(parameter.Name)+" string")
		}
		args = append(args, "contents io.Reader", "fileName string", "folder string")
	} else {
		for _, parameter := range pathArgs {
			args = append(args, parameterName(parameter.Name)+" "+goType(parameter))
		}
		for _, parameter := range queryArgs {
			args = append(args, parameterName(parameter.Name)+" "+goType(parameter))
		}
		if binaryBody {
			args = append(args, "contents io.Reader", "contentLength int64")
		} else if len(bodyArgs) > 0 {
			args = append(args, "body "+bodyType)
		}
	}

	fmt.Fprintf(out, "// %s calls %s %s (%s).\n", operation.Name, operation.Method, operation.Path, operation.Details.Summary)
	if binaryBody {
		fmt.Fprintln(out, "// Body is raw application/octet-stream bytes.")
	} else if len(bodyArgs) > 0 {
		if reference, ok := bodyArgs[0].Schema["$ref"].(string); ok && reference != "" {
			fmt.Fprintf(out, "// Body schema: %s.\n", reference)
		} else {
			fmt.Fprintln(out, "// The OpenAPI body schema does not reference a generated type.")
		}
	}
	if len(formArgs) > 0 {
		fmt.Fprintln(out, "// The current reference buffers the complete upload into memory.")
	}
	fmt.Fprintf(out, "func %s(%s) (*Result, error) {\n", operation.Name, strings.Join(args, ", "))

	path := pathExpression(operation.Path, pathArgs)

	if len(formArgs) > 0 {
		fmt.Fprintf(out, "\treturn multipartUpload(ctx, c, %s, contents, fileName, folder)\n", path)
	} else {
		if len(queryArgs) == 0 {
			fmt.Fprintln(out, "\tvar query url.Values")
		} else {
			fmt.Fprintln(out, "\tquery := url.Values{}")
			for _, parameter := range queryArgs {
				arg := parameterName(parameter.Name)
				switch {
				case parameter.Required && parameter.Type == "boolean":
					fmt.Fprintf(out, "\tif %s {\n\t\tquery.Set(%q, \"true\")\n\t} else {\n\t\tquery.Set(%q, \"false\")\n\t}\n", arg, parameter.Name, parameter.Name)
				case parameter.Required:
					fmt.Fprintf(out, "\tquery.Set(%q, %s)\n", parameter.Name, arg)
				case parameter.Type == "boolean":
					fmt.Fprintf(out, "\tif %s {\n\t\tquery.Set(%q, \"true\")\n\t}\n", arg, parameter.Name)
				default:
					fmt.Fprintf(out, "\tif %s != \"\" {\n\t\tquery.Set(%q, %s)\n\t}\n", arg, parameter.Name, arg)
				}
			}
		}

		body := "nil"
		if binaryBody {
			body = "contents"
		} else if len(bodyArgs) > 0 {
			body = "body"
		}
		if binaryBody {
			fmt.Fprintln(out, "\tif contentLength <= 0 {")
			fmt.Fprintln(out, "\t\treturn nil, fmt.Errorf(\"upload part content length must be positive\")")
			fmt.Fprintln(out, "\t}")
			fmt.Fprintf(out, "\treturn c.Do(ctx, Call{Method: http.Method%s, Path: %s, Query: query, Body: contents, ContentType: \"application/octet-stream\", ContentLength: contentLength})\n", title(strings.ToLower(operation.Method)), path)
		} else {
			fmt.Fprintf(out, "\treturn c.Do(ctx, Call{Method: http.Method%s, Path: %s, Query: query, Body: %s})\n", title(strings.ToLower(operation.Method)), path, body)
		}
	}
	fmt.Fprintln(out, "}")
	fmt.Fprintln(out)
}

func renderRequestTypes(version string, typeKeys []string, definitions map[string]schema) ([]byte, error) {
	return renderRequestTypesForPackage(version, "reference", typeKeys, definitions)
}

func renderRequestTypesForPackage(version, packageName string, typeKeys []string, definitions map[string]schema) ([]byte, error) {
	var out bytes.Buffer
	fmt.Fprintln(&out, "// Code generated by internal/genref; DO NOT EDIT.")
	fmt.Fprintf(&out, "// Source: fluxsend-backend docs/swagger.json, API version %s.\n\n", version)
	fmt.Fprintf(&out, "package %s\n", packageName)
	fmt.Fprintln(&out)

	for _, key := range typeKeys {
		definition := definitions[key]
		typeName := sdkTypeName(key)
		required := make(map[string]bool, len(definition.Required))
		for _, name := range definition.Required {
			required[name] = true
		}

		fmt.Fprintf(&out, "// %s is generated from %s.\n", typeName, key)
		fmt.Fprintf(&out, "type %s struct {\n", typeName)
		for _, field := range sortedKeys(definition.Properties) {
			property := definition.Properties[field]
			tag := field
			if !required[field] {
				tag += ",omitempty"
			}
			fmt.Fprintf(&out, "\t%s %s `json:%q`\n", fieldName(field), schemaGoType(property), tag)
		}
		fmt.Fprintln(&out, "}")
		fmt.Fprintln(&out)
	}
	return format.Source(out.Bytes())
}

func collectPublicTypes(document spec, operations []generatedOperation) ([]string, error) {
	seen := make(map[string]bool)
	var pending []string
	for _, operation := range operations {
		for _, parameter := range operation.Details.Parameters {
			if parameter.In != "body" {
				continue
			}
			reference, _ := parameter.Schema["$ref"].(string)
			if key := definitionKey(reference); key != "" {
				pending = append(pending, key)
			}
		}
		responseType, ok, err := operationResponseType(operation, document)
		if err != nil {
			return nil, err
		}
		if ok {
			pending = append(pending, schemaReferences(responseType)...)
		}
	}

	for len(pending) > 0 {
		reference := pending[0]
		pending = pending[1:]
		key := definitionKey(reference)
		if key == "" || seen[key] {
			continue
		}
		definition, ok := document.Definitions[key]
		if !ok {
			return nil, fmt.Errorf("response schema references missing definition %q", key)
		}
		seen[key] = true
		for _, nested := range schemaReferences(definition) {
			pending = append(pending, nested)
		}
	}

	typeNames := make(map[string]string, len(seen))
	keys := make([]string, 0, len(seen))
	for key := range seen {
		name := sdkTypeName(key)
		if previous, ok := typeNames[name]; ok {
			return nil, fmt.Errorf("SDK type name %q is shared by %q and %q", name, previous, key)
		}
		typeNames[name] = key
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func operationResponseType(operation generatedOperation, document spec) (schema, bool, error) {
	statuses := make([]string, 0, len(operation.Details.Responses))
	for status := range operation.Details.Responses {
		if strings.HasPrefix(status, "2") {
			statuses = append(statuses, status)
		}
	}
	sort.Strings(statuses)
	if len(statuses) == 0 {
		return schema{}, false, nil
	}
	responseSchema := operation.Details.Responses[statuses[0]].Schema
	if responseSchema.Ref == "" {
		if responseSchema.Type == "" {
			return schema{}, false, nil
		}
		return responseSchema, true, nil
	}
	definition, ok := document.Definitions[definitionKey(responseSchema.Ref)]
	if !ok {
		return schema{}, false, fmt.Errorf("success response references missing definition %q", responseSchema.Ref)
	}
	payload, ok := definition.Properties["response"]
	if !ok {
		return schema{}, false, fmt.Errorf("success response definition %q has no response property", responseSchema.Ref)
	}
	return payload, true, nil
}

func renderPublicOperations(version string, document spec, operations []generatedOperation) ([]byte, error) {
	var out bytes.Buffer
	fmt.Fprintln(&out, "// Code generated by internal/genref; DO NOT EDIT.")
	fmt.Fprintf(&out, "// Source: fluxsend-backend docs/swagger.json, API version %s.\n\n", version)
	fmt.Fprintln(&out, "package fluxsend")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "import (")
	fmt.Fprintln(&out, "\t\"context\"")
	fmt.Fprintln(&out, "\t\"encoding/json\"")
	fmt.Fprintln(&out, "\t\"fmt\"")
	fmt.Fprintln(&out, "\t\"io\"")
	fmt.Fprintln(&out, "\t\"net/http\"")
	fmt.Fprintln(&out, "\t\"net/url\"")
	fmt.Fprintln(&out, "\t\"strconv\"")
	fmt.Fprintln(&out, ")")
	fmt.Fprintln(&out)

	for _, operation := range operations {
		if strings.TrimSpace(operation.Path) == "" {
			continue
		}
		if key := operationKey(operation); manualPublicOperations[key] {
			continue
		}
		renderPublicOperation(&out, operation, document)
	}
	return format.Source(out.Bytes())
}

var manualPublicOperations = map[string]bool{
	"POST /api/files/upload":                            true,
	"POST /api/workspaces/{workspace_id}/files/upload":  true,
	"GET /api/d/private/{token}":                        true,
	"GET /api/d/{token}":                                true,
	"GET /api/workspaces/{workspace_id}/files/download": true,
}

func renderPublicOperation(out *bytes.Buffer, operation generatedOperation, document spec) {
	pathArgs := parametersByLocation(operation.Details.Parameters, "path")
	queryArgs := parametersByLocation(operation.Details.Parameters, "query")
	bodyArgs := parametersByLocation(operation.Details.Parameters, "body")
	binaryBody := len(bodyArgs) > 0 && contains(operation.Details.Consumes, "application/octet-stream")
	bodyType := "any"
	if len(bodyArgs) > 0 {
		if reference, ok := bodyArgs[0].Schema["$ref"].(string); ok {
			bodyType = sdkTypeName(definitionKey(reference))
		}
	}

	var args []string
	args = append(args, "ctx context.Context")
	for _, parameter := range pathArgs {
		args = append(args, parameterName(parameter.Name)+" "+goType(parameter))
	}
	for _, parameter := range queryArgs {
		args = append(args, parameterName(parameter.Name)+" "+goType(parameter))
	}
	if binaryBody {
		args = append(args, "contents io.Reader", "contentLength int64")
	} else if len(bodyArgs) > 0 {
		args = append(args, "body "+bodyType)
	}

	responseType, ok, err := publicResponseType(operation, document)
	if err != nil {
		panic(err)
	}
	if !ok {
		return
	}
	requiresAuth := !publicOperation(operation)
	retryable := operation.Method == "GET" || operation.Method == "HEAD"

	fmt.Fprintf(out, "// %s calls %s %s (%s).\n", operation.Name, operation.Method, operation.Path, operation.Details.Summary)
	fmt.Fprintf(out, "func (c *Client) %s(%s) (%s, error) {\n", operation.Name, strings.Join(args, ", "), responseType)
	if len(queryArgs) == 0 {
		fmt.Fprintln(out, "\tvar query url.Values")
	} else {
		fmt.Fprintln(out, "\tquery := url.Values{}")
		for _, parameter := range queryArgs {
			arg := parameterName(parameter.Name)
			switch {
			case parameter.Required && parameter.Type == "boolean":
				fmt.Fprintf(out, "\tif %s {\n\t\tquery.Set(%q, \"true\")\n\t} else {\n\t\tquery.Set(%q, \"false\")\n\t}\n", arg, parameter.Name, parameter.Name)
			case parameter.Required:
				fmt.Fprintf(out, "\tquery.Set(%q, %s)\n", parameter.Name, arg)
			case parameter.Type == "boolean":
				fmt.Fprintf(out, "\tif %s {\n\t\tquery.Set(%q, \"true\")\n\t}\n", arg, parameter.Name)
			default:
				fmt.Fprintf(out, "\tif %s != \"\" {\n\t\tquery.Set(%q, %s)\n\t}\n", arg, parameter.Name, arg)
			}
		}
	}

	path := pathExpressionWith(operation.Path, pathArgs, "url.PathEscape")
	switch {
	case binaryBody:
		fmt.Fprintln(out, "\tif contentLength <= 0 {")
		fmt.Fprintf(out, "\t\tvar zero %s\n", responseType)
		fmt.Fprintln(out, "\t\treturn zero, fmt.Errorf(\"upload part content length must be positive\")")
		fmt.Fprintln(out, "\t}")
		fmt.Fprintf(out, "\treturn c.doReaderJSON[%s](ctx, request{method: http.Method%s, path: %s, query: query, contentType: \"application/octet-stream\", contentLength: contentLength, requiresAuth: %t, retryable: %t}, contents)\n", responseType, title(strings.ToLower(operation.Method)), path, requiresAuth, retryable)
	case len(bodyArgs) > 0:
		fmt.Fprintf(out, "\treturn c.doJSON[%s](ctx, request{method: http.Method%s, path: %s, query: query, body: body, requiresAuth: %t, retryable: %t})\n", responseType, title(strings.ToLower(operation.Method)), path, requiresAuth, retryable)
	default:
		fmt.Fprintf(out, "\treturn c.doJSON[%s](ctx, request{method: http.Method%s, path: %s, query: query, requiresAuth: %t, retryable: %t})\n", responseType, title(strings.ToLower(operation.Method)), path, requiresAuth, retryable)
	}
	fmt.Fprintln(out, "}")
	fmt.Fprintln(out)
}

func publicResponseType(operation generatedOperation, document spec) (string, bool, error) {
	value, ok, err := operationResponseType(operation, document)
	if err != nil || !ok {
		return "", ok, err
	}
	if value.Ref != "" {
		return sdkTypeName(definitionKey(value.Ref)), true, nil
	}
	switch value.Type {
	case "string":
		return "string", true, nil
	case "array":
		if value.Items == nil {
			return "json.RawMessage", true, nil
		}
		return "[]" + schemaGoType(*value.Items), true, nil
	}
	return "json.RawMessage", true, nil
}

func publicOperation(operation generatedOperation) bool {
	switch operationKey(operation) {
	case "GET /api/d/{token}", "GET /api/share/info/{token}", "POST /api/share/resolve/{token}":
		return true
	default:
		return false
	}
}

func parametersByLocation(parameters []parameter, location string) []parameter {
	var result []parameter
	for _, parameter := range parameters {
		if parameter.In == location {
			result = append(result, parameter)
		}
	}
	return result
}

func parameterName(raw string) string {
	parts := strings.Split(raw, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] == "id" {
			parts[i] = "ID"
			continue
		}
		parts[i] = title(parts[i])
	}
	return parts[0] + strings.Join(parts[1:], "")
}

func exportedName(summary string) string {
	words := strings.FieldsFunc(summary, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for i, word := range words {
		upper := strings.ToUpper(word)
		if upper == "API" || upper == "ID" {
			words[i] = upper
			continue
		}
		if len(words) > 1 && (word == "a" || word == "an" || word == "the") {
			words[i] = ""
			continue
		}
		words[i] = title(strings.ToLower(word))
	}
	return strings.Join(words, "")
}

func title(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func goType(parameter parameter) string {
	switch parameter.Type {
	case "boolean":
		return "bool"
	case "integer":
		return "int"
	default:
		return "string"
	}
}

func pathExpression(path string, parameters []parameter) string {
	return pathExpressionWith(path, parameters, "escapePath")
}

func pathExpressionWith(path string, parameters []parameter, escapeFunction string) string {
	types := make(map[string]string, len(parameters))
	for _, parameter := range parameters {
		types[parameter.Name] = parameter.Type
	}

	matches := pathParameter.FindAllStringSubmatchIndex(path, -1)
	if len(matches) == 0 {
		return strconv.Quote(path)
	}

	var expression []string
	start := 0
	for _, match := range matches {
		if match[0] > start {
			expression = append(expression, strconv.Quote(path[start:match[0]]))
		}
		name := path[match[2]:match[3]]
		argument := parameterName(name)
		if types[name] == "integer" {
			expression = append(expression, "strconv.Itoa("+argument+")")
		} else {
			expression = append(expression, escapeFunction+"("+argument+")")
		}
		start = match[1]
	}
	if start < len(path) {
		expression = append(expression, strconv.Quote(path[start:]))
	}
	return strings.Join(expression, " + ")
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func definitionKey(reference string) string {
	return strings.TrimPrefix(reference, "#/definitions/")
}

func sdkTypeName(definition string) string {
	if definition == "" {
		return "any"
	}
	parts := strings.Split(definition, ".")
	name := parts[len(parts)-1]
	var words []string
	for _, part := range strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		words = append(words, title(part))
	}
	return strings.Join(words, "")
}

func schemaReferences(value schema) []string {
	var references []string
	if value.Ref != "" {
		references = append(references, value.Ref)
	}
	if value.Items != nil {
		references = append(references, schemaReferences(*value.Items)...)
	}
	for _, property := range value.Properties {
		references = append(references, schemaReferences(property)...)
	}
	return references
}

func schemaGoType(value schema) string {
	if value.Ref != "" {
		return sdkTypeName(definitionKey(value.Ref))
	}
	switch value.Type {
	case "array":
		if value.Items == nil {
			return "[]any"
		}
		return "[]" + schemaGoType(*value.Items)
	case "boolean":
		return "bool"
	case "integer":
		return "int64"
	case "number":
		return "float64"
	case "object":
		return "map[string]any"
	default:
		return "string"
	}
}

func fieldName(raw string) string {
	parts := strings.Split(raw, "_")
	for i, part := range parts {
		switch strings.ToLower(part) {
		case "api":
			parts[i] = "API"
		case "http":
			parts[i] = "HTTP"
		case "id":
			parts[i] = "ID"
		case "md5":
			parts[i] = "MD5"
		case "url":
			parts[i] = "URL"
		default:
			parts[i] = title(part)
		}
	}
	return strings.Join(parts, "")
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
