#!/bin/sh
set -eu

MEDIA_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
CATALOG="$MEDIA_DIR/MEDIA_CATALOG"
MEDIA_SUMS="$MEDIA_DIR/MEDIA_SHA256SUMS"
MEDIA_REVISION="$MEDIA_DIR/RELEASE_REVISION"

fail() {
  echo "StageCore offline media error: $*" >&2
  exit 1
}

usage() {
  cat >&2 <<'EOF'
usage: ./stagecore-offline <verify|info|install|update> [options]
  verify   verify the complete offline media without changing the host
  info     verify media and print revision/platform bundle information
  install  install/reinstall the matching local bundle through F-005
  update   update from the matching local bundle through F-010 rollback policy
EOF
}

catalog_value() {
  key=$1
  awk -F= -v wanted="$key" '$1 == wanted { sub(/^[^=]*=/, ""); print; found=1; exit } END { if (!found) exit 1 }' "$CATALOG"
}

host_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf '%s\n' amd64 ;;
    aarch64|arm64) printf '%s\n' arm64 ;;
    *) fail "unsupported Linux architecture: $(uname -m)" ;;
  esac
}

validate_relative_bundle_path() {
  arch=$1
  relative=$2
  expected="bundles/stagecore-linux-$arch"
  [ "$relative" = "$expected" ] || fail "catalog bundle path for linux/$arch must be $expected"
  case "$relative" in
    /*|*..*) fail "unsafe bundle path in catalog: $relative" ;;
  esac
}

validate_checksum_manifest() {
  manifest=$1
  label=$2
  awk '
    NF != 2 { exit 1 }
    length($1) != 64 || $1 !~ /^[0-9A-Fa-f]+$/ { exit 1 }
    $2 ~ /^\// { exit 1 }
    $2 ~ /(^|\/)\.\.(\/|$)/ { exit 1 }
    $2 ~ /\\/ { exit 1 }
    END { if (NR == 0) exit 1 }
  ' "$manifest" || fail "unsafe or malformed checksum manifest: $label"
}

verify_media() {
  [ "$(uname -s)" = "Linux" ] || fail "offline installation currently supports Linux only"
  command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required to verify offline media"
  command -v awk >/dev/null 2>&1 || fail "awk is required to read offline media metadata"
  command -v find >/dev/null 2>&1 || fail "find is required to verify offline media"
  command -v grep >/dev/null 2>&1 || fail "grep is required to verify offline media"

  [ -f "$CATALOG" ] && [ ! -L "$CATALOG" ] || fail "MEDIA_CATALOG is missing or symlinked"
  [ -f "$MEDIA_SUMS" ] && [ ! -L "$MEDIA_SUMS" ] || fail "MEDIA_SHA256SUMS is missing or symlinked"
  [ -f "$MEDIA_REVISION" ] && [ ! -L "$MEDIA_REVISION" ] || fail "RELEASE_REVISION is missing or symlinked"

  format=$(catalog_value format) || fail "catalog format is missing"
  [ "$format" = "stagecore-offline-media-v1" ] || fail "unsupported offline media format: $format"
  revision=$(catalog_value revision) || fail "catalog revision is missing"
  [ -n "$revision" ] || fail "catalog revision is empty"
  actual_revision=$(cat "$MEDIA_REVISION")
  [ "$actual_revision" = "$revision" ] || fail "media revision does not match catalog revision"

  [ -d "$MEDIA_DIR/bundles" ] && [ ! -L "$MEDIA_DIR/bundles" ] || fail "bundles directory is missing or symlinked"
  if find "$MEDIA_DIR/bundles" -type l -print -quit 2>/dev/null | grep -q .; then
    fail "symlinks are not permitted inside offline release bundles"
  fi

  validate_checksum_manifest "$MEDIA_SUMS" MEDIA_SHA256SUMS

  for arch in amd64 arm64; do
    relative=$(catalog_value "bundle.linux.$arch") || fail "catalog entry bundle.linux.$arch is missing"
    validate_relative_bundle_path "$arch" "$relative"
    bundle="$MEDIA_DIR/$relative"
    [ -d "$bundle" ] && [ ! -L "$bundle" ] || fail "bundle directory is missing or symlinked: $relative"
    [ -f "$bundle/RELEASE_REVISION" ] && [ ! -L "$bundle/RELEASE_REVISION" ] || fail "bundle revision is missing or symlinked: $relative"
    [ -f "$bundle/SHA256SUMS" ] && [ ! -L "$bundle/SHA256SUMS" ] || fail "bundle checksum manifest is missing or symlinked: $relative"
    bundle_revision=$(cat "$bundle/RELEASE_REVISION")
    [ "$bundle_revision" = "$revision" ] || fail "bundle revision mismatch: $relative"
    validate_checksum_manifest "$bundle/SHA256SUMS" "$relative/SHA256SUMS"
    (
      cd "$bundle"
      sha256sum -c SHA256SUMS
    ) || fail "bundle checksum verification failed: $relative"
  done

  (
    cd "$MEDIA_DIR"
    sha256sum -c MEDIA_SHA256SUMS
  ) || fail "offline media checksum verification failed"
}

require_deployment_prerequisites() {
  command -v bwrap >/dev/null 2>&1 || fail "Bubblewrap (bwrap) is required before install/update so F-015 extensions can run inside the supported sandbox; install the distribution-provided bubblewrap package using an approved offline OS administration path, then retry"
}

selected_bundle() {
  arch=$(host_arch)
  relative=$(catalog_value "bundle.linux.$arch") || fail "catalog does not contain a Linux/$arch bundle"
  validate_relative_bundle_path "$arch" "$relative"
  printf '%s\n' "$MEDIA_DIR/$relative"
}

run_update() {
  bundle=$1
  shift
  setup="$bundle/stagecore-setup"
  [ -x "$setup" ] && [ ! -L "$setup" ] || fail "matching bundle stagecore-setup is missing, not executable, or symlinked"
  if [ "$(id -u)" -ne 0 ]; then
    command -v sudo >/dev/null 2>&1 || fail "update requires root privileges and sudo is unavailable"
    exec sudo "$setup" update --bundle "$bundle" "$@"
  fi
  exec "$setup" update --bundle "$bundle" "$@"
}

command=${1:-}
[ -n "$command" ] || { usage; exit 2; }
shift

case "$command" in
  verify)
    [ "$#" -eq 0 ] || fail "verify does not accept additional arguments"
    verify_media
    echo "StageCore offline media verification: PASS"
    ;;
  info)
    [ "$#" -eq 0 ] || fail "info does not accept additional arguments"
    verify_media
    echo "Format: $(catalog_value format)"
    echo "Revision: $(catalog_value revision)"
    echo "Linux amd64: $(catalog_value bundle.linux.amd64)"
    echo "Linux arm64: $(catalog_value bundle.linux.arm64)"
    ;;
  install)
    verify_media
    require_deployment_prerequisites
    bundle=$(selected_bundle)
    installer="$bundle/install.sh"
    [ -x "$installer" ] && [ ! -L "$installer" ] || fail "matching bundle install.sh is missing, not executable, or symlinked"
    exec "$installer" "$@"
    ;;
  update)
    verify_media
    require_deployment_prerequisites
    bundle=$(selected_bundle)
    run_update "$bundle" "$@"
    ;;
  help|-h|--help)
    usage
    ;;
  *)
    usage
    fail "unknown command: $command"
    ;;
esac
