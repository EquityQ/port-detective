package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/EquityQ/port-detective/internal/updater"
	"github.com/EquityQ/port-detective/internal/version"
	"github.com/spf13/cobra"
)

var (
	updaterHookMu    sync.RWMutex
	newUpdaterForCLI = func(timeout time.Duration) *updater.Updater {
		return updater.New(timeout)
	}
	currentExecutablePath = os.Executable
)

func setUpdaterHooksForTesting(
	factory func(time.Duration) *updater.Updater,
	execFn func() (string, error),
) func() {
	updaterHookMu.Lock()
	prevFactory := newUpdaterForCLI
	prevExec := currentExecutablePath
	if factory != nil {
		newUpdaterForCLI = factory
	}
	if execFn != nil {
		currentExecutablePath = execFn
	}
	updaterHookMu.Unlock()
	return func() {
		updaterHookMu.Lock()
		newUpdaterForCLI = prevFactory
		currentExecutablePath = prevExec
		updaterHookMu.Unlock()
	}
}

func getUpdaterHooks() (func(time.Duration) *updater.Updater, func() (string, error)) {
	updaterHookMu.RLock()
	defer updaterHookMu.RUnlock()
	return newUpdaterForCLI, currentExecutablePath
}

// newUpdateCommand checks for or installs a SHA-256-verified release binary from GitHub Releases.
func newUpdateCommand(opts *options) *cobra.Command {
	var (
		checkOnly bool
		dryRun    bool
		yes       bool
		force     bool
		toVersion string
		timeout   time.Duration
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for or install a newer port-detective release",
		Long: strings.Join([]string{
			"Check GitHub Releases for a newer port-detective CLI binary and optionally",
			"download, verify (SHA-256), and install it in place.",
			"",
			"Use --check or --dry-run to inspect availability without modifying anything.",
			"Installations managed by npm or the Python SDK cache are detected and protected",
			"unless --force is supplied.",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective update --check",
			"  port-detective update --check --json",
			"  port-detective update",
			"  port-detective update -y",
			"  port-detective update --to-version 1.5.0 -y",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeout <= 0 {
				return usageError(fmt.Errorf("--timeout must be positive"))
			}
			var normalizedTarget string
			if strings.TrimSpace(toVersion) != "" {
				norm, err := updater.NormalizeTargetVersion(toVersion)
				if err != nil {
					return usageError(err)
				}
				normalizedTarget = norm
			}

			factory, execFn := getUpdaterHooks()
			execPath, _ := execFn()
			u := factory(timeout)

			updOpts := updater.Options{
				CurrentVersion: version.Value,
				TargetVersion:  normalizedTarget,
				ExecutablePath: execPath,
				Force:          force,
			}

			out := stdout(cmd)
			p := newPalette(out)

			if checkOnly || dryRun {
				res, err := u.Check(cmd.Context(), updOpts)
				if err != nil {
					return err
				}
				if opts.jsonOutput {
					return writeJSON(out, res)
				}
				renderUpdateCheck(out, p, res)
				return nil
			}

			if opts.jsonOutput && !yes {
				return commandFailure("confirmation_required", 2, "update --json requires --yes (or pass --check / --dry-run to preview)")
			}

			preview, err := u.Check(cmd.Context(), updOpts)
			if err != nil {
				return err
			}
			if preview.ManagedBy != "" && !force {
				return commandFailure("managed_install", 1, "CLI is managed by %s; upgrade with `%s` (or pass --force to overwrite)", preview.ManagedBy, preview.UpgradeHint)
			}
			if !preview.UpdateAvailable && !force {
				if opts.jsonOutput {
					return writeJSON(out, preview)
				}
				renderUpdateCheck(out, p, preview)
				return nil
			}

			if !yes {
				fmt.Fprintf(out, "  Update port-detective %s -> %s (%s)? %s ",
					preview.CurrentVersion, preview.LatestVersion, preview.ExecutablePath, p.paint(p.dim, "[y/N]:"))
				confirmed, err := confirm(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintf(out, "  %s\n", p.paint(p.dim, "Update canceled."))
					return nil
				}
			}

			res, err := u.Apply(cmd.Context(), updOpts)
			if err != nil {
				if errors.Is(err, updater.ErrManagedInstall) {
					return commandFailure("managed_install", 1, "%s", err)
				}
				return err
			}
			if opts.jsonOutput {
				return writeJSON(out, res)
			}
			renderUpdateApplied(out, p, res)
			return nil
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, "check for updates without downloading or modifying files")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview available update without modifying the binary")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip interactive confirmation")
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if already up to date or bypass managed-install check")
	cmd.Flags().StringVar(&toVersion, "to-version", "", "install a specific release version (e.g. 1.5.0)")
	cmd.Flags().DurationVar(&timeout, "timeout", updater.DefaultTimeout, "maximum time to wait for release metadata and downloads")
	return cmd
}

func renderUpdateCheck(out io.Writer, p palette, res *updater.CheckResult) {
	sectionTitle(out, p, "Port Detective Update Check", 64)
	field(out, p, "Current", keyWidth, res.CurrentVersion)
	field(out, p, "Latest", keyWidth, res.LatestVersion)
	statusLabel := "Up to date"
	if res.UpdateAvailable {
		statusLabel = p.paint(p.accent, "Update available")
	}
	field(out, p, "Status", keyWidth, statusLabel)
	if res.AssetName != "" {
		field(out, p, "Asset", keyWidth, res.AssetName)
	}
	if res.ExecutablePath != "" {
		field(out, p, "Binary", keyWidth, res.ExecutablePath)
	}
	if res.ReleaseURL != "" {
		field(out, p, "Release", keyWidth, res.ReleaseURL)
	}
	if res.ManagedBy != "" {
		hint(out, p, fmt.Sprintf("Managed by %s; upgrade via: %s", res.ManagedBy, res.UpgradeHint))
	} else if res.UpdateAvailable {
		fmt.Fprintf(out, "  Run %s to install.\n", p.paint(p.accent, "port-detective update"))
	}
}

func renderUpdateApplied(out io.Writer, p palette, res *updater.CheckResult) {
	sectionTitle(out, p, "Port Detective Updated", 64)
	field(out, p, "Current", keyWidth, res.CurrentVersion)
	field(out, p, "Updated", keyWidth, p.paint(p.accent, res.LatestVersion))
	if res.ExecutablePath != "" {
		field(out, p, "Binary", keyWidth, res.ExecutablePath)
	}
	if res.ReleaseURL != "" {
		field(out, p, "Release", keyWidth, res.ReleaseURL)
	}
	if runtime.GOOS == "windows" && res.ExecutablePath != "" {
		if _, err := os.Stat(res.ExecutablePath + ".old"); err == nil {
			hint(out, p, fmt.Sprintf("Previous executable backup remains at %s.old and will be removed on the next update.", res.ExecutablePath))
		}
	}
	fmt.Fprintf(out, "  %s\n", res.Message)
}
