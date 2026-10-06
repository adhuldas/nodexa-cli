#!/usr/bin/env bash
# scripts/release.sh - builds the nodex CLI for every platform and, with
# --publish, tags the commit and uploads it as a GitHub Release. The same thing
# .github/workflows/release.yml does, run on this machine, so a release
# doesn't need GitHub Actions minutes: assets go up through `gh`, which
# only needs release storage.
#
# The release version is the VERSION file at the repo root: change it, commit,
# push, run this with --publish, and that version is built, tagged (vX.Y.Z)
# and released. The binary reports it too (it is embedded; see version.go).
#
# Output in --out-dir (default dist/), named as install.sh / install.ps1 expect:
#   nodex_<os>_<arch>.tar.gz   (windows also .zip)   and   checksums.txt
#
# Usage:
#   scripts/release.sh [--version X.Y.Z] [--out-dir <path>]
#                      [--targets os/arch,os/arch,...]
#                      [--publish] [--draft] [--prerelease] [--replace]
#                      [--notes <file>] [--skip-tests] [--allow-dirty]
#
# Without --publish it only builds, so it is safe to run any time.
#
# Examples:
#   scripts/release.sh                              # build the VERSION file's version, no upload
#   scripts/release.sh --publish                    # build, tag it, upload
#   scripts/release.sh --version 0.2.0-rc.1 --draft --publish   # override VERSION for one run
#   scripts/release.sh --targets darwin/arm64       # one platform, for a quick check
#
# --publish needs the GitHub CLI (`gh auth login` once), a clean working tree
# (so VERSION is committed) and a HEAD that is already pushed. It creates an
# annotated tag vX.Y.Z on HEAD, pushes it, then creates the release for it.
# A version that already has a tag or release is refused: bump VERSION. To
# redo one anyway, --replace re-uploads the assets to the existing release.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

log()  { printf '\033[1;34m[nodex]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[nodex][warn]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[nodex][error]\033[0m %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed ($2)"; }

# Same six platforms as the release workflow.
ALL_TARGETS="darwin/amd64,darwin/arm64,linux/amd64,linux/arm64,windows/amd64,windows/arm64"

VERSION=""
OUT_DIR="${REPO_ROOT}/dist"
TARGETS="${ALL_TARGETS}"
PUBLISH=false
DRAFT=false
PRERELEASE=false
REPLACE=false
NOTES_FILE=""
RUN_TESTS=true
ALLOW_DIRTY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="${2:?--version needs a value}"; shift 2 ;;
    --version=*) VERSION="${1#*=}"; shift ;;
    --out-dir) OUT_DIR="${2:?--out-dir needs a value}"; shift 2 ;;
    --out-dir=*) OUT_DIR="${1#*=}"; shift ;;
    --targets) TARGETS="${2:?--targets needs a value}"; shift 2 ;;
    --targets=*) TARGETS="${1#*=}"; shift ;;
    --notes) NOTES_FILE="${2:?--notes needs a file}"; shift 2 ;;
    --notes=*) NOTES_FILE="${1#*=}"; shift ;;
    --publish) PUBLISH=true; shift ;;
    --draft) DRAFT=true; shift ;;
    --prerelease) PRERELEASE=true; shift ;;
    --replace) REPLACE=true; shift ;;
    --skip-tests) RUN_TESTS=false; shift ;;
    --allow-dirty) ALLOW_DIRTY=true; shift ;;
    -h|--help) sed -n '2,/^set -euo/p' "$0" | sed '$d' | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done

need go "https://go.dev/dl/"
need tar "needed to package the archives"
cd "${REPO_ROOT}"

# --- version ---------------------------------------------------------------
# The VERSION file is the source of truth; --version overrides it for one run.
if [[ -z "${VERSION}" ]]; then
  [[ -f "${REPO_ROOT}/VERSION" ]] || die "no VERSION file at ${REPO_ROOT}/VERSION (or pass --version)"
  VERSION="$(tr -d '[:space:]' < "${REPO_ROOT}/VERSION")"
  log "version ${VERSION} (from the VERSION file)"
