package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// SystemInfo holds host system information.
type SystemInfo struct {
	Host          string `json:"host"`
	OS            string `json:"os"`
	CPUs          int    `json:"cpus"`
	MemTotalMB    int    `json:"mem_total_mb"`
	MemUsedMB     int    `json:"mem_used_mb"`
	DiskTotal     string `json:"disk_total"`
	DiskUsed      string `json:"disk_used"`
	DiskPercent   string `json:"disk_percent"`
	Uptime        string `json:"uptime"`
	DockerVersion string `json:"docker_version"`
}

// SystemInfo gathers system information from the named host.
func (d *DockerClient) SystemInfo(ctx context.Context, host string) (*SystemInfo, error) {
	info := &SystemInfo{Host: host}

	osResult, err := d.transport.Exec(ctx, host, "uname -s")
	if err != nil {
		return nil, fmt.Errorf("uname on %s: %w", host, err)
	}
	info.OS = strings.TrimSpace(osResult.Stdout)

	if info.OS == "Darwin" {
		if err := d.collectDarwinInfo(ctx, host, info); err != nil {
			return nil, err
		}
	} else {
		if err := d.collectLinuxInfo(ctx, host, info); err != nil {
			return nil, err
		}
	}

	// Docker version (best-effort)
	verResult, err := d.transport.Exec(ctx, host, "docker --version")
	if err == nil && verResult.ExitCode == 0 {
		info.DockerVersion = strings.TrimSpace(verResult.Stdout)
	}

	return info, nil
}

func (d *DockerClient) collectDarwinInfo(ctx context.Context, host string, info *SystemInfo) error {
	r, err := d.transport.Exec(ctx, host, "sysctl -n hw.ncpu")
	if err != nil {
		return fmt.Errorf("sysctl hw.ncpu on %s: %w", host, err)
	}
	info.CPUs, _ = strconv.Atoi(strings.TrimSpace(r.Stdout))

	r, err = d.transport.Exec(ctx, host, "sysctl -n hw.memsize")
	if err != nil {
		return fmt.Errorf("sysctl hw.memsize on %s: %w", host, err)
	}
	totalBytes, _ := strconv.ParseInt(strings.TrimSpace(r.Stdout), 10, 64)
	info.MemTotalMB = int(totalBytes / 1024 / 1024)

	r, err = d.transport.Exec(ctx, host, "vm_stat")
	if err != nil {
		return fmt.Errorf("vm_stat on %s: %w", host, err)
	}
	freePages := parseDarwinVMStatPage(r.Stdout, "Pages free")
	inactivePages := parseDarwinVMStatPage(r.Stdout, "Pages inactive")
	specPages := parseDarwinVMStatPage(r.Stdout, "Pages speculative")
	availableBytes := (freePages + inactivePages + specPages) * 4096
	info.MemUsedMB = info.MemTotalMB - int(availableBytes/1024/1024)

	r, err = d.transport.Exec(ctx, host, "df -h /")
	if err != nil {
		return fmt.Errorf("df -h on %s: %w", host, err)
	}
	info.DiskTotal, info.DiskUsed, info.DiskPercent = parseDFOutput(r.Stdout)

	r, err = d.transport.Exec(ctx, host, "uptime")
	if err != nil {
		return fmt.Errorf("uptime on %s: %w", host, err)
	}
	info.Uptime = strings.TrimSpace(r.Stdout)

	return nil
}

func (d *DockerClient) collectLinuxInfo(ctx context.Context, host string, info *SystemInfo) error {
	r, err := d.transport.Exec(ctx, host, "nproc")
	if err != nil {
		return fmt.Errorf("nproc on %s: %w", host, err)
	}
	info.CPUs, _ = strconv.Atoi(strings.TrimSpace(r.Stdout))

	r, err = d.transport.Exec(ctx, host, "free -m")
	if err != nil {
		return fmt.Errorf("free -m on %s: %w", host, err)
	}
	info.MemTotalMB, info.MemUsedMB = parseLinuxFreeOutput(r.Stdout)

	r, err = d.transport.Exec(ctx, host, "df -h /")
	if err != nil {
		return fmt.Errorf("df -h on %s: %w", host, err)
	}
	info.DiskTotal, info.DiskUsed, info.DiskPercent = parseDFOutput(r.Stdout)

	r, err = d.transport.Exec(ctx, host, "uptime")
	if err != nil {
		return fmt.Errorf("uptime on %s: %w", host, err)
	}
	info.Uptime = strings.TrimSpace(r.Stdout)

	return nil
}

// parseDarwinVMStatPage extracts the page count for the named stat from vm_stat output.
func parseDarwinVMStatPage(output, key string) int64 {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, key+":") {
			val := strings.TrimSpace(strings.TrimPrefix(line, key+":"))
			val = strings.TrimSuffix(val, ".")
			n, _ := strconv.ParseInt(val, 10, 64)
			return n
		}
	}
	return 0
}

// parseLinuxFreeOutput parses `free -m` output and returns total and used MB.
func parseLinuxFreeOutput(output string) (totalMB, usedMB int) {
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "Mem:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			totalMB, _ = strconv.Atoi(fields[1])
			usedMB, _ = strconv.Atoi(fields[2])
		}
		return
	}
	return
}

// parseDFOutput parses `df -h /` output and returns size, used, and percent strings.
func parseDFOutput(output string) (total, used, percent string) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return
	}
	fields := strings.Fields(lines[1])
	if len(fields) >= 5 {
		total = fields[1]
		used = fields[2]
		percent = fields[4]
	}
	return
}
