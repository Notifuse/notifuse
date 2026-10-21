# Auth hardening plan

Target: 42.0 (unreleased). No database migration.

## Goals

1. Signing gets its own key, `SIGNING_KEY`, separate from `SECRET_KEY`, which keeps encrypting stored data.
   An installation can then stop signing with a key without re-encrypting anything.
2. Notifuse refuses to start when it would sign with the `SECRET_KEY` value that `compose.yaml` used to
   ship as a default.
3. `compose.yaml` requires `SECRET_KEY`, as `compose.alloydb.yaml` already does.
4. A token authenticates only the kind of user it was issued for: an API key token an API key user, a
   session token a regular user.
5. Accepting an invitation requires the token to name the stored invitation's workspace and email, and
   the invitation to be unexpired.

## Why a separate signing key rather than a new SECRET_KEY

`SECRET_KEY` encrypts each workspace's own secret key, its integrations' credentials and the system
settings. `Workspace.AfterLoad` fails when it cannot decrypt the workspace secret key, and
`WorkspaceRepository.List` returns that error, so after a `SECRET_KEY` change no workspace loads. Asking an
installation to replace `SECRET_KEY` would therefore need a re-encryption step. Moving signing to its own
key needs none: stored data stays readable, and only signatures change.

## 1. Signing key resolution (`config/config.go`)

- `SecurityConfig` gains `SigningKey string`. `JWTSecret` is derived from `SigningKey` with today's rule
  (base64 that decodes to 32 bytes or more, else the raw bytes), and the short-key warning applies to it.
- Resolution, after `SECRET_KEY` / `PASETO_PRIVATE_KEY` resolution:
  - `SIGNING_KEY` set (non-empty after trimming) → `SigningKey` is the trimmed `SIGNING_KEY`. Surrounding
    whitespace is never part of a key, and secret files often end with a newline.
  - Unset → `SigningKey = SecretKey`, byte for byte. Nothing changes for an installation that never set it:
    its sessions, API keys and signatures stay valid. `SecretKey` itself is never modified, since it
    decrypts stored data.
  - The resolved `SigningKey` is the compose default → `Load` returns an error naming `SIGNING_KEY`, telling
    the operator to generate one with `openssl rand -base64 32`, to keep `SECRET_KEY` as it is because it
    decrypts stored data, and that existing sessions and API keys stop working. This covers `SECRET_KEY`,
    `PASETO_PRIVATE_KEY` and `SIGNING_KEY` set to that value.
- "Is the compose default" means either:
  - the trimmed string has the default's SHA-256 digest; or
  - the key's bytes, or the bytes any standard base64 alphabet (padded or not, whitespace removed) decodes
    it to, have the SHA-256 digest of the default's decoded bytes. This catches the same key re-encoded,
    stripped of its padding, or given raw, all of which would derive the same `JWTSecret`.
- Both digests are kept as hex constants, so the code carries no key literal.

## 2. Signing consumers move to `SigningKey`

Signing or integrity uses, moved:

| Consumer | Wiring today |
| --- | --- |
| JWTs (sessions, API keys, invitations) | `Security.JWTSecret` via `getJWTSecret` and `AuthService` |
| Magic code hashes and root sign-in HMAC | `UserServiceConfig.SecretKey` (`user_service.go`) |
| OIDC flow-state cookie seal | `OIDCServiceConfig.SecretKey` (`oidc_service.go`) |
| `/api/tasks.execute` signatures | `TaskService.SetSecretKey`, `NewTaskHandler` |
| Usage read signatures | `NewUsageHandler` |
| Demo root email HMAC | `DemoService.VerifyRootEmailHMAC` |

Encryption uses, unchanged on `SecretKey`: `WorkspaceRepository`, `SettingService` / `SetupService` /
`SettingsHandler`, `WorkspaceService` and `WorkspaceHandler` validation (integration secrets),
migrations v11 and v15, `loadSystemSettings`.

The field names say which key each consumer gets (`SigningKey` beside `SecretKey`), so a later consumer
cannot pick the wrong one by accident. `cmd/hmac` and `make demo-hmac` take the signing key, and the
root sign-in API reference (`openapi/`, `openapi.json`) signs with `SIGNING_KEY`.

The integration harness runs with `SECRET_KEY` and `SIGNING_KEY` set to different values
(`testutil.TestSigningKey`), so the whole suite exercises the wiring in `app.go`.

## 3. Compose

- `compose.yaml`: `SECRET_KEY` becomes `${SECRET_KEY:?…}` with a message that gives the generation command,
  quoted so YAML does not read the colon.
- Both compose files pass `SIGNING_KEY=${SIGNING_KEY:-}` through. An empty value means unset.
- `env.example` documents `SIGNING_KEY`.

## 4. Token type check (`auth_repository.go`, `auth_service.go`)

- `SQLAuthRepository.GetUserByID` also selects `type`.
- `AuthService.AuthenticateUserFromContext`: the API key branch treats a loaded user whose type is not
  `api_key` like a missing one, so it surfaces as `ErrAPIKeyRevoked` (401 through `writePermissionError`).
  The session branch (`AuthService.VerifyUserSession`) returns `ErrUserNotFound` unless the type is `user`,
  the same answer as for a session naming a user that does not exist.
- `UserService.VerifyUserSession` (used by `/api/user.me`) applies the same session rule and wraps the
  refusal in `ErrSessionAuthFailed`, so it answers 401. Its repository already selects `type`.
