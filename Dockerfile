FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

ARG TARGETARCH
ARG MINIGO_REVISION=local
LABEL org.opencontainers.image.title="Minigo" \
      org.opencontainers.image.description="MinIO-compatible object storage with the Minigo management console" \
      org.opencontainers.image.revision="${MINIGO_REVISION}" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later"

# The Go binaries are static. Keep the original shell/curl/GNU chroot contract
# without carrying UBI's package-manager stack and its unpatched dependencies.
RUN apk upgrade --no-cache && apk add --no-cache bash ca-certificates coreutils curl
ENV MINIO_ACCESS_KEY_FILE=access_key \
    MINIO_SECRET_KEY_FILE=secret_key \
    MINIO_ROOT_USER_FILE=access_key \
    MINIO_ROOT_PASSWORD_FILE=secret_key \
    MINIO_KMS_SECRET_KEY_FILE=kms_master_key \
    MINIO_UPDATE_MINISIGN_PUBKEY="RWTx5Zr1tiHQLwG9keckT0c45M3AGeHD6IvimQHpyRywVWGbP1aVSGav" \
    MINIO_CONFIG_ENV_FILE=config.env \
    MC_CONFIG_DIR=/tmp/.mc

COPY dist/minigo/linux-${TARGETARCH}/minio /usr/bin/minio
COPY dist/minigo/linux-${TARGETARCH}/minio.sha256sum /usr/bin/minio.sha256sum
COPY dist/minigo/linux-${TARGETARCH}/mc /usr/bin/mc
COPY dist/minigo/linux-${TARGETARCH}/mc.sha256sum /usr/bin/mc.sha256sum
COPY LICENSE NOTICE CREDITS /licenses/
COPY console/LICENSE console/NOTICE console/CREDITS /licenses/console/
COPY dist/minigo/licenses/mc/ /licenses/mc/
COPY dockerscripts/docker-entrypoint.sh /usr/bin/docker-entrypoint.sh
RUN ln -s /usr/bin/minio /usr/bin/minigo

EXPOSE 9000
VOLUME ["/data"]
ENTRYPOINT ["/usr/bin/docker-entrypoint.sh"]
CMD ["minio"]
