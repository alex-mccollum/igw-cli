package wsl

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

var execIPRoute = func() ([]byte, error) {
	return exec.Command("ip", "route").Output()
}

var readResolvConf = func() ([]byte, error) {
	return os.ReadFile("/etc/resolv.conf")
}

func ParseDefaultGatewayFromIPRoute(routeOutput string) (string, bool) {
	scanner := bufio.NewScanner(strings.NewReader(routeOutput))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "default" {
			continue
		}

		for i := 1; i < len(fields)-1; i++ {
			if fields[i] != "via" {
				continue
			}

			candidate := fields[i+1]
			if ip := net.ParseIP(candidate); ip != nil && ip.To4() != nil {
				return candidate, true
			}
		}
	}

	return "", false
}

func ParseNameserverFromResolvConf(resolvConf string) (string, bool) {
	scanner := bufio.NewScanner(strings.NewReader(resolvConf))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}

		candidate := fields[1]
		if ip := net.ParseIP(candidate); ip != nil && ip.To4() != nil {
			return candidate, true
		}
	}

	return "", false
}

// DetectWindowsHostIP returns the Windows host IP reachable from WSL.
func DetectWindowsHostIP() (ip string, source string, err error) {
	routeOutput, routeErr := execIPRoute()
	if routeErr == nil {
		if gateway, ok := ParseDefaultGatewayFromIPRoute(string(routeOutput)); ok {
			return gateway, "ip route default gateway", nil
		}
	}

	resolvBytes, resolvErr := readResolvConf()
	if resolvErr == nil {
		if nameserver, ok := ParseNameserverFromResolvConf(string(resolvBytes)); ok {
			return nameserver, "resolv.conf nameserver", nil
		}
	}

	if routeErr != nil && resolvErr != nil {
		return "", "", fmt.Errorf("failed to inspect ip route (%v) and resolv.conf (%v)", routeErr, resolvErr)
	}

	return "", "", errors.New("could not detect Windows host IP from ip route or /etc/resolv.conf")
}
