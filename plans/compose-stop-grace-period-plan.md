# Compose stop grace period plan

## Problem

On SIGTERM, `cmd/api/main.go` runs `App.Shutdown` inside a 70 s context (and sets the app's own
shutdown timeout to 65 s) so that work in flight can finish: the automation step under way (up to
20 s, `automationShutdownDrain`), the emails being sent (queue worker grace 10 s + 5 s), the web
analytics buffer flush, the HTTP requests in progress.

`compose.yaml` and `compose.alloydb.yaml` set no `stop_grace_period` on the `api` service, so
`docker compose stop`, `down`, `up` with a changed image, or a restart sends SIGTERM then SIGKILL
after Docker's default 10 s. Everything still running at 10 s is killed: an automation step whose
effect went out but whose journey write had not landed yet runs again after the restart (email sent
twice), and unflushed web analytics sessions are lost.

## Fix

- `compose.yaml` and `compose.alloydb.yaml`, `api` service: `stop_grace_period: 75s`, with a short
  comment saying why (70 s graceful shutdown in `cmd/api/main.go` plus a margin).
- `cmd/api/main.go`: extract the 70 s literal into `const gracefulShutdownTimeout = 70 * time.Second`
  (no behaviour change) so a test can tie the Compose value to it.
- The Dockerfile already runs the server in exec form (`CMD ["/app/server"]`), so the process gets
  SIGTERM directly: nothing to change there.
- No change to the `postgres` services: Compose stops `api` first (it depends on `postgres`).
- `go.mod`: `gopkg.in/yaml.v3` moves from indirect to direct (the test imports it; no go.sum change).
- `plans/postgresql-18-upgrade-plan.md`: strike its `compose.yaml` row, which planned this same change.

Known limits, not addressed:
- The value applies to containers created from the new file: a container created before keeps
  Docker's 10 s until it is recreated (`docker compose up -d` recreates it on a config change).
- PostgreSQL keeps Docker's default 10 s; a longer shutdown checkpoint means crash recovery at the
  next start (WAL-safe).
- Stopping the Docker daemon or rebooting the host stops all containers in parallel, so the
  database can go down while Notifuse drains.
- `docker run` (docs "Standalone Docker", `make docker-run`) and Kubernetes (30 s default) need
  their own setting (`--stop-timeout 75`, `terminationGracePeriodSeconds: 75`): mentioned in the
  changelog line, no docs change here.

## Tests (written first)

Order: extract the constant first (no behaviour change), then write the test (red on the missing
key), then add the Compose key.

`cmd/api/compose_test.go` (new, package `main`, no build tag: `main_test.go` is behind
`//go:build runserver` and is not used):

- `TestComposeFilesGiveTheGracefulShutdownTime`: for each of `../../compose.yaml` and
  `../../compose.alloydb.yaml`, parse the YAML (`gopkg.in/yaml.v3`, already in the module graph),
  read `services.api.stop_grace_period`, parse it with `time.ParseDuration`, and assert it is at
  least `gracefulShutdownTimeout + 5*time.Second` (`Shutdown` can run about 2 s past the 70 s
  context, plus the database close). Red today (the key is missing), green after.
- `TestGracefulShutdownTimeout`: `gracefulShutdownTimeout == 70*time.Second` (literal, pins it).

`docker compose -f compose.yaml config -q` and the same for `compose.alloydb.yaml` validate both
files (needs `SECRET_KEY` set in the environment for the `:?` check).

## Docs and changelog

- `CHANGELOG.md`, 44.0: one **Improvement** line: the Compose files give Notifuse 75 seconds to stop
  instead of Docker's 10, so work in flight (an automation step, emails being sent) finishes before
  the container exits, once the container is recreated (`docker compose up -d`). With your own
  Compose file, `docker run` or Kubernetes, set `stop_grace_period: 75s`, `--stop-timeout 75` or
  `terminationGracePeriodSeconds: 75`.
- `../docs`: no change in this PR (the standalone `docker run` and Kubernetes cases are listed as
  known, not addressed).

## Test commands

```
go test -race ./cmd/api -count=1   # make test-unit does not cover cmd/api
SECRET_KEY=x docker compose -f compose.yaml config -q
SECRET_KEY=x docker compose -f compose.alloydb.yaml config -q
make test-unit
```
