package cmd

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/0xarkstar/remops/internal/docker"
	"github.com/0xarkstar/remops/internal/output"
	"github.com/0xarkstar/remops/internal/transport"
	"github.com/spf13/cobra"
)

// hostResult combines system info and containers for a single host.
type hostResult struct {
	Host       string                 `json:"host"`
	Info       *docker.SystemInfo     `json:"info"`
	Containers []docker.ContainerInfo `json:"containers"`
}

func (hr *hostResult) HostName() string { return hr.Host }

func (hr *hostResult) ContainerRows() []output.ContainerRow {
	rows := make([]output.ContainerRow, len(hr.Containers))
	for i, c := range hr.Containers {
		rows[i] = output.ContainerRow{
			Name:   c.Name,
			Image:  c.Image,
			Status: c.Status,
			State:  c.State,
		}
	}
	return rows
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show container status across hosts",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	start := time.Now()
	hosts := resolveHosts()

	t := transport.NewSSHTransport(cfg)
	defer t.Close()

	dc := docker.NewDockerClient(t)
	resp := output.NewResponse()

	type collected struct {
		result  *hostResult
		host    string
		errCode string
		errMsg  string
	}

	results := make([]collected, 0, len(hosts))
	var mu sync.Mutex
	var wg sync.WaitGroup

	ctx := cmd.Context()

	for _, h := range hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()

			hostCfg, ok := cfg.Hosts[host]
			var hostTimeout time.Duration
			if ok {
				hostTimeout = hostCfg.EffectiveTimeout()
			} else {
				hostTimeout = 10 * time.Second
			}
			hostCtx, cancel := context.WithTimeout(ctx, hostTimeout)
			defer cancel()

			res, err := gatherHostData(hostCtx, dc, host)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errCode := "exec_error"
				if hostCtx.Err() == context.DeadlineExceeded {
					errCode = "timeout"
				}
				results = append(results, collected{host: host, errCode: errCode, errMsg: err.Error()})
			} else {
				results = append(results, collected{result: res})
			}
		}(h)
	}

	wg.Wait()

	for _, c := range results {
		if c.result != nil {
			resp.AddResult(c.result)
		} else {
			resp.AddFailure(c.host, c.errCode, c.errMsg, "check SSH connectivity and Docker installation")
		}
	}

	resp.Finalize(start)
	return getFormatter().Format(os.Stdout, resp)
}

// gatherHostData collects system info and containers for one host.
func gatherHostData(ctx context.Context, dc *docker.DockerClient, host string) (*hostResult, error) {
	info, err := dc.SystemInfo(ctx, host)
	if err != nil {
		return nil, err
	}
	containers, err := dc.ListContainers(ctx, host)
	if err != nil {
		return nil, err
	}
	return &hostResult{
		Host:       host,
		Info:       info,
		Containers: containers,
	}, nil
}
