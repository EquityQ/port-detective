package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	var ports []int
	switch preset {
	case "web":
		ports = []int{3000, 3001, 4173, 5173, 8000, 8080}
	case "python":
		ports = []int{5000, 5001, 8000, 8080, 8888}
	case "database":
		ports = []int{3306, 5432, 6379, 27017}
	case "project":
		root, err := core.ResolveProjectPath(projectPath)
		if err != nil {
			return nil, "", usageError(err)
		}
		ports, err = readProjectPorts(root)
		return ports, filepath.Join(root, ".port-detective.json"), err
	default:
		return nil, "", usageError(fmt.Errorf("--preset must be web, python, database, or project"))
	}
	return ports, "preset " + preset, nil
}

func readProjectPorts(root string) ([]int, error) {
	path := filepath.Join(root, ".port-detective.json")
	file, err := os.Open(path)
	if err != nil {
		return nil, usageError(fmt.Errorf("read project preset %s: %w; expected {\"ports\":\"3000-3005,8081\"}", path, err))
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || info.Size() > 65536 {
		return nil, usageError(fmt.Errorf("project preset %s cannot be read or exceeds 64 KiB", path))
	}
	var config struct {
		Ports string `json:"ports"`
	}
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, usageError(fmt.Errorf("invalid project preset %s: %w", path, err))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, usageError(fmt.Errorf("project preset %s must contain one JSON object", path))
	}
	ports, err := parsePortsList(config.Ports)
	if err != nil {
		return nil, usageError(fmt.Errorf("invalid ports in %s: %w", path, err))
	}
	return ports, nil
}
