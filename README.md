# FluxSend Go SDK

The FluxSend Go SDK provides typed access to the FluxSend developer API used by
the FluxSend CLI.

> Status: `v0.1.0` release candidate. The public API is not stable until
> `v1.0.0`.

## Scope

The initial SDK covers all 60 API-key-backed operations exposed by the FluxSend
CLI server. The three public sharing operations are also available because they
are part of the same OpenAPI contract.

The SDK owns HTTP communication and FluxSend API semantics. The CLI owns command
parsing, profiles, credential storage, interactive prompts, output formatting,
and exit codes.

## Public package

The public import path is:

```go
import "github.com/tscrond/fluxsend-sdk"
```

The package name is `fluxsend`. The existing `reference` package is a generated
development aid, not a compatibility promise.

## Client contract

The public client is configured explicitly:

```go
type Config struct {
    BaseURL           string
    APIKey            string
    HTTPClient        *http.Client
    UserAgent         string
    AllowInsecureHTTP bool
}

func NewClient(Config) (*Client, error)
```

- `BaseURL` must be an absolute HTTP or HTTPS URL without a query or fragment.
  A deployment path prefix is allowed.
- Plain HTTP is rejected unless `AllowInsecureHTTP` is explicitly enabled for
  local development.
- `APIKey` is sent only as `X-API-Key` to the configured deployment origin.
- `HTTPClient` is injected so callers control proxies, TLS, and timeouts.
- `UserAgent` defaults to a versioned FluxSend SDK identifier.
- A `Client` is safe for concurrent use after construction.
- Public sharing calls may be made without an API key; API-key-backed calls
  fail locally with a typed authentication error when the key is missing.

Representative call:

```go
client, err := fluxsend.NewClient(fluxsend.Config{
    BaseURL: "https://fluxsend.app",
    APIKey:  apiKey,
})
if err != nil {
    return err
}

workspaces, err := client.ListWorkspaces(ctx)
```

## Method conventions

- Every network operation accepts `context.Context` as its first argument.
- API-key-backed operations are methods on `*Client`.
- Method names follow stable OpenAPI operation IDs.
- Request inputs use SDK-owned `...Request` types.
- Success payload types are returned directly; callers do not decode the
  FluxSend response envelope.
- JSON and error behavior is deterministic across operations.
- Path parameters are escaped by the SDK.
- Non-idempotent mutations are never retried implicitly.

## Errors

The SDK distinguishes:

- Configuration and missing-credential errors.
- Transport and context-cancellation errors.
- Authentication failures.
- Authorization and scope failures.
- Not-found and conflict failures.
- Rate limits, including `Retry-After`.
- Server and malformed-response failures.

Typed errors preserve the HTTP status and safe FluxSend API message. Error
strings, logs, and examples must never include API keys, signed URLs, or file
contents.

Read-only requests may retry transient transport failures and rate limits.
Retries must honor `Retry-After` and remain bounded. Unsafe mutations require an
explicit caller retry decision.

## Files

- Direct uploads accept streaming readers and do not buffer complete files.
- Downloads return a stream whose ownership and close behavior are explicit.
- Chunked uploads expose initialize, part, complete, and abort operations.
- The higher-level chunked upload workflow supports bounded concurrency,
  progress, cancellation, and best-effort abort.

## Compatibility

- SDK releases are versioned independently from the backend.
- `v0.x` may introduce breaking API changes; each release documents them.
- Starting with `v1.0.0`, backwards-incompatible exported API changes require a
  major version.
- Deprecated APIs remain available for at least one minor release when feasible.
- Generated operation coverage is checked against the FluxSend CLI contract.

## Known limits

- Go only.
- Cross-process chunked-upload resume is not persisted.
- Browser OAuth and CLI credential storage are outside the SDK.

## Development

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
go test ./...
go vet ./...
```

Run the real backend integration suite:

```sh
./scripts/run-integration.sh
```
