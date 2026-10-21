# Fix Vite dev-server advisories plan

**Status**: implemented
**Date**: 2026-10-21
**Scope**: the open Dependabot alerts on `Notifuse/notifuse`, as listed by `gh api repos/Notifuse/notifuse/dependabot/alerts?state=open` on 2026-10-21.

## 1. What the alerts are

Dependabot scans the default branch, `main`. The open alerts, checked against the lockfiles of `origin/main` and `origin/dev`:

| Alert | Manifest | Package | Advisory | Patched | `main` | `dev` |
|---|---|---|---|---|---|---|
| 259, 260, 261 | `web_analytics_sdk/package-lock.json` | `vite` | GHSA-698v-4r3x-3vcv, GHSA-587h-7wmj-h5pm, GHSA-r825-w8mm-5pxw | 8.3.3 | 8.3.0 | **8.3.0** |
| 251, 257 | `web_analytics_sdk/package-lock.json` | `brace-expansion` | GHSA-q2hr-2g5m-vwhr | 1.1.21 / 2.1.7 | 1.1.18 / 2.1.4 | 1.1.21 / 2.1.7 |
| 254, 255, 256 | console, notification center, SDK | `source-map-js` | GHSA-68fv-2mgg-jv7q | 1.2.2 | 1.2.1 | 1.2.2 |
| 253 | `web_analytics_sdk/package-lock.json` | `proxy-addr` | GHSA-jqcg-44mw-7w3h | 2.0.8 | 2.0.7 | 2.0.8 |
| 246 | `console/package-lock.json` | `dompurify` | GHSA-p98j-92pf-mc4p | 3.4.16 | 3.4.13 | 3.4.17 |

Everything except `vite` is already fixed on `dev` (the 42.0 dependency update) and closes when `dev` reaches `main`. **Only `vite` needs a change.**

The same three advisories, plus GHSA-9jrq-w75r-8gcw and GHSA-rq7h-c2jc-7f22, also cover `vite >=7.0.0 <7.3.7` (the console and the notification center lock 7.3.5) and `vite >=8.3.0 <8.3.3` (the SDK locks 8.3.0). GHSA-vfpm-58rq-9qcg (`?vite-wasm-instance` exposes the first 4 bytes of a file outside `server.fs`) covers `vite >=8.3.0 <8.3.3` only. `npm audit` reports five vite advisories in the console and the notification center and six in the SDK; Dependabot has only opened alerts for three of them, in the SDK. The console and notification center are fixed in the same change, since the same advisories apply.

All six advisories concern Vite's **development server** (`launch-editor` argument handling and file-existence check on Windows, `server.fs.allow` / `server.fs` bypasses, cross-origin script execution through custom middleware). The production bundles do not contain Vite's server. The risk is on a developer machine running `npm run dev`, not on a self-hosted instance.

## 2. Target versions: the minimum patched release of each line

- **console**: `vite` 7.3.5 → **7.3.7**, the first 7.x release with the fixes. 7.3.6 only widens its esbuild range to `^0.27.0 || ^0.28.0`; 7.3.7 updates `launch-editor` to 2.14.2 and fixes `safeModulePaths` and the `transformIndexHtml` filename. No 8.x major.
- **notification center**: same, 7.3.5 → **7.3.7**.
- **web analytics SDK**: `vite` 8.3.0 → **8.3.3**, the first 8.3.x release with the fixes (`launch-editor` 2.14.2, `fs.serve` check for `?vite-wasm-instance`, `safeModulePaths`, `transformIndexHtml` filename), not the latest 8.3.5. Moving from 8.3.0 cannot avoid 8.3.1 and 8.3.2, which are not security releases: optimizer and watcher fixes, `forwardConsole` limits, a cross-spawn → tinyexec refactor, and rolldown updates (§2 lockfile notes). 8.3.4 and 8.3.5 (8.3.5 published on 2026-10-21) are skipped because nothing here needs them. In the SDK, Vite only runs under Vitest: the bundle is built by Rollup, so the committed `dist/` must not change, and the Vitest suite is what would show a behaviour change from 8.3.1–8.3.3.

`package.json` floors move with the lock so a fresh `npm install` cannot resolve a vulnerable release:

