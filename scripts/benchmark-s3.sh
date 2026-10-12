#!/usr/bin/env bash
# Alternating before/after workloads against new disposable volumes only.
set -euo pipefail
umask 077
ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
BASELINE_IMAGE=${BASELINE_IMAGE:-minigo:perf-baseline-arm64}
TARGET_IMAGE=${TARGET_IMAGE:-minigo:local-arm64}
PERF_PLATFORM=${PERF_PLATFORM:-linux/arm64}
PERF_ORDER=${PERF_ORDER:-baseline target target baseline}
RUN_DIR="$ROOT/dist/performance/s3-$(date +%Y%m%d%H%M%S)-$(openssl rand -hex 3)"
mkdir -p "$RUN_DIR/private"
export MINIO_ROOT_USER="perf$(openssl rand -hex 8)"
export MINIO_ROOT_PASSWORD=$(openssl rand -hex 24)
export MINIGO_PERF_DISPOSABLE=1
printf 'MINIO_ROOT_USER=%s\nMINIO_ROOT_PASSWORD=%s\nMINIO_UPDATE=off\n' "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" > "$RUN_DIR/private/server.env"
BASELINE_ID=$(docker image inspect "$BASELINE_IMAGE" --format '{{.Id}}')
TARGET_ID=$(docker image inspect "$TARGET_IMAGE" --format '{{.Id}}')
[[ "$BASELINE_ID" != "$TARGET_ID" ]] || { echo 'Baseline and target must differ' >&2; exit 1; }
printf '%s %s\n' "$BASELINE_IMAGE" "$BASELINE_ID" > "$RUN_DIR/baseline-image.txt"
printf '%s %s\n' "$TARGET_IMAGE" "$TARGET_ID" > "$RUN_DIR/target-image.txt"
go build -o "$RUN_DIR/driver" ./tests/performance
container=
volume=
cleanup() {
  if [[ -n "$container" ]]; then docker rm -f "$container" >/dev/null 2>&1 || true; container=; fi
  if [[ -n "$volume" ]]; then docker volume rm "$volume" >/dev/null 2>&1 || true; volume=; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
index=0
for label in $PERF_ORDER; do
  case "$label" in baseline) image=$BASELINE_ID ;; target) image=$TARGET_ID ;; *) echo "Unknown run label: $label" >&2; exit 1 ;; esac
  index=$((index + 1))
  run="$index-$label"
  container="minigo-perf-$(date +%Y%m%d%H%M%S)-$(openssl rand -hex 4)"
  volume="$container-data"
  docker volume create "$volume" >/dev/null
  docker run --detach --pull never --platform "$PERF_PLATFORM" --name "$container" \
    --env-file "$RUN_DIR/private/server.env" -p 127.0.0.1::9000 \
    --mount "type=volume,src=$volume,dst=/data" "$image" \
    server /data --console-address :9001 >/dev/null
  export PERF_ENDPOINT
  PERF_ENDPOINT=$(docker port "$container" 9000/tcp)
  ready=false
  for ((attempt=0; attempt<120; attempt++)); do
    if curl --fail --silent "http://$PERF_ENDPOINT/minio/health/ready" >/dev/null; then ready=true; break; fi
    sleep 1
  done
  [[ "$ready" == true ]] || { docker logs "$container" > "$RUN_DIR/$run-server.log" 2>&1; echo 'Server not ready' >&2; exit 1; }
  if ! "$RUN_DIR/driver" -out "$RUN_DIR/$run.json" 2>&1 | tee "$RUN_DIR/$run.log"; then
    docker logs "$container" > "$RUN_DIR/$run-server.log" 2>&1
    exit 1
  fi
  docker stats --no-stream --format '{{json .}}' "$container" > "$RUN_DIR/$run-post-workload-stats.json"
  docker logs "$container" > "$RUN_DIR/$run-server.log" 2>&1
  cleanup
done
printf 'S3 performance evidence: %s\n' "$RUN_DIR"
