# Changelog

## v0.1.0 - Unreleased

### Added

- Go client for all 63 operations in the FluxSend CLI OpenAPI contract.
- Typed request and response models generated from the backend specification.
- API-key authentication with typed validation, authorization, rate-limit, and server errors.
- Safe retries for idempotent requests with `Retry-After` support.
- Streaming direct uploads and downloads.
- Concurrent chunked uploads for personal and workspace files with progress and abort cleanup.
- Stable operation-name manifest and automated API coverage checks.
- PostgreSQL and MinIO integration suite for uploads, downloads, scopes, and workspace bindings.

### Known limits

- The SDK is Go-only.
- Public APIs may change before `v1.0.0`.
- Chunked uploads resume within a process only; cross-process resume is not persisted.
- Browser OAuth and CLI credential storage are outside the SDK.
