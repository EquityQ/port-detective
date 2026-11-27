package cli

import (
	"fmt"
	"strings"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/spf13/cobra"
)

func selectScanPorts(cmd *cobra.Command, raw, preset, projectPath string) ([]int, string, error) {
	portsChanged, presetChanged := cmd.Flags().Changed("ports"), cmd.Flags().Changed("preset")
	if portsChanged && presetChanged {
		return nil, "", usageError(fmt.Errorf("--ports and --preset cannot be used together"))
	}
	if cmd.Flags().Changed("project") && preset != "project" {
		return nil, "", usageError(fmt.Errorf("scan --project requires --preset project"))
	}
	if cmd.Flags().Changed("project") && strings.TrimSpace(projectPath) == "" {
		return nil, "", usageError(fmt.Errorf("--project must name a directory when provided"))
	}
	if portsChanged {
		ports, err := parsePortsList(raw)
		return ports, "selected ports", err
	}
	if !presetChanged {
		return core.DefaultDevPorts, "default development ports", nil
	}
	ports, scope, err := core.ResolvePresetPorts(preset, projectPath)
	if err != nil {
		return nil, "", usageError(err)
	}
	return ports, scope, nil
}

func readProjectPorts(root string) ([]int, error) {
	ports, err := core.ReadProjectPresetPorts(root)
	if err != nil {
		return nil, usageError(err)
	}
	return ports, nil
}
