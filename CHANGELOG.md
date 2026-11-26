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

### Supported operations

API keys:

- `CreatePrivateAPIKey`, `DeletePrivateAPIKey`, `ListPrivateAPIKeys`
- `CreateWorkspaceAPIKey`, `DeleteWorkspaceAPIKey`, `ListWorkspaceAPIKeys`

Files, folders, and sharing:

- `DeleteFile`, `DeleteMultipleFiles`, `MoveFile`, `GetFileTree`
- `UploadFile`, `DownloadPublicShare`, `DownloadPrivateFile`
- `DeleteFolder`, `ListFolders`, `MoveFolder`
- `GetFileNote`, `EditFileNote`
- `CreateQuickShare`, `ShareFiles`, `RevokeShare`
- `ListReceivedShares`, `MarkReceivedShareAsSeen`, `CountUnseenReceivedFiles`
- `ListOutgoingShares`, `GetPublicShareInfo`, `ResolvePublicShare`
- `InitializePersonalFileMultipartUpload`, `UploadPersonalFileMultipartPart`
- `CompletePersonalFileMultipartUpload`, `AbortPersonalFileMultipartUpload`

Users:

- `DeleteAccount`, `GetUserBucketData`, `GetCurrentUserData`
- `GetLinkedIdentities`, `CreatePrivateDownloadToken`, `GetUserStatistics`

Workspaces:

- `CreateWorkspace`, `DeleteWorkspace`, `ListWorkspaces`, `RenameWorkspace`
- `ListWorkspaceInvites`, `CreateWorkspaceInvite`, `DeleteWorkspaceInvite`
- `ListMyWorkspaceInvites`, `AcceptWorkspaceInvite`, `RejectWorkspaceInvite`
- `ListWorkspaceMembers`, `RemoveWorkspaceMember`, `ChangeMemberRole`
- `CreateWorkspaceFolder`, `MoveWorkspaceFolder`, `DeleteWorkspaceFolder`
- `MoveWorkspaceFile`, `DeleteWorkspaceFile`, `GetWorkspaceFileTree`
- `UploadWorkspaceFile`, `DownloadWorkspaceFile`, `GetWorkspaceQuota`
- `InitializeWorkspaceFileMultipartUpload`, `UploadWorkspaceFileMultipartPart`
- `CompleteWorkspaceFileMultipartUpload`, `AbortWorkspaceFileMultipartUpload`

### Known limits

- The SDK is Go-only.
- Public APIs may change before `v1.0.0`.
- Chunked uploads resume within a process only; cross-process resume is not persisted.
- Browser OAuth and CLI credential storage are outside the SDK.
