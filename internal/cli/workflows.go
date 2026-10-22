package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/EquityQ/port-detective/internal/core"
	"github.com/spf13/cobra"
)

func pause(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func watchPort(cmd *cobra.Command, opts *options, port int, interval time.Duration) error {
	out := stdout(cmd)
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	for {
		begin := time.Now()
		diag, err := core.InspectPort(cmd.Context(), port)
		if err != nil {
			return err
		}
		if opts.jsonOutput {
			if err := encoder.Encode(diag); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(out, "\nObserved at %s\n", time.Now().Format(time.RFC3339))
			renderCheck(out, newPalette(out), diag, time.Since(begin))
		}
		if err := pause(cmd.Context(), interval); err != nil {
			return err
		}
	}
}

func newWaitCommand(opts *options) *cobra.Command {
	var state string
	var timeout, interval time.Duration
	cmd := &cobra.Command{
		Use:     "wait <port>",
		Short:   "Wait for a TCP port to become open or free",
		Long:    "Wait for TCP occupancy with a deadline. An open TCP port does not prove application readiness.",
		Example: "  port-detective wait 3000 --state open --timeout 30s\n  port-detective wait 3000 --state free --timeout 10s --json",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(fmt.Errorf("exactly one port is required, for example: port-detective wait 3000"))
			}
			_, err := parsePort(args[0])
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if state != "open" && state != "free" {
				return usageError(fmt.Errorf("--state must be open or free"))
			}
			if timeout <= 0 || interval <= 0 {
				return usageError(fmt.Errorf("--timeout and --interval must be greater than zero"))
			}
			port, _ := parsePort(args[0])
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			begin := time.Now()
			for {
				diag, err := core.InspectPort(ctx, port)
				if err != nil {
					return waitError(cmd.Context(), ctx, port, state, timeout, err)
				}
				if diag.IsOpen == (state == "open") {
					if opts.jsonOutput {
						return writeJSON(stdout(cmd), diag)
					}
					fmt.Fprintf(stdout(cmd), "Port %d is %s (elapsed: %s).\n", port, state, humanDuration(time.Since(begin)))
					return nil
				}
				if err := pause(ctx, interval); err != nil {
					return waitError(cmd.Context(), ctx, port, state, timeout, err)
				}
			}
		},
	}
	cmd.Flags().StringVar(&state, "state", "open", "desired TCP state: open or free")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "maximum time to wait")
	cmd.Flags().DurationVar(&interval, "interval", 250*time.Millisecond, "delay between inspections")
	return cmd
}

func waitError(parent, ctx context.Context, port int, state string, timeout time.Duration, err error) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return commandFailure("timeout", 3, "timed out after %s waiting for port %d to become %s", timeout, port, state)
	}
	return err
}

type freePortResult struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	IPv4     bool   `json:"ipv4"`
	IPv6     bool   `json:"ipv6"`
	Reserved bool   `json:"reserved"`
	Message  string `json:"message"`
}

func newFindFreeCommand(opts *options) *cobra.Command {
	var from, to int
	cmd := &cobra.Command{
		Use:     "find-free",
		Short:   "Find a currently bindable loopback TCP port",
		Long:    "Find the first TCP port bindable on IPv4 and supported IPv6 loopback. The listener is closed before returning: the result is a snapshot, not a reservation, and says nothing about other interface bindings.",
		Example: "  port-detective find-free --from 3000 --to 3010\n  port-detective find-free --from 8000 --to 8100 --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if from < 1 || to > 65535 || from > to {
				return usageError(fmt.Errorf("expected 1 <= --from <= --to <= 65535"))
			}
			for port := from; port <= to; port++ {
				if err := cmd.Context().Err(); err != nil {
					return err
				}
				available, ipv6, err := bindablePort(cmd.Context(), port)
				if err != nil {
					return err
				}
				if !available {
					continue
				}
				result := freePortResult{Port: port, Protocol: "tcp", IPv4: true, IPv6: ipv6, Message: "Loopback bind checks passed. Port is not reserved; another process may claim it."}
				if opts.jsonOutput {
					return writeJSON(stdout(cmd), result)
				}
				families := "IPv4 and IPv6"
				if !ipv6 {
					families = "IPv4; IPv6 loopback unavailable"
				}
				fmt.Fprintf(stdout(cmd), "Port %d is available on loopback (%s).\n%s\n", port, families, result.Message)
				return nil
			}
			return commandFailure("no_free_port", 4, "no bindable loopback TCP port found in %d-%d", from, to)
		},
	}
	cmd.Flags().IntVar(&from, "from", 3000, "first candidate port (inclusive)")
	cmd.Flags().IntVar(&to, "to", 3100, "last candidate port (inclusive)")
	return cmd
}

func bindablePort(ctx context.Context, port int) (bool, bool, error) {
	var config net.ListenConfig
	return checkPortBindings(ctx, port, config.Listen)
}

func checkPortBindings(ctx context.Context, port int, listen func(context.Context, string, string) (net.Listener, error)) (bool, bool, error) {
	listener4, err := listen(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		if ctx.Err() != nil {
			return false, false, ctx.Err()
		}
		if socketError(err, syscall.EADDRINUSE, 10048) || socketError(err, syscall.EACCES, 10013) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("IPv4 loopback bind check failed: %w", err)
	}
	defer listener4.Close()
	listener6, err := listen(ctx, "tcp6", net.JoinHostPort("::1", strconv.Itoa(port)))
	if err != nil {
		if ctx.Err() != nil {
			return false, false, ctx.Err()
		}
		if socketError(err, syscall.EAFNOSUPPORT, 10047) || socketError(err, syscall.EPROTONOSUPPORT, 10043) || socketError(err, syscall.EADDRNOTAVAIL, 10049) {
			return true, false, nil
		}
		if socketError(err, syscall.EADDRINUSE, 10048) || socketError(err, syscall.EACCES, 10013) {
			return false, true, nil
		}
		return false, false, fmt.Errorf("IPv6 loopback bind check failed: %w", err)
	}
	defer listener6.Close()
	return true, true, nil
}

// Winsock uses distinct error numbers from the syscall package's POSIX values.
func socketError(err error, posix syscall.Errno, winsock syscall.Errno) bool {
	return errors.Is(err, posix) || (runtime.GOOS == "windows" && errors.Is(err, winsock))
}
