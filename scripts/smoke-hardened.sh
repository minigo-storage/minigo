#!/bin/sh
# Exercise the supported non-root, read-only deployment contract on disposable data.
set -eu
container=
cleanup() { if [ -n "$container" ]; then docker rm -fv "$container" >/dev/null 2>&1 || true; fi; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
for arch in ${MINIGO_ARCHES:-amd64 arm64}; do
    image="${MINIGO_IMAGE_PREFIX:-minigo}:local-$arch"
    container="minigo-hardened-$arch-$$"
    docker run -d --platform "linux/$arch" --name "$container" \
        --user 1000:1000 --read-only --cap-drop ALL --security-opt no-new-privileges \
        --tmpfs /tmp:rw,noexec,nosuid,nodev,mode=1777 \
        --tmpfs /data:rw,noexec,nosuid,nodev,uid=1000,gid=1000,mode=0700 \
        -e HOME=/tmp -e MINIO_UPDATE=off -e MINIO_ROOT_USER=hardened-test \
        -e MINIO_ROOT_PASSWORD=hardened-disposable-password \
        "$image" server /data --certs-dir /tmp/certs --console-address :9001 >/dev/null
    ready=false
    for _attempt in $(seq 1 60); do
        if docker exec "$container" curl --fail --silent http://127.0.0.1:9000/minio/health/ready >/dev/null 2>&1; then ready=true; break; fi
        sleep 1
    done
    if [ "$ready" != true ]; then docker logs "$container" >&2; exit 1; fi
    docker exec "$container" /bin/sh -ec '
        test "$(id -u)" = 1000
        test ! -w /usr/bin
        mc alias set local http://127.0.0.1:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
        mc mb local/hardened >/dev/null
        printf "non-root round trip" | mc pipe local/hardened/object >/dev/null
        test "$(mc cat local/hardened/object)" = "non-root round trip"
        mc rm local/hardened/object >/dev/null
        mc rb local/hardened >/dev/null
        curl --fail --silent http://127.0.0.1:9001/ >/dev/null
    '
    printf 'PASS linux/%s: non-root, read-only root, no capabilities, S3 and console\n' "$arch"
    cleanup
    container=
done
