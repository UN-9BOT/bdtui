# Release Runbook

This document defines the repeatable release process for `bdtui`.

## Scope

- Platforms: `darwin`, `linux`
- Architectures: `amd64`, `arm64`
- Distribution: GitHub Releases
- Release trigger: merged PR to `master`
- Tag format (auto-generated): `YYYY.MM.DD-pr<PR_NUMBER>-<MERGE_SHA7>`

## Prerequisites

- CI is green on `master`.
- Branch protection requires PR-based changes to `master`.
- Required files are up to date:
  - `CHANGELOG.md`
  - `README.md`
  - `.github/workflows/ci.yml`
  - `.github/workflows/release.yml`

## Release Steps

1. Update `CHANGELOG.md` in the PR that is intended for release.
2. Ensure `go test ./...` and `go build ./...` pass locally and in CI.
3. Merge PR into `master`.
4. Wait for `release` workflow completion in GitHub Actions.
5. Verify GitHub Release contains:
  - `bdtui-darwin-amd64`
  - `bdtui-darwin-arm64`
  - `bdtui-linux-amd64`
  - `bdtui-linux-arm64`
  - `checksums.txt`
6. Confirm release tag follows the generated format:
   - `YYYY.MM.DD-pr<PR_NUMBER>-<MERGE_SHA7>`
7. Run post-release verification checklist:
   - `docs/POST_RELEASE_CHECKLIST.md`

## Release Notes Flow

- `CHANGELOG.md` is the source of truth for curated release notes.
- GitHub Release uses auto-generated notes; keep changelog entries concise and user-facing.
- If auto-notes miss critical context, edit release text manually in GitHub UI.

## Rollback / Hotfix

If release artifacts are broken:

1. Mark the release as problematic in release notes.
2. Prepare a fix PR with corrective changes.
3. Merge the fix PR into `master` to produce a new automated release.
4. Re-run verification checklist after new release publish.

Do not overwrite existing release artifacts in-place; publish a new release tag.