- `AuthService.GetUserByID` has no other caller; the console handlers go through `UserService`.

## 5. Invitation acceptance (`workspace_service.go`, `workspace_handler.go`)

- `WorkspaceService.AcceptInvitation` checks, right after loading the invitation and before any user
  lookup or write: `invitation.WorkspaceID == workspaceID`, `invitation.Email == email` (the comparison
  `handleVerifyInvitationToken` already makes) and `invitation.ExpiresAt` in the future. A mismatch returns
  a new `domain.ErrInvalidInvitation`; nothing is created.
- The account lookup and creation use the stored invitation's email. An existing account found by that
  email whose type is not `user` (an API key account) is refused with the same error, as the OIDC sign-in
  and magic code paths already refuse API key accounts.
- The audit target uses the stored email.
- `handleAcceptInvitation` answers 401 "Invalid or expired invitation token" for that error, instead of 500.
  Its response keeps the token's email, which the check above makes equal to the stored one.
- `handleVerifyInvitationToken` also refuses an invitation whose stored `ExpiresAt` has passed (401).
- `ValidateInvitationToken` parses with `jwt.WithExpirationRequired()`: every invitation token Notifuse
  issues carries `exp`, so a token without one is refused.

## 6. CHANGELOG and docs

- One `**Security**` bullet under 42.0 saying what is now refused and checked, with the operator action in
  bold: an installation whose `SECRET_KEY` is the old compose default sets `SIGNING_KEY` before upgrading.
- `make docker-compose-up` runs `docker compose up -d`, so it now needs `SECRET_KEY` in the environment or
  in `.env`, as the documented quick start will say. No CI job runs `compose.yaml`; the integration
  environment uses `tests/compose.test.yaml`. `env.example` carries a placeholder, not the default.
- Docs (Notifuse/docs, separate PR to `dev`): installation page (compose quick start writes `.env` with a
  generated `SECRET_KEY`; `SIGNING_KEY` in the variables table; root sign-in example signs with
  `SIGNING_KEY` when set).

## Tests, written first

| Implementation file | Test file | Cases |
| --- | --- | --- |
| `config/config.go` | `config/config_test.go` | default as `SECRET_KEY` without `SIGNING_KEY` refused, error names `SIGNING_KEY`; same through `PASETO_PRIVATE_KEY`; same with surrounding spaces; default as `SIGNING_KEY` refused; default without padding, in the URL alphabet, with embedded newlines, and as raw decoded bytes each refused; default `SECRET_KEY` with a fresh `SIGNING_KEY` loads, `SecretKey` keeps the default, `SigningKey` and `JWTSecret` come from `SIGNING_KEY`; `SIGNING_KEY` with a trailing newline is trimmed; whitespace-only `SIGNING_KEY` counts as unset; no `SIGNING_KEY` and another `SECRET_KEY` gives `SigningKey == SecretKey` and the same `JWTSecret` as before |
| `internal/repository/auth_repository.go` | `auth_repository_test.go` | query selects `type`, `Type` is scanned |
| `internal/service/auth_service.go` | `auth_service_test.go` | API key token naming a regular user refused; naming an API key user accepted; revoked key still answers `ErrAPIKeyRevoked`; session token whose user is an API key user refused; invitation token without `exp` refused |
| `internal/service/user_service.go` (sessions) | `user_service_test.go` | `VerifyUserSession` for an API key user refused with `ErrSessionAuthFailed`; regular user accepted |
| `internal/service/user_service.go` | `user_service_test.go` | root sign-in HMAC under the signing key accepted, under a different `SecretKey` refused; magic codes hashed and verified with the signing key |
| `internal/service/oidc_service.go` | `oidc_service_test.go` | flow state sealed with the signing key opens; sealed with the encryption key does not |
| `internal/http/task_handler.go`, `usage_handler.go` | their `_test.go` | signature from the signing key accepted, from a different `SecretKey` refused |
| `internal/service/workspace_service.go` | `workspace_service_test.go` | workspace mismatch, email mismatch (including letter case) and expired invitation each refused with `ErrInvalidInvitation` and no user lookup, creation, membership or session; existing account of type `api_key` refused with no membership or session; existing fixtures carry a stored expiry and account type |
| `internal/http/workspace_handler.go` | `workspace_handler_test.go` | accept: `ErrInvalidInvitation` answers 401; verify: expired stored invitation answers 401 |
| `internal/service/demo_service.go` | `demo_service_test.go` | demo reset HMAC verifies under `SigningKey`, not `SecretKey` |
| `cmd/hmac/main.go` | `cmd/hmac/main_test.go` | helper output verifies under the key it was given and not another |
| `internal/app/app.go` | `app_test.go` and the integration suite | test config carries a distinct `SigningKey`; the wiring itself is exercised end to end by the harness running on distinct keys |
| full stack | `tests/integration/signing_key_test.go` | harness on distinct keys: root sign-in and `/api/tasks.execute` signed with `SECRET_KEY` refused, with `SIGNING_KEY` accepted; an API key token naming the owner refused while a real API key works; invitation tokens naming another email or workspace refused, seating nobody and creating no account, while the issued token is accepted |

## Test run

`make test-domain test-service test-repo test-http test-migrations test-pkg`, `go test ./config/...`,
`go test ./internal/app/...`, then the integration suite (`make test-integration`), plus
`docker compose config` on both compose files with and without `SECRET_KEY`.
