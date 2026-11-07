# Publishing Minigo images

Minigo publishes `minigostorage/minigo` for Linux amd64 and arm64. Builds and
publication are separate, manually dispatched workflows on `main`. Ordinary
commits and pull requests do not publish images. A release always promotes the
same image archives that passed native CI tests; it never rebuilds them.

## Versions and candidates

Versions start at 3.0.0. Choose `patch`, `minor` or `major`; compatible security
fixes use `patch`. The candidate workflow selects the next version from stable
Git tags and records it, the increment, source SHA, run/attempt, architecture and
security-release classification beside each image archive.

```sh
gh workflow run publish-images.yml --repo minigo-storage/minigo --ref main \
  -f increment=patch -f security_release=true
gh run list --repo minigo-storage/minigo --workflow publish-images.yml
gh run watch RUN_ID --repo minigo-storage/minigo --exit-status
```

Both native architectures must pass frontend tests, backend/console tests,
ordinary S3 smoke tests, non-root/read-only runtime tests, Go vulnerability checks
and the npm audit. Archives, their checksums and image metadata are retained for
seven days. Go reports are retained for thirty days. Candidate creation does not
reserve a Git tag or change Docker Hub tags.

## Focused security releases

Trivy runs **only on a maintainer's local workstation**, never on hosted or
self-hosted GitHub Actions runners. Do not scan images for every routine feature,
fix or test-cleanup release. The scan and qualification scripts refuse to run
when `GITHUB_ACTIONS=true`; workflow regression checks also enforce this boundary.

For a security candidate, check out its exact source commit with no tracked
changes and run:

```sh
scripts/qualify-release.sh RUN_ID
```

Prerequisites are Docker with cross-architecture execution, Go, npm-installed
frontend dependencies and Playwright Chromium, `gh`, `mc`, curl and Python 3. The
existing official MinIO arm64 baseline must be available locally for compatibility
testing; see [the suite](../tests/compatibility/README.md). The command:

1. Downloads artifacts from the successful candidate run, verifies repository,
   workflow, main branch, source SHA, run attempt, platform, version and checksums,
   then loads and verifies both image IDs.
2. Tests both hardened runtimes, runs desktop/mobile browser journeys against a
   disposable deployment, and runs the full 10,000-object / >5 GiB same-volume
   upgrade, interrupted-write recovery and rollback suite on arm64.
3. Scans the exact archives with digest-pinned Trivy, without mounting the Docker
   socket. It retains all severities and unfixed findings, evaluates the narrowly
   documented source assessments, and generates CycloneDX SBOMs.
4. Creates a receipt binding the reports, SBOMs, scanner/database metadata and
   acceptance evidence to both archive checksums, image IDs and source SHA.
5. Uploads evidence to a draft GitHub release and prints the publication command.

Only the evidence directory is uploaded. Compatibility test credentials remain
in the ignored local test directory. Reports expire after 24 hours and the
vulnerability database must be at most 48 hours old. Requalify if they expire or
if the source, candidate attempt, images or evidence changes.

Run the printed command to publish:

```sh
gh workflow run promote-images.yml --repo minigo-storage/minigo --ref main \
  -f candidate_run_id=RUN_ID -f assessment_sha256=PRINTED_SHA256
```

The promotion workflow verifies the receipt checksum and every referenced file,
re-evaluates raw findings, and confirms the acceptance tests used the same images.
The checksum is an explicit maintainer-approved binding to locally produced
evidence, not an independent scanner attestation. Only trusted repository writers
should dispatch publication or update draft release assets. The source must still
match the promotion workflow's `main` commit.

For a routine release, build with `security_release=false` (the default), then
invoke `promote-images.yml` with its candidate run ID. No Trivy scan or security
receipt is required. Native tests, Go checks, npm audit and artifact integrity
checks still apply.

## Publication and recovery

The promotion job rechecks version ordering, reserves the Git tag at the tested
source, pushes the verified images and checks the combined index has exactly
Linux amd64 and arm64. It updates these tags to that index:

- `3.0.1` and `v3.0.1` (for example).
- `sha-FULL_COMMIT_SHA`.
- `latest`.
- A unique `build-CANDIDATE_RUN-promote-RUN-ATTEMPT` tag, plus architecture tags.

The index digest, source SHA and version are recorded in the workflow summary
and GitHub release. Use the digest for immutable deployments. A source tag alone
is not an immutable package selection because fresh builds refresh OS packages.

If publication fails after reserving a tag, rerun **failed jobs** on the same run
while artifacts and security evidence remain valid. Do not move or delete release
tags. If only the description job fails, rerun that job; the images and release
are already published. A new release from a later commit advances past any
abandoned reserved version.

After publication, inspect the index and test the remote images:

```sh
docker buildx imagetools inspect minigostorage/minigo:3.0.1
docker pull --platform linux/amd64 minigostorage/minigo:3.0.1
docker pull --platform linux/arm64 minigostorage/minigo:3.0.1
```

Source versions inside the server retain timestamp compatibility with `mc` and
the admin API. The OCI version label identifies the complete Minigo distribution.

## Credentials and description

The repository needs the Actions variable `DOCKERHUB_USERNAME` and secret
`DOCKERHUB_TOKEN`. The Docker ID must have admin access to the public
`minigostorage/minigo` repository. The current shared token needs **Repo Read,
Write, Delete** permission because Docker Hub description updates require it.
Only the login and description steps receive this secret; it is not passed to
build or test code. Repository access is read-only except in the promotion job.

```sh
gh variable set DOCKERHUB_USERNAME --repo minigo-storage/minigo --body YOUR_DOCKER_ID
gh secret set DOCKERHUB_TOKEN --repo minigo-storage/minigo --app actions
```

The second command prompts for the token. Never put its value in a command,
commit or issue. The permission requirement is documented by the
[description action](https://github.com/peter-evans/dockerhub-description) and
[Docker](https://docs.docker.com/security/access-tokens/reference/). A token with
only Read & Write can push images but receives `Forbidden` when updating the
README. Docker personal tokens can affect other repositories their owner manages.

Successful publication calls the description workflow using the same source
commit. Refresh the description independently with:

```sh
gh workflow run dockerhub-description.yml --repo minigo-storage/minigo --ref main
```

Release safeguards can be tested locally with:

```sh
python3 scripts/test-release-version.py
python3 scripts/test-release-artifacts.py
```
