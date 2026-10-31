# ADR 0001: Releases are cut by pushing a tag; Homebrew lives in a separate tap

- Status: accepted
- Date: 2026-10-31

## Context

pgnx is an open-source CLI that most of the time is written by an AI agent.
The agent must be able to build and fully dry-run a release for macOS,
Linux and Windows, but a release reaching users must stay a deliberate human
decision. Users expect `brew install` on macOS and Linux and plain binaries
elsewhere.

## Decision

1. **The release trigger is a human pushing a semver tag** (`v1.2.3`, or
   `v1.2.3-rc1` for a prerelease). Nothing that lands on `main` publishes.
   The release workflow refuses a tag whose commit is not reachable from
   `main`, so releases always come from reviewed history.
2. **GoReleaser is the single release engine**, configured in
   `.goreleaser.yaml`. Every artifact (six archives, `checksums.txt`,
   changelog, GitHub Release, Homebrew cask) comes from that one file, and
   `goreleaser release --snapshot` reproduces the whole release locally
   without a tag or token. `mise run ci` and `mise run snapshot` are the
   same commands locally and in CI; `mise.toml` pins the Go and GoReleaser
   versions for both.
3. **Homebrew distribution is a cask in a separate tap repository**,
   `kyleboon/homebrew-tap`, installed as `brew install kyleboon/tap/pgnx`.
   GoReleaser commits the cask directly to the tap's `main` branch using a
   fine-grained personal access token with `contents: write` on the tap
   repo only, stored as the `HOMEBREW_TAP_TOKEN` Actions secret. Prerelease
   tags do not update the cask.

## Consequences

- The agent can take a release to the brink (version stamping, snapshot
  build, changelog) and verify it, but cannot ship one: pushing the tag
  needs a human with push rights.
- A tag is the release record. Deleting and re-pushing a tag is the only
  way to redo a release, and GitHub Releases keep the history.
- Two human-only setup steps exist and are not derivable from the repo:
  creating the tap repo and minting the token. `scripts/release-setup-wizard.sh`
  walks through them. The token expires and must be renewed; a lapsed token
  fails the Homebrew step of a release.
- The workflow's `GITHUB_TOKEN` cannot write to a second repository, which
  is why the tap token exists at all. Moving the formula into this repo would
  remove the token but break `brew install kyleboon/tap/pgnx`.
- Binaries are not signed or notarised. The cask strips the macOS quarantine
  attribute after install, and Windows shows a SmartScreen warning. Signing
  is deferred and would be a new decision.
- Submitting to homebrew-core is possible later without changing anything
  here, since the tap is additive.
