# FluxSend Go SDK

The FluxSend Go SDK provides typed, context-aware access to the FluxSend developer
API used by the FluxSend CLI.

[Read the full Go SDK documentation](https://docs.fluxsend.app/sdks/go/).

> Latest release: `v0.1.1`. The SDK remains on `v0.x`; exported APIs may change
> until `v1.0.0`.

## Requirements

- Go 1.27.4 or newer
- A FluxSend deployment URL
- A personal or workspace API key for authenticated operations

Public sharing operations can be used without an API key.

## Install

```sh
go get github.com/tscrond/fluxsend-sdk@v0.1.1
```

The public package name is `fluxsend`:

```go
import fluxsend "github.com/tscrond/fluxsend-sdk"
```

## First request

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

client, err := fluxsend.NewClient(fluxsend.Config{
	BaseURL: "https://fluxsend.app",
	APIKey:  os.Getenv("FLUXSEND_API_KEY"),
})
if err != nil {
	return err
}

workspaces, err := client.ListWorkspaces(ctx)
if err != nil {
	return err
}
```

Keep API keys in server-side configuration or a secrets manager. Do not embed
them in browser code.

## Documentation

| Topic | Guide |
| --- | --- |
| Installation and first request | [Go SDK quickstart](https://docs.fluxsend.app/sdks/go/quickstart/) |
| Hosted/self-hosted configuration and API keys | [Configuration & authentication](https://docs.fluxsend.app/sdks/go/configuration-authentication/) |
| Typed errors, retries, and cancellation | [Errors & reliability](https://docs.fluxsend.app/sdks/go/errors-reliability/) |
| Direct uploads, chunked uploads, and downloads | [Files & transfers](https://docs.fluxsend.app/sdks/go/files-transfers/) |
| Operation coverage and compatibility | [Operations & versions](https://docs.fluxsend.app/sdks/go/operations-versions/) |
| Generated API reference | [docs.fluxsend.app/api](https://docs.fluxsend.app/api/) |

The SDK covers all 63 operations in the FluxSend CLI API contract: 60
API-key-backed operations and 3 public sharing operations.

## Development

Regenerate the typed SDK from the backend contract:

```sh
go run ./internal/genref \
  -spec ../fluxsend-backend/docs/swagger.json \
  -out reference/operations.go \
  -types-out reference/types.go \
  -names internal/genref/names.json \
  -public-out operations.go \
  -public-types-out types.go
```

Run the main checks:

```sh
go test -race -count=1 ./...
go vet ./...
govulncheck ./...
```

Run the PostgreSQL and MinIO integration suite:

```sh
./scripts/run-integration.sh
```

Release and rollback instructions are in [RELEASING.md](RELEASING.md). Supported
operations and known limits are listed in [CHANGELOG.md](CHANGELOG.md).

## License

See [LICENSE](LICENSE).
