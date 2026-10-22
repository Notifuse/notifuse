# Auth hardening plan

Target: 42.0 (unreleased). No schema migration: the re-encryption below rewrites values in place.

## Goals

1. `SECRET_KEY` accepts several values, comma-separated. The first encrypts and signs everything; the
   others only decrypt. At startup Notifuse re-encrypts with the first value whatever is stored under
   another one, so an installation can replace its key without losing its data and without a new variable.
2. Notifuse refuses to start when the first value is a key Notifuse itself published (the `SECRET_KEY`
   that `compose.yaml` used to ship as a default, or the `env.example` placeholder) or is shorter than a
   generated key, as the documentation's placeholders are. The error gives the rotation to apply.
3. `compose.yaml` requires `SECRET_KEY`, as `compose.alloydb.yaml` already does.
4. A token authenticates only the kind of user it was issued for: an API key token an API key user, a
   session token a regular user.
5. Accepting an invitation requires the token to name the stored invitation's workspace and email, and
   the invitation to be unexpired.
6. The API no longer serves a workspace's encrypted secret key.

## Why a key list rather than a second variable

`SECRET_KEY` encrypts each workspace's own secret key, its integrations' credentials and the system
settings, and signs every token. `Workspace.AfterLoad` fails when it cannot decrypt the workspace secret
key, and `WorkspaceRepository.List` returns that error, so after a plain `SECRET_KEY` change no
workspace loads. A second variable for signing (the previous version of this plan) avoided that, but left
stored data under the published key and added a variable. Giving the old value after the new one lets
Notifuse open the old data once, re-encrypt it, and sign with the new key only.

Encryption is AES-256-GCM (`pkg/crypto`), which authenticates: a wrong key fails to open a value instead
of producing garbage, and every integration's `DecryptSecretKeys` returns that error. So for each stored
value Notifuse can tell which listed key opens it.

## 1. Reading SECRET_KEY (`config/config.go`, `config/secret_key.go`)

- `SecurityConfig` keeps `SecretKey` (the first value: it encrypts, signs, and `JWTSecret` is derived
  from it as today) and gains `PreviousSecretKeys []string` (the other values). The field name says the
  previous keys never sign; they are handed to exactly two places, `loadSystemSettings` and the startup
  re-encryption. Nothing that signs or verifies receives them, so a token signed with an old value, the
  published key included, is refused.
- The value comes from `SECRET_KEY`, else `PASETO_PRIVATE_KEY`, as today, and both read the same way:
  - No comma: one key, byte for byte as today. An installation that changes nothing reads the same key.
  - A comma: the value is a list. One pair of matching quotes around the whole value is removed first
    (`docker --env-file` keeps them; a list is new, so no stored data depends on them). A single comma
    separates two keys; two commas in a row stand for one comma inside a key, so a hand-written key that
    contains commas can still be listed. Whitespace around each key is trimmed. An empty key is refused,
    and so is a first key that starts or ends with a quote. A duplicate of an earlier key is dropped.
  - Each previous key is also tried exactly as written (spaces and a trailing newline kept, as a key read
    from a file may carry), without one pair of matching quotes, and wrapped in double or single quotes: a
    single key read through `docker --env-file` kept its quotes, and its data was encrypted with them. The
    current key in quotes, listed after it, is tried the same way. These candidates only decrypt, so
    trying them costs nothing. The whole value is never a candidate: a legacy key containing a single
    comma would then open its data, be re-encrypted under its first piece without a word, and break
    every API key, where refusing to start says to write the comma twice.
  - A legacy key that contains a comma is read as a list after the upgrade. It cannot slip through: the
    refusals below and the startup check (section 2), which also runs whenever the value contains a
    comma, stop it before any write, and their message says to write each comma of such a key twice.
- Refusals, all at startup before any write, each naming the variable the value came from:
  - The first value is a published value: the compose default in any encoding (padded or not, standard
    or URL alphabet, raw decoded bytes, whitespace removed), the `env.example` placeholder, either in one
    pair of matching quotes. Checked against SHA-256 digests kept as hex constants, so the code carries no
    key literal.
  - The first value gives fewer than 32 bytes of key material (`deriveJWTSecret`), as `abc123#xyz789` or
    `your-secret-key` do. A legacy `PASETO_PRIVATE_KEY` decodes to 64 bytes and is unaffected.
  - Both refusals give the same remedy: generate a key with `openssl rand -base64 32` and put it first,
    keeping the current key after a comma (`SECRET_KEY=<new key>,<current key>`, each comma inside the
    current key written twice); every instance is stopped, then started with it; Notifuse re-encrypts
    stored data with the new key when it starts, and once every instance runs the new key is set alone;
    users sign in again, API keys must be deleted and created again, pending invitations sent again.
  - A published value later in the list is accepted: that is how an installation leaves it.
