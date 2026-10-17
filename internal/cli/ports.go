package cli

import (
	"fmt"
	"strconv"
	"strings"
)

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("端口号 %q 不是合法数字", s)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("端口号 %d 超出有效范围 1-65535", port)
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
		return nil, fmt.Errorf("未指定任何有效端口号")
	}
	return ports, nil
}
