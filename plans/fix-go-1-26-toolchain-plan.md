# Go 1.26 toolchain plan

**Status**: implemented, reviewed by Gemini (§7)
**Date**: 2026-10-22
**Decision taken** (Pierre, 2026-10-22): move from Go 1.25 to Go 1.26 in a PR of its own, after the vite advisories PR (#465).

## 1. Why

Go 1.25 left support when Go 1.27 shipped; its last release is 1.25.14. The Docker image builds the server with `golang:1.25-alpine`, which today resolves to 1.25.14, and CI uses `setup-go` `'1.25'`, the same.

`govulncheck` v1.9.0 on `origin/dev` (5a89efe, and again on 2369599 after the rebase, same result) with the go1.25.14 toolchain, i.e. the binary we ship:

- **Root module**: "Your code is affected by 13 vulnerabilities from the Go standard library", all fixed only in Go 1.26.9 or 1.26.10:
  GO-2026-6809 (crypto/x509, distrust compromised intermediates), GO-2026-6617 (HTTP/2 server crash, HPACK encoder race), GO-2026-6613 (HTTP/1 server desync after 2xx CONNECT), GO-2026-6612 (HTTP/2 double flow-control refund), GO-2026-6611 (HTTP/2 CPU on repeated window changes), GO-2026-6610 (HTTP/2 transport accepts malformed framing headers), GO-2026-6609 (unbounded Range header parsing), GO-2026-6608 (MIME header memory limit bypass, net/textproto and mime/multipart), GO-2026-6607 (crypto/tls malformed ECH references), GO-2026-6605 (HTTP/1 client desync after CONNECT rejection), GO-2026-6603 (HTTP/2 server memory exhaustion via Trailers), GO-2026-6600 and GO-2026-6599 (html/template context tracking).
  Five of them (6617, 6612, 6611, 6610, 6603) are also reported against `golang.org/x/net` v0.58.0, fixed in v0.60.0.
- Not called but required: `golang.org/x/crypto` v0.55.0, GO-2026-6355 and GO-2026-6354 (ssh channel DoS), fixed in v0.56.0; GO-2026-6604 (`os.Root` on Windows), fixed in 1.26.9.
- **telemetry module** (`telemetry/`, a separate Cloud Function): 10 of the same standard-library vulnerabilities, plus the same `x/net` and `x/crypto` ones (indirect).

None of this is in Dependabot: GitHub does not alert on the Go standard library, and its advisory database has no alert for these `x/net` / `x/crypto` versions yet.

**Go 1.26 is not optional for the module fixes**: `golang.org/x/net` v0.60.0 and `golang.org/x/crypto` v0.56.0 both declare `go 1.26.0`, so `go get` raises the `go` directive to 1.26 by itself.

## 2. Changes

### 2.1 Root module (`go.mod`, `go.sum`)

`GOTOOLCHAIN=go1.26.10 go get golang.org/x/net@v0.60.0 golang.org/x/crypto@v0.57.0 && go mod tidy`:

- `golang.org/x/net` v0.58.0 → **v0.60.0** (first release with the fixes).
- `golang.org/x/crypto` v0.55.0 → **v0.57.0**: v0.56.0 is the first fixed release, but x/net v0.60.0 requires v0.57.0, so MVS takes it anyway.
- Pulled by MVS, nothing else: `golang.org/x/sync` v0.22.0 → v0.23.0, `golang.org/x/sys` v0.47.0 → v0.48.0, `golang.org/x/text` v0.41.0 → v0.42.0.
- `go` directive **1.25.4 → 1.26.10**, and **`godebug urlstrictcolons=0`** to keep Go 1.25's URL host parsing (§3). `go get` writes 1.26.0; 1.26.10 is the first release with all thirteen standard-library fixes. The official `golang` images set `GOTOOLCHAIN=local`, so with 1.26.10 in `go.mod` a Docker build on a stale cached 1.26 image fails ("go.mod requires go >= 1.26.10") instead of shipping an unpatched binary. A developer on an older 1.26 gets 1.26.10 downloaded automatically (`GOTOOLCHAIN=auto`, the default). The repository already pins a patch level today (`go 1.25.4`).

What the module bumps change, read from their git logs between the two tags:

- `x/net` v0.58.0..v0.60.0, read from the module diff: `http2` (the security fixes, IDNA lookup profile, deprecation notices), `internal/httpcommon` (stricter `:authority`/`Host` checks for the x/net HTTP/2 server), `trace` and `html/escape.go` (`min`/`max` refactors), plus `dns/dnsmessage`, `icmp`, `quic` and `webdav`, and `http3` removed. Linked into `cmd/api`: `html` and `idna`, imported directly, with no behaviour change; `http2`, `internal/httpcommon` and `trace` through gRPC and the Google API clients, as clients only, since the API server uses `net/http`'s own HTTP/2 and nothing imports `x/net/http2` directly.
- `x/crypto` v0.55.0..v0.57.0: `ssh` fixes and `acme/jws`. Notifuse imports only `x/crypto/bcrypt`; it and the other x/crypto packages linked into `cmd/api` (`blowfish`, `chacha20`, `chacha20poly1305`, `cryptobyte`, `hkdf`, `internal/poly1305`) did not change.
- `x/sys` v0.47.0..v0.48.0: new CPU feature fields for ppc64, riscv64 and sparc64, and new Linux ioctl, type and constant tables. `cpu` and `unix` are linked in; nothing changes on linux/amd64 or arm64 beyond added definitions.
- `x/text` v0.41.0..v0.42.0: `unicode/norm` and `internal/export/idna` bug fixes, `encoding/japanese`, and `unicode/bidi` (`log.Panic` replaced by `panic`). Not imported directly; `unicode/norm` and `unicode/bidi` are linked in through `idna` and `precis`.
- `x/sync` v0.22.0..v0.23.0: `semaphore` now panics on a negative capacity. The only `semaphore.NewWeighted` call (`internal/service/broadcast/message_sender.go:190`) clamps its capacity to at least 1.

### 2.2 telemetry module (`telemetry/go.mod`, `telemetry/go.sum`, deploy files)

- Same `go get` (all indirect there): x/net v0.60.0, x/crypto v0.57.0, x/sync v0.23.0, x/sys v0.48.0, x/text v0.42.0. `go` directive 1.25.0 → **1.26.0**, as `go get` writes it, and **no `toolchain` line**.
  The function is built by Cloud Functions on its `go126` runtime, which supplies the toolchain and its patch updates. Whether that build could honour a `go 1.26.10` or `toolchain go1.26.10` line depends on its `GOTOOLCHAIN` and `GOPROXY`, which this repository does not control: GoogleCloudPlatform/buildpacks sets `GOPROXY=off` in its Go workspace layer (`pkg/golang/golang.go`); the source suggests that layer belongs to the app build (`google.go.gomod`) and not to the function sequence (`google.go.runtime`, `google.go.functions-framework`, `google.go.build`), in which case a `toolchain` line could be downloaded, but the code reviews read this differently and the platform may add settings of its own. Without a test deploy, the safe choice is a `go.mod` the runtime's Go always accepts, so the patch level is checked after the deploy instead (below). Making the build enforce it (`toolchain go1.26.10`, or `--set-build-env-vars=GOOGLE_GO_VERSION=1.26.10` in `deploy.sh`) is left for a change that can be tried with a real deploy. An earlier version of this plan added the `toolchain` line for local scans (Gemini's suggestion, §7); it was removed after the third code review. Local scans of `telemetry/` set `GOTOOLCHAIN=go1.26.10` instead (§8); CI runs its tests on the 1.26.10 that `setup-go` installs for the root module.
