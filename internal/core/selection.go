package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ParsePortsList parses comma-separated ports and inclusive ranges into a sorted set.
func ParsePortsList(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return []int{}, nil
	}
	var ports []int
	for _, item := range strings.Split(raw, ",") {
		parts := strings.Split(strings.TrimSpace(item), "-")
		if len(parts) > 2 {
			return nil, fmt.Errorf("invalid port range %q", item)
		}
		start, err := parsePortNumber(parts[0])
		if err != nil {
			return nil, err
		}
		end := start
		if len(parts) == 2 {
			end, err = parsePortNumber(parts[1])
			if err != nil {
				return nil, err
			}
			if end < start {
				return nil, fmt.Errorf("invalid descending port range %q", item)
			}
		}
		for port := start; port <= end; port++ {
			ports = append(ports, port)
		}
	}
	return normalizePorts(ports)
}

func parsePortNumber(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.Trim(value, "0123456789") != "" {
		return 0, fmt.Errorf("invalid port %q: expected an integer from 1 to 65535", raw)
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", raw, err)
	}
	return port, validatePort(port)
}

func normalizePorts(ports []int) ([]int, error) {
	seen := make(map[int]struct{}, len(ports))
	for _, port := range ports {
		if err := validatePort(port); err != nil {
			return nil, err
		}
		seen[port] = struct{}{}
	}
	unique := make([]int, 0, len(seen))
	for port := range seen {
		unique = append(unique, port)
	}
	sort.Ints(unique)
	return unique, nil
}

// ResolveProjectPath validates a directory and selects its nearest manifest root.
// Directories without a recognized manifest retain their own absolute path.
func ResolveProjectPath(path string) (string, error) {
	if path == "" {
		path = "."
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("project directory %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path %q is not a directory", path)
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve project directory links: %w", err)
	}
	for dir := absolute; ; dir = filepath.Dir(dir) {
		for _, manifest := range []string{"package.json", "go.mod", "pyproject.toml", "requirements.txt"} {
			info, err := os.Stat(filepath.Join(dir, manifest))
			if err == nil && !info.IsDir() {
				return dir, nil
			}
			if err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("inspect project manifest: %w", err)
			}
		}
		if filepath.Dir(dir) == dir {
			return absolute, nil
		}
	}
}

// DiscoverProjectPorts finds project listeners. nil ports means every local TCP
// listener; an explicit empty list selects none. Full discovery never silently
// falls back to the incomplete default development-port list.
func DiscoverProjectPorts(ctx context.Context, path string, ports []int) ([]PortDiagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	projectPath, err := ResolveProjectPath(path)
	if err != nil {
		return nil, err
	}
	if ports != nil {
		ports, err = normalizePorts(ports)
		if err != nil {
			return nil, err
		}
		if len(ports) == 0 {
			return []PortDiagnostic{}, nil
		}
	}
	index, err := buildProcIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot discover project listeners from the local TCP table: %w", err)
	}
	if ports == nil {
		ports = make([]int, 0, len(index))
		for key := range index {
			ports = append(ports, int(key.port))
		}
	}
	occupied, err := scanPortsConcurrent(ctx, ports, index, true)
	if err != nil {
		return nil, err
	}
	targets := make([]PortDiagnostic, 0)
	for _, diag := range occupied {
		if projectDiagnosticMatches(diag, projectPath) {
			targets = append(targets, diag)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func projectDiagnosticMatches(diag PortDiagnostic, projectPath string) bool {
	if diag.Project == nil || diag.Project.Path == "" {
		return false
	}
	resolved, err := ResolveProjectPath(diag.Project.Path)
	return err == nil && projectPathsMatch(resolved, projectPath)
}
