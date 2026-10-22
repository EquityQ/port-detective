package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/EquityQ/port-detective/internal/core"
)

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, usageError(fmt.Errorf("port %q is not a valid number", s))
	}
	if port < 1 || port > 65535 {
		return 0, usageError(fmt.Errorf("port %d is outside the valid range 1-65535", port))
	}
	return port, nil
}

func parsePortsList(s string) ([]int, error) {
	ports, err := core.ParsePortsList(s)
	if err != nil {
		return nil, usageError(err)
	}
	if len(ports) == 0 {
		return nil, usageError(fmt.Errorf("no valid ports were provided"))
	}
	return ports, nil
}