fi
VERSION="${VERSION#v}"
[[ "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.]+)?$ ]] \
  || die "version '${VERSION}' must look like 1.2.3 (optionally 1.2.3-rc.1)"
TAG="v${VERSION}"

# --- checks before any work --------------------------------------------------
if [[ "${PUBLISH}" == true ]]; then
  need gh "https://cli.github.com/, then run: gh auth login"
  gh auth status >/dev/null 2>&1 || die "gh isn't logged in; run: gh auth login"
  if [[ "${ALLOW_DIRTY}" != true && -n "$(git status --porcelain)" ]]; then
    die "working tree has uncommitted changes; commit them first (or --allow-dirty)"
  fi
  # The tag points at this commit, so GitHub must have it.
  git fetch --quiet --tags origin 2>/dev/null || warn "could not fetch origin to check HEAD and tags"
  if ! git branch -r --contains HEAD 2>/dev/null | grep -q .; then
    die "HEAD isn't on any remote branch; push it first so the ${TAG} tag points at real code"
  fi
  # A released version is never silently reused: bump VERSION instead.
  if [[ "${REPLACE}" != true ]]; then
    if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null || gh release view "${TAG}" >/dev/null 2>&1; then
      die "${TAG} already exists. Bump the VERSION file (currently ${VERSION}), or pass --replace to re-upload its assets"
    fi
  fi
  # Going backwards is almost always a forgotten bump or a typo.
  newest="$(git tag --list 'v[0-9]*' --sort=-v:refname | head -n1)"
  if [[ -n "${newest}" && "${newest}" != "${TAG}" && "$(printf '%s\n%s\n' "${newest#v}" "${VERSION}" | sort -V | tail -n1)" != "${VERSION}" ]]; then
    warn "${TAG} is older than the newest tag ${newest}"
  fi
fi
if [[ -n "${NOTES_FILE}" && ! -f "${NOTES_FILE}" ]]; then
  die "notes file not found: ${NOTES_FILE}"
fi

# --- tests -----------------------------------------------------------------
if [[ "${RUN_TESTS}" == true ]]; then
  log "go vet + go test"
  go vet ./...
  go test ./...
fi

# --- build + package ---------------------------------------------------------
rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

IFS=',' read -r -a TARGET_LIST <<< "${TARGETS}"
for target in "${TARGET_LIST[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  case "${os}/${arch}" in
    darwin/amd64|darwin/arm64|linux/amd64|linux/arm64|windows/amd64|windows/arm64) ;;
    *) die "unsupported target '${target}' (expected one of: ${ALL_TARGETS})" ;;
  esac

  ext=""
  [[ "${os}" == "windows" ]] && ext=".exe"
  stage="${WORK}/${os}_${arch}"
  mkdir -p "${stage}"
  log "building ${os}/${arch} (${TAG})"
  GOOS="${os}" GOARCH="${arch}" CGO_ENABLED=0 \
    go build -ldflags "-s -w -X main.Version=${VERSION}" -o "${stage}/nodex${ext}" ./cmd/nodex

  base="nodex_${os}_${arch}"
  # COPYFILE_DISABLE: macOS tar would otherwise add ._ metadata files.
  COPYFILE_DISABLE=1 tar -czf "${OUT_DIR}/${base}.tar.gz" -C "${stage}" "nodex${ext}"
  if [[ "${os}" == "windows" ]]; then
    need zip "needed to package the Windows .zip"
    (cd "${stage}" && zip -q "${OUT_DIR}/${base}.zip" "nodex${ext}")
  fi
done

# Checksums, in the format sha256sum writes ("<hash>  <file>").
(
  cd "${OUT_DIR}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum nodex_* > checksums.txt
  else
    shasum -a 256 nodex_* > checksums.txt
  fi
)

# Run the build for this machine, so a binary that doesn't start is caught
# here and not by the first person to install it.
host_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in x86_64) host_arch=amd64 ;; aarch64|arm64) host_arch=arm64 ;; *) host_arch="" ;; esac
if [[ -f "${OUT_DIR}/nodex_${host_os}_${host_arch}.tar.gz" ]]; then
  smoke="${WORK}/smoke"
  mkdir -p "${smoke}"
  tar -xzf "${OUT_DIR}/nodex_${host_os}_${host_arch}.tar.gz" -C "${smoke}"
  reported="$("${smoke}/nodex" --version 2>&1 || true)"
  [[ "${reported}" == *"${VERSION}"* ]] \
    || die "the ${host_os}/${host_arch} build reports '${reported}', expected it to contain ${VERSION}"
  log "smoke test ok: ${reported}"
fi

log "built ${TAG} in ${OUT_DIR}:"
(cd "${OUT_DIR}" && ls -l nodex_* checksums.txt | awk '{printf "    %10s  %s\n", $5, $NF}')

if [[ "${PUBLISH}" != true ]]; then
  log "not published (no --publish). To tag and upload ${TAG}: scripts/release.sh --publish"
  exit 0
fi

# --- publish -------------------------------------------------------------------
commit="$(git rev-parse HEAD)"
assets=("${OUT_DIR}"/*)

if gh release view "${TAG}" >/dev/null 2>&1; then
  # Only reached with --replace (checked above).
  warn "release ${TAG} exists; replacing its assets"
  gh release upload "${TAG}" "${assets[@]}" --clobber
else
  # Annotated tag on the commit that was built, pushed before the release so
  # the release attaches to it rather than inventing one.
  if ! git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
    log "tagging ${TAG} at ${commit:0:7}"
    git tag -a "${TAG}" "${commit}" -m "nodex ${VERSION}"
  fi
  git push origin "refs/tags/${TAG}"

  args=(release create "${TAG}" "${assets[@]}" --title "${TAG}" --verify-tag)
  if [[ -n "${NOTES_FILE}" ]]; then args+=(--notes-file "${NOTES_FILE}"); else args+=(--generate-notes); fi
  [[ "${DRAFT}" == true ]] && args+=(--draft)
  [[ "${PRERELEASE}" == true ]] && args+=(--prerelease)
  log "creating release ${TAG}"
  gh "${args[@]}" || die "the ${TAG} tag was pushed but the release failed; re-run with --replace, or delete the tag: git push origin :refs/tags/${TAG} && git tag -d ${TAG}"
fi
log "published ${TAG}: $(gh release view "${TAG}" --json url -q .url)"
