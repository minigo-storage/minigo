#!/usr/bin/env bash
# Run only on a maintainer's workstation for a focused security release.
set -euo pipefail
if [[ ${GITHUB_ACTIONS:-false} == true ]]; then
  echo 'Security image qualification runs locally only.' >&2
  exit 1
fi
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
run_id=${1:?Usage: scripts/qualify-release.sh CANDIDATE_RUN_ID}
[[ $run_id =~ ^[0-9]+$ ]] || exit 1
git diff --exit-code HEAD -- . >/dev/null
mkdir -p dist/releases
directory=$(mktemp -d "$root/dist/releases/$run_id.XXXXXX")
images="$directory/images"
evidence="$directory/evidence"
mkdir -p "$evidence"
gh run download "$run_id" --repo minigo-storage/minigo --pattern 'minigo-image-*' --dir "$images"
python3 scripts/release-artifacts.py inspect "$images" "$run_id" > "$directory/candidate.json"
version=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$directory/candidate.json")
python3 - "$directory/candidate.json" <<'PY'
import json, sys
assert json.load(open(sys.argv[1]))['security_release'], 'Dispatch a security candidate first'
PY
for arch in amd64 arm64; do
  docker load --input "$images/minigo-image-$arch/image.tar.gz"
  expected=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["images"][sys.argv[2]]["image_id"])' "$directory/candidate.json" "$arch")
  [[ $(docker image inspect "minigo:local-$arch" --format '{{.Id}}') == "$expected" ]]
done
MINIGO_ARCHES='amd64 arm64' ./scripts/smoke-hardened.sh | tee "$evidence/hardened-smoke.txt"
./scripts/test-console-browser.sh | tee "$evidence/browser-smoke.txt"
TARGET_IMAGE=minigo:local-arm64 COMPAT_PLATFORM=linux/arm64 SMALL_OBJECTS=10000 LARGE_BYTES=5369757696 \
  ./scripts/test-compatibility.sh | tee "$directory/compatibility.log"
compatibility=$(sed -n 's/^Compatibility evidence: //p' "$directory/compatibility.log" | tail -1)
test -n "$compatibility"
cp "$compatibility" "$evidence/compatibility.json"
MINIGO_ARCHIVE_ROOT="$images" MINIGO_SCAN_DIR="$evidence" MINIGO_ARCHES='amd64 arm64' ./scripts/check-image-security.sh
assessment=$(python3 scripts/release-artifacts.py record "$images" "$run_id" --evidence "$evidence")
sha=$(git rev-parse HEAD)
if gh release view "v$version" --repo minigo-storage/minigo --json isDraft,targetCommitish > "$directory/release.json" 2>/dev/null; then
  python3 - "$directory/release.json" "$sha" <<'PY'
import json,sys
r=json.load(open(sys.argv[1]))
assert r['isDraft'] and r['targetCommitish'] == sys.argv[2], 'Refusing to modify a published or unrelated release'
PY
else
  if [[ -f "docs/releases/$version.md" ]]; then
    cp "docs/releases/$version.md" "$directory/release-notes.md"
  else
    printf "Minigo %s security release. See attached local assessments and SBOMs.\n" "$version" > "$directory/release-notes.md"
  fi
  printf '\nCandidate run: %s\nSource: %s\n' "$run_id" "$sha" >> "$directory/release-notes.md"
  gh release create "v$version" --repo minigo-storage/minigo --draft --target "$sha" \
    --title "Minigo $version" --notes-file "$directory/release-notes.md"
fi
gh release upload "v$version" --repo minigo-storage/minigo --clobber "$evidence"/*
printf 'Security evidence: %s\nAssessment SHA-256: %s\n' "$evidence" "$assessment"
printf 'Publish: gh workflow run promote-images.yml --repo minigo-storage/minigo --ref main -f candidate_run_id=%s -f assessment_sha256=%s\n' "$run_id" "$assessment"
