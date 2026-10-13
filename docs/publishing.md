# Publishing Minigo images

The [Publish Docker images workflow](../.github/workflows/publish-images.yml)
publishes `minigostorage/minigo` on Docker Hub. It is manually dispatched from
`main`; ordinary commits and pull requests do not publish images.

## Semantic versions

Minigo starts at **3.0.0**, one major version above the bundled upstream
Console/Object Browser's **2.0.4** (see its [changelog](../console/CHANGELOG.md)).
MinIO Server used timestamp releases; its last tagged release was
[`RELEASE.2025-10-15T17-29-55Z`](https://github.com/minio/minio/releases/tag/RELEASE.2025-10-15T17-29-55Z).
Minigo's semantic versions identify the complete server and console distribution.
Internal server version timestamps remain compatible with `mc` and the admin
protocol; the image's `org.opencontainers.image.version` label records the
Minigo release version. Local builds use `dev` unless `MINIGO_VERSION` is set.

Choose an increment when dispatching the workflow:

| Increment | Use for | Example after 3.0.0 |
| --- | --- | --- |
| `patch` (default) | Compatible fixes and maintenance | 3.0.1 |
| `minor` | Compatible new features | 3.1.0 |
| `major` | Breaking changes | 4.0.0 |

The first release is always 3.0.0. Subsequent versions are calculated from the
highest stable `vMAJOR.MINOR.PATCH` Git tag. Prerelease and upstream timestamp
tags are ignored. A fresh run must include the latest release and contain a
new source commit. The workflow checks the version again before publication
to prevent an older run from replacing a newer release.

After both architectures pass, the publish job reserves the Git tag at the
tested source commit, publishes the images and creates a GitHub release with
generated notes and the image digest. The job uses `contents: write` permission;
build jobs retain read-only repository access.

If publication fails after reserving a tag, rerun **failed jobs** on the same
workflow run while its tested image artifacts are still available. This resumes
the reserved version using those artifacts. Do not move or delete release tags;
an abandoned reservation can leave a gap in version numbers. A new release from
a later commit advances beyond the reserved version.

## Credentials

The GitHub repository needs the Actions variable `DOCKERHUB_USERNAME` and the
Actions secret `DOCKERHUB_TOKEN`. For a Docker personal access token, the username
is the token owner's personal Docker ID. That account needs write access to
`minigostorage/minigo`, and the token needs **Repo Read & Write** permission.
The Docker Hub repository must be public.

```sh
gh variable set DOCKERHUB_USERNAME --repo minigo-storage/minigo --body YOUR_DOCKER_ID
gh secret set DOCKERHUB_TOKEN --repo minigo-storage/minigo --app actions
```

The second command prompts for the token; do not put the token in a command
argument, commit it, or paste it into an issue. The workflow authenticates before
starting the builds and passes the token only to authentication steps.

## Build, test and publish

The Docker Hub overview can be refreshed independently from the current README
using the [description workflow](../.github/workflows/dockerhub-description.yml):

```sh
gh workflow run dockerhub-description.yml --repo minigo-storage/minigo --ref main
```

This updates the short description and full README, converting relative links
to GitHub URLs so the contribution guide, security policy and other documents
remain accessible from Docker Hub. It uses the existing Docker Hub credentials
and does not rebuild images, change image tags or increment the release version.

To publish new images:

```sh
gh workflow run publish-images.yml --repo minigo-storage/minigo --ref main -f increment=patch
gh run list --repo minigo-storage/minigo --workflow publish-images.yml
gh run watch RUN_ID --repo minigo-storage/minigo --exit-status
```

Each platform runs on its own native GitHub-hosted Ubuntu runner. The workflow
uses the existing build scripts, frontend tests, backend and console API tests,
real image startup and S3 smoke tests, source vulnerability checks and image
scans. The documented scanner assessments remain visible and unassessed image
findings fail the run.

Both platforms must pass before the publish job starts. It downloads the tested
Docker image archives, verifies their archive checksums, image IDs, architecture
and source revision, and pushes those exact images without rebuilding. It then
checks that the combined index contains exactly Linux amd64 and arm64 before
updating `latest`.

## Tags and evidence

- `latest`: the most recently published successful build from `main`.
- `3.0.0` and `v3.0.0` (for example): the versioned multi-platform image. The
  matching Git tag and GitHub release use `v3.0.0`.
- `sha-FULL_COMMIT_SHA`: a build of that source revision. Rerunning a source
  revision can refresh OS packages, so use a digest for an immutable deployment.
- `build-RUN_ID-ATTEMPT`: the combined image for one specific workflow attempt.
- `build-RUN_ID-ATTEMPT-amd64` and `build-RUN_ID-ATTEMPT-arm64`: the individual
  platform images used by that index.

The workflow summary records the source SHA, tags and index digest. Tested image
archives are retained as GitHub Actions artifacts for seven days and raw security
reports for thirty days. Test output remains in the workflow logs.

```sh
docker buildx imagetools inspect minigostorage/minigo:latest
docker pull minigostorage/minigo:3.0.0
docker pull --platform linux/amd64 minigostorage/minigo:latest
docker pull --platform linux/arm64 minigostorage/minigo:latest
```

These checks complement the existing-volume compatibility evidence in
[validation](validation.md) and [performance results](performance.md). The
publication workflow does not rerun the large cached-official-image volume-swap
suite; distributed upgrades and production-specific qualification remain outside
this release check.

The version selection and recovery checks can be run locally with
`python3 scripts/test-release-version.py`.
