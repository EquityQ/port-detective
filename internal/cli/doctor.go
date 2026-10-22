package cli

import (
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/EquityQ/port-detective/internal/version"
	"github.com/spf13/cobra"
)

type doctorReport struct {
	Version            string               `json:"version"`
	OS                 string               `json:"os"`
	Arch               string               `json:"arch"`
	BinaryPath         string               `json:"binary_path"`
	Cwd                string               `json:"cwd"`
	ProjectPath        string               `json:"project_path,omitempty"`
	Status             string               `json:"status"`
	SelfCheck          *core.PortDiagnostic `json:"self_check,omitempty"`
	MetadataAccessible bool                 `json:"metadata_accessible"`
	Hints              []string             `json:"hints"`
}

func newDoctorCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose binary, project path, and local listener visibility",
		Long:  "Show runtime details and inspect a temporary local TCP listener. Permission and namespace limitations are reported with next steps. This checks listener visibility, not application protocol readiness.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := doctorReport{Version: version.Value, OS: runtime.GOOS, Arch: runtime.GOARCH, Status: "ok", Hints: []string{}}
			var err error
			report.BinaryPath, err = os.Executable()
			if err != nil {
				report.Hints = append(report.Hints, "Cannot resolve the running executable: "+err.Error())
			}
			report.Cwd, err = os.Getwd()
			if err != nil {
				report.Hints = append(report.Hints, "Cannot read the working directory: "+err.Error())
			}
			report.ProjectPath, err = core.ResolveProjectPath(report.Cwd)
			if err != nil {
				report.Hints = append(report.Hints, "Select a valid project using clean --project <directory>: "+err.Error())
			}
			listener, listenErr := net.Listen("tcp4", "127.0.0.1:0")
			if listenErr != nil {
				report.Hints = append(report.Hints, "Unable to create a temporary loopback listener; check local firewall or sandbox settings: "+listenErr.Error())
			} else {
				defer listener.Close()
				port := listener.Addr().(*net.TCPAddr).Port
				report.SelfCheck, err = core.InspectPort(cmd.Context(), port)
				if cmd.Context().Err() != nil {
					return cmd.Context().Err()
				}
				if err != nil {
					report.Hints = append(report.Hints, "Temporary listener inspection failed: "+err.Error())
				} else if report.SelfCheck == nil || !report.SelfCheck.IsOpen {
					report.Hints = append(report.Hints, "A local listener was not detected. Run the CLI in the same host and network namespace as the application; check WSL, containers, or sandbox restrictions.")
				} else {
					report.MetadataAccessible = report.SelfCheck.PID == int32(os.Getpid()) && report.SelfCheck.Status == core.StatusOccupied
					if !report.MetadataAccessible {
						report.Hints = append(report.Hints, "TCP visibility works, but process ownership is restricted. Inspect the target with check <port> --json; consider Administrator/root only when you understand the target and need its metadata.")
					}
				}
			}
			if report.SelfCheck != nil {
				report.SelfCheck.Cmdline = ""
				report.SelfCheck.Project = nil
			}
			if len(report.Hints) > 0 {
				report.Status = "degraded"
			}
			if opts.jsonOutput {
				return writeJSON(stdout(cmd), report)
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
			fmt.Fprintln(out, "Self-check confirms local listener visibility; it does not test application readiness.")
			return nil
		},
	}
}
