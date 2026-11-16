#!/bin/sh
set -eu

TEMPER_DB="${TEMPER_DB:-/data/temper.db}"
BACKUP_DIR="${BACKUP_DIR:-/data/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"

fail() {
	printf 'backup: %s\n' "$1" >&2
	exit 1
}

command -v sqlite3 >/dev/null 2>&1 || fail "sqlite3 is required but was not found in PATH"
[ -f "$TEMPER_DB" ] || fail "database $TEMPER_DB does not exist"
mkdir -p "$BACKUP_DIR" || fail "cannot create backup directory $BACKUP_DIR"
case "$RETENTION_DAYS" in
	''|*[!0-9]*) fail "RETENTION_DAYS must be a positive integer, got $RETENTION_DAYS" ;;
esac
[ "$RETENTION_DAYS" -ge 1 ] || fail "RETENTION_DAYS must be a positive integer, got $RETENTION_DAYS"

today="$(date -u +%F)"
partial="$BACKUP_DIR/.temper-$today.db.partial"
target="$BACKUP_DIR/temper-$today.db"

discard() {
	rm -f "$partial"
	fail "$1"
}

if ! sqlite3 "$TEMPER_DB" ".backup '$partial'"; then
	discard "cannot back up $TEMPER_DB"
fi

if ! integrity="$(sqlite3 "$partial" 'PRAGMA integrity_check;')"; then
	discard "cannot read the partial backup"
fi
if [ "$integrity" != "ok" ]; then
	discard "integrity check failed: $integrity"
fi
if ! sqlite3 "$partial" 'SELECT count(*) FROM schema_migrations;' >/dev/null 2>&1; then
	discard "schema_migrations is missing from the partial backup"
fi

if ! mv "$partial" "$target"; then
	discard "cannot publish $target"
fi

size="$(wc -c < "$target")"
printf '%s %s bytes\n' "$target" "$size"

cutoff="$(sqlite3 :memory: "SELECT date('$today', '-$((RETENTION_DAYS - 1)) days');")" || fail "cannot compute the retention cutoff"
for file in "$BACKUP_DIR"/temper-*.db; do
	[ -e "$file" ] || continue
	name="${file##*/}"
	case "$name" in
		temper-[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9].db) ;;
		*) continue ;;
	esac
	file_date="${name#temper-}"
	file_date="${file_date%.db}"
	if [ "$file_date" \< "$cutoff" ]; then
		rm -f "$file" || fail "cannot remove expired backup $file"
	fi
done