- `console/package.json` and `notification_center/package.json`: `"vite": "^7.3.2"` → `"^7.3.7"` (devDependencies).
- `web_analytics_sdk/package.json`: Vite is today only a transitive dependency of `vitest` (`^6 || ^7 || ^8`, declared both as a dependency and as a required peer). Add `"vite": "^8.3.3"` to devDependencies, as the console and the notification center list it. An `overrides` entry was tried and dropped: it makes npm refuse a later `npm install -D vite` (`EOVERRIDE … conflicts with direct dependency`), and it would silently force Vite 8 onto a future Vitest that needs another Vite major, where a devDependency lets npm report that peer conflict at install time.

Commands, one per frontend, all with npm 11, which wrote the current lockfiles. npm 10 rewrites the console lockfile's `typescript` entry from `devOptional` to `dev`, and npm 11 writes it back, so a lockfile written by npm 10 would flip on the next npm 11 install. npm 10.9.4 also crashes on `npm install -D vite@…` in the SDK (`Cannot read properties of null (reading 'edgesOut')` in arborist's `#loadPeerSet`), as it already did on `dev`. CI installs with `npm ci`, which ignores these flags, under Node 22 (npm 10) for the console and the notification center and Node 24 (npm 11) for the SDK.

From the repository root:

```
(cd console && npm ci && npx -y npm@11 install -D vite@7.3.7)
(cd notification_center && npm ci && npx -y npm@11 install -D vite@7.3.7)
(cd web_analytics_sdk && npm ci && npx -y npm@11 install -D vite@8.3.3 --before=2026-10-07)
```

`--before=2026-10-07` (the day after vite 8.3.3 was published) only constrains the packages npm has to resolve anew, here `rolldown` and `@oxc-project/types`. Without it npm takes the newest `rolldown` 1.2.x, 1.2.15, published on 2026-10-21, the day of this change, along with its native `@rolldown/binding-*` packages. With it, the SDK gets `rolldown` 1.2.12, the release that was current when vite 8.3.3 shipped.

The SDK's Docker stage (`web-analytics-sdk-builder`) still runs `npm ci` on `node:22-alpine` (npm 10). `npm ci` installs from the lockfile and does not hit the `#loadPeerSet` crash: checked with npm 10.9.4 and npm 11 on the final lockfile, both install `vite@8.3.3`, and a following `npm install` under npm 11 leaves the lockfile unchanged.

Then check each lockfile diff: only `vite` and the dependencies it requires may change. Any unrelated package bump is reverted. A dry run in a scratch worktree of `origin/dev` gave:

- **console**: `vite` 7.3.5 → 7.3.7 and the root range. Nothing else.
- **notification center**: `vite` 7.3.5 → 7.3.7 and the root range. Nothing else (same lockfile with npm 10 and npm 11).
- **SDK**: `vite` 8.3.0 → 8.3.3 and the root devDependency, plus `rolldown` 1.2.9 → 1.2.12 with its `@rolldown/binding-*` platform packages and `@oxc-project/types` 0.150.0 → 0.152.0 (pinned exactly by `rolldown` 1.2.12). vite 8.3.3 depends on `rolldown ~1.2.11`, where 8.3.0 accepted `~1.2.6`, so `rolldown` 1.2.9 cannot stay. Nothing else.

## 3. Out of scope (reported to Pierre, not changed here)

- **`braces`** (GHSA-vfj7-8cjw-p6xm, console, dev-only): no patched release exists (`last_affected: 3.0.3`). Only `@lingui/cli` 6 drops it, a major upgrade of the whole i18n toolchain. Already noted as open in the 42.0 changelog. `npm audit` also marks `@tanstack/router-vite-plugin` → `@tanstack/router-plugin` → `chokidar` → `braces` as `fix=true`, because `@tanstack/router-plugin` ≥ 1.168.7 dropped `chokidar`; bumping it would still leave `braces` in the console through `@lingui/cli` 5, so it is not done here.
- **`katex`** (GHSA-238p-pmpm-9mq7, console): fixed in 0.18.2, but `@ant-design/x-markdown` 2.9.0 (latest) pins `^0.16.22` and `mermaid` 11 (required by `@ant-design/x`) pins `^0.16.45`. Kept open, as decided on 2026-10-15.
- **Go**: no Go alert is open on Dependabot. `govulncheck` does report `golang.org/x/net` and `golang.org/x/crypto` advisories and standard-library ones, but their fixes require `go 1.26` (Go 1.25 is out of support). That is a toolchain change with its own behaviour changes and is put to Pierre as a separate decision.

## 4. Tests first

There is no runtime behaviour to change, so the failing check written first is the scanner itself:

