package cli

import (
	"fmt"
	"strconv"
	"strings"
)

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("port %q is not a valid number", s)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port %d is outside the valid range 1-65535", port)
	}
	return port, nil
}

func parsePortsList(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	ports := make([]int, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		port, err := parsePort(trimmed)
		if err != nil {
			return nil, err
		}
		ports = append(ports, port)
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("no valid ports were provided")
	}
	return ports, nil
}
