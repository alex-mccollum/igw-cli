package wsl

import (
	"errors"
	"testing"
)

func TestParseDefaultGatewayFromIPRoute(t *testing.T) {
	t.Parallel()

	ip, ok := ParseDefaultGatewayFromIPRoute("default via 172.25.80.1 dev eth0 proto kernel\n")
	if !ok {
		t.Fatalf("expected parse success")
	}
	if ip != "172.25.80.1" {
		t.Fatalf("unexpected ip %q", ip)
	}
}

func TestParseNameserverFromResolvConf(t *testing.T) {
	t.Parallel()

	ip, ok := ParseNameserverFromResolvConf("# comment\nnameserver 10.255.255.254\n")
	if !ok {
		t.Fatalf("expected parse success")
	}
	if ip != "10.255.255.254" {
		t.Fatalf("unexpected ip %q", ip)
	}
}

func TestDetectWindowsHostIPUsesIPRouteFirst(t *testing.T) {
	t.Parallel()

	origExec := execIPRoute
	origRead := readResolvConf
	t.Cleanup(func() {
		execIPRoute = origExec
		readResolvConf = origRead
	})

	execIPRoute = func() ([]byte, error) {
		return []byte("default via 172.25.80.1 dev eth0"), nil
	}
	readResolvConf = func() ([]byte, error) {
		return []byte("nameserver 10.255.255.254"), nil
	}

	ip, source, err := DetectWindowsHostIP()
	if err != nil {
		t.Fatalf("detect host ip: %v", err)
	}
	if ip != "172.25.80.1" {
		t.Fatalf("unexpected ip %q", ip)
	}
	if source != "ip route default gateway" {
		t.Fatalf("unexpected source %q", source)
	}
}

func TestDetectWindowsHostIPFallsBackToResolvConf(t *testing.T) {
	t.Parallel()

	origExec := execIPRoute
	origRead := readResolvConf
	t.Cleanup(func() {
		execIPRoute = origExec
		readResolvConf = origRead
	})

	execIPRoute = func() ([]byte, error) {
		return nil, errors.New("ip route unavailable")
	}
	readResolvConf = func() ([]byte, error) {
		return []byte("nameserver 10.255.255.254"), nil
	}

	ip, source, err := DetectWindowsHostIP()
	if err != nil {
		t.Fatalf("detect host ip: %v", err)
	}
	if ip != "10.255.255.254" {
		t.Fatalf("unexpected ip %q", ip)
	}
	if source != "resolv.conf nameserver" {
		t.Fatalf("unexpected source %q", source)
	}
}
