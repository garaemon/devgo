---
name: release
description: |
  Release a new version of the devgo CLI: pick the next semantic version, bump the
  version string in cmd/root.go, open and merge a "Bump version" PR, then tag the
  commit and publish a GitHub Release.
  Use this skill whenever the user wants to cut, ship, or publish a release of devgo,
  or says things like "release a new version", "cut a release", "ship a new version",
  "リリースして", "新しいバージョンを出して", "バージョンを上げてリリース",
  "タグを切ってリリース". Also trigger when the user asks to bump the devgo version
  or create a GitHub Release for this repo, even if they don't say the word "release".
---

# Release Skill

Cut a new release of the devgo CLI end to end: decide the version, bump it, land the
bump through a PR, tag the merge commit, and publish a GitHub Release whose notes match
the established format.

This skill is devgo-specific. The single source of truth for the version is the string
printed by `showVersionInfo()` in `cmd/root.go` (e.g. `devgo version 0.3.0`). Releases
are tagged `vX.Y.Z` and published with `gh release`.

## Workflow Overview

1. Gather release state (current version, last tag/release, unreleased commits)
2. Decide the next semantic version
3. Create a dated branch and bump the version in `cmd/root.go`
4. Run tests and the linter
5. Commit, push, and open a "Bump version" PR
6. Wait for CI, then squash-merge and sync local `main`
7. Tag the merge commit `vX.Y.Z` and push the tag
8. Publish the GitHub Release with the standard notes

## Step 1: Gather Release State

Run these to understand what is being released:

- `git status` — confirm a clean working tree on an up-to-date `main`
- Current version: `grep -n "devgo version" cmd/root.go`
- Tags and releases: `git tag --sort=-version:refname` and `gh release list`
- Unreleased work: `git log --oneline <lastTag>..HEAD`

The commits since the last tag are exactly what this release ships — keep that list; it
drives both the version decision and the release notes.

## Step 2: Decide the Next Version

Apply semantic versioning to the unreleased commits. The project is pre-1.0, so:

- A new user-facing feature → minor bump (e.g. `0.2.0` → `0.3.0`)
- Only bug fixes / internal changes → patch bump (e.g. `0.3.0` → `0.3.1`)
- A breaking change → still a minor bump while pre-1.0, but call it out explicitly

If the changes are ambiguous (a mix that could read as either), state the chosen version
and the one-line reason rather than asking — the user can correct it. Pick the version
that best reflects the most significant change in the set.

## Step 3: Branch and Bump

Make a branch before editing (the repo's convention is a `YYYY.MM.DD-` prefix; use
today's date):

```bash
git checkout -b YYYY.MM.DD-bump-version-X.Y.Z
```

Edit the single version line in `cmd/root.go`:

```go
func showVersionInfo() {
	fmt.Println("devgo version X.Y.Z")
}
```

No other file holds the version (no CHANGELOG, no Makefile version). Confirm with a quick
`grep -rn "OLD.VERSION"` that nothing else references the old number.

## Step 4: Verify

Run the same gates CI enforces, since a release should never ship red:

```bash
make test
make lint
```

Both must pass. The version line is not covered by a unit test, so no test edits are
needed — but if a future version string ever becomes test-asserted, update that test too.

## Step 5: Commit and Open the PR

Stage only the changed file (never `git add .`):

```bash
git add cmd/root.go
git commit -m "$(cat <<'EOF'
Bump version to X.Y.Z

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
EOF
)"
git push -u origin YYYY.MM.DD-bump-version-X.Y.Z
```

Open the PR with a body that previews the release notes:

```bash
gh pr create --title "Bump version to X.Y.Z" --body "$(cat <<'EOF'
Bump the version from OLD to X.Y.Z for the next release.

## What's Changed since vOLD

- <one line per shipped PR/change> (#NN)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

## Step 6: Merge and Sync

Wait for CI to finish:

```bash
gh pr checks <PR#> --watch
```

`codecov/patch` is expected to fail on a release PR — a one-line version bump adds no
covered code, so the patch has 0% coverage. This is not a blocker; every prior release PR
hit the same thing. All other checks (test, integration, lint, codecov/project) must pass.

Squash-merge and clean up, then fast-forward local `main`:

```bash
gh pr merge <PR#> --squash --delete-branch
git checkout main && git pull --ff-only
```

After this, `git log --oneline -1` should show `Bump version to X.Y.Z (#PR#)`.

## Step 7: Tag the Release

Tag the squash-merge commit (the bump commit) and push the tag:

```bash
git tag -a vX.Y.Z -m "Release vX.Y.Z" <mergeCommitSha>
git push origin vX.Y.Z
```

## Step 8: Publish the GitHub Release

Match the existing release-note format exactly (a `What's Changed` bullet list plus a
Full Changelog compare link). Reference each shipped change by its PR number:

```bash
gh release create vX.Y.Z --title "vX.Y.Z" --notes "$(cat <<'EOF'
## What's Changed

- <change description> (#NN)

**Full Changelog**: https://github.com/garaemon/devgo/compare/vOLD...vX.Y.Z
EOF
)"
```

To see the exact phrasing of a prior release, run `gh release view vOLD`.

## Final Check

Confirm the release is live and marked latest:

```bash
gh release list
gh release view vX.Y.Z
```

Report the new version, the release URL, and the changes it shipped.
