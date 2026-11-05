#!/usr/bin/env bash

# golang-migrate stores one integer version in schema_migrations and Up() only
# applies migrations numbered above it. A database that followed the LTS line to
# version V therefore never receives a migration numbered below V, so the feature
# line must satisfy two things for an LTS -> feature upgrade to be complete:
#
#   1. every migration on the LTS line also exists here, under the same number,
#      so the upgrade does not lose an LTS schema fix; and
#   2. every migration that exists only here is numbered above the band the LTS
#      line draws from, so none of them can be silently skipped by a database
#      that is already past that number.
#
# Hence the reserved bands: LTS fixes stay below FEATURE_BAND_START, feature work
# starts at it.
#
# The LTS band only matters while the two lines have diverged. As long as the
# LTS ref is an ancestor of HEAD (master fast-forwarded to dev, or master checked
# out itself), the two refs are one line and the feature-band migrations on it
# are simply the line's own, so the check that LTS migrations stay below
# FEATURE_BAND_START does not apply and is skipped. The other checks run in both
# cases: both dialects present, no LTS migration missing here, and a migration
# that exists only here numbered in the feature band (on one line too, a number
# below one already applied is skipped by every database past it).

set -euo pipefail

readonly LTS_REF="${LTS_REF:-origin/master}"
readonly FEATURE_BAND_START=3000
readonly MIGRATION_DIR=internal/app/migration/schema/database
readonly LEGACY_MIGRATION_DIR=initialize/migrate/database

extract_numbers() {
  sed -n 's#.*/\([0-9]\{5\}\)_.*\.up\.sql$#\1#p' | sort -u
}

ref_migration_numbers() {
  # A branch may still predate the package reorganization. Directory moves do
  # not create new migrations, so compare the same versions at either path.
  git ls-tree -r --name-only "$1" -- "$MIGRATION_DIR/$2" "$LEGACY_MIGRATION_DIR/$2" | extract_numbers
}

# The working tree rather than HEAD, so an uncommitted migration is caught too.
# In CI the two are identical.
local_migration_numbers() {
  find "$MIGRATION_DIR/$1" -name '*.up.sql' | extract_numbers
}

if ! git rev-parse --verify --quiet "$LTS_REF" >/dev/null; then
  echo "check-migration-lines: $LTS_REF is not available, skipping."
  echo "Fetch it first (git fetch origin master) to run this check."
  exit 0
fi

status=0

# One line or two. merge-base --is-ancestor also holds for equal refs; on a
# shallow clone it cannot see the ancestry and reports two lines, so CI checks
# out the full history.
if git merge-base --is-ancestor "$LTS_REF" HEAD; then
  one_line=1
  echo "check-migration-lines: $LTS_REF is an ancestor of HEAD (or HEAD itself), so the two refs are one"
  echo "migration line; skipping only the check that its migrations stay below the feature band."
else
  one_line=0
fi

# Every migration needs both dialects: one that ships for only one of them takes
# down every deployment on the other.
mysql_numbers="$(local_migration_numbers mysql)"
postgres_numbers="$(local_migration_numbers postgres)"
asymmetric="$(comm -3 <(echo "$mysql_numbers") <(echo "$postgres_numbers") | tr -d '\t')"
if [ -n "$asymmetric" ]; then
  echo "These migrations are missing one of the two dialects:"
  echo "$asymmetric" | sed 's/^/  /'
  status=1
fi

lts_numbers="$(ref_migration_numbers "$LTS_REF" mysql)"
head_numbers="$mysql_numbers"

missing="$(comm -23 <(echo "$lts_numbers") <(echo "$head_numbers"))"
if [ -n "$missing" ]; then
  echo "These migrations exist on $LTS_REF but not here:"
  echo "$missing" | sed 's/^/  /'
  echo "Forward-port them under their original numbers, or an LTS database that"
  echo "upgrades to this line loses the schema fix they carry."
  status=1
fi

if [ "$one_line" -eq 0 ]; then
  while read -r number; do
    [ -n "$number" ] || continue
    if [ "$((10#$number))" -ge "$FEATURE_BAND_START" ]; then
      echo "Migration $number is on $LTS_REF but sits in the feature band (>= $FEATURE_BAND_START)."
      echo "LTS migrations must stay below it so the feature line can always add"
      echo "migrations above every number an LTS database has already applied."
      status=1
    fi
  done <<<"$lts_numbers"
fi

feature_only="$(comm -13 <(echo "$lts_numbers") <(echo "$head_numbers"))"
while read -r number; do
  [ -n "$number" ] || continue
  if [ "$((10#$number))" -lt "$FEATURE_BAND_START" ]; then
    echo "Migration $number exists only on this line but is numbered below $FEATURE_BAND_START."
    echo "An LTS database that has already passed $number would skip it forever."
    echo "Renumber it into the feature band, or forward-port it to $LTS_REF."
    status=1
  fi
done <<<"$feature_only"

if [ "$status" -eq 0 ]; then
  if [ "$one_line" -eq 1 ]; then
    echo "Migration lines agree: $(echo "$lts_numbers" | grep -c .) on ${LTS_REF}, $(echo "$feature_only" | grep -c .) added since (one line)."
  else
    echo "Migration lines agree: $(echo "$lts_numbers" | grep -c .) on ${LTS_REF}, $(echo "$feature_only" | grep -c .) feature-only."
  fi
fi

exit "$status"
