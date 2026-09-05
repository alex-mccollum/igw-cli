package testgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Container limits are separate from the Linux validation job's budget.
// Both are required: the Docker daemon is outside the caller's cgroup.
func requireValidationBudget(read func(string) ([]byte, error)) error {
	entry, err := read("/proc/self/cgroup")
	group := strings.TrimSpace(strings.TrimPrefix(string(entry), "0::"))
	if err != nil || !strings.HasPrefix(group, "/") || !strings.Contains(group, "/igw-validation-") || !strings.HasSuffix(group, ".scope") {
		return errors.New("capture requires bash scripts/bounded-run.sh -- <capture binary>; build it in a separate job first")
	}
	for file, maximum := range map[string]int64{"memory.max": 8 << 30, "memory.swap.max": 0, "pids.max": 256} {
		b, err := read("/sys/fs/cgroup" + group + "/" + file)
		n, parseErr := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		if err != nil || parseErr != nil || n < 0 || n > maximum || (maximum > 0 && n == 0) {
			return fmt.Errorf("capture cannot verify validation limit %s", file)
		}
	}
	return nil
}

type imageConfiguration struct {
	Entrypoint   []string
	User         string
	ID           string
	OS           string
	Architecture string
}

var imageID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func (s *Session) preflight(ctx context.Context) (imageConfiguration, error) {
	var image imageConfiguration
	b, err := s.command(ctx, nil, "info", "--format", "{{json .}}")
	if err != nil {
		return image, err
	}
	var info struct {
		OSType, CgroupVersion                          string
		MemoryLimit, SwapLimit, CPUCfsQuota, PidsLimit bool
	}
	if json.Unmarshal(b, &info) != nil || info.OSType != "linux" || info.CgroupVersion != "2" || !info.MemoryLimit || !info.SwapLimit || !info.CPUCfsQuota || !info.PidsLimit {
		return image, errors.New("capture requires a running Linux Docker engine with cgroup v2 memory, swap, CPU, and PID limit support")
	}
	b, err = s.command(ctx, nil, "ps", "--all", "--filter", "label="+ownerLabel, "--format", "{{.ID}}")
	if err != nil {
		return image, err
	}
	if strings.TrimSpace(string(b)) != "" {
		return image, errors.New("a qualification container already exists; finish or inspect its owned cleanup before another capture")
	}
	b, err = s.command(ctx, nil, "image", "inspect", "--format", `{"entrypoint":{{json .Config.Entrypoint}},"user":{{json .Config.User}},"id":{{json .Id}},"os":{{json .Os}},"architecture":{{json .Architecture}}}`, s.Image)
	if err != nil {
		return image, err
	}
	if json.Unmarshal(b, &image) != nil || len(image.Entrypoint) == 0 || len(image.Entrypoint) > 16 || image.Entrypoint[0] == "" {
		return image, errors.New("capture image has no supported entrypoint")
	}
	if !imageID.MatchString(image.ID) || image.OS != "linux" || image.Architecture != "amd64" {
		return image, errors.New("capture requires a verified linux/amd64 image configuration digest")
	}
	return image, nil
}

func (s *Session) verifyConfiguration(ctx context.Context) error {
	b, err := s.command(ctx, nil, "inspect", "--format", "{{json .HostConfig}}", s.ID)
	if err != nil {
		return err
	}
	var cfg struct {
		Memory, MemorySwap, NanoCpus, PidsLimit int64
		RestartPolicy                           struct{ Name string }
		PortBindings                            map[string][]struct{ HostIP, HostPort string }
	}
	if json.Unmarshal(b, &cfg) != nil || cfg.Memory != 2<<30 || cfg.MemorySwap != 2<<30 || cfg.NanoCpus != 2e9 || cfg.PidsLimit != 256 || cfg.RestartPolicy.Name != "no" {
		return errors.New("Docker did not retain the requested capture limits")
	}
	bindings := cfg.PortBindings["8088/tcp"]
	if len(cfg.PortBindings) != 1 || len(bindings) != 1 || bindings[0].HostIP != "127.0.0.1" {
		return errors.New("Docker did not retain loopback-only capture networking")
	}
	b, err = s.command(ctx, nil, "inspect", "--format", "{{.Image}}", s.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(b)) != s.ImageID {
		return errors.New("capture container does not use the inspected image configuration")
	}
	return nil
}

func (s *Session) verifyKernelLimits(ctx context.Context) error {
	b, err := s.command(ctx, nil, "exec", s.ID, "cat", "/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory.swap.max", "/sys/fs/cgroup/cpu.max", "/sys/fs/cgroup/pids.max")
	if err != nil {
		return err
	}
	fields := strings.Fields(string(b))
	if len(fields) != 5 || fields[0] != "2147483648" || fields[1] != "0" || fields[4] != "256" {
		return errors.New("capture container's kernel memory, swap, or PID limits were not applied")
	}
	quota, e1 := strconv.ParseInt(fields[2], 10, 64)
	period, e2 := strconv.ParseInt(fields[3], 10, 64)
	if e1 != nil || e2 != nil || period <= 0 || period > 1e6 || quota <= 0 || quota > period*2 {
		return errors.New("capture container's kernel CPU limit was not applied")
	}
	return nil
}
