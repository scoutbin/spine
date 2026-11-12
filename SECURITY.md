# Security Policy

## Supported versions

spine is not tagged or released. Only the latest commit on `main` receives security fixes. Tools pin an exact commit, so a fix reaches a tool only after the tool updates its pinned commit.

## Reporting a vulnerability

Report security vulnerabilities privately through GitHub private vulnerability reporting. In the spine repository, open the Security tab and select "Report a vulnerability".

Do not report security issues in public issues, discussions or pull requests.

## What to include

- The spine commit you use. Take it from the tool's `go.mod`, or run `git rev-parse HEAD` in a spine clone.
- The tool that embeds spine, its version, and the Go version (`go version`).
- The impact: what an attacker can do and what conditions they need.
- Steps to reproduce, with a proof of concept if you have one.
- Remove secrets, tokens and private data from any logs or files you attach.

## What happens next

1. We acknowledge your report. We aim to do this promptly.
2. We investigate and confirm whether it is a vulnerability in spine. We may ask for more detail. If it is not, we tell you why.
3. We prepare a fix. Where we can, we agree a disclosure date with you.
4. We fix the issue on `main` and publish a GitHub security advisory that names the fixed commit.
5. If you want, we credit you in the advisory.

Please keep the issue private until a fix lands on `main` or we agree on a disclosure date. We aim to keep you informed of progress.

## Scope

In scope:

- The spine Go module and the dev CLI in `cmd/spine`.
- The installer and action files that spine generates into tool repositories.
- The reusable workflows in `.github/workflows/`: `go-ci.yml` and `go-release.yml`.

Out of scope:

- Tools built on spine, such as pin, scout and digest. Report issues in a tool's own code, or in its release artifacts, to that tool's repository.
- Third-party dependencies. Report them to the upstream project.
- Vulnerabilities in GitHub, npm, Homebrew or other platforms. Report them to the platform.

## Pull requests

This project does not accept pull requests, including security fixes. Report the issue as described above, and the maintainers will write the fix.
