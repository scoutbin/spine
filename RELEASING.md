# Releasing

This covers releasing a tool that calls spine's reusable workflows, upgrading spine in a tool, and changing spine itself. Workflow paths are relative to `.github/workflows/` in the spine repository unless noted.

## Tool repository setup

- `spine.json` at the repository root. Only the channels it enables run.
- `CHANGELOG.md` at the repository root. `go tool spine change` creates it on the first release. See [Releasing a tool](#releasing-a-tool).
- `.github/workflows/ci.yml` and `.github/workflows/release.yml`, shown below.
- `HOMEBREW_TAP_DEPLOY_KEY` as a repository secret, if `channels.homebrew.tap` is set. It is needed only for a publish of a stable tag. Dry runs do not use it.
- A trusted publisher on npmjs.com for the package, if `channels.npm.package` is set. See [npm](#npm).

Chocolatey is not wired. `channels.chocolatey.id` is validated, but no job uses it and no secret is needed for it.

The release jobs do not use GitHub environments, so no environment protection rules are needed.

Keep each `uses:` pinned to a full commit SHA with the version in a comment, in step with the spine version in `go.mod`. See [Upgrading spine in a tool](#upgrading-spine-in-a-tool).

`ci.yml`:

```yaml
name: CI
on:
  pull_request:
  push:
    branches: [main]
jobs:
  ci:
    permissions:
      contents: read
    uses: scoutbin/spine/.github/workflows/go-ci.yml@<full commit SHA> # <pseudo-version>
```

`release.yml`:

```yaml
name: Release
on:
  workflow_dispatch:
    inputs:
      tag:
        description: Existing tag to release, such as v1.2.3 or v1.2.3-rc.1
        required: true
        type: string
      publish:
        description: Publish the release and its channels. Off runs a dry run only
        required: false
        type: boolean
        default: false
concurrency:
  group: release
  cancel-in-progress: false
jobs:
  release:
    permissions:
      contents: write
      id-token: write
      attestations: write
    uses: scoutbin/spine/.github/workflows/go-release.yml@<full commit SHA> # <pseudo-version>
    with:
      tag: ${{ inputs.tag }}
      publish: ${{ inputs.publish }}
    secrets:
      homebrew-tap-deploy-key: ${{ secrets.HOMEBREW_TAP_DEPLOY_KEY }}
```

Why the caller looks like this:

- The reusable workflows run only when called, so the caller owns the triggers. The release caller runs only on dispatch, and it publishes only when `publish` is true.
- A called workflow cannot get more permissions than the calling job grants. The attest, release and npm jobs in `go-release.yml` request `attestations: write`, `contents: write` and `id-token: write`, so the caller grants all three. Keep the grants on dry runs too.
- The `release` concurrency group allows one release at a time per tool repository. The already-published check and the publish are separate steps, so two overlapping runs could both pass the check before either one publishes. `cancel-in-progress: false` makes a second run wait instead of cancelling a release in progress. The group is per repository and does not serialize different tools.

## Tags

- Stable: `vX.Y.Z`.
- Prerelease: `vX.Y.Z-alpha.N`, `vX.Y.Z-beta.N` or `vX.Y.Z-rc.N`, with N of 1 or more.

Any other tag fails in the `resolve` job. That includes a missing `v`, build metadata (`+`), an unknown prerelease such as `-nightly.1`, and `-rc.0`.

The release job checks that the tag still points at the commit that was built, and fails if it has moved. Do not move or delete a release tag once it is pushed.

Every release tag, prereleases included, needs a section in `CHANGELOG.md`. `resolve` fails without one. If a tag was pushed without a section, do not move or delete it. Run `go tool spine change` again instead. It picks a number above the orphan tag.

| Tag                                  | GitHub release         | npm dist-tag   | Homebrew formula |
| ------------------------------------ | ---------------------- | -------------- | ---------------- |
| `vX.Y.Z`                             | Latest                 | `latest`       | Updated          |
| `vX.Y.Z-alpha.N`, `-beta.N`, `-rc.N` | Prerelease, not latest | The track name | Not updated      |

## Releasing a tool

1. Update main and run the release command on it. The command adds a `## vX.Y.Z - YYYY-MM-DD` section to `CHANGELOG.md`, commits that file as `release vX.Y.Z` and creates the tag on that commit. It does not push.

   ```sh
   git pull --ff-only
   git fetch --tags
   go tool spine change patch "Fix config loading on Windows"
   ```

   The bump is `major`, `minor` or `patch`, and it names the core version to release. The message is required. It becomes the first paragraph of the section. It can span lines, but no line may start with `## `.

   - Without `--pre`, the tag is the core version. The base is the highest stable tag reachable from HEAD, or `v0.0.0` when there is none. This also promotes a prerelease: `minor` after `v1.3.0-rc.2` gives `v1.3.0`.
   - With `--pre alpha`, `beta` or `rc`, the tag is a prerelease of that core version. Its number is one more than the highest existing one for that core version and track, or 1 when there is none.
   - The new tag must be higher than every valid tag reachable from HEAD. Only reachable tags count, so a backport branch ignores newer tags on main.
   - For the first release, `patch` gives `v0.0.1`, and `minor --pre alpha` gives `v0.1.0-alpha.1`.

   The section lists one line per non-merge commit since the previous tag, newest first. Commits that only change `CHANGELOG.md` are left out, so earlier release commits are not repeated. A first release has only the message.

   The command writes nothing unless every check passes. It refuses an empty message, a detached HEAD, tracked changes, a tag or section that already exists for the new version, a next tag that is not higher than the highest reachable tag, and no commits since the previous tag. It does not fetch. If it fails after writing `CHANGELOG.md`, the error says what state is left. Nothing is rolled back.

   Check the commit with `git show`. To change it before pushing, edit `CHANGELOG.md`, run `git commit -a --amend` and move the local tag with `git tag -f vX.Y.Z`. Nothing has been pushed at that point.

   Run the push the command prints. The commit and the tag go together, and `--atomic` means neither lands if main has moved.

   ```sh
   git push --atomic origin main vX.Y.Z
   ```

   The flow pushes straight to main, so main must accept direct pushes. The tag's `go.mod` must pin a spine version whose contract matches the reusable workflow. See [Upgrading spine in a tool](#upgrading-spine-in-a-tool).

   If the push is rejected, main moved and nothing reached origin. Run `git show --stat HEAD`. The subject must be `release vX.Y.Z` and the only changed file must be `CHANGELOG.md`. If either differs, stop, because `git reset --hard HEAD~1` would discard that other commit. Otherwise, recover locally and run the command again.

   ```sh
   git tag -d vX.Y.Z
   git reset --hard HEAD~1
   git pull --ff-only
   git fetch --tags
   ```

2. Run a dry run. GitHub runs the `release.yml` on the ref you dispatch from. The code built is the tag's commit.

   ```sh
   gh workflow run release.yml -f tag=vX.Y.Z -f publish=false
   ```

   The dry run reads the manifest, checks the contract and checks that `CHANGELOG.md` has a section for the tag. It runs `go test ./...` and builds the archives and `checksums.txt`. It checks the checksums, and checks that there are six archives plus the checksum file. For stable tags it renders the Homebrew formula and runs `ruby -c` on it. It renders the npm package. It creates no release, pushes nothing and needs no secret. It uploads the built files as a workflow artifact.

3. Publish the same tag.

   ```sh
   gh workflow run release.yml -f tag=vX.Y.Z -f publish=true
   ```

   Before anything is published, the run refuses a tag that already has a published release. For a stable tag with Homebrew enabled, it also fails if `HOMEBREW_TAP_DEPLOY_KEY` is not set. Then it attests the archives and checksums, creates a draft release whose notes are the tag's section in `CHANGELOG.md`, uploads the assets and publishes the release. After that, the Homebrew job pushes the formula to the tap (stable tags only), and the npm job publishes the package (when npm is enabled).

4. Check the result.

   ```sh
   gh release view vX.Y.Z
   npm view <package> dist-tags
   ```

## Re-runs

- A new run with `publish` set to true is refused when the tag already has a published release. The refusal says so and points to "Re-run failed jobs".
- To recover a run that failed partway, use "Re-run failed jobs" on the original run.
- "Re-run all jobs" on the original run rebuilds the archives. While the release is still a draft, the release job compares each uploaded asset with the new build and refuses to overwrite a difference. Delete the draft release and run again in that case.
- A draft release is completed in place. The release job deletes assets that are not fully uploaded, uploads the missing ones, and fails if an uploaded asset differs from the build.
- A published release is accepted if every asset is byte-identical to the build, and nothing is changed. If a published release is missing assets, the job fails and does not modify the release.
- The Homebrew job exits without a push when the tap already has the same `version` line.
- The npm job skips the publish when that version is already on npm. It still reconciles the dist-tags.
- Release notes are set only when the release is created. A re-run does not change the body of an existing draft or published release.

## Homebrew

Homebrew is enabled by `channels.homebrew.tap` in `spine.json`. The tap is `scoutbin/homebrew-tap`. The formula is rendered at release time and committed to the tap as `Formula/<name>.rb` on `main`. It is not committed to the tool repository.

Only stable tags update the formula. For a prerelease tag the Homebrew job is skipped.

The push job reads the `version` line of the rendered formula. If the tap's formula already has that exact line, the job exits without a push. Otherwise it commits and pushes to `main`. It retries up to five times, rebasing onto the tap before each retry, because several tools share the tap. The check compares only the version line, not version numbers. Do not remove the `version` line from the formula template without changing this check.

Set up the deploy key once for the tap:

1. Create a key pair outside the repositories, for example with `ssh-keygen -t ed25519 -N "" -f ./homebrew-tap-deploy`.
2. In `scoutbin/homebrew-tap`, go to Settings, Deploy keys, add the `.pub` file and select "Allow write access".
3. In each tool repository, go to Settings, Secrets and variables, Actions, and add the private key as `HOMEBREW_TAP_DEPLOY_KEY`.
4. Delete the local private key.

One key serves all tools, because they share the tap.

CI runs `ruby -c` on the formula. CI does not run `brew audit`. The commands below for a local audit have not been run.

```sh
gh release download vX.Y.Z --pattern checksums.txt --dir dist
go tool spine package homebrew --tag vX.Y.Z
brew tap-new --no-git local/check
cp dist/packages/homebrew/<name>.rb "$(brew --repository local/check)/Formula/"
brew audit --strict --online local/check/<name>
```

Run these from a checkout of the tool at `vX.Y.Z`. `--online` checks the download URLs, so the release must be public.

## npm

npm is enabled by `channels.npm.package`. The rendered `package.json` sets `publishConfig.access` to `public` and `engines.node` to `>=22`.

The npm job publishes with OIDC trusted publishing, so no npm token is stored. The npm job is the only job with `id-token: write`. It installs npm 11.21.1, set by `NPM_VERSION` in `go-release.yml`, on the Node version pinned by spine's `.node-version`. The same npm runs the dist-tag reconcile. Changing the pin is a change to `go-release.yml`, so it needs a spine release.

Stable publishes use `--tag latest`. Prereleases use `--tag alpha`, `--tag beta` or `--tag rc`. After each publish, the reconcile moves each alpha, beta and rc dist-tag that already exists to the highest version on its upgrade path:

- alpha: alpha, beta, rc or stable versions
- beta: beta, rc or stable versions
- rc: rc or stable versions

A dist-tag that does not exist is not created. The reconcile never moves `latest`.

Trusted publisher, set up once per package on npmjs.com. Add a GitHub Actions trusted publisher to the package settings, naming the tool repository and the caller workflow file, `release.yml`.

To be confirmed:

- Whether npm matches the trusted publisher against the caller file in the tool repository or against the reusable `go-release.yml` in spine, since the publish step runs inside the called workflow. If a publish is refused, compare the error text with the trusted publisher settings.
- Whether npm lets a trusted publisher be added before the package exists. If not, the first version has to be published by hand. The steps would be to render the package from the release's `checksums.txt` and run `npm publish` with the dist-tag for its track. This procedure has not been run.

```sh
gh release download vX.Y.Z --pattern checksums.txt --dir dist
go tool spine package npm --tag vX.Y.Z
npm publish ./dist/packages/npm --tag latest
```

Once the trusted publisher is set, re-run the failed npm job. It finds the version on npm, skips the publish and reconciles the dist-tags.

The installed package downloads the release archives from GitHub, so the tool repository must be public for npm installs to work.

## Attestations

Attestations are created on publish only. `actions/attest` covers the release archives and `checksums.txt`. The Homebrew formula and the npm package are not attested.

To verify a downloaded archive:

```sh
gh attestation verify <file> --repo scoutbin/<tool> --signer-workflow scoutbin/spine/.github/workflows/go-release.yml
```

Attestations work for public repositories on free plans. To be confirmed on the first published release: that this signer identity matches for a reusable workflow called from another repository.

## Backports

A stable release is marked latest when it is published, even if a higher stable release already exists. If v1.2.4 is published after v1.3.0:

- The release job marks v1.2.4 as the GitHub latest release.
- The npm job publishes 1.2.4 with `--tag latest`, so the npm `latest` dist-tag moves to 1.2.4.
- The Homebrew job replaces the formula with 1.2.4. It does not compare version numbers.
- The installers and the action choose the newest stable release. Whether that choice follows GitHub's latest flag is to be confirmed.

To restore the higher release as the default, mark it as latest, restore the npm `latest` dist-tag and restore the formula. These commands have not been run.

```sh
gh release edit vHIGHER --latest
npm dist-tag add <package>@HIGHER latest
gh release download vHIGHER --pattern checksums.txt --dir dist
go tool spine package homebrew --tag vHIGHER
```

Run the `package` command from a checkout of the `vHIGHER` tag. Copy `dist/packages/homebrew/<name>.rb` to `Formula/` in the tap and push it to `main`. Do not re-run the Homebrew job for the backport, because it renders the backport's formula again.

## Upgrading spine in a tool

spine is pre-1.0 and has no tagged releases, so any commit may break compatibility. Pin an exact commit.

1. Get the full commit SHA of the spine commit to use. In a spine clone, run `git rev-parse <commit>`.
2. Update the spine version in `go.mod`.

   ```sh
   go get github.com/scoutbin/spine@<full commit SHA>
   go mod tidy
   ```

3. Set the SHA in the `uses:` lines of `ci.yml` and `release.yml`, and the `# <pseudo-version>` comment to the version `go.mod` now pins.
4. Check the contract. `go tool spine manifest` prints `contract=N`. The reusable workflows expect a fixed value, set by `SPINE_CONTRACT` in `go-ci.yml` and `go-release.yml`. If they differ, the contract check fails in the CI workflow, and the release workflow fails in `resolve`. The message says to bump the workflow ref and `go.mod` together.
5. Regenerate the installers and check them.

   ```sh
   go tool spine gen
   go tool spine gen --check
   ```

6. Run `gofmt -l .` (it must print nothing), `go vet ./...` and `go test ./...`.
7. Commit `go.mod`, `go.sum`, both workflow files and the generated files in one change. Then run the CI workflow.

The release workflow checks out the tag and reads `spine.json` and `go.mod` from it. Raising the workflow SHA to a spine version with a different contract, while the tag's `go.mod` still pins the older spine, fails at `resolve`.

## Changing spine

spine is not tagged or released. Tools consume a commit: a Go pseudo-version from `go get github.com/scoutbin/spine@<full commit SHA>`, and the same SHA in the workflow `uses:` lines. See [Upgrading spine in a tool](#upgrading-spine-in-a-tool).

Run `ci.yml` from the Actions tab on the commit a tool will pin. It is the only workflow that runs the installer, Homebrew, npm, pnpm and action suites.

When the dev CLI output or the `spine.json` schema changes incompatibly, raise `Contract` in `internal/devcmd` and set `SPINE_CONTRACT` to the same value in `go-ci.yml` and `go-release.yml`, in the same change. The value is `2` at the time of writing.

## Private repositories

- The installers, the action and the npm package download release assets without credentials. For them to work for users, the tool repository must be public. Do not enable the publishing channels before it is public.
- Whether a private caller can use spine's reusable workflows depends on spine's Actions access settings. Not verified.
