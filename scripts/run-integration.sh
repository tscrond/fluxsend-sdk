#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="$root_dir/integration/docker-compose.yaml"
compose=(docker compose -f "$compose_file")
if [[ -d "$root_dir/fluxsend-backend" ]]; then
  export FLUXSEND_BACKEND_DIR="$root_dir/fluxsend-backend"
fi

cleanup() {
  status=$?
  if [[ $status -ne 0 ]]; then
    "${compose[@]}" logs --no-color backend postgres minio || true
  fi
  "${compose[@]}" down -v --remove-orphans || true
  exit "$status"
}
trap cleanup EXIT

cd "$root_dir"
"${compose[@]}" up -d --build postgres minio backend

ready=false
for _ in $(seq 1 120); do
  if curl -fsS http://127.0.0.1:18091/api/swagger/json >/dev/null; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  echo "FluxSend integration backend did not become ready" >&2
  exit 1
fi

"${compose[@]}" exec -T minio sh -c '
  mc alias set local http://127.0.0.1:9000 minio minio123 >/dev/null
  mc mb --ignore-existing local/fluxsend >/dev/null
'

private_key="sdk-private-integration-key"
readonly_key="sdk-readonly-integration-key"
workspace_key="sdk-workspace-integration-key"
private_hash="$(printf %s "$private_key" | sha256sum | cut -d' ' -f1)"
readonly_hash="$(printf %s "$readonly_key" | sha256sum | cut -d' ' -f1)"
workspace_hash="$(printf %s "$workspace_key" | sha256sum | cut -d' ' -f1)"

"${compose[@]}" exec -T postgres psql \
  -U fluxsend \
  -d fluxsend \
  -v ON_ERROR_STOP=1 \
  -v private_hash="$private_hash" \
  -v readonly_hash="$readonly_hash" \
  -v workspace_hash="$workspace_hash" <<'SQL'
BEGIN;

INSERT INTO users (id, user_bucket, user_email, plan_id)
VALUES (
  '11111111-1111-4111-8111-111111111111',
  'fluxsend-11111111-1111-4111-8111-111111111111',
  'sdk-integration@example.invalid',
  '00000000-0000-0000-0000-000000000002'
);

INSERT INTO identities (user_id, provider, provider_user_id, email, email_verified, name)
VALUES (
  '11111111-1111-4111-8111-111111111111',
  'password',
  'sdk-integration',
  'sdk-integration@example.invalid',
  true,
  'SDK Integration'
);

INSERT INTO workspaces (id, slug, name, owner_id)
VALUES
  ('22222222-2222-4222-8222-222222222222', 'sdk-integration', 'SDK Integration', '11111111-1111-4111-8111-111111111111'),
  ('33333333-3333-4333-8333-333333333333', 'sdk-integration-other', 'SDK Integration Other', '11111111-1111-4111-8111-111111111111');

INSERT INTO workspace_members (workspace_id, user_id, role)
VALUES
  ('22222222-2222-4222-8222-222222222222', '11111111-1111-4111-8111-111111111111', 'owner'),
  ('33333333-3333-4333-8333-333333333333', '11111111-1111-4111-8111-111111111111', 'owner');

INSERT INTO api_keys (id, created_by_user_id, name, key_hash, key_lookup_hash, status)
VALUES
  ('44444444-4444-4444-8444-444444444444', '11111111-1111-4111-8111-111111111111', 'private integration', :'private_hash', :'private_hash', 'active'),
  ('55555555-5555-4555-8555-555555555555', '11111111-1111-4111-8111-111111111111', 'readonly integration', :'readonly_hash', :'readonly_hash', 'active'),
  ('66666666-6666-4666-8666-666666666666', '11111111-1111-4111-8111-111111111111', 'workspace integration', :'workspace_hash', :'workspace_hash', 'active');

INSERT INTO api_key_user_assignments (api_key_id, user_id)
VALUES
  ('44444444-4444-4444-8444-444444444444', '11111111-1111-4111-8111-111111111111'),
  ('55555555-5555-4555-8555-555555555555', '11111111-1111-4111-8111-111111111111');

INSERT INTO api_key_workspaces (api_key_id, workspace_id)
VALUES ('66666666-6666-4666-8666-666666666666', '22222222-2222-4222-8222-222222222222');

INSERT INTO api_key_scopes (api_key_id, scope)
VALUES
  ('44444444-4444-4444-8444-444444444444', 'private_files:read'),
  ('44444444-4444-4444-8444-444444444444', 'private_files:write'),
  ('44444444-4444-4444-8444-444444444444', 'private_files:delete'),
  ('44444444-4444-4444-8444-444444444444', 'private_files:share'),
  ('55555555-5555-4555-8555-555555555555', 'private_files:read'),
  ('66666666-6666-4666-8666-666666666666', 'workspaces:read'),
  ('66666666-6666-4666-8666-666666666666', 'workspaces:write'),
  ('66666666-6666-4666-8666-666666666666', 'workspaces:delete'),
  ('66666666-6666-4666-8666-666666666666', 'workspaces:members:read'),
  ('66666666-6666-4666-8666-666666666666', 'workspaces:members:manage'),
  ('66666666-6666-4666-8666-666666666666', 'workspaces:invites:manage'),
  ('66666666-6666-4666-8666-666666666666', 'workspaces:files:read'),
  ('66666666-6666-4666-8666-666666666666', 'workspaces:files:write'),
  ('66666666-6666-4666-8666-666666666666', 'workspaces:files:delete');

COMMIT;
SQL

FLUXSEND_INTEGRATION_BASE_URL=http://127.0.0.1:18091 \
FLUXSEND_INTEGRATION_PRIVATE_KEY="$private_key" \
FLUXSEND_INTEGRATION_READONLY_KEY="$readonly_key" \
FLUXSEND_INTEGRATION_WORKSPACE_KEY="$workspace_key" \
FLUXSEND_INTEGRATION_WORKSPACE_ID='22222222-2222-4222-8222-222222222222' \
FLUXSEND_INTEGRATION_OTHER_WORKSPACE_ID='33333333-3333-4333-8333-333333333333' \
go test -tags=integration ./integration -count=1 -v