- `loadSystemSettings` decrypts each of the five `encrypted_*` settings it reads with the first value,
  then each previous one. A setting no key opens is skipped as today; with a list, the startup check
  below then refuses to start.
- `SecurityConfig.SecretKeyIsList` records that the value contained a comma, for the startup check, and
  `SecurityConfig.KeyVariable` the variable the value came from, which the startup check names.
- `config.NewSecurityConfig(raw, variable)` turns the value into the `SecurityConfig` (`JWTSecret`,
  `SecretKey`, `PreviousSecretKeys`, `SecretKeyIsList`) or the refusal; `Load`, `cmd/hmac` and the test
  harness all go through it. `SecurityConfig.DecryptionKeys()` lists the first key, then the others.

## 2. Re-encryption at startup (`pkg/crypto`, `internal/repository/secret_key_rotation.go`, `internal/app/app.go`)

- Every value Notifuse encrypts with `SECRET_KEY` sits in the system database, in a typed `Encrypted*`
  field of a workspace's settings or integrations (the workspace key, the file storage key, each
  integration's credentials) or in a `settings` row named `encrypted_*`.
  `domain.EncryptedFieldPaths()` derives, by reflection over `WorkspaceSettings` and `Integrations`, the
  JSON path of every `Encrypted*` string field, through pointers, structs and slices but never into a map
  or an interface: those hold free-form content (custom field labels, template block attributes) whose
  keys a user chooses, so a label named `encrypted_note` is never taken for a credential. A later
  credential is included without anyone listing it.
- `crypto.ReencryptFields(doc, paths, current, previous)` walks a JSON document (numbers kept as written)
  and, for every non-empty string at one of `paths`, opens it with `current`, then each
  previous key in order; a value a previous key opened is re-encrypted with `current`. A value `current`
  opens, or that no key opens, is left as it is. It returns the document and whether it changed. It
  never decrypts into the document, so nothing plain is written. The workspace's `AfterLoad`/`BeforeSave` are
  not used: `BeforeSave` re-encrypts only the credentials it has in clear, and leaves an SMTP username
  as it was loaded, so a round trip would keep that value under the old key.
- `App.Initialize` runs it (`App.InitSecretKey`, calling `repository.RotateSecretKey`) right after
  `InitDB` (migrations done), before the services start, when the value was a list: it
  re-encrypts what a previous key opens and checks the rest, so a legacy key read differently fails at
  startup and not at the first workspace load. A single key whose commas were doubled is a list of
  one: it only checks.
- A value without a comma is one key, used byte for byte, so quotes or a trailing newline around it are
  part of it. At startup Notifuse checks that it opens the key of at least one stored workspace
  (`repository.SecretKeyOpensAWorkspace`: one small field per workspace, read without locks, stopping at
  the first that opens). When there are workspaces and it opens none, every workspace load would fail:
  the start is refused, naming the variable, and when the key without its quotes or surrounding spaces
  opens one, the message says to remove them. This catches the last step of a replacement written
  differently from the first key of the list (`SECRET_KEY="<new>"` through `docker --env-file`, or a
  secret file ending with a newline), and a new key set alone without the list. A single workspace that
  does not open while others do starts as before.
- One transaction on the system database, holding `pg_advisory_xact_lock` on a fixed id, so instances
  starting together rotate once; the next one finds nothing left to do.
- `workspaces`: `SELECT id, settings, integrations ... ORDER BY id FOR UPDATE`; both columns go, each
  with its paths, through `ReencryptFields`. The result is then opened as the server loads a workspace:
  `domain.CheckStoredWorkspace(settings, integrations, current)` decodes both documents as
  `ScanWorkspace` does and runs `AfterLoad`, which decrypts what the workspace uses (its key, its file
  storage key, the credentials of each integration's active provider). A row is updated (those two
  columns only) when one changed.
- Which values must open is therefore decided by the code that reads them, not by the paths. A value no
  key opens under a provider an integration does not use (an API client could store one, and every
  version before 42.0 kept it as sent and never read it) does not stop the start; one the workspace
  uses does, as every load of that workspace would fail on it.
