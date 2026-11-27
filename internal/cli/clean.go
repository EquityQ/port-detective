package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/spf13/cobra"
)

type cleanEnvelope struct {
	Cwd                  string                `json:"cwd"`
	Scope                string                `json:"scope"`
	DryRun               bool                  `json:"dry_run"`
	Count                int                   `json:"count"`
	Cleaned              []core.PortDiagnostic `json:"cleaned"`
	PreviewID            string                `json:"preview_id,omitempty"`
	PreviewExpiresAtUnix int64                 `json:"preview_expires_at_unix,omitempty"`
	Message              string                `json:"message,omitempty"`
	Error                string                `json:"error,omitempty"`
}

// newCleanCommand processes listeners associated with the selected project.
func newCleanCommand(opts *options) *cobra.Command {
	var (
		dryRun      bool
		yes         bool
		envelope    bool
		projectPath string
		portsStr    string
		previewID   string
	)

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean project-scoped development port processes",
		Long: strings.Join([]string{
			"Discover local TCP listeners belonging to the selected project, show targets, then confirm termination.",
			"The project defaults to the nearest manifest above the current directory. Use --ports to limit discovery.",
			"Use --json --envelope to wrap results with cwd, scope, count, cleaned, and dry-run preview_id metadata.",
		}, "\n"),
		Example: strings.Join([]string{
			"  port-detective clean",
			"  port-detective clean --dry-run",
			"  port-detective clean -y",
			"  port-detective clean --project ../my-app --ports 3000-3010 --dry-run",
			"  port-detective clean --json --dry-run",
			"  port-detective clean --json --envelope --dry-run",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if envelope && !opts.jsonOutput {
				return usageError(fmt.Errorf("--envelope requires --json"))
			}
			out := stdout(cmd)
			p := newPalette(out)
			if cmd.Flags().Changed("project") && strings.TrimSpace(projectPath) == "" {
				return usageError(fmt.Errorf("--project must name a directory when provided"))
			}
			cwd, err := core.ResolveProjectPath(projectPath)
			if err != nil {
				return usageError(err)
			}
			var ports []int
			scope := "all local TCP listeners"
			if cmd.Flags().Changed("ports") {
				ports, err = parsePortsList(portsStr)
				if err != nil {
					return err
				}
				// An explicitly empty --ports must not widen into a full listener sweep.
				if len(ports) == 0 {
					return usageError(fmt.Errorf("--ports was provided but selected no ports; omit the flag to scan all local listeners"))
				}
				scope = fmt.Sprintf("%d selected TCP port(s): %v", len(ports), ports)
			}

			if !dryRun && opts.jsonOutput && !yes {
				return commandFailure("confirmation_required", 2, "refusing destructive JSON operation without explicit --yes; use --dry-run to preview")
			}

			fromPreview := strings.TrimSpace(previewID) != ""
			var targets []core.PortDiagnostic
			if fromPreview {
				// Validate a supplied preview token before any live discovery so
				// invalid tokens are always rejected and confirmation runs once.
				targets, _, err = core.RestoreCLIPreviewToken(core.PreviewScopeClean, previewID, ports)
				if err != nil {
					return commandFailure("preview_rejected", 6, "preview id rejected for project cleanup: %v", err)
				}
				scope = fmt.Sprintf("%d previewed TCP port(s)", len(targets))
			} else {
				targets, err = opts.inspector.DiscoverProjectPorts(cmd.Context(), cwd, ports)
				if err != nil {
					return err
				}
			}

			if !opts.jsonOutput {
				sectionTitle(out, p, "Project Ports Clean: Target Preview", 80)
				field(out, p, "Project", keyWidth, cwd)
				field(out, p, "Scope", keyWidth, scope)
				if len(targets) > 0 {
					fmt.Fprintln(out)
					renderPortTable(out, p, targets)
					degradedNote(out, p, targets)
				} else {
					fmt.Fprintln(out, "  No project-scoped listeners found in this scope.")
				}
			}

			if len(targets) == 0 {
				if opts.jsonOutput {
					if envelope {
						return writeJSON(out, cleanEnvelope{
							Cwd:     cwd,
							Scope:   scope,
							DryRun:  dryRun,
							Count:   0,
							Cleaned: []core.PortDiagnostic{},
							Message: "no project-scoped listeners found in this scope; nothing to preview",
						})
					}
					return writeJSON(out, []core.PortDiagnostic{})
				}
				return nil
			}

			if !dryRun && !yes && !opts.jsonOutput {
				promptMsg := fmt.Sprintf("Terminate project listeners in %s?", cwd)
				if fromPreview {
					promptMsg = fmt.Sprintf("Terminate %d previewed project listener(s) in %s?", len(targets), cwd)
				}
				fmt.Fprintf(out, "  %s %s ", promptMsg, p.paint(p.dim, "[y/N]:"))
				confirmed, err := confirm(cmd.Context(), cmd.InOrStdin())
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintf(out, "  %s\n", p.paint(p.dim, "Cancelled. No processes were touched."))
					return nil
				}
			}

			// Execute exactly the previewed snapshot; the core revalidates every target.
			cleaned, cleanErr := opts.inspector.CleanProjectTargets(cmd.Context(), cwd, targets, dryRun)
			return renderCleanResult(out, p, opts.jsonOutput, envelope, cwd, scope, cleaned, dryRun, fromPreview, cleanErr)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without terminating processes")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip interactive confirmation")
	cmd.Flags().BoolVar(&envelope, "envelope", false, "wrap --json output in an object with cwd, scope, count, cleaned, and preview_id")
	cmd.Flags().StringVar(&projectPath, "project", "", "project directory (default: nearest project above current directory)")
	cmd.Flags().StringVar(&portsStr, "ports", "", "limit discovery to comma-separated ports or ranges, e.g. 3000-3010,8080")
	cmd.Flags().StringVar(&previewID, "preview-id", "", "token from a preview; binds the action to the previewed targets")
	return cmd
}

