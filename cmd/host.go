package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/0xarkstar/remops/internal/docker"
	"github.com/0xarkstar/remops/internal/output"
	"github.com/0xarkstar/remops/internal/transport"
	"github.com/spf13/cobra"
)

var hostCmd = &cobra.Command{
	Use:   "host",
	Short: "Host management commands",
}

var hostInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show system info and containers for a host",
	Args:  cobra.ExactArgs(1),
	RunE:  runHostInfo,
}

func init() {
	hostCmd.AddCommand(hostInfoCmd)
	rootCmd.AddCommand(hostCmd)
}

func runHostInfo(cmd *cobra.Command, args []string) error {
	hostName := args[0]
	if _, ok := cfg.Hosts[hostName]; !ok {
		return fmt.Errorf("unknown host %q", hostName)
	}

	start := time.Now()
	t := transport.NewSSHTransport(cfg)
	defer t.Close()

	dc := docker.NewDockerClient(t)
	ctx := cmd.Context()

	resp := output.NewResponse()
	result, err := gatherHostData(ctx, dc, hostName)
	if err != nil {
		resp.AddFailure(hostName, "exec_error", err.Error(), "check SSH connectivity and Docker installation")
	} else {
		resp.AddResult(result)
	}

	resp.Finalize(start)
	return getFormatter().Format(os.Stdout, resp)
}
