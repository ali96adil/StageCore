#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
expected="0123456789abcdef0123456789abcdef01234567"
other="ffffffffffffffffffffffffffffffffffffffff"
cat >"$tmp/match" <<EOF
/opt/stagecore/bin/stagecore-hub: go1.26
  build vcs.revision=$expected
  build vcs.modified=false
EOF
python3 "$ROOT/tools/qualification/assert-installed-revision.py" --input "$tmp/match" --expected "$expected" >/dev/null
sed "s/$expected/$other/" "$tmp/match" >"$tmp/old"
for file in old missing dirty; do
  case "$file" in
    missing) printf 'build go=go1.26\n' >"$tmp/$file" ;;
    dirty) sed 's/vcs.modified=false/vcs.modified=true/' "$tmp/match" >"$tmp/$file" ;;
  esac
  set +e
  python3 "$ROOT/tools/qualification/assert-installed-revision.py" --input "$tmp/$file" --expected "$expected" >"$tmp/$file.log"
  result=$?
  set -e
  [[ "$result" -eq 3 ]]
  grep -q BLOCKED "$tmp/$file.log"
done
echo "installed candidate revision self-test PASS"
