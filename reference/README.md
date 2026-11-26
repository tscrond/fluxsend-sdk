# FluxSend API call reference

This package is a compile-checked development reference generated from the
FluxSend backend's Swagger 2.0 document. It is intentionally separate from the
eventual public SDK API.

Each exported function in `operations.go` represents one valid OpenAPI
operation. The functions expose operation-specific path and query arguments,
execute through a small shared HTTP client, and return both the raw HTTP
response and the decoded FluxSend response envelope.

```go
client, err := reference.NewClient("https://fluxsend.app", apiKey, nil)
if err != nil {
    return err
}

result, err := reference.ListWorkspaces(context.Background(), client)
if err != nil {
    return err
}
fmt.Println(string(result.Envelope.Response))
```

The base URL is the deployment origin, not the `/api` route. The API key is
sent in `X-API-Key`.

## Coverage

- The generated API reference contains all 63 operations currently declared in
  the backend Swagger document.
- The 63 operations comprise all 60 API-key-backed CLI operations and three
  public sharing operations.
- The three public sharing operations remain available in the generated
  reference for completeness.

## OpenAPI gaps

- Generated JSON-body functions still accept `body any`. The backend now
  documents typed request schemas; typed SDK request mapping is still pending.
- Direct form uploads are represented by `UploadFile` and
  `UploadWorkspaceFile`. They validate multipart call shape, but currently
  buffer the entire file in memory. Chunked upload sessions use their four
  generated operations each and accept raw chunks with an explicit content
  length.
- Redirect and error response bodies remain available in `Result` for
  inspection. Non-2xx responses also return `*APIError`.

Regenerate the operation functions after updating the backend specification:

```sh
go run ./internal/genref \
  -spec ../fluxsend-backend/docs/swagger.json \
  -out reference/operations.go \
  -types-out reference/types.go \
  -names internal/genref/names.json \
  -public-out operations.go \
  -public-types-out types.go
```

The generated reference currently contains 55 operations from API version
1.8.8.
