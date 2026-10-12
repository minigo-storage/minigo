#!/bin/sh
set -eu

container=
cleanup() {
    if [ -n "$container" ]; then
        docker rm -fv "$container" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

for arch in ${MINIGO_ARCHES:-amd64 arm64}; do
    image="${MINIGO_IMAGE_PREFIX:-minigo-reference}:local-$arch"
    container="minigo-reference-smoke-$arch-$$"
    docker run --rm --platform "linux/$arch" "$image" --version
    docker run -d --platform "linux/$arch" --name "$container" \
        -e MINIO_ROOT_USER=minigo-test \
        -e MINIO_ROOT_PASSWORD=minigo-test-password \
        "$image" server /data --console-address :9001 >/dev/null

    ready=false
    for attempt in $(seq 1 60); do
        if docker exec "$container" curl --fail --silent \
            http://127.0.0.1:9000/minio/health/ready >/dev/null 2>&1; then
            ready=true
            break
        fi
        sleep 1
    done
    if [ "$ready" != true ]; then
        docker logs "$container" >&2
        exit 1
    fi

    docker exec "$container" /bin/sh -ec '
        mc alias set local http://127.0.0.1:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
        mc mb local/minigo-smoke >/dev/null
        printf "minigo reference round trip\n" | mc pipe local/minigo-smoke/object.txt >/dev/null
        test "$(mc cat local/minigo-smoke/object.txt)" = "minigo reference round trip"
        mc stat local/minigo-smoke/object.txt >/dev/null
        mc ls local/minigo-smoke/ | grep -q object.txt
        mc rm local/minigo-smoke/object.txt >/dev/null
        mc rb local/minigo-smoke >/dev/null
    '
    printf 'PASS linux/%s: startup, readiness, bucket/object create, read, stat, list and delete\n' "$arch"
    cleanup
    container=
done