func renderCleanResult(out io.Writer, p palette, jsonOutput, envelope bool, cwd, scope string, cleaned []core.PortDiagnostic, dryRun, fromPreview bool, cleanErr error) error {
	if jsonOutput {
		if !envelope {
			if err := writeJSON(out, cleaned); err != nil {
				return err
			}
			return cleanErr
		}
		if cleaned == nil {
			cleaned = []core.PortDiagnostic{}
		}
		payload := cleanEnvelope{
			Cwd:     cwd,
			Scope:   scope,
			DryRun:  dryRun,
			Count:   len(cleaned),
			Cleaned: cleaned,
		}
		if dryRun && len(cleaned) > 0 && cleanErr == nil {
			if token, expiresAt, tokenErr := core.IssueCLIPreviewToken(core.PreviewScopeClean, cleaned, core.DefaultPreviewTTL); tokenErr == nil {
				payload.PreviewID = token
				payload.PreviewExpiresAtUnix = expiresAt
				payload.Message = fmt.Sprintf(
					"dry run: would terminate %d project listener(s); pass preview_id with --yes --preview-id within %s to execute",
					len(cleaned), core.DefaultPreviewTTL)
			}
		} else if !dryRun && cleanErr == nil {
			payload.Message = fmt.Sprintf("processed %d project listener(s)", len(cleaned))
		}
		if cleanErr != nil {
			payload.Error = cleanErr.Error()
		}
		if err := writeJSON(out, payload); err != nil {
			return err
		}
		return cleanErr
	}

	title := "Project Ports Clean"
	if dryRun {
		title += " (Dry Run)"
	}
	sectionTitle(out, p, title, 80)

	if len(cleaned) == 0 {
		fmt.Fprintf(out, "  %s\n\n", p.paint(p.dim, "No project-scoped listeners found for the current project."))
		fmt.Fprintf(out, "%s\n", p.paint(p.dim, rule(80)))
		return cleanErr
	}

	fmt.Fprintf(out, "\n")
	renderPortTable(out, p, cleaned)
	fmt.Fprintf(out, "\n")

	summaryLabel := "port(s)"
	if fromPreview {
		summaryLabel = "previewed port(s)"
	}
	summaryLine(out, p, fmt.Sprintf("Processed %d %s for current project (dry-run: %v)", len(cleaned), summaryLabel, dryRun))
	if dryRun && cleanErr == nil {
		if token, _, tokenErr := core.IssueCLIPreviewToken(core.PreviewScopeClean, cleaned, core.DefaultPreviewTTL); tokenErr == nil {
			field(out, p, "Preview ID", 10, token)
		}
	}
	fmt.Fprintf(out, "%s\n", p.paint(p.dim, rule(80)))
	return cleanErr
}
