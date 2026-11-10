# AGENTS.md

spine is the shared core for scoutbin tools: a Go module (`github.com/scoutbin/spine`) and reusable GitHub workflows.

## Layout

- The root package, `cli/` and `term/` are public. Tools import only these directly.
- `internal/` holds everything else, including `internal/devcmd`, `internal/manifest` and `internal/release`.
- `cmd/spine` is a dev CLI that tools run as `go tool spine`. It is not shipped to end users.
- `test/<pkg>` holds black-box tests in external `_test` packages. Shared fixtures live in `test/testutil`.

## Go

- Go 1.24, standard library only. `go.mod` has no `require` lines. Keep it that way.
- Classify release tags with `release.ValidatePushedTag`, never `TrackOf`.
- Before handing work back, run `gofmt`, `go vet` and `go test ./...`.
- Bump `Contract` in `internal/devcmd` on any incompatible change to the dev CLI output or the `spine.json` schema. The reusable workflows hardcode the value they expect.

## Repo rules

- The scoutbin org is on GitHub's free plan. Do not rely on paid features.
- Make minimal changes.
- Own the code you touch. Fix nearby defects, unclear names and missing tests you notice, keep the change focused on the task, and call out larger improvements instead of silently widening scope.
- Confirm before any action with side effects, such as pushing, tagging, publishing or editing repo settings.
- Keep commit messages short and imperative. No ticket prefix, no AI or co-author trailers.
- Write tests table-driven, using `testcases` arrays with no duplicated structure.
- No no-op additions: do not add comments, tests or source that add no value. No stateful comments, meaning comments that describe history, plans, tasks or work in progress, or that refer to plan files. Comments explain why the code is the way it is, timelessly.
- `.claude/` is gitignored. Planning files live there.
