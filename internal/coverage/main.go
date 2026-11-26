// Command coverage verifies that the SDK covers every FluxSend CLI API route.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	routePattern = regexp.MustCompile(`r\.(Handle|Get|Put|Post|Patch|Delete)\("([^"]+)"`)
	httpMethods  = map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true, "head": true, "options": true}
	publicPaths  = map[string]bool{
		"/api/d/{token}":             true,
		"/api/share/info/{token}":    true,
		"/api/share/resolve/{token}": true,
	}
)

type swagger struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

func main() {
	backendPath := flag.String("backend", "../fluxsend-backend", "path to fluxsend-backend")
	specPath := flag.String("spec", "../fluxsend-backend/docs/swagger.json", "path to backend Swagger JSON")
	namesPath := flag.String("names", "internal/genref/names.json", "path to stable operation names")
	flag.Parse()

	document, err := readSwagger(*specPath)
	if err != nil {
		log.Fatal(err)
	}
	specRoutes := swaggerRoutes(document)
	cliRoutes, err := readCLIRoutes(*backendPath + "/internal/api/cli.go")
	if err != nil {
		log.Fatal(err)
	}
	names, err := readNames(*namesPath)
	if err != nil {
		log.Fatal(err)
	}

	if err := compareRoutes(specRoutes, cliRoutes); err != nil {
		log.Fatal(err)
	}
	if err := validateNames(document, names); err != nil {
		log.Fatal(err)
	}

	public := 0
	for _, route := range cliRoutes {
		if publicPaths[route] {
			public++
		}
	}
	fmt.Printf(
		"FluxSend SDK coverage: spec=%s total=%d api_key=%d public=%d\n",
		document.Info.Version,
		len(cliRoutes),
		len(cliRoutes)-public,
		public,
	)
}

func readSwagger(path string) (swagger, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return swagger{}, fmt.Errorf("read Swagger: %w", err)
	}
	var document swagger
	if err := json.Unmarshal(raw, &document); err != nil {
		return swagger{}, fmt.Errorf("decode Swagger: %w", err)
	}
	return document, nil
}

func readCLIRoutes(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CLI routes: %w", err)
	}
	var routes []string
	for _, match := range routePattern.FindAllStringSubmatch(string(raw), -1) {
		routes = append(routes, "/api"+match[2])
	}
	sort.Strings(routes)
	return routes, nil
}

func readNames(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read operation names: %w", err)
	}
	var names map[string]string
	if err := json.Unmarshal(raw, &names); err != nil {
		return nil, fmt.Errorf("decode operation names: %w", err)
	}
	return names, nil
}

func swaggerRoutes(document swagger) []string {
	var routes []string
	for path, operations := range document.Paths {
		for method := range operations {
			if httpMethods[strings.ToLower(method)] {
				routes = append(routes, path)
			}
		}
	}
	sort.Strings(routes)
	return routes
}

func compareRoutes(specRoutes, cliRoutes []string) error {
	if len(specRoutes) != len(cliRoutes) {
		return fmt.Errorf("route count differs: OpenAPI=%d CLI=%d", len(specRoutes), len(cliRoutes))
	}
	for i := range specRoutes {
		if specRoutes[i] != cliRoutes[i] {
			return fmt.Errorf("route inventory differs at position %d: OpenAPI=%s CLI=%s", i, specRoutes[i], cliRoutes[i])
		}
	}
	return nil
}

func validateNames(document swagger, names map[string]string) error {
	seen := make(map[string]string, len(names))
	for _, key := range sortedUniqueNames(names) {
		name := names[key]
		if previous, ok := seen[name]; ok {
			return fmt.Errorf("duplicate SDK operation name %q for %q and %q", name, previous, key)
		}
		seen[name] = key
	}

	for path, operations := range document.Paths {
		for method := range operations {
			if !httpMethods[strings.ToLower(method)] {
				continue
			}
			key := strings.ToUpper(method) + " " + path
			if names[key] == "" {
				return errors.New("missing stable operation name for " + key)
			}
		}
	}
	return nil
}

func sortedUniqueNames(names map[string]string) []string {
	keys := make([]string, 0, len(names))
	for key := range names {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