- `telemetry/deploy.sh`: `--runtime=go125` → `--runtime=go126`. Google deprecated `go125` on 2026-10-01; `go126` is supported until Feb/March 2027.
- `telemetry/function.yaml` (`runtime: go124`, already stale) and the manual command in `telemetry/README.md` (`--runtime=go121`) → `go126`, so no file still points at a runtime that cannot build a `go 1.26` module. The README's other Go versions follow ("Go 1.21 or later for local development", "Runtime: Go 1.21" → 1.26), and its manual command's `--trigger=http`, which `gcloud` does not accept, becomes `--trigger-http` as in `deploy.sh`, and the command gains the `--set-env-vars=GCP_PROJECT=...` that `deploy.sh` passes (gen2 functions do not set it, and without it `getLogger` fails and every payload is answered 500); the README's "set automatically" note on `GCP_PROJECT` is corrected. The command is being edited anyway and should run as written.
- Deploying the function is a manual step for Pierre after the merge. Nothing in the repository pins the patch level the `go126` runtime builds with, so the deploy is checked by hand, and `telemetry/README.md` says how: the Cloud Build log names the Go version, which must be 1.26.10 or later (1.26.9 still has GO-2026-6809, 1.26.8 and earlier all ten); if it is older, deploy again once the runtime has moved.

### 2.3 Build and CI