1. **Red**, on the unmodified branch: `npm audit --json` in each of the three frontends reports `vite` with GHSA-587h-7wmj-h5pm (and the other vite advisories). Record the output.
2. Apply §2.
3. **Green**: the same `npm audit` reports no `vite` advisory in any of the three. The notification center and the SDK audit clean (`npm audit` exits 0). The console keeps exactly the two advisories from §3, `katex` (GHSA-238p-pmpm-9mq7) and `braces` (GHSA-vfj7-8cjw-p6xm), reported through their dependents (`chokidar`, `micromatch`, `@lingui/*`, `@tanstack/router-*`, `mermaid`, `@ant-design/x*`). This is asserted with `jq` on the advisory URLs, because `npm audit` exits 1 whenever anything remains, and because a check on package names would let a new advisory on `mermaid` or `@ant-design/x`, which ship in the console bundle, pass unnoticed. The check returns `false` on `origin/dev` and `true` after §2:

   ```
   npm audit --json | jq -e '[.vulnerabilities[].via[] | objects | .url] | unique == ["https://github.com/advisories/GHSA-238p-pmpm-9mq7","https://github.com/advisories/GHSA-vfj7-8cjw-p6xm"]'
   ```
4. `npm ls vite` in each frontend shows a single deduplicated `vite` at the target version.

## 5. Files and their tests

| Implementation file | Test that covers it | Cases |
|---|---|---|
| `console/package.json`, `console/package-lock.json` | `npx tsc -b`, `npx vitest run`, `npx eslint .`, `npm run build` (the `console.yml` CI steps) | full unit suite green; production build (lingui compile → tsc → vite build) succeeds; no new type or lint error |
| `notification_center/package.json`, `notification_center/package-lock.json` | `npx tsc -b`, `npx vitest run`, `npm run lint`, `npm run build` (the `notification-center.yml` CI steps) | full suite and build green |
| `web_analytics_sdk/package.json`, `web_analytics_sdk/package-lock.json` | under Node 24 like CI: `npm run type-check`, `npx vitest run`, `npm run build` + `git diff --exit-code -- dist package.json`, `npx playwright test` (the `web-analytics-sdk.yml` CI steps) | full unit suite green; rebuilt `dist/` byte-identical to the committed one; e2e suite green |
| `CHANGELOG.md` | — | the existing, unreleased 42.0 dependency-update **Security** bullet gains one clause naming `vite` 7.3.7 and 8.3.3 for the development-server advisories, rather than a second bullet on the same subject |

No Go file changes; the Go suites are not affected.

## 6. Counter-review

Gemini (`gemini-4-argon`) reviewed this plan on 2026-10-21; the raw answer is kept with the project's review notes, outside this repository. Checked and taken in: the sixth SDK advisory (GHSA-vfpm-58rq-9qcg, confirmed in the `npm audit` output), the Node 22 `npm ci` in the SDK's Docker stage (verified to install the new lockfile), the `jq` assertion for the console audit, running the SDK checks under Node 24, and the `@tanstack/router-plugin` note. Its other points confirmed the plan.

Gemini also suggested `overrides` instead of a devDependency for the SDK, while accepting the devDependency as functional. The override was implemented first, and the code review of that commit showed the `EOVERRIDE` and silent-major problems described in §2, so the SDK went back to the devDependency.

## 7. Final step: run the test commands for the touched layers

From the repository root; the SDK under Node 24, as `web-analytics-sdk.yml` pins it for the byte-for-byte `dist/` check:

```
(cd console && npm ci && npx tsc -b && npx vitest run && npx eslint . && npm run build && \
  npm audit --json | jq -e '[.vulnerabilities[].via[] | objects | .url] | unique == ["https://github.com/advisories/GHSA-238p-pmpm-9mq7","https://github.com/advisories/GHSA-vfj7-8cjw-p6xm"]')
(cd notification_center && npm ci && npx tsc -b && npx vitest run && npm run lint && npm run build && npm audit)
(cd web_analytics_sdk && npm ci && npm run type-check && npx vitest run && npm run build && \
  git diff --exit-code -- dist package.json && npm run test:e2e:install && npx playwright test && npm audit)
```

`npm ci` downloads no browser, hence `npm run test:e2e:install` (CI has its own `npx playwright install --with-deps chromium` step). The e2e suite serves the committed bundle through `tsx` servers and never starts Vite: it is run because the SDK workflow runs it, not because it exercises this change.

Then the full CI on the PR (Go, console, notification center, SDK workflows) must be green on the last commit.
