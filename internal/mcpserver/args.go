package mcpserver

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/mark3labs/mcp-go/mcp"
)

func requirePort(request mcp.CallToolRequest) (int, error) {
	value := request.GetArguments()["port"]
	var port float64
	switch v := value.(type) {
	case int:
		port = float64(v)
	case float64:
		port = v
	default:
		return 0, fmt.Errorf("port is required and must be an integer number from 1 to 65535")
	}
	if math.IsNaN(port) || port < 1 || port > 65535 || math.Trunc(port) != port {
		return 0, fmt.Errorf("port must be an integer number from 1 to 65535")
	}
	return int(port), nil
}

func optionalInt(request mcp.CallToolRequest, name string, defaultVal, minVal, maxVal int) (int, error) {
	value, found := request.GetArguments()[name]
	if !found {
		return defaultVal, nil
	}
	var num float64
	switch v := value.(type) {
	case int:
		num = float64(v)
	case float64:
		num = v
	default:
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minVal, maxVal)
	}
	if math.IsNaN(num) || math.IsInf(num, 0) || math.Trunc(num) != num || num < float64(minVal) || num > float64(maxVal) {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minVal, maxVal)
	}
	return int(num), nil
}

func optionalString(request mcp.CallToolRequest, name string) (string, error) {
	value, found := request.GetArguments()[name]
	if !found {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return text, nil
}

// optionalStringPresent distinguishes "argument omitted" from "argument supplied
// but empty". Destructive scope decisions must never treat those as equivalent:
// an explicitly empty selection is rejected instead of widening to everything.
func optionalStringPresent(request mcp.CallToolRequest, name string) (bool, string, error) {
	value, found := request.GetArguments()[name]
	if !found {
		return false, "", nil
	}
	text, ok := value.(string)
	if !ok {
		return true, "", fmt.Errorf("%s must be a string", name)
	}
	return true, text, nil
}

func optionalDryRun(request mcp.CallToolRequest) (bool, error) {
	value, found := request.GetArguments()["dry_run"]
	if !found {
		return true, nil
	}
	dryRun, ok := value.(bool)
	if !ok {
		return true, fmt.Errorf("dry_run must be a boolean; omit it for a safe preview")
	}
	return dryRun, nil
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorf("result serialization failed: %v", err), nil
	}
	return mcp.NewToolResultText(string(payload)), nil
}
