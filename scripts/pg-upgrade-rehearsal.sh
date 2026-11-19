#!/usr/bin/env bash
#
# Rehearses what `git pull && docker compose up -d` does to an installation made with
# the PostgreSQL 17 compose file: the automatic copy, the upgrade to 18, the data
# afterwards, the documented backup and the documented rollback. Everything runs in a throwaway Compose project
# that is deleted at the end; your own stacks are not touched.
#
# Usage, from the repository root:
#   docker build -t notifuse/notifuse:rehearsal .
#   scripts/pg-upgrade-rehearsal.sh
#
# Variables:
#   IMAGE          Notifuse image used before and after (default notifuse/notifuse:rehearsal),
#                  so that only PostgreSQL changes between the two.
#   OLD_REF        git ref of the PostgreSQL 17 compose file (default v43.0).
#   DB_USER        PostgreSQL role of the installation (default postgres).
#   KILL_OLD=1     SIGKILL the running 17 stack before the upgrade (unclean shutdown).
#   SMALL_BACKUP=1 give the copy a 1 MB volume: the upgrade must refuse, and succeed
#                  once the volume is replaced.
#   INTERRUPT=1    make the first upgrade fail half-way (pgautoupgrade pointed at a
#                  database that does not exist): the next `up` must start again
#                  from the copy.
#   INTERRUPT_RESTORE=1  with INTERRUPT=1, also leave the putting back of the copy stopped
#                  half-way (the data volume emptied, lock included, and only PG_VERSION
#                  copied back): the next `up` must put the copy back again, never take the
#                  partial data for the 17 data and delete the copy.
#   KEEP=1         keep the project at the end, to look around.
#   NOTIFUSE_LICENSE_KEY  a licence the image accepts: the demo fixtures use template
#                  translations. An image built with -tags licdev accepts the dev
#                  licence of tests/testutil (DevLicenseKey).
#   HTTP_PORT, POSTGRES_HOST_PORT  host ports of the throwaway project (18081, 15433).

set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
IMAGE=${IMAGE:-notifuse/notifuse:rehearsal}
OLD_REF=${OLD_REF:-v43.0}
DB_USER=${DB_USER:-postgres}
HTTP_PORT=${HTTP_PORT:-18081}
POSTGRES_HOST_PORT=${POSTGRES_HOST_PORT:-15433}
project="pgrehearsal$(date +%s)"
work=$(mktemp -d)
root_email="root@rehearsal.example.com"
secret_key=$(openssl rand -base64 32)

dc() { docker compose --project-directory "$work" -p "$project" "$@"; }
psql_at() { dc exec -T postgres psql -U "$DB_USER" -d "${2:-postgres}" -Atc "$1"; }
step() { printf '\n=== %s\n' "$*"; }
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

