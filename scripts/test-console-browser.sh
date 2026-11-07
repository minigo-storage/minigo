#!/bin/sh
# Disposable browser acceptance; no existing deployment or volume is touched.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) exit 1 ;; esac
container="minigo-browser-$arch-$$"
cleanup() { docker rm -fv "$container" >/dev/null 2>&1 || true; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export MINIO_ROOT_USER=browser-test
MINIO_ROOT_PASSWORD=$(openssl rand -hex 24)
export MINIO_ROOT_PASSWORD
docker run -d --name "$container" --platform "linux/$arch" \
    --user 1000:1000 --read-only --cap-drop ALL --security-opt no-new-privileges \
    --tmpfs /tmp:rw,noexec,nosuid,nodev,mode=1777 \
    --tmpfs /data:rw,noexec,nosuid,nodev,uid=1000,gid=1000,mode=0700 \
    -e HOME=/tmp -e MINIO_UPDATE=off -e MINIO_ROOT_USER -e MINIO_ROOT_PASSWORD \
    -p 127.0.0.1::9001 "${MINIGO_IMAGE_PREFIX:-minigo}:local-$arch" \
    server /data --certs-dir /tmp/certs --console-address :9001 >/dev/null
MINIGO_UI_URL="http://$(docker port "$container" 9001/tcp)"
export MINIGO_UI_URL
ready=false
for _attempt in $(seq 1 60); do
    if curl --fail --silent "$MINIGO_UI_URL/api/v1/login" >/dev/null 2>&1; then ready=true; break; fi
    sleep 1
done
if [ "$ready" != true ]; then docker logs "$container" >&2; exit 1; fi
node console/web-app/tests-minigo/integration.mjs
