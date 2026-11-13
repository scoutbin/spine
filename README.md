# spine

spine is the shared core of scoutbin's command-line tools. It has two parts: a Go module (`github.com/scoutbin/spine`) that tools build on, and reusable GitHub workflows that test and release them. spine does not ship a CLI of its own. Tools run its development CLI with `go tool spine`.

## What a tool gets

- A binary with the built-in commands `version`, `info`, `check` and `upgrade`, plus the tool's own commands.
- Release archives for darwin, linux and windows, on amd64 and arm64, with a `checksums.txt`.
- Installers, `install.sh` and `install.ps1`, generated into the tool repository.
- Distribution through GitHub Releases, the installers, Homebrew (tap `scoutbin/homebrew-tap`), npm (under `@scoutbin`) and a GitHub Action. GitHub Releases and the installers are always on. Homebrew, npm and the action are enabled in `spine.json`.
- Four release tracks: stable, alpha, beta and rc.

Status: spine is pre-1.0 and is not tagged or released. Tools should pin an exact spine commit, because any commit may break compatibility. The installers and the action download release assets without credentials, so the tool repository must be public for them to work for users.

## Repository layout

| Path                 | Contents                                                                                                      |
| -------------------- | ------------------------------------------------------------------------------------------------------------- |
| `spine` (root)       | `Run`, `Env`, `OSEnv`, `Tool`, `Load` and `MustLoad`                                                          |
| `cli`                | Command and flag framework, used by tools and the dev CLI                                                     |
| `term`               | Decides whether progress and color are shown. Output goes to stderr                                           |
| `cmd/spine`          | The dev CLI, run as `go tool spine`                                                                           |
| `internal/`          | Everything else, including release, channel, render and manifest code. Not importable from outside the module |
| `internal/example`   | A minimal tool. It is the fixture for spine's tests and CI                                                    |
| `.github/workflows/` | `go-ci.yml`, `go-release.yml` and `node-ci.yml` (reusable), `ci.yml` (spine's own CI)                         |

## spine.json

Each tool has one `spine.json` at the repository root. The binary, the dev CLI and the workflows all read it. Unknown fields are errors.

```json
{
  "name": "mytool",
  "repo": "scoutbin/mytool",
  "summary": "One-line description of the tool",
  "license": "Apache-2.0",
  "channels": {
    "homebrew": { "tap": "scoutbin/homebrew-tap" },
    "npm": { "package": "@scoutbin/mytool" },
    "action": true
  }
}
```

| Field                    | Required | Rules                                                                                                                             |
| ------------------------ | -------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `name`                   | yes      | Lower-case letters and digits, starting with a letter. Sets the binary, archive, install directory and environment variable names |
| `alias`                  | no       | A second command name for the tool. Same rules as `name`, and it must differ from `name`                                          |
| `repo`                   | yes      | `owner/name` of the tool repository. Sets the release URLs and the package owner                                                  |
| `summary`                | yes      | One line description                                                                                                              |
| `license`                | yes      | Used in the Homebrew formula                                                                                                      |
| `channels.homebrew.tap`  | no       | `owner/homebrew-suffix`. Enables Homebrew. The formula is `owner/suffix/name` in lower case                                       |
| `channels.npm.package`   | no       | npm package name, optionally scoped, lower case, at most 214 characters. Enables npm                                              |
| `channels.action`        | no       | `true` generates `action.yml`                                                                                                     |
| `channels.chocolatey.id` | no       | Validated, but no channel uses it yet                                                                                             |

These values are derived from `name`, so they have no fields of their own:

- Binary: `mytool`, or `mytool.exe` on Windows.
- Archives: `mytool_<version>_<os>_<arch>.tar.gz`, or `.zip` on Windows. The version has no leading `v`.
- Environment variables: the upper-cased name as prefix, such as `MYTOOL_VERSION`.
- Default install directories: `~/.local/bin` for `install.sh`, `%LOCALAPPDATA%\mytool\bin` for `install.ps1`.

## Command alias

`alias` sets one extra command name for the tool. It follows the same rules as `name` and must differ from it.

Each channel makes the alias available as follows:

- `install.sh` creates a relative symlink named `<alias>` that points to `<name>`. If anything else already exists at that path, the installer keeps it, prints a warning and exits 0. A rerun leaves a correct link alone.
- `install.ps1` copies the binary to `<alias>.exe` next to `<name>.exe`. Each run overwrites the copy.
- Homebrew links the alias with `install_symlink`.
- npm adds a second `bin` entry, so a global install creates both commands.
- The GitHub Action gets the alias from the installers. `outputs.path` still points to the canonical executable.
- Release archives are unchanged and contain only the canonical binary.

An npm global install fails with EEXIST if another globally installed package already owns the alias's bin name.

Messages and `info` always use the canonical name. On install.sh and Homebrew, `info` reports the canonical path, because the symlink resolves to it. On Windows, the copy is a separate file, so `info` reports its path.

## Adopting spine in a tool

1. Add `spine.json`, as above.

2. Require spine and register the dev CLI. This needs Go 1.24 or later.

   ```sh
   go get github.com/scoutbin/spine@<full commit SHA>
   go get -tool github.com/scoutbin/spine/cmd/spine
   ```

3. Add `main.go` at the module root, next to `spine.json`. The embed needs the file in the same directory.

   ```go
   package main

   import (
   	_ "embed"
   	"os"

   	"github.com/scoutbin/spine"
   )

   //go:embed spine.json
   var manifest []byte

   func main() {
   	os.Exit(spine.Run(spine.MustLoad(manifest), spine.OSEnv(), os.Args[1:]))
   }
   ```

   Pass the tool's own commands as `*cli.Command` values after the args. `spine.Run` returns 0 on success, 1 when `check` finds an update or a command returns a `*cli.ExitError`, and 2 on any other error. The names `version`, `info`, `check` and `upgrade` are reserved, and `Run` returns 2 if a tool command uses one of them.

   Tests call `spine.Run` with a `spine.Env` that has fake streams or lookups. Fields left nil use the real process.

4. Generate the installers and commit the output:

   ```sh
   go tool spine gen
   ```

   This writes `install.sh` and `install.ps1`, and `action.yml` when `channels.action` is true. `go tool spine gen --check` fails when the committed files are stale or when a disabled file is present. `go-ci.yml` runs this check.

5. Add the caller workflows. spine's reusable workflows run only when called, so the triggers belong to the caller. The examples below are a starting point. Pin each `uses:` to a full commit SHA, with the version in a comment, and keep it in step with the spine version in `go.mod`.

   `.github/workflows/ci.yml`:

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

   `.github/workflows/release.yml`:

   ```yaml
   name: Release
   on:
     workflow_dispatch:
       inputs:
         tag:
           description: Existing tag to build
           required: true
           type: string
         publish:
           description: Publish the release
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

6. `go tool spine change <bump> "<message>"` creates `CHANGELOG.md` at the repository root on the first release and writes each release's section. The release workflow publishes that section as the release notes.

Secrets, the switch that enables publishing, and the spine upgrade procedure are in [RELEASING.md](RELEASING.md).

## Reusable workflows

`go-ci.yml` (`workflow_call`, no inputs, no secrets) runs:

- a contract check against the spine version (`SPINE_CONTRACT`),
- tests on Ubuntu, macOS and Windows, with gofmt, `go vet` and `go test`,
- a dist build of the six archives and `checksums.txt`,
- a check that the generated files are up to date.

`node-ci.yml` (`workflow_call`, no inputs, no secrets) runs `prettier --check .` on the caller's repository. The Node version, pnpm version and Prettier version come from spine's `.node-version`, `package.json` and `pnpm-lock.yaml` at the pinned commit, so it needs no setup in the caller. Call it from a job with `permissions: contents: read`, the same way as `go-ci.yml`.

`go-release.yml` (`workflow_call`) takes the inputs `tag` (required) and `publish` (default `false`). The secret `homebrew-tap-deploy-key` is optional. It is used by the Homebrew job, which runs only for stable tags. Jobs:

- `resolve`: reads the manifest and the tag. When `publish` is true, it refuses a tag that already has a published release. It fails when `CHANGELOG.md` has no section for the tag, on dry runs too.
- `build`: runs the tests and builds the archives. It dry-runs the Homebrew package (stable only) and the npm package.
- `attest`: attests the release artifacts.
- `release`: creates a draft release with the tag's `CHANGELOG.md` section as its notes, uploads the assets and publishes it. Notes are set only when the release is created, so a re-run keeps the body of an existing release. It reuses an existing release whose assets are identical, and fails on missing or differing assets.
- `homebrew`: pushes the formula to the tap over SSH, using the deploy key. It runs only when `publish` is true, Homebrew is enabled in `spine.json` and the track is stable.
- `npm`: publishes the package with OIDC trusted publishing, then reconciles the npm dist-tags.

With `publish: false`, the publishing jobs do not run.

## Tags and tracks

Release tags are `vX.Y.Z` for stable, or `vX.Y.Z-alpha.N`, `vX.Y.Z-beta.N` or `vX.Y.Z-rc.N` with N of 1 or more. Other tags are rejected.

- stable: the GitHub latest release, the npm `latest` dist-tag, and the Homebrew formula.
- alpha, beta, rc: a GitHub prerelease that is not marked latest, the npm dist-tag with the track name, and no Homebrew formula.

The installers and the action pick the newest release on the track's upgrade path. A stable install takes stable only. An rc install takes rc or stable. A beta install takes beta, rc or stable. An alpha install takes any prerelease or stable.

## Installing and upgrading a tool

`install.sh` runs on macOS and Linux. In a Windows shell it stops and points to `install.ps1` or npm. `install.ps1` runs on Windows.

Both installers read these variables. `NAME` is the upper-cased tool name.

| Variable                 | Default                     | Purpose                                                                                              |
| ------------------------ | --------------------------- | ---------------------------------------------------------------------------------------------------- |
| `NAME_VERSION`           | newest release on the track | Install this tag. Takes precedence over `NAME_TRACK`                                                 |
| `NAME_TRACK`             | `stable`                    | `stable`, `alpha`, `beta` or `rc`                                                                    |
| `NAME_INSTALL_DIR`       | see above                   | Install directory                                                                                    |
| `NAME_NO_MODIFY_PATH`    | unset                       | `1` stops the installer from changing PATH                                                           |
| `NAME_DOWNLOAD_BASE_URL` | GitHub                      | `install.sh` only. Mirror for assets, starting with `https://` or `file://`. Requires `NAME_VERSION` |
| `NAME_GITHUB_TOKEN`      | none                        | Token for the release lookup. It is sent only to `api.github.com`, not with downloads                |

The installed binary has these built-in commands:

- `version` prints `<name> <version> (commit <c>, built <date>, <os>/<arch>)`. `--json` prints `version`, `commit`, `build_time`, `os` and `arch`. The root `--version` flag is also available.
- `info` prints the executable path, the channel (`install.sh`, `npm`, `homebrew` or `unknown`) and the version. `--json` uses the keys `path`, `channel` and `version`.
- `check` compares the build with the newest release on its track and exits 1 when an update exists. It reads `GITHUB_TOKEN`, without the tool prefix. `--json` uses the keys `current`, `latest`, `update_available` and `track`.
- `upgrade` runs only on darwin and linux. On an `install.sh` install it replaces the binary in place after the checksum passes, and `--version <tag>` pins a tag. On npm and Homebrew installs it prints the command to run, such as `npm install -g @scoutbin/mytool@latest` or `brew upgrade scoutbin/tap/mytool`. The npm command uses the build's dist-tag (`@latest` for stable, `@rc` for rc builds). On Windows, installs from `install.ps1` report the channel `unknown` and `upgrade` errors, so update by running `install.ps1` again.

Global flags are `--json` and `--no-color`. `--no-color` is also set by `NO_COLOR`. Exit codes are 0 for success, 1 when `check` finds an update, and 2 for any other error.

The GitHub Action runs the installer for the tool. It is defined in the tool repository's `action.yml`, so it is used as `uses: <owner>/<repo>@<ref>`. Its inputs are `version` (takes precedence over `track`), `track` (default `stable`), `install-dir` and `token`. Its outputs are `version` and `path`. The default install directory is `$RUNNER_TEMP/<name>/bin` on Linux and macOS, and `RUNNER_TEMP\<name>\bin` on Windows.

```yaml
- uses: <owner>/<repo>@<ref>
  with:
    version: v1.2.3
```

## Development

- Go 1.24 or later, standard library only. `go.mod` has no `require` lines. Keep it that way.
- Before committing, run `gofmt -l .` (it must print nothing), `go vet ./...` and `go test ./...`.
- Tests are black-box, in external `_test` packages under `test/`.
- An incompatible change to the dev CLI output or the `spine.json` schema needs a new `Contract` in `internal/devcmd`, and the reusable workflows must be updated to the same value.
- `ci.yml` runs on pull requests, on pushes to `main` and from `workflow_dispatch`. It covers the Go tests, actionlint, the rendered-file checks, and the installer, Homebrew, npm, pnpm and action suites against `internal/example`.