- `settings`: every row whose key starts with `encrypted_` and whose value is not empty (SMTP username
  and password, SMTP bridge TLS certificate and key, OIDC client secret, and rows nothing reads any more,
  such as the `encrypted_smtp_relay_*` rows v29 copied to their `smtp_bridge` names), read
  `ORDER BY key FOR UPDATE`, is re-encrypted when a previous key opens it (its `value` only). A setting
  no key opens is left as it is: the server skips such a setting when it loads the settings, as before
  42.0, so it would not fail on it either.
- A value a workspace uses that no listed key opens rolls the transaction back and fails startup,
  naming the workspace id and the failing credential, saying no key of the variable (`KeyVariable`)
  opens it and that each comma inside a key is written twice. Nothing is half-rotated. A wrong previous
  key is caught this way, as it opens no workspace key.
- It logs how many workspaces and settings it re-encrypted and that, once every instance runs with the
  list, the first key is set alone.
- Out of reach, said in the docs: workspace databases hold nothing under `SECRET_KEY` (message data and
  web identify tokens use each workspace's own key, which keeps its value); database backups taken
  before the rotation stay readable with the old key; an instance still running with the old key alone
  cannot read rotated rows, and an instance running the list reads only the first key at runtime, so
  every instance is stopped, then started with the list (no rolling restart). The list is set in the
  same step as the upgrade to 42.0, never on an earlier version, which would read it as one key. Once
  every instance runs, the new key is set alone: each start with a list opens every workspace again.

## 3. No row written that the current key cannot open (`workspace_postgres.go`)

The console keeps the encrypted forms it loaded and echoes them back when an owner saves an
integration; validation accepts an echoed form in place of the credential. A page loaded before a
rotation echoes a form under the old key, `BeforeSave` keeps it as is, and after the write every load of
that workspace fails. `workspaceRepository.Update` already opens the saved row with `AfterLoad`, but
after the `UPDATE`. `Create` and `Update` now open the marshalled `settings` and `integrations` with
`domain.CheckStoredWorkspace(settings, integrations, current)` before writing, and on failure write
nothing and return `domain.ErrStaleCredential`, which the workspace and integration handlers answer 400
("A saved credential of this workspace changed or can no longer be read. Reload the page and save again, or enter the credential again."). This also stops a
member from storing a value that is no ciphertext at all where the workspace reads it. A value under a
provider the integration does not use is saved as before 42.0, as nothing reads it.

## 4. Signing stays on the first value (`SIGNING_KEY` removed)

- Every signer uses `SecretKey`, the first value, as on `dev`: JWTs (sessions, API keys, invitations),
  magic code hashes, root sign-in HMAC, the OIDC flow-state cookie, `/api/tasks.execute` and usage read
  signatures, the demo reset HMAC. `SIGNING_KEY` leaves config, both compose files, the Makefile,
  `env.example`, the API reference and the docs. The consumer renames made for it are reverted to their
  `dev` names, so the diff carries behaviour changes only. Kept: `domain.VerifyEmailHMAC` compares in
  constant time.
- `cmd/hmac` and `make demo-hmac` sign with the first value of the key they are given, and refuse a value
  the server refuses.
- The test harness takes its keys from `config.NewSecurityConfig(TestSecretKey)`, so its JWT secret is
  the one an installation with that key derives, and `ServerManager.SetSecretKey` gives the next start
  another `SECRET_KEY` value.
- The control plane, outside this repository, derives the usage key from the tenant's key. After a
  rotation it must use the new first value, or that tenant's usage reads answer 401.

## 5. Compose and env.example

- `compose.yaml`: `SECRET_KEY` becomes `${SECRET_KEY:?…}`, quoted so YAML does not read the colon. The
  message tells a new installation to generate a key with `openssl rand -base64 32`, and an installation
  that relied on the former default to read the 42.0 entry of the CHANGELOG; the comment above it says how
  to replace the key with a list. Compose checks a required variable for every
  command, `down`, `ps` and `logs` included, as it already does for `compose.alloydb.yaml`.
- `env.example` leaves `SECRET_KEY` empty with the generation command beside it, and says how to rotate.

## 6. Token type check (`auth_repository.go`, `auth_service.go`)

- `SQLAuthRepository.GetUserByID` also selects `type`, and `name` while at it: the audit log names the
  actor from this row, and both were missing (shipped in 41.0, hence a `Fix` line).
- `AuthService.AuthenticateUserFromContext`: the API key branch treats a loaded user whose type is not
  `api_key` like a missing one, so it surfaces as `ErrAPIKeyRevoked` (401 through `writePermissionError`).
  The session branch (`AuthService.VerifyUserSession`) returns `ErrUserNotFound` unless the type is `user`,
  the same answer as for a session naming a user that does not exist.
- `UserService.VerifyUserSession` (used by `/api/user.me`) applies the same session rule and wraps the
  refusal in `ErrSessionAuthFailed`, so it answers 401. Its repository already selects `type`.
- The SMTP bridge authenticates with an API key token outside the HTTP middleware, so after the email
  match it builds the same context and calls `AuthenticateUserFromContext`: one copy of the rule. A
  revoked or mismatched account answers "invalid API key"; a lookup failure does not read as one.
- The root-only routes (`/api/settings.*`, `/api/licence.*`) checked only that the token's user id was a
  root email. Root is a person, so `requireRootUser` now requires a user token with a session and checks
  it through `UserService.VerifyUserSession`. An API key token answers 403 (a working key that lacks the
  access, not a dead one), a signed-out root session 401, a failed check 500 (the console signs out on
  any 401). That last mapping is `user.me`'s; both take it from one helper, `sessionCheckStatus`.
- The demo seed acted as root by labelling it an API key, which the rule above refuses. It now opens a
  session for root through `UserService.startSession`, as a sign-in does, and closes it with
  `UserService.endSession` when the seed is done (a session signed out meanwhile counts as closed). Root
  sign-in opens its session through the same `startSession`. Right after creating the demo workspace it
  authenticates once for it (`AuthenticateUserForWorkspace`, through the `AuthService` its constructor
  receives), so a demo visitor signing root out mid-seed, which ends every root session, does not stop
  the seed. A sign-out during the few seconds of workspace creation itself still fails that reset, which
  then answers 500 and can be run again.

## 7. Invitation acceptance (`workspace_service.go`, `workspace_handler.go`)

- `WorkspaceService.AcceptInvitation` checks, right after loading the invitation and before any user
  lookup or write: `invitation.WorkspaceID == workspaceID`, `invitation.Email == email` (the comparison
  `handleVerifyInvitationToken` already makes) and `invitation.ExpiresAt` in the future. A mismatch returns
  `domain.ErrInvitationUnavailable`; nothing is created. An invitation already accepted or cancelled
  (`ErrInvitationNotFound`) gives the same error instead of a 500, and so does an invitee found already
  seated, or whose account the other request just created (`ErrUserExists`).
- `InviteMember` refuses an address that belongs to an API key account (`ErrUnauthorized`, 403).
- As `JoinInvitation` does, the audit record is given the invitation's workspace and target before any
  check, and each refusal a reason (`invitation_mismatch`, `invitation_expired`, `api_key_identity`,
  `already_member`), so a refused attempt lands in that workspace's audit log.
- The account lookup and creation use the stored invitation's email. An existing account found by that
  email whose type is not `user` is refused with the same error.
- Both public invitation routes answer every link that does not open an invitation with 404 "Invitation
  not found or no longer valid" (`writeInvitationUnavailable`): a token that is expired, malformed or
  signed with another key, a stored invitation that no longer matches or has expired, and
  `ErrInvitationUnavailable`. Not 401: the console signs the user out on any 401, and a new first value
  turns every pending link into one signed with another key.
- `ValidateInvitationToken` parses with `jwt.WithExpirationRequired()` and refuses a token that names no
  invitation, workspace or email (a session token, signed with the same key, reached the lookup with an
  empty id and answered 500).

## 8. Served ciphertexts (`workspace_redact.go`)

- `Workspace.Redact` clears `settings.encrypted_secret_key`, the workspace's own key, which members, API
  keys and, through the public `workspaces.verifyInvitationToken`, holders of an invitation link were
  served. Nothing reads it from a client.
- The integrations' `encrypted_*` fields stay served: the console echoes them back (section 3). After a
  rotation they are under the new key, which the published one does not open.

## 9. CHANGELOG and docs

- One `**Security**` bullet under 42.0: `SECRET_KEY` accepts a list and re-encrypts stored data under its
  first value at startup; Notifuse refuses to start with a published or short first value; `compose.yaml`
  requires `SECRET_KEY`; what is now checked and no longer served. The operator action in bold: an
  installation whose `SECRET_KEY` is a published value or shorter than 32 bytes sets
  `SECRET_KEY=<new key>,<current key>` before upgrading and removes the old value once Notifuse has
  started; everyone signs in again, API keys are deleted and created again, pending invitations sent
  again; scripts calling `user.rootSignin` or `demo.reset` sign with the new key. An installation that
  never set `SECRET_KEY` runs on the former compose default, which the installation guide gives.
- One `**Fix**` bullet: audit log entries (shipped in 41.0) for workspace actions carry the actor's name,
  and actions taken with an API key the `api_key` actor type.
- One `**Improvement**` bullet: an invitation link already accepted, cancelled or expired says so instead
  of failing with an error or signing out the person who opened it.
- Docs (Notifuse/docs, a new branch from `dev`, PR to `dev`): installation page (quick start for a new
  installation with a warning for upgraders, generating `SECRET_KEY` only when `.env` has no
  `SECRET_KEY` line; `docker run` passes `SECRET_KEY`; a "Rotating SECRET_KEY" section with the list
  syntax, the restart of every instance, removing the old value afterwards, and what stays readable with
  the old key); root sign-in example signs with the first value.

## Tests, written first

| Implementation file | Test file | Cases |
| --- | --- | --- |
| `config/secret_key.go`, `config/config.go` | `config/secret_key_test.go`, `config/config_test.go` | `NewSecurityConfig` signs with the first key of a list and decrypts with all of them, uses a single key byte for byte, and refuses what `Load` refuses naming the variable; one key without a comma read byte for byte, quotes included, not marked as a list; `new,old` gives `SecretKey` new and `PreviousSecretKeys` old with its quoted forms, marked as a list; quotes around the whole list removed; two commas read as one inside a key, alone or among others; spaces around keys trimmed; an empty key refused; a first key starting or ending with a quote refused; a duplicate dropped; a legacy key with a comma refused when its first piece is short, with the doubled-comma hint; a previous key also tried exactly as written (a trailing newline kept) and the current key in quotes, listed after it, tried as a previous key; the whole value is not a candidate; `KeyVariable` names the variable the value came from; first value the compose default refused in every encoding and in quotes, through `PASETO_PRIVATE_KEY` naming it; placeholder refused; first value under 32 bytes refused with the rotation remedy, at 32 accepted; the compose default as a previous value accepted; `JWTSecret` derived from the first value only; `loadSystemSettings` opens a setting under a previous value |
| `pkg/crypto/reencrypt.go` (`ReencryptValue`, `ReencryptFields`) | `pkg/crypto/reencrypt_test.go` | value under `current` untouched and not reported as changed; value under a previous key re-encrypted and opening with `current` only; values at the listed paths walked through nested objects and arrays; values off the paths untouched whatever their key is named; empty strings and non-strings untouched; large numbers kept as written; a value no key opens left as it is; with no previous key nothing changes |
| `internal/domain/workspace_encrypted_fields.go` | `workspace_encrypted_fields_test.go` | the paths reach the workspace key, the file storage key and nested integration credentials; no path enters a map or a template block; no map of either type holds a struct with an `Encrypted*` field; every `Encrypted*` field is a string, and the only interface reachable is a template block's; every `Encrypted*` field of a filled workspace, once marshalled, sits at one of the paths |
| `internal/domain/workspace.go` (`CheckStoredWorkspace`) | `workspace_test.go` | documents a workspace saved under the key opens; the workspace key, or the active provider's credential, under another key fails with `crypto.ErrUndecryptable` naming the integration; a value no key opens under a provider the integration does not use passes; documents that do not decode fail without `ErrUndecryptable` |
| `internal/repository/secret_key_rotation.go` | `secret_key_rotation_test.go` | the advisory lock is taken first; rows read in id and key order; a workspace row under a previous key updated (settings and integrations only), one under the first value not updated, free-form content named like a credential ignored; a value no key opens under a provider the integration does not use left as it is without stopping; with no previous key, values are only checked and nothing is updated; settings `encrypted_*` rows re-encrypted, other settings untouched, a setting no key opens left as it is; a workspace key or active credential no key opens rolls back with the workspace id in the error; `SecretKeyOpensAWorkspace` is true with no workspace, true when one of several opens, false when none does, and reads without locks |
| `internal/repository/workspace_postgres.go` | `workspace_core_test.go` | `Update` and `Create` with an integration echoing a form under another key, or a value that is no ciphertext, write nothing and return `ErrStaleCredential`; a form under the current key saves; free-form content named like a credential (`custom_field_labels.encrypted_note`) saves; a value no key opens under a provider the integration does not use saves |
| `internal/http/workspace_handler.go` | `workspace_handler_test.go` | `ErrStaleCredential` answers 400 on workspace create and update and integration create and update, saying to reload the page or enter the credentials again; existing invitation cases (404 for every dead link) |
| `internal/app/app.go` | `app_test.go` | `InitSecretKey` with a single key starts when it opens a stored workspace key or there is none, and refuses naming the variable when it opens none, saying to remove the quotes or spaces when the key without them opens one; with a list it re-encrypts under the first key; a value no listed key opens stops the start naming the workspace, the variable the keys came from and the doubled-comma rule; a database failure stops it as a database failure; the startup telemetry goroutine keeps its own reference to the service, as a restart in the same process sets the field again (the rotation integration test restarts under `-race`) |
| `internal/repository/auth_repository.go` | `auth_repository_test.go` | query selects `name` and `type`, both are scanned |
| `internal/service/auth_service.go` | `auth_service_test.go` | API key token naming a regular user refused; naming an API key user accepted; revoked key still answers `ErrAPIKeyRevoked`; session token whose user is an API key user refused; invitation token without `exp` refused; a session token refused as an invitation token |
| `internal/service/user_service.go` | `user_service_test.go` | `VerifyUserSession` for an API key user refused with `ErrSessionAuthFailed`; regular user accepted |
| `internal/service/workspace_service.go` | `workspace_service_test.go`, `workspace_users_service_test.go` | workspace mismatch, email mismatch, expired invitation, already-seated invitee and account created meanwhile each refused with `ErrInvitationUnavailable`, audited with the reason, nothing created; existing `api_key` account refused; inviting an API key address refused with `ErrUnauthorized` |
| `internal/domain/workspace_redact.go` | `workspace_redact_test.go` | `Redact` clears the workspace's encrypted secret key; `RedactForPublic` serves no encrypted form; integration encrypted forms still served to members |
| `internal/http/utils.go`, `user_handler.go`, `settings_handler.go` | their `_test.go` | `sessionCheckStatus` 401 for `ErrSessionAuthFailed`, 500 otherwise; root-only routes: API key 403, signed-out session 401, failed check 500, live root session 200 |
| `internal/service/smtp_bridge_handler.go` | `smtp_bridge_handler_test.go` | token naming a regular account refused; missing account refused; lookup failure not read as a revoked key; API key account accepted |
| `internal/domain/contact.go` | `contact_test.go` | existing `VerifyEmailHMAC` cases pass with the constant-time comparison |
| `internal/service/demo_service.go` | `demo_service_test.go` | root person gets a session of its own whose context `AuthService` accepts, closed at the end; a session that cannot be closed only logs; a root API key account opens no session; a session that cannot be opened stops the seed; a session signed out meanwhile counts as closed; a refused workspace check stops the seed |
| `cmd/hmac/main.go` | `cmd/hmac/main_test.go` | prints the HMAC and reset URL the server verifies; given a list, signs with its first value; a key the server refuses prints the refusal on stderr and exits 1; missing arguments print the usage and exit 1 |
| full stack | `tests/integration/secret_key_rotation_test.go` | a workspace with integration credentials (an SMTP integration with username and password among them) and encrypted system settings stored under key A; an app started with `B,A` re-encrypts them: every raw `encrypted_*` value opens with B and not A, credentials unchanged through the API; a value no key opens under a provider the integration does not use, written straight to the row, is left as it is and stops nothing; a second start with `B,A` changes nothing; a start with `C,D` fails and leaves every row as it was; a start with a legacy key containing a comma, written once, fails, and written twice starts; after the rotation, the new key alone in quotes is refused at startup and the new key alone starts; root sign-in and a JWT signed with A refused, with B accepted; an integration save echoing a form under A answers 400 and writes nothing |
| full stack | `tests/integration/auth_hardening_test.go` (from `signing_key_test.go`) | an API key token naming the owner refused while a real API key works; invitation tokens naming another email or workspace refused, seating nobody; the issued token accepted once and 404 after; an expired link and one signed with another key answer 404 on verify and accept; inviting an API key address answers 403; no `settings.encrypted_secret_key` in `verifyInvitationToken` or `workspaces.get`; `/api/settings.get` answers a root session only |

## Test run

`make test-domain test-service test-repo test-http test-migrations test-pkg`, `go test ./config/...`,
`go test ./cmd/hmac/...`, `go test ./internal/app/...`, then the integration suite (`make test-integration`), plus
`docker compose config` on both compose files with and without `SECRET_KEY`.
