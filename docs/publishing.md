# Publishing Minigo images

The [Publish Docker images workflow](../.github/workflows/publish-images.yml)
publishes `minigostorage/minigo` on Docker Hub. It is manually dispatched from
`main`; ordinary commits and pull requests do not publish images.

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

```sh
gh workflow run publish-images.yml --repo minigo-storage/minigo --ref main
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
docker pull --platform linux/amd64 minigostorage/minigo:latest
docker pull --platform linux/arm64 minigostorage/minigo:latest
```

These checks complement the existing-volume compatibility evidence in
[validation](validation.md) and [performance results](performance.md). The
publication workflow does not rerun the large cached-official-image volume-swap
suite; distributed upgrades and production-specific qualification remain outside
this release check.
