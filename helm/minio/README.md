# Minigo Helm chart

This chart retains the upstream `minio` chart/resource naming for upgrade
compatibility and deploys `minigostorage/minigo:3.0.1` for both the storage server
and `mc` setup jobs. Install it from this repository; the upstream Helm repository
does not distribute this fork. Chart version: 5.4.1.

Supply a Kubernetes Secret with `rootUser` and `rootPassword` keys containing
unique credentials, and review storage and topology values before installation:

```sh
helm upgrade --install minigo ./helm/minio --namespace minigo --create-namespace \
  --set existingSecret=minigo-root -f values.production.yaml
```

[values.yaml](values.yaml) documents the complete configuration. Standalone and
distributed modes support persistent volumes. `persistence.enabled=false` uses
`emptyDir` and loses data when the pod is removed; use it only for disposable tests.
Do not attach an existing volume to two server instances at the same time.

The chart runs storage and setup jobs as UID/GID 1000 with non-root enforcement,
RuntimeDefault seccomp, no privilege escalation, all capabilities dropped and a
read-only root filesystem. Writable temporary, certificate and data paths have
explicit mounts. Security contexts apply with persistence enabled or disabled.
Ensure existing storage is accessible to UID/GID 1000. Custom commands and sidecars
may need their own writable mounts or explicit security-context overrides.

No default console administrator is created. To create users, configure unique
credentials using existing Secrets. **Upgrades do not delete existing accounts.**
If the old chart created `console/console123`, explicitly remove or rotate that
account and inspect its access keys. Review saved Helm values for that account and
for old `quay.io/minio` image overrides before upgrading.

Setup jobs verify TLS using the mounted CA certificate. TLS certificates must
match the configured service endpoint. For HTTPS termination at an ingress, set
`MINIO_BROWSER_REDIRECT_URL` to the public HTTPS console URL so session cookies
remain Secure.

Validate rendered configurations locally with:

```sh
helm lint helm/minio
go test ./tests/helm
```

These checks cover rendered standalone/distributed controllers, setup jobs,
TLS mounts, persistent/ephemeral storage and security contexts. Runtime acceptance
also exercises the same restricted container profile. They do not constitute a
live Kubernetes cluster upgrade or distributed storage qualification.
