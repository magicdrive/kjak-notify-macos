# Releasing

kjak-notify-macos is released with [Hephaestus](https://github.com/magicdrive/hephaestus)
v1.5.0 (its GitHub Action, pinned to the commit of v1.5.0; GoReleaser is
not used). One configuration, [hephaestus.yaml](../hephaestus.yaml),
describes every artifact:

| stage | what | where |
|---|---|---|
| build | `go build -trimpath` for `darwin/amd64` and `darwin/arm64` (`CGO_ENABLED=0`) | `dist/bin/darwin_<arch>/kjak-feather-notify-macos` |
| archive | `kjak-notify-macos_<version>_darwin_<arch>.tar.gz`: `kjak-feather-notify-macos` (0755), `LICENSE`, `README.md`; byte-for-byte reproducible | `dist/` |
| checksum | `kjak-notify-macos_checksums.txt` (SHA-256, `shasum -a 256 -c`) | `dist/` |
| release | GitHub Release `v<version>` with the archives and the checksum file, notes from [RELEASE_NOTES.md](../RELEASE_NOTES.md); a `-rc.N` tag is a prerelease | `magicdrive/kjak-notify-macos` |
| formula | `kjak-notify-macos.rb`: `depends_on :macos`, `depends_on "terminal-notifier"`, `bin.install "kjak-feather-notify-macos"`, caveats, a test that posts nothing | `dist/homebrew/` |
| tap | a pull request with the formula at the tap's root, for stable versions only (`skip_upload: auto`) | `magicdrive/homebrew-tap` (`brew install magicdrive/tap/kjak-notify-macos`) |

Names: the formula and the archives are named after the project,
`kjak-notify-macos`; the executable is `kjak-feather-notify-macos`, the
name kjak looks for (`kjak-feather-<plugin>`, plugin `notify-macos`).

## Workflows

- [ci.yml](../.github/workflows/ci.yml): lint (gofmt, vet, staticcheck,
  `go mod tidy`, govulncheck), shellcheck and the tag provenance tests, unit
  and race tests and the integration with kjak v1.2.0 and v1.1.0 on Apple silicon and
  Intel runners, the Hephaestus configurations, a snapshot with
  [scripts/verify-dist.sh](../scripts/verify-dist.sh) and the release plan
  of `v0.1.0` and `v0.1.0-rc.1`, and
  [scripts/homebrew-e2e.sh](../scripts/homebrew-e2e.sh) on both Homebrew
  prefixes. `ci-gate` is the one check to require.
- [release.yml](../.github/workflows/release.yml), on a `v*` tag: CI at
  the tagged commit; the tag must be on `main`
  ([verify-tag-provenance.sh](../.github/release/verify-tag-provenance.sh),
  from Hephaestus); a read-only build job without tokens (build, archive,
  checksum, verify); then the `release` environment job: verify again,
  plan, a dry run of the tap (stable only), a provenance attestation, the
  GitHub Release, the formula and the tap pull request (stable only).
- [release-rehearsal.yml](../.github/workflows/release-rehearsal.yml),
  dispatched by hand with a version such as `0.1.0-rehearsal.1`: the same
  jobs with [hephaestus.rehearsal.yaml](../hephaestus.rehearsal.yaml) — a
  **draft** release (no tag, nothing public) and a pull request on
  `magicdrive/homebrew-tap-sandbox` — and each publishing stage twice: the
  second run must be a no-op (no asset uploaded, no tap commit).

`release_test.go` keeps these true: the rehearsal configuration differs
only by `draft`, `skip_upload` and the sandbox tap; every action is pinned
to a commit; the tap token reaches only the two steps the plan gates; a
checkout never keeps its credentials.

### Release candidates never update the tap

Three layers: Hephaestus's plan (`hephaestus publish plan v0.1.0-rc.1`:
`prerelease=true`, `tap=skip`) skips both tap steps, so the tap token never
reaches the job's processes; the workflow fails if a prerelease's plan
would publish; and `brews[0].skip_upload: auto` makes the tap stage skip a
prerelease by itself (`tap: kjak-notify-macos: skipped: skip_upload is auto
and 0.1.0-rc.1 is a prerelease`). CI checks the plan of `v0.1.0-rc.1`.

## Setup on GitHub (owner)

1. Environment `release`: secret `HOMEBREW_TAP_GITHUB_TOKEN`, a
   fine-grained token (or GitHub App token) with contents and pull requests
   write on `magicdrive/homebrew-tap` only; deployments restricted to `v*`
   tags; a required reviewer (the owner).
2. Environment `release-rehearsal`: secret `HOMEBREW_TAP_SANDBOX_TOKEN`
   with the same rights on `magicdrive/homebrew-tap-sandbox` only;
   deployments restricted to `main`.
3. Never store a tap token as a repository secret: only in these
   environments.
4. A ruleset on `main` requiring `ci-gate`, and a tag ruleset that lets only
   the owner create, move or delete `v*` tags. A tag runs the `release.yml`
   of its own commit: whoever can push a `v*` tag on another commit could
   drop the provenance job from it, so who may push tags is the real gate
   (with the `release` reviewer).
5. Immutable releases, if wanted (a published release's assets can then
   never change; Hephaestus never replaces them anyway).

## Steps

1. `make check release-check snapshot` and `make homebrew-e2e` (installs
   into this Mac's Homebrew prefix and removes it again).
2. Update [RELEASE_NOTES.md](../RELEASE_NOTES.md) and
   [CHANGELOG.md](../CHANGELOG.md) (it must name the tag).
3. Rehearse: Actions → Release rehearsal → `0.1.0-rehearsal.1`. Check the
   draft release and the sandbox pull request (`brew install` from the
   sandbox branch on an Apple silicon and an Intel Mac, if at hand), then
   run the cleanup commands of the summary.
4. Optionally a release candidate: tag `v0.1.0-rc.1` on `main`. It publishes
   a **public prerelease** and leaves the tap alone.
5. `hephaestus preflight --tag v0.1.0`, then tag `v0.1.0` on `main` and push
   the tag.
6. Merge the tap pull request on `magicdrive/homebrew-tap` (its own checks
   run first), then `brew install magicdrive/tap/kjak-notify-macos` and
   `brew test kjak-notify-macos`.

A failed run can be re-run: Hephaestus leaves identical assets and an
identical formula alone, never replaces a published release's assets and
refuses a formula of the same version with another sha256. A published
release cannot be taken back: fix forward with a new version.
