#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
umask 077
mkdir -p dist/preview
if [ -z "${MINIGO_IMAGE:-}" ]; then
    case "$(uname -m)" in
        arm64|aarch64) MINIGO_IMAGE=minigo:local-arm64 ;;
        x86_64|amd64) MINIGO_IMAGE=minigo:local-amd64 ;;
        *) echo 'Set MINIGO_IMAGE for this host architecture.' >&2; exit 1 ;;
    esac
fi
export MINIGO_IMAGE
if [ -z "${MINIGO_LAN_IP:-}" ]; then
    if command -v ipconfig >/dev/null 2>&1; then
        interface=$(route -n get default | awk '/interface:/{print $2}')
        MINIGO_LAN_IP=$(ipconfig getifaddr "$interface")
    else
        MINIGO_LAN_IP=$(ip -4 route get 1.1.1.1 | awk '{for(i=1;i<=NF;i++)if($i=="src"){print $(i+1);exit}}')
    fi
fi
export MINIGO_LAN_IP
if [ ! -f dist/preview/credentials.env ]; then
    {
        echo 'MINIO_ROOT_USER=minigo-preview'
        printf 'MINIO_ROOT_PASSWORD=%s\n' "$(openssl rand -hex 16)"
    } > dist/preview/credentials.env
fi
{
    cat dist/preview/credentials.env
    printf 'MINIGO_LAN_IP=%s\nMINIGO_IMAGE=%s\n' "$MINIGO_LAN_IP" "$MINIGO_IMAGE"
} > dist/preview/runtime.env
docker compose --env-file dist/preview/runtime.env -f compose.preview.yaml up -d
for attempt in $(seq 1 60); do
    if curl --fail --silent "http://$MINIGO_LAN_IP:19000/minio/health/ready" >/dev/null && \
       curl --fail --silent "http://$MINIGO_LAN_IP:19001/" >/dev/null; then
        printf 'Console: http://%s:19001/\nS3: http://%s:19000/\nCredentials: %s/dist/preview/credentials.env\n' "$MINIGO_LAN_IP" "$MINIGO_LAN_IP" "$root"
        exit 0
    fi
    sleep 1
done
docker compose --env-file dist/preview/runtime.env -f compose.preview.yaml logs --tail=50
exit 1
