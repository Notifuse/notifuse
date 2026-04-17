# Plan: SMTP Bridge TLS Mode — Setup Wizard & Settings UI

## Context

Shipping [`SMTP_BRIDGE_TLS`](../CHANGELOG.md#unreleased) (off / starttls / implicit) closed GitHub issue #314 via env var only. The wizard and system-settings drawer still treat TLS as an implicit on/off derived from whether cert+key were provided. Two gaps remain from the post-ship review:

1. **Setup wizard** — no input for the mode. First-run operators cannot choose `off` (reverse-proxy deployment) or `implicit` (SMTPS) unless they also set the env var.
2. **System-settings drawer** — root-user runtime config editor has no mode input and currently hard-requires cert+key when the bridge is enabled, which breaks for mode=off.

Secondary gap: `internal/service/setting_service.go`'s `SystemConfig` struct has no `SMTPBridgeTLSMode` field, so the DB read/write path via the settings service ignores mode entirely. `config.go` already *reads* `smtp_bridge_tls_mode` from the `settingsMap` at startup, but no code path writes that key — so today the column is always empty.

We want mode to be a first-class, DB-persisted, UI-editable setting, with env-var override still winning at load time.

## Design

### Persistence

- Store `smtp_bridge_tls_mode` in the same settings table as other SMTP-bridge keys. **Unencrypted** — the value is an enum, not a secret, and encrypting it forces all consumers through the decrypt path for nothing.
- `setting_service.go`'s `SystemConfig.SMTPBridgeTLSMode` round-trips verbatim with validation at write time (reject unknown values; accept empty string meaning "auto-resolve at load").
- Both wizard and settings-drawer writes funnel through `SetSystemConfig` → DB.

### Load-time precedence (unchanged)

`config.Load` → `resolveSMTPBridgeTLSMode` in precedence order: env > DB > auto-resolve. Already implemented; no logic change needed. Adding the DB writer just means the middle tier stops being dead.

### UI contract

Both screens show a dropdown: **STARTTLS (default)** / **Implicit TLS** / **Off (behind reverse proxy)**. The cert/key inputs become **mode-conditional**:

- starttls or implicit → required
- off → ignored (rendered greyed out with a one-line hint)

When `env_overrides.smtp_bridge_tls_mode === true`, the dropdown is disabled and shows the "configured via environment variable" hint (existing `renderEnvHint` / `isOverridden` pattern).

### Cross-field validation

`setting_service.SetSystemConfig` should run `smtp_bridge.ValidateMode` on non-empty values and reject starttls/implicit without cert+key. Mirror the resolver's contract so invalid DB state cannot be written.

`setup_service.Initialize` does the same on user-supplied wizard input. Empty string remains legal (means "auto-resolve at startup").

## Files to modify

### Backend

| File | Lines | Change |
|------|-------|--------|
| `internal/service/setting_service.go` | 13-32 | Add `SMTPBridgeTLSMode string` to `SystemConfig` (after `SMTPBridgeTLSKeyBase64`) |
| `internal/service/setting_service.go` | ~167 | In `GetSystemConfig`, after the TLS key decrypt block, add `if setting, err := s.repo.Get(ctx, "smtp_bridge_tls_mode"); err == nil { config.SMTPBridgeTLSMode = setting.Value }` |
| `internal/service/setting_service.go` | ~332 | In `SetSystemConfig`, after the TLS key block, validate + persist `smtp_bridge_tls_mode`. If non-empty, call `smtp_bridge.ValidateMode` and reject unknowns. Also reject starttls/implicit when cert or key is empty. Write via `s.repo.Set(ctx, "smtp_bridge_tls_mode", config.SMTPBridgeTLSMode)`. Always write (including empty) so admins can clear back to auto-resolve |
| `internal/service/setting_service.go` | imports | Add `"github.com/Notifuse/notifuse/pkg/smtp_bridge"` |
| `internal/service/setup_service.go` | 17-35 | Add `SMTPBridgeTLSMode string` to `SetupConfig` |
| `internal/service/setup_service.go` | 283-302 | Extend the env-vs-user merge for the bridge: declare `var smtpBridgeTLSMode string`, set from `s.envConfig.SMTPBridgeTLSMode` when `status.SMTPBridgeConfigured`, else from `config.SMTPBridgeTLSMode` |
| `internal/service/setup_service.go` | 305-324 | Include `SMTPBridgeTLSMode: smtpBridgeTLSMode` in the `SystemConfig` literal |
| `internal/http/setup_handler.go` | 52-70 | Add `SMTPBridgeTLSMode string \`json:"smtp_bridge_tls_mode"\`` to `InitializeRequest` (after `SMTPBridgeTLSKeyBase64`) |
| `internal/http/setup_handler.go` | ~193 | Include `SMTPBridgeTLSMode: req.SMTPBridgeTLSMode` in the `setupConfig` literal |
| `internal/http/settings_handler.go` | 22-40 | Add `SMTPBridgeTLSMode string \`json:"smtp_bridge_tls_mode"\`` to `SystemSettingsData` |
| `internal/http/settings_handler.go` | 144 | Include `SMTPBridgeTLSMode: sysConfig.SMTPBridgeTLSMode` in the GET response. No masking — enum isn't sensitive |
| `internal/http/settings_handler.go` | 213-232 | Include `SMTPBridgeTLSMode: reqData.SMTPBridgeTLSMode` in the `newConfig` literal on POST |

### Frontend

| File | Lines | Change |
|------|-------|--------|
| `console/src/types/setup.ts` | 1-19 | Add `smtp_bridge_tls_mode?: 'off' \| 'starttls' \| 'implicit'` to `SetupConfig` |
| `console/src/types/settings.ts` | 1-19 | Add `smtp_bridge_tls_mode?: 'off' \| 'starttls' \| 'implicit'` to `SystemSettingsData`. Match whatever nullability pattern the rest of that interface uses |
| `console/src/pages/SetupWizard.tsx` | ~612 (before the cert textarea) | Add `<Form.Item label=…name="smtp_bridge_tls_mode" initialValue="starttls">` rendering an antd `<Select>` with 3 options. Label strings wrapped in lingui `t\`…\`` |
| `console/src/pages/SetupWizard.tsx` | 613-647 | Change cert/key validation rules from `required: true` to `required: mode !== 'off'`. Read the mode off the form via `Form.useWatch('smtp_bridge_tls_mode', form)` |
| `console/src/pages/SetupWizard.tsx` | 117-126 | In `handleSubmit`, add `setupConfig.smtp_bridge_tls_mode = typeof values.smtp_bridge_tls_mode === 'string' ? values.smtp_bridge_tls_mode : undefined`. When mode is `off`, do NOT send cert/key (even if the form still holds stale values) |
| `console/src/components/settings/SystemSettingsDrawer.tsx` | 421-501 | Insert a new row above the cert/key row for the TLS mode select. Disabled via `isOverridden('smtp_bridge_tls_mode')`; hint via `renderEnvHint('smtp_bridge_tls_mode')` |
| `console/src/components/settings/SystemSettingsDrawer.tsx` | 475, 490 | Change cert/key `required: bridgeEnabled` to `required: bridgeEnabled && mode !== 'off'`. Use `Form.useWatch('smtp_bridge_tls_mode', form)` |
| `console/src/components/settings/SMTPBridgeSettings.tsx` | (from previous work) | No change — read-only display already three-state |

### Docs

| File | Change |
|------|--------|
| `CHANGELOG.md` | Extend the existing `[Unreleased]` entry with "…also selectable via the setup wizard and system-settings UI" |

## Tests

### Unit — `internal/service/setting_service_test.go`

Existing tests (around line 78+) use `MockSettingRepository`. Add:

- `TestSetSystemConfig_PersistsSMTPBridgeTLSMode` — round-trip `"off"`, `"starttls"`, `"implicit"`, `""` through Set then Get. Assert unencrypted (look at the raw repo key `smtp_bridge_tls_mode`, not `encrypted_*`).
- `TestSetSystemConfig_RejectsUnknownTLSMode` — `SystemConfig{SMTPBridgeTLSMode: "bogus"}` → error, repo untouched.
- `TestSetSystemConfig_RejectsSTARTTLSWithoutCerts` — starttls + empty cert/key → error.
- `TestSetSystemConfig_AllowsOffWithoutCerts` — off + empty cert/key → success.
- `TestGetSystemConfig_ReadsSMTPBridgeTLSMode` — mock repo returns `"implicit"` for the key → `config.SMTPBridgeTLSMode == "implicit"`.

### Unit — `internal/service/setup_service_test.go`

- Extend the "all fields set" `GetEnvOverrides` case (already done in prior work — just verify it still passes).
- `TestInitialize_PersistsBridgeTLSMode_FromUser` — `SetupConfig{...SMTPBridgeTLSMode: "off", ...}` + no env override → settingService receives `SystemConfig.SMTPBridgeTLSMode == "off"`.
- `TestInitialize_EnvBridgeTLSMode_Wins` — env has `SMTPBridgeTLSMode: "implicit"` (and full bridge config so `SMTPBridgeConfigured` is true), user input has `"off"` → settingService receives `"implicit"`.

### Unit — `internal/http/setup_handler_test.go`

- Add a subtest to the existing Initialize table: request JSON includes `"smtp_bridge_tls_mode": "off"` → captured `SetupConfig` has `SMTPBridgeTLSMode: "off"`.

### Unit — `internal/http/settings_handler_test.go`

Mirror the existing TLS cert round-trip test:

- `TestSettingsHandler_Update_PersistsSMTPBridgeTLSMode` — POST with `"smtp_bridge_tls_mode": "implicit"` → settingService receives mode. Return 200.
- `TestSettingsHandler_Get_IncludesSMTPBridgeTLSMode` — mocked sysConfig with mode=starttls → GET response JSON contains `"smtp_bridge_tls_mode": "starttls"` (not masked).

### Integration — `tests/integration/settings_handler_test.go`

Extend the existing update-and-get test (line ~128) with a `smtp_bridge_tls_mode` field in the request; assert round-trip.

### Frontend — new

- `console/src/pages/SetupWizard.test.tsx` (if absent, add; or add a new `describe` block if the file exists) — render wizard, toggle bridge enabled, verify the mode `<Select>` is present with 3 options. Select `off`, verify cert/key inputs become non-required by submitting with them empty (form validates OK).
- `console/src/components/settings/SystemSettingsDrawer.test.tsx` (if absent, add) — render with mocked settings GET response, verify the mode select defaults to the current value. Verify `isOverridden`/disabled when env_overrides include `smtp_bridge_tls_mode`.

Skip frontend tests if no existing harness/fixtures make them cheap; at minimum, the settings-drawer test is worth having since a regression here silently breaks root-user config editing.

### Frontend — Lingui

Run `npm run lingui:extract && npm run lingui:compile` after adding the new labels:
- `STARTTLS (default)`
- `Implicit TLS (SMTPS)`
- `Off (behind reverse proxy)`
- Any helper strings for the "ignored when mode is off" hint.

## Verification

1. `make test-unit` — setting_service, setup_service, setup_handler, settings_handler.
2. `make test-integration` — settings_handler_test round-trip.
3. `cd console && npm test`.
4. Manual smoke:
   - Fresh `docker compose up` with an empty DB. Setup wizard: enable bridge, leave cert/key blank, select "Off", submit → succeeds. Restart. Server starts in plaintext mode with STARTTLS not advertised.
   - Setup wizard: enable bridge, select "STARTTLS", leave cert blank → form validation error.
   - Installed app as root user → settings drawer → change mode to "Implicit" + paste certs → save → server restarts → `swaks --tls-on-connect` succeeds on port 465.
   - Set `SMTP_BRIDGE_TLS=off` in env, open settings drawer → the mode dropdown is disabled with the env-override hint.
5. Back-compat: existing installations (DB has no `smtp_bridge_tls_mode` row) continue to work — `GetSystemConfig` returns empty string, resolver auto-resolves to starttls if certs present.

## Out of scope

- Changing env-var precedence (env still wins at load).
- A dedicated migration to backfill `smtp_bridge_tls_mode` for existing installs — the resolver handles empty-mode gracefully, and defaulting existing installs would accidentally lock them out of the "unset = auto-resolve" contract.
- Any change to `ApiCommandModal.tsx`, `SMTPBridgeSettings.tsx`, or the read-only endpoints — already updated in the prior PR.