- `Dockerfile`: `FROM golang:1.25-alpine AS backend-builder` → `golang:1.26-alpine` (resolves to 1.26.10, published 2026-10-20).
- `tests/README.md`: the prerequisite "Go 1.21+" → "Go 1.26.10 or later". No other file in this repository, `docs` or `website` names a Go version.
- `.github/workflows/go.yml`: both `go-version: '1.25'` → `go-version-file: 'go.mod'`. With `'1.26'`, `setup-go` takes whatever 1.26.x the runner image has cached (images lag patch releases), and the `go` command then downloads go1.26.10 again on each step because of the `go` line. Reading `go.mod` installs exactly 1.26.10 and leaves one place that names the version. The unit job also runs `make test-telemetry`, whose `go 1.26.0` the installed 1.26.10 satisfies.

### 2.4 CHANGELOG

One **Security** bullet under 42.0, after the dependency-update bullet: the server is now built with Go 1.26.10, which fixes vulnerabilities in its HTTP/1 and HTTP/2 handling, MIME parsing, TLS, certificate verification and HTML templates; building from source now requires Go 1.26.10 or later, with `--pull` for a Docker build that stops on a stale cached `golang:1.26-alpine` (`GOTOOLCHAIN=local` refuses it, §2.1); nothing else, since URL parsing does not change.

## 3. Behaviour changes in Go 1.26, checked against the code

Settings whose default follows the `go` directive (the `Changed: 26` entries of `internal/godebugs/table.go` in go1.26.10), and what they do here:

- `urlstrictcolons=1`: `net/url.Parse` rejects a host with colons outside brackets. **Opted out**, with `godebug urlstrictcolons=0` in `go.mod`, because it breaks URLs that work today. Checked with probes on go1.25.14 and go1.26.10:
  - An IPv6 address written without brackets and followed by a port, `http://fd12:3456::10:9000/hook` or `https://2606:4700::6810:84e5:8443/feed`, parses on 1.25 as host `fd12:3456::10`, port `9000`, and `net/http` dials `net.JoinHostPort(host, port)` = `[fd12:3456::10]:9000` (`canonicalAddr`, `net/http/transport.go`), so it delivers. On 1.26 it fails to parse ("invalid port ... after host"). Webhook subscriptions and data feeds accept it at save time on 1.25 (the private-host check reads the same hostname the dial uses), and automation webhook nodes save any URL. After an upgrade without the opt-out, such a webhook subscription would fail at `http.NewRequestWithContext` in the delivery worker, a branch that drains each row; a data feed would fail every fetch; an automation node would fail each journey after its retries. The fix for the user is to add brackets, but nothing tells them before their events are lost.
  - What strict parsing would buy here is small: malformed hosts that never worked (`http://localhost:80:80/`, `http://::1/`) would be refused at save time instead of failing at dial time. It closes no hole: the private-host checks and the dial read the same `Hostname()`, so the lenient parse gives no parser differential to slip through (`localhost:80` is not refused by spelling, but it does not resolve either).
  - `go list -f '{{.DefaultGODEBUG}}' ./cmd/api` reports `urlstrictcolons=0`, and the `go.mod` line applies to tests too, so the cases in §5.2 fail if the line is removed. Go keeps a GODEBUG setting for at least two years; adopting strict parsing is a later, separate change that needs a changelog action (add brackets) and a decision on what the worker does with a saved URL that stops parsing.
- `cryptocustomrand=0`: crypto APIs ignore a caller-supplied random reader. Every call that takes one, in tests only (`ed25519.GenerateKey`, `x509.CreateCertificate`, `rsa.SignPKCS1v15` and others), passes `crypto/rand.Reader` or `nil`; nothing passes a seeded or deterministic reader, so nothing depended on it being honoured.
- `httpservecontentmaxranges`: `http.ServeContent` and `http.FileServer` ignore a `Range` header with more than 200 ranges and serve the whole file (part of the GO-2026-6609 fix). It reaches the console and notification center file servers (`internal/http/root_handler.go`); no browser sends that many ranges. (Built by 1.26.10 under a `go` directive below 1.26, the old value would allow a single range only; the directive here is 1.26.10.)
- `tlssecpmlkem`: two more hybrid post-quantum key exchanges (SecP256r1MLKEM768, SecP384r1MLKEM1024) are enabled for TLS clients and servers. Hybrid key exchange is not new here: Go 1.24 already enabled X25519MLKEM768 by default. A client still sends key shares for X25519MLKEM768 and X25519 only (`crypto/tls/handshake_client.go`: the first preference and its fallback); the two new groups only add two IDs to `supported_groups`, so the ClientHello an SMTP server or webhook receiver sees barely grows. Should an old server refuse it anyway, `GODEBUG=tlssecpmlkem=0` at run time restores the 1.25 list without a rebuild; nothing sets it here.

Other changes:

- `ServeMux` redirects become 307 instead of 301, both kinds (checked in `net/http/server.go`, `findHandler`, 1.25.14 against 1.26.10). The trailing-slash one only concerns `/t/` and `/r/` (`internal/http/email_handler.go`), the only subtree patterns, so only a bare `/t` or `/r`; no real link has that form. The clean-path one concerns any non-canonical path (`//api/...`, `/a/../b`), on every route: a 307 is not cached and keeps the method and body, so a `POST //api/contacts.upsert` that a 301 turned into a GET now reaches the endpoint, and a `https://host//r/...` link (written by pre-42.0 builds with a trailing-slash custom endpoint) still lands on `/r/...`. The 301s asserted in `root_handler_test.go` are written by the blog handler, not the mux.
- `httptest.Server.Client()` now routes `example.com` and its subdomains to the test server. Of the test files that start an `httptest` server and mention `example.com`, three call `srv.Client()`: `internal/service/inbound_reply_ses_test.go`, and, since #467, `internal/app/app_test.go` and `internal/http/inbound_webhook_event_handler_test.go`. All three send to `srv.URL` (127.0.0.1); their `example.com` strings are e-mail addresses in payloads. The SSRF, webhook, favicon and `safehttpclient` tests use their own clients, which the change does not touch. The unit and integration suites ran green on go1.26.10 before the rebase onto #467 and #468; the run that counts is the one in §8 on the final commit.
- `http.Client` scopes cookies to `Request.Host`; `ReverseProxy.Director` (not used), `image/jpeg` encoder (only the MIME type string is used), `io.ReadAll` capacity, and the new GC are not expected to matter; the suites cover them.

One `GODEBUG` opt-out, `urlstrictcolons=0`; the other defaults follow the `go` directive.

## 4. Out of scope

- `golang.org/x/crypto/openpgp` (GO-2026-5932) and `github.com/aws/aws-sdk-go` v1 S3 crypto client (GO-2022-0646, GO-2022-0635): no fixed version exists, and neither package is imported (`git grep` finds no `openpgp` or `s3crypto`). Moving off aws-sdk-go v1 (end of life) is a separate change.
- Go 1.27: 1.26 is supported until Go 1.28 ships and is enough for every fix above.
- Adopting Go 1.26's strict URL host parsing. It needs, first, the delivery worker to count a request that cannot be built against the subscription (today that branch drains the row without counting, which is harmless only while no saved URL can stop parsing), and a way to bring saved unbracketed IPv6 URLs to the bracketed form or to warn about them; then removing the `godebug` line, with a changelog action.

## 5. Tests first

The change is a toolchain and dependency upgrade with no feature behind it, so the check written first is the scanner, then the existing suites run on the new toolchain:

1. **Red, scanner** (recorded): `GOTOOLCHAIN=go1.25.14 govulncheck ./...` on `origin/dev` (5a89efe, then 2369599) exits 3 with the 13 findings above in the root module and 10 in `telemetry/`.
2. **Red, URL parsing kept**: new cases, run on go1.26.10 with the bumped `go.mod` before the `godebug` line is added, fail there and pass once it is:
   - `TestValidateURL` (`internal/service/webhook_subscription_service_test.go`): `http://fd12:3456::10:9000/hook` is accepted with `allowPrivateHosts` true, and refused with it false by the private-address check, whose message names the host it read: `(fd12:3456::10)`. That pins the host/port split, since the whole authority is a valid private IPv6 address too.
   - `TestValidateFeedURL_ValidPublicIPs` (`internal/domain/data_feed_url_validation_test.go`): `https://2606:4700::6810:84e5:8443/feed` is accepted; `TestValidateFeedURL_PrivateIPs`: `https://fd12:3456::10:9000/feed` is refused as a private address (added after Gemini's second review).
   (A first version of this plan adopted strict parsing and added the opposite cases, plus a delivery-worker change for saved URLs that stop parsing; the second code review found the IPv6 case above, and both were dropped in favour of the opt-out.)
3. Apply §2.
4. **Green**: those cases pass with the `godebug` line, and `govulncheck ./...` exits 0 in both modules: "No vulnerabilities found" for called code. `govulncheck -show verbose` may still list the two §4 modules with no fix.
5. Build and vet under go1.26.10: `go build ./...`, `go vet ./...` and `go vet -tags integration,licdev ./...` clean.
6. If a suite in §6 fails because of a Go 1.26 behaviour change, that change gets a focused test in the failing package first, then the fix.

## 6. Files and their tests

| Implementation file | Test that covers it | Cases |
|---|---|---|
| `go.mod` (`godebug urlstrictcolons=0`) | `TestValidateURL`, `TestValidateFeedURL_ValidPublicIPs`, `TestValidateFeedURL_PrivateIPs` | unbracketed IPv6 with a port still parses, and is still checked as an address |
| `go.mod`, `go.sum` | `go test -race $(go list ./... \| grep -v /tests/integration)` (the CI unit job), `make test-licence-dev`, `INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout=45m ./tests/integration/...` against Postgres 17 and Mailpit (the CI integration job), `govulncheck ./...` | every unit and integration test green on go1.26.10; govulncheck exits 0 |
| `telemetry/go.mod`, `telemetry/go.sum` | `make test-telemetry`, `cd telemetry && GOTOOLCHAIN=go1.26.10 go vet ./... && GOTOOLCHAIN=go1.26.10 govulncheck ./...` | green; exits 0 |
| `telemetry/deploy.sh`, `telemetry/function.yaml`, `telemetry/README.md` | — (deployment configuration, deployed by hand) | runtime reads `go126` everywhere; README command uses `--trigger-http` and sets `GCP_PROJECT`; README says to check the build's Go version |
| `Dockerfile` | `docker build .` | image builds; the backend stage runs go1.26.10 under `GOTOOLCHAIN=local` |
| `tests/README.md` | — | prerequisite reads 1.26.10 |
| `.github/workflows/go.yml` | the Go workflow on the PR | both jobs log "go1.26.10" from `go-version-file` and pass |
| `CHANGELOG.md` | — | one Security bullet under 42.0 |

## 7. Gemini review of this plan (2026-10-22)

Gemini (`gemini-4-argon`) reviewed this plan on 2026-10-22; the raw answer is kept with the project's review notes, outside this repository. Each point checked against the code:

- **Vulnerability analysis, dependency bumps, CHANGELOG wording**: OK, nothing to change.
- **CI should read the version from `go.mod`** (PROBLEM): accepted, §2.3. The failure it describes is a slow path rather than a red build (the `go` command downloads 1.26.10 itself), but `go-version-file` removes the duplicate version and the extra download.
- **`go get github.com/DATA-DOG/go-sqlmock` in the CI install step**: not taken. It predates this change, the module is already required (v1.5.2), and nothing about it depends on the Go version.
- **telemetry `toolchain go1.26.10`** (SUGGESTION): accepted at first, then removed (§2.2): whether the Cloud Functions build can download the toolchain it names could not be settled without a test deploy, and if it cannot, the line fails the deploy.
- **README `--trigger=http`**: accepted, §2.2, because the same command is edited.
- **`urlstrictcolons` in link rewriting** (PROBLEM): Gemini's own reading concluded it was harmless. The code reviews that followed found two things it missed: saved webhook subscriptions that stop parsing were drained without retiring the subscription, and an unbracketed IPv6 address with a port, which works on 1.25, stops parsing. The second decided it: §3 now opts out, and this revision was sent back to Gemini.
- **`httptest` `example.com` routing** (PROBLEM): accepted as a documentation gap. Checked, §3: the tests that use `srv.Client()`, three since #467, send to `srv.URL`.
- **Targeted tests** (SUGGESTION): accepted, §5.2; with the opt-out they pin that URL parsing does not change.

The revised plan (opt-out) went back to Gemini the same day. It agreed with the opt-out, the `go.mod` mechanism (a `//go:debug` line would miss test binaries, a Dockerfile `GODEBUG` would miss CI and source builds) and the absence of a parser differential, and suggested three things: a private unbracketed IPv6 case for feeds (taken, §5.2), a note on `tlssecpmlkem` for old SMTP servers (taken in §3, corrected: the key shares sent do not change, only two group IDs are added), and a follow-up for strict parsing (§4).
- **`govulncheck` in CI**: not in this PR. A scheduled scan is worth having, but a gate on every push would turn a newly published advisory into a red build on unrelated work; it is its own decision.

## 8. Final step: run the test commands for the touched layers

From the repository root, with Postgres 17 on port 5433 and Mailpit on 1025/8025 (`tests/compose.test.yaml`):

```
go build ./... && go vet ./... && go vet -tags integration,licdev ./...
govulncheck ./... && (cd telemetry && GOTOOLCHAIN=go1.26.10 go vet ./... && GOTOOLCHAIN=go1.26.10 govulncheck ./...)
go test -race $(go list ./... | grep -v '/tests/integration')
make test-licence-dev
make test-telemetry
INTEGRATION_TESTS=true go test -race -tags integration,licdev -timeout=45m ./tests/integration/...
docker build -t notifuse:go126 .
```

Then the full CI on the PR must be green on the last commit.
