package selfcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/scoutbin/spine/cli"
	"github.com/scoutbin/spine/internal/channel"
	"github.com/scoutbin/spine/internal/manifest"
	"github.com/scoutbin/spine/internal/release"
	"github.com/scoutbin/spine/term"
)

const (
	downloadTimeout   = 5 * time.Minute
	maxChecksumsBytes = 1 << 20
	maxArchiveBytes   = 200 << 20
)

type upgradeAction int

const (
	printInstruction upgradeAction = iota + 1
	upgradeInPlace
)

// upgradePlan is the outcome of routing an upgrade. For printInstruction, text
// is written to stdout. For upgradeInPlace, target is the normalized tag, or
// empty for the latest release.
type upgradePlan struct {
	action upgradeAction
	text   string
	target string
}

// planUpgrade decides what `<tool> upgrade` does for a channel and an
// optional --version value. A returned error means exit code 2.
func (a *App) planUpgrade(ch channel.Channel, versionFlag string) (upgradePlan, error) {
	reinstallHint := "reinstall " + a.Tool.Name + " with install.sh"

	if ch == channel.InstallSh {
		target, err := normalizeTag(versionFlag)
		if err != nil {
			return upgradePlan{}, err
		}
		return upgradePlan{action: upgradeInPlace, target: target}, nil
	}

	command, managed := channel.UpgradeCommand(a.Tool, ch, a.buildTrack().DistTag())
	if versionFlag != "" {
		if !managed {
			return upgradePlan{}, fmt.Errorf("--version only works for install.sh installs, but this install is of unknown origin; %s", reinstallHint)
		}
		return upgradePlan{}, fmt.Errorf("--version only works for install.sh installs, but this install is managed by %s (to upgrade, run: %s)", ch, command)
	}
	if managed {
		return upgradePlan{action: printInstruction, text: fmt.Sprintf("%s is managed by %s. To upgrade, run:\n  %s\n", a.Tool.Name, ch, command)}, nil
	}
	return upgradePlan{}, fmt.Errorf("cannot upgrade: install channel is unknown; %s", reinstallHint)
}

// normalizeTag returns tag with a leading "v", or "" when tag is empty. The tag
// must be one a person can push for a release.
func normalizeTag(tag string) (string, error) {
	if tag == "" {
		return "", nil
	}
	normalized := "v" + strings.TrimPrefix(tag, "v")
	if _, err := release.ValidatePushedTag(normalized); err != nil {
		return "", &cli.UsageError{Kind: cli.InvalidValue, Msg: fmt.Sprintf("invalid --version %q: want a release tag such as v0.2.0", tag)}
	}
	return normalized, nil
}

func (a *App) upgrade(ctx *cli.Context) error {
	got, err := a.installChannel()
	if err != nil {
		return err
	}
	plan, err := a.planUpgrade(got.Channel, ctx.String("version"))
	if err != nil {
		return err
	}
	if plan.action == printInstruction {
		_, err = fmt.Fprint(a.Stdout, plan.text)
		return err
	}
	return a.upgradeInPlace(ctx, got.Path, plan.target)
}

// upgradeInPlace replaces the executable at path with the release tagged
// target (latest when empty). The checksum is verified before the installed
// binary is touched.
func (a *App) upgradeInPlace(ctx *cli.Context, path, target string) error {
	if a.Build.OS != "darwin" && a.Build.OS != "linux" {
		return fmt.Errorf("self-upgrade is not supported on %s", a.Build.OS)
	}
	client := a.client()
	reporter := a.reporter(ctx)

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	latest := target == ""
	if latest {
		task := reporter.Spin("Resolving latest version")
		tag, err := a.latestRelease(runCtx, client)
		if err != nil {
			task.Fail("Could not resolve the latest version")
			return fmt.Errorf("look up latest release: %w", err)
		}
		task.Done("Latest release is " + tag)
		target = tag
	}

	order := release.Compare(a.Build.Version, target)
	if order == 0 || (latest && order > 0) {
		_, err := fmt.Fprintf(a.Stdout, "Already up to date (%s)\n", a.Build.Version)
		return err
	}

	archiveName := a.Tool.ArchiveName(target, a.Build.OS, a.Build.Arch)
	checksums, err := download(runCtx, client, reporter, target, manifest.ChecksumsFile, maxChecksumsBytes)
	if err != nil {
		return err
	}
	archive, err := download(runCtx, client, reporter, target, archiveName, maxArchiveBytes)
	if err != nil {
		return err
	}

	task := reporter.Spin("Verifying checksum")
	if err := release.VerifyChecksum(checksums, archiveName, bytes.NewReader(archive)); err != nil {
		task.Fail("Checksum verification failed")
		return err
	}
	task.Done("Checksum verified")

	task = reporter.Spin("Installing " + target)
	binary, err := release.ExtractBinary(archive, a.Tool.BinaryName(a.Build.OS))
	if err == nil {
		err = release.ReplaceBinary(a.Tool, path, binary)
	}
	if err != nil {
		task.Fail("Could not install " + target)
		return err
	}
	task.Done("Installed " + path)

	verb := "Upgraded"
	if order > 0 {
		verb = "Downgraded"
	}
	_, err = fmt.Fprintf(a.Stdout, "%s %s %s → %s\n", verb, a.Tool.Name, a.Build.Version, target)
	return err
}

// download reads a release asset into memory, showing a progress bar.
func download(ctx context.Context, client release.Client, reporter term.Reporter, tag, asset string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	body, size, err := client.OpenAsset(ctx, tag, asset)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	task := reporter.Bar("Downloading "+asset, size)
	data, err := io.ReadAll(io.LimitReader(&progressReader{r: body, update: task.Update}, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = fmt.Errorf("%s exceeds %d MB", asset, limit>>20)
	}
	if err != nil {
		task.Fail("Could not download " + asset)
		return nil, fmt.Errorf("download %s: %w", asset, err)
	}
	task.Done("Downloaded " + asset)
	return data, nil
}

// progressReader reports the running byte count to update on every read.
type progressReader struct {
	r      io.Reader
	done   int64
	update func(done int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	p.update(p.done)
	return n, err
}
