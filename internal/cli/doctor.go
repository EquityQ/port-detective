package cli

import (
	"fmt"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/EquityQ/port-detective/internal/version"
	"github.com/spf13/cobra"
)

type doctorReport = core.DoctorReport

func newDoctorCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose binary, project path, and local listener visibility",
		Long:  "Show runtime details and inspect a temporary local TCP listener. Permission and namespace limitations are reported with next steps. This checks listener visibility, not application protocol readiness.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := opts.inspector.RunDoctor(cmd.Context(), "", "", version.Value)
			if err != nil {
				return err
			}
			if opts.jsonOutput {
				// Keep the exit status honest: a degraded self-check must not report
				// success to automation that only inspects the exit code.
				if err := writeJSON(stdout(cmd), report); err != nil {
					return err
				}
				if report.Status != "ok" {
					return commandFailure("doctor_degraded", 5, "doctor self-check degraded: see hints in the JSON report")
				}
				return nil
			}
			out := stdout(cmd)
			p := newPalette(out)
			sectionTitle(out, p, "Port Detective Doctor", 80)
			field(out, p, "Version", keyWidth, report.Version)
			field(out, p, "Platform", keyWidth, report.OS+"/"+report.Arch)
			field(out, p, "Binary", keyWidth, report.BinaryPath)
			field(out, p, "Directory", keyWidth, report.Cwd)
			field(out, p, "Project", keyWidth, report.ProjectPath)
			field(out, p, "Status", keyWidth, report.Status)
			field(out, p, "Metadata", keyWidth, fmt.Sprintf("self-process accessible: %v", report.MetadataAccessible))
			for _, message := range report.Hints {
				hint(out, p, message)
			}
			// Only a fully successful self-check earns an unqualified conclusion.
			if report.Status == "ok" {
				fmt.Fprintln(out, "Self-check confirms local listener visibility; it does not test application readiness.")
				return nil
			}
			fmt.Fprintf(out, "  %s\n", p.paint(p.warn, "Self-check did not fully pass; the limitations above apply to every result on this host."))
			return commandFailure("doctor_degraded", 5, "doctor self-check degraded; see the hints above")
		},
	}
}
