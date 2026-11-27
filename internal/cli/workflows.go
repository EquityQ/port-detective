package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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

func watchPort(cmd *cobra.Command, opts *options, port int, protocol string, interval time.Duration) error {
	out := stdout(cmd)
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	for {
		begin := time.Now()
		diag, err := core.InspectPortProtocol(cmd.Context(), port, protocol)
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
			begin := time.Now()
			diag, err := core.WaitForPort(cmd.Context(), port, state, timeout, interval)
			if err != nil {
				if errors.Is(err, core.ErrWaitTimeout) {
					return commandFailure("timeout", 3, "%s", err.Error())
				}
				return err
			}
			if opts.jsonOutput {
				return writeJSON(stdout(cmd), diag)
			}
			fmt.Fprintf(stdout(cmd), "Port %d is %s (elapsed: %s).\n", port, state, humanDuration(time.Since(begin)))
			return nil
		},
	}
	cmd.Flags().StringVar(&state, "state", "open", "desired TCP state: open or free")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "maximum time to wait")
	cmd.Flags().DurationVar(&interval, "interval", 250*time.Millisecond, "delay between inspections")
	return cmd
}

type freePortResult = core.FreePortResult

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
			result, err := core.FindFreePort(cmd.Context(), from, to)
			if err != nil {
				if errors.Is(err, core.ErrNoFreePort) {
					return commandFailure("no_free_port", 4, "%s", err.Error())
				}
				return err
			}
			if opts.jsonOutput {
				return writeJSON(stdout(cmd), result)
			}
			families := "IPv4 and IPv6"
			if !result.IPv6 {
				families = "IPv4; IPv6 loopback unavailable"
			}
			fmt.Fprintf(stdout(cmd), "Port %d is available on loopback (%s).\n%s\n", result.Port, families, result.Message)
			return nil
		},
	}
	cmd.Flags().IntVar(&from, "from", 3000, "first candidate port (inclusive)")
	cmd.Flags().IntVar(&to, "to", 3100, "last candidate port (inclusive)")
	return cmd
}

func checkPortBindings(ctx context.Context, port int, listen func(context.Context, string, string) (net.Listener, error)) (bool, bool, error) {
	return core.CheckPortBindings(ctx, port, listen)
}