cleanup() {
  if [ "${KEEP:-}" = 1 ]; then
    echo "Kept: project $project in $work"
    return
  fi
  dc down -v --remove-orphans >/dev/null 2>&1 || true
  docker rm -f "${project}-restore" >/dev/null 2>&1 || true
  docker volume rm "${project}_postgres-upgrade-backup" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

# The image tag also names NOTIFUSE_VERSION in the new file.
[ "${INTERRUPT_RESTORE:-}" != 1 ] || [ "${INTERRUPT:-}" = 1 ] || fail "INTERRUPT_RESTORE=1 needs INTERRUPT=1"

image_repo=${IMAGE%:*}
image_tag=${IMAGE##*:}
[ "$image_repo" = notifuse/notifuse ] || fail "IMAGE must be a notifuse/notifuse:<tag> image (got $IMAGE)"
docker image inspect "$IMAGE" >/dev/null || fail "build $IMAGE first: docker build -t $IMAGE ."

cat > "$work/.env" <<EOF
SECRET_KEY=$secret_key
DB_USER=$DB_USER
DB_PASSWORD=rehearsal
NOTIFUSE_LICENSE_KEY=${NOTIFUSE_LICENSE_KEY:-}
EOF

step "PostgreSQL 17 installation from the $OLD_REF compose file"
# The old file builds from source: point it at the image, and at free host ports.
git -C "$repo" show "$OLD_REF:compose.yaml" |
  python3 -c '
import sys
s = sys.stdin.read()
build = "    build:\n      context: .\n      dockerfile: Dockerfile\n"
assert build in s, "no build block in the old compose file"
s = s.replace(build, "    image: " + sys.argv[1] + "\n", 1)
# The old file passes only the variables it names: add the licence.
i = s.rindex("\n", 0, s.index("SECRET_KEY=")) + 1
s = s[:i] + "      - NOTIFUSE_LICENSE_KEY=${NOTIFUSE_LICENSE_KEY:-}\n" + s[i:]
s = s.replace("\x278081:8080\x27", "\x27" + sys.argv[2] + ":8080\x27").replace("\x275433:5432\x27", "\x27" + sys.argv[3] + ":5432\x27")
sys.stdout.write(s)
' "$IMAGE" "$HTTP_PORT" "$POSTGRES_HOST_PORT" > "$work/compose.yaml"
grep -q 'postgres:17-alpine' "$work/compose.yaml" || fail "$OLD_REF compose file is not on postgres:17-alpine"
cp "$work/compose.yaml" "$work/compose.old.yaml"
# No --wait: the 17 file starts the api before PostgreSQL is ready, and the api
# restarts until it is (the race compose.yaml now waits out).
dc up -d

api="http://localhost:$HTTP_PORT"
until curl -fsS "$api/healthz" >/dev/null 2>&1; do sleep 2; done

step "Setup and demo fixtures, through the API"
curl -fsS -X POST "$api/api/setup.initialize" -H 'Content-Type: application/json' -d "{
  \"root_email\": \"$root_email\", \"api_endpoint\": \"$api\",
  \"smtp_host\": \"localhost\", \"smtp_port\": 1025, \"smtp_from_email\": \"noreply@rehearsal.example.com\",
  \"smtp_from_name\": \"Rehearsal\", \"smtp_use_tls\": false, \"check_for_updates\": false
}" >/dev/null
# setup.initialize restarts the server to load its configuration.
sleep 3
until curl -fsS "$api/healthz" >/dev/null 2>&1; do sleep 2; done
hmac=$(cd "$repo" && go run ./cmd/hmac "$root_email" "$secret_key" | sed -n 's/^HMAC: //p')
[ -n "$hmac" ] || fail "no HMAC from cmd/hmac"
code=$(curl -sS -o "$work/seed.json" -w '%{http_code}' --max-time 600 "$api/api/demo.seed?hmac=$hmac")
[ "$code" = 200 ] || fail "demo.seed answered $code: $(cat "$work/seed.json")"

snapshot() {
  {
    psql_at "SELECT datname FROM pg_database WHERE NOT datistemplate ORDER BY 1"
    psql_at "SELECT value FROM settings WHERE key = 'db_version'" notifuse_system
    psql_at "SELECT count(*) FROM workspaces" notifuse_system
    for db in $(psql_at "SELECT datname FROM pg_database WHERE datname LIKE 'notifuse\_ws\_%' ORDER BY 1"); do
      for table in contacts contact_lists message_history contact_timeline web_sessions web_pages; do
        echo "$db.$table $(psql_at "SELECT count(*) FROM $table" "$db")"
      done
      # Collation canary: the order of the primary key of the largest table.
      echo "$db.order $(psql_at "SELECT md5(string_agg(email, ',' ORDER BY email)) FROM contacts" "$db")"
    done
  }
}
snapshot > "$work/before.txt"
grep -q 'contacts [1-9]' "$work/before.txt" || fail "the seed left no contacts: $(cat "$work/before.txt")"
echo "Snapshot: $(grep -c . "$work/before.txt") lines, $(grep -E '\.contacts ' "$work/before.txt" | tr '\n' ' ')"
[ "$(psql_at 'SHOW server_version_num')" -lt 180000 ] || fail "the old installation is not on 17"

if [ "${KILL_OLD:-}" = 1 ]; then
  step "SIGKILL of the running 17 stack (unclean shutdown)"
  dc kill -s KILL postgres api
fi

if [ "${SMALL_BACKUP:-}" = 1 ]; then
  docker volume create --driver local --opt type=tmpfs --opt device=tmpfs --opt o=size=1m \
    "${project}_postgres-upgrade-backup" >/dev/null
fi

step "git pull: the new compose.yaml, then docker compose up -d, nothing stopped first"
cp "$repo/compose.yaml" "$work/compose.yaml"
cat >> "$work/.env" <<EOF
NOTIFUSE_VERSION=$image_tag
HTTP_PORT=$HTTP_PORT
POSTGRES_HOST_PORT=$POSTGRES_HOST_PORT
SMTP_BRIDGE_HOST_PORT=$((HTTP_PORT + 1))
EOF

if [ "${SMALL_BACKUP:-}" = 1 ]; then
  if dc up -d --wait --wait-timeout 300 >"$work/up.log" 2>&1; then
    fail "the upgrade ran without room for the copy"
  fi
  # Through a file: under pipefail, grep -q closing the pipe early fails the pipeline.
  dc logs --no-log-prefix pg-upgrade > "$work/pg-upgrade.log"
  grep -q 'Not enough space to copy' "$work/pg-upgrade.log" || fail "no space message: $(tail -5 "$work/pg-upgrade.log")"
  version=$(docker run --rm -v "${project}_postgres-data:/v" alpine cat /v/PG_VERSION)
  [ "$version" = 17 ] || fail "the 17 data was changed after the refusal (PG_VERSION $version)"
  echo "Refused without room, 17 data untouched. Replacing the backup volume."
  dc down >/dev/null 2>&1
  docker volume rm "${project}_postgres-upgrade-backup" >/dev/null
fi

if [ "${INTERRUPT:-}" = 1 ]; then
  # Only pg-upgrade's POSTGRES_DB, the first one in the file.
  python3 -c '
import sys
s = open(sys.argv[1]).read()
line = "- POSTGRES_DB=${POSTGRES_DB:-postgres}"
assert s.index(line) < s.index("\n  postgres:\n")
sys.stdout.write(s.replace(line, "- POSTGRES_DB=no_such_database", 1))
' "$repo/compose.yaml" > "$work/compose.yaml"
  grep -q 'POSTGRES_DB=no_such_database' "$work/compose.yaml" || fail "could not plant the failure"
  if dc up -d --wait --wait-timeout 300 >"$work/up.log" 2>&1; then
    fail "the planted failure did not stop the upgrade"
  fi
  docker run --rm -v "${project}_postgres-data:/v" alpine test -e /v/18/docker/upgrade_in_progress.lock ||
    fail "the failed upgrade left no lock: $(tail -5 "$work/up.log")"
  echo "The first upgrade stopped half-way. Running it again with the real file."
  dc down >/dev/null 2>&1
  cp "$repo/compose.yaml" "$work/compose.yaml"
  if [ "${INTERRUPT_RESTORE:-}" = 1 ]; then
    # What a putting back stopped by a kill leaves: its mark, the data volume emptied
    # (the upgrade lock with it), and the first file of the copy back in place.
    docker run --rm -v "${project}_postgres-data:/v" -v "${project}_postgres-upgrade-backup:/b" alpine \
      sh -c 'touch /b/restore_in_progress && find /v -mindepth 1 -delete && cp -a /b/pg17/PG_VERSION /v/'
    echo "The putting back stopped half-way too: only PG_VERSION is back, the lock is gone."
  fi
fi

start=$(date +%s)
dc up -d --wait --wait-timeout 900
echo "Upgrade and start: $(( $(date +%s) - start )) s"

step "Checks on PostgreSQL 18"
dc logs --no-log-prefix pg-upgrade > "$work/pg-upgrade.log"
if [ "${INTERRUPT:-}" = 1 ]; then
  grep -q 'did not finish: putting back the data copied before it' "$work/pg-upgrade.log" || fail "no restart from the copy: $(head -20 "$work/pg-upgrade.log")"
  docker run --rm -v "${project}_postgres-upgrade-backup:/b" alpine sh -c 'test -s /b/pg17/PG_VERSION && test ! -e /b/restore_in_progress' ||
    fail "the copy is gone, or the putting back left its mark"
else
  grep -q 'Copying the PostgreSQL 17 data' "$work/pg-upgrade.log" || fail "no copy logged: $(head -20 "$work/pg-upgrade.log")"
fi
[ "$(psql_at 'SHOW server_version_num' | cut -c1-2)" = 18 ] || fail "not on 18: $(psql_at 'SELECT version()')"
[ "$(psql_at 'SHOW data_directory')" = /var/lib/postgresql/18/docker ] || fail "data_directory $(psql_at 'SHOW data_directory')"
snapshot > "$work/after.txt"
diff "$work/before.txt" "$work/after.txt" || fail "the data differs after the upgrade"
echo "Snapshot identical."
until curl -fsS "$api/healthz" >/dev/null 2>&1; do sleep 2; done
# A write into each workspace runs the contact triggers, whose bodies were compiled
# on 17 and have no dependency PostgreSQL could have checked.
for db in $(psql_at "SELECT datname FROM pg_database WHERE datname LIKE 'notifuse\_ws\_%' ORDER BY 1"); do
  n=$(psql_at "SELECT count(*) FROM contact_timeline" "$db")
  psql_at "UPDATE contacts SET first_name = coalesce(first_name, '') || ' rehearsal' WHERE email = (SELECT email FROM contacts ORDER BY email LIMIT 1)" "$db" >/dev/null
  [ "$(psql_at "SELECT count(*) FROM contact_timeline" "$db")" -gt "$n" ] || fail "no timeline entry after a contact update in $db"
done
echo "Contact triggers write the timeline in every workspace."

step "Second up: nothing to do"
dc up -d --wait --wait-timeout 180
dc logs --no-log-prefix pg-upgrade > "$work/pg-upgrade.log"
[ "$(grep -c 'Copying the PostgreSQL' "$work/pg-upgrade.log")" -le 1 ] || fail "a second copy was made"
grep -q 'did not finish' "$work/pg-upgrade.log" && [ "$(grep -c 'did not finish' "$work/pg-upgrade.log")" -gt 1 ] && fail "the data was put back twice"
[ "$(psql_at 'SHOW server_version_num' | cut -c1-2)" = 18 ] || fail "not on 18 after a restart"

first_ws=$(grep -m1 -E '^notifuse_ws_.*\.contacts ' "$work/before.txt")
ws_db=${first_ws%%.*}
ws_contacts=${first_ws##* }

step "Backup as documented: pg_dump of every database"
snapshot > "$work/backup.txt"
backup_dir="$work/backup"
mkdir "$backup_dir"
(
  cd "$work"
  for db in $(dc exec -T postgres psql -U "$DB_USER" -d postgres -Atc \
      "SELECT datname FROM pg_database WHERE datname = 'notifuse_system' OR datname LIKE 'notifuse\_ws\_%'"); do
    dc exec -T postgres pg_dump -U "$DB_USER" -Fc "$db" > "$backup_dir/$db.dump"
  done
)
[ -s "$backup_dir/notifuse_system.dump" ] && [ -s "$backup_dir/$ws_db.dump" ] || fail "the documented backup dumped nothing: $(ls "$backup_dir")"
echo "Dumped: $(ls "$backup_dir" | tr '\n' ' ')"

step "Rollback as documented: the copy back into the data volume, the 17 compose file"
dc down
cp "$work/compose.old.yaml" "$work/compose.yaml"
docker run --rm -v "${project}_postgres-upgrade-backup:/from" -v "${project}_postgres-data:/to" alpine \
  sh -c 'find /to -mindepth 1 -delete && cp -a /from/pg17/. /to/'
dc up -d
until psql_at 'SELECT 1' >/dev/null 2>&1; do sleep 2; done
[ "$(psql_at 'SHOW server_version_num' | cut -c1-2)" = 17 ] || fail "not back on 17"
snapshot > "$work/rollback.txt"
diff "$work/before.txt" "$work/rollback.txt" || fail "the rolled-back data differs from before the upgrade"
until curl -fsS "$api/healthz" >/dev/null 2>&1; do sleep 2; done
echo "Back on PostgreSQL 17 with the data as before the upgrade."

step "Upgrade again after the rollback: the copy is made again, with what 17 wrote since"
psql_at "UPDATE contacts SET last_name = 'after-rollback' WHERE email = (SELECT email FROM contacts ORDER BY email LIMIT 1)" "$ws_db" >/dev/null
cp "$repo/compose.yaml" "$work/compose.yaml"
dc up -d --wait --wait-timeout 900
[ "$(psql_at 'SHOW server_version_num' | cut -c1-2)" = 18 ] || fail "not on 18 after the second upgrade"
[ "$(psql_at "SELECT count(*) FROM contacts WHERE last_name = 'after-rollback'" "$ws_db")" = 1 ] || fail "the write made on 17 after the rollback is lost"
dc stop >/dev/null 2>&1
docker run -d --name "${project}-restore" -e POSTGRES_PASSWORD=x -e PGDATA=/backup/pg17 \
  -v "${project}_postgres-upgrade-backup:/backup" postgres:17-alpine >/dev/null
until docker exec "${project}-restore" psql -U "$DB_USER" -d postgres -Atc 'SELECT 1' >/dev/null 2>&1; do sleep 1; done
got=$(docker exec "${project}-restore" psql -U "$DB_USER" -d "$ws_db" -Atc "SELECT count(*) FROM contacts WHERE last_name = 'after-rollback'")
docker rm -f "${project}-restore" >/dev/null
[ "$got" = 1 ] || fail "the copy made before the second upgrade is the old one"
echo "The second upgrade copied the data as 17 left it."

step "Restore as documented, into an empty stack on the new compose.yaml"
dc down -v
cp "$repo/compose.yaml" "$work/compose.yaml"
dc up -d --wait --wait-timeout 300 postgres
(
  cd "$backup_dir"
  for f in notifuse_*.dump; do
    db=${f%.dump}
    dc exec -T postgres createdb -U "$DB_USER" "$db"
    dc exec -T postgres pg_restore -U "$DB_USER" -d "$db" --exit-on-error < "$f"
  done
)
dc up -d --wait --wait-timeout 300
snapshot > "$work/restored.txt"
diff "$work/backup.txt" "$work/restored.txt" || fail "the restored data differs from the backup"
until curl -fsS "$api/healthz" >/dev/null 2>&1; do sleep 2; done
echo "Restored on PostgreSQL $(psql_at 'SHOW server_version'), identical to the backup."

printf '\nPASS (DB_USER=%s%s%s%s%s)\n' "$DB_USER" "${KILL_OLD:+, KILL_OLD}" "${SMALL_BACKUP:+, SMALL_BACKUP}" "${INTERRUPT:+, INTERRUPT}" "${INTERRUPT_RESTORE:+, INTERRUPT_RESTORE}"
