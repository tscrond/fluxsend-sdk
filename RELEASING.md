# Releasing the FluxSend Go SDK

## Before tagging

```sh
go test -race -count=1 ./...
go vet ./...
govulncheck ./...
./scripts/run-integration.sh
```

Regenerate and verify the contract:

```sh
go run ./internal/genref \
  -spec ../fluxsend-backend/docs/swagger.json \
  -out reference/operations.go \
  -types-out reference/types.go \
  -names internal/genref/names.json \
  -public-out operations.go \
  -public-types-out types.go
go run ./internal/coverage \
  -backend ../fluxsend-backend \
  -spec ../fluxsend-backend/docs/swagger.json \
  -names internal/genref/names.json
git diff --exit-code
```

Update `CHANGELOG.md`, review the complete diff, and merge through the normal
branch-protected workflow.

## Tag

Only after the release commit is on `main`:

```sh
git tag -a v0.1.0 -m "FluxSend Go SDK v0.1.0"
git push origin v0.1.0
```

Verify install from a clean module:

```sh
mkdir /tmp/fluxsend-sdk-release-check
cd /tmp/fluxsend-sdk-release-check
go mod init example.com/fluxsend-release-check
go get github.com/tscrond/fluxsend-sdk@v0.1.0
```

## Rollback

A published Go module version is immutable. If `v0.1.0` is defective:

1. Do not retag or rewrite `v0.1.0`.
2. Publish a fixed `v0.1.1`.
3. Mark `v0.1.0` as retracted in the corrected module's `go.mod`:

```go
retract v0.1.0
```

4. Document the affected behavior and upgrade command in the release notes.
