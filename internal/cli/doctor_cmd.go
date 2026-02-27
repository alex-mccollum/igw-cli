package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var checkWrite bool

	bindWrapperCommonWithDefaults(fs, &common, 5*time.Second, false)
	fs.BoolVar(&checkWrite, "check-write", false, "Include mutating write-permission check (scan projects)")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	selectOpts, selectErr := newJSONSelectOptions(common.jsonOutput, common.compactJSON, common.rawOutput, common.selectors)
	if selectErr != nil {
		return selectErr
	}

	if common.apiKeyStdin {
		if common.apiKey != "" {
			return &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"}
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return igwerr.NewTransportError(err)
		}
		common.apiKey = strings.TrimSpace(string(tokenBytes))
	}

	resolved, err := c.resolveRuntimeConfig(common.profile, common.gatewayURL, common.apiKey)
	if err != nil {
		return err
	}

	if strings.TrimSpace(resolved.GatewayURL) == "" {
		return &igwerr.UsageError{Msg: "required: --gateway-url (or IGNITION_GATEWAY_URL/config)"}
	}
	if strings.TrimSpace(resolved.Token) == "" {
		return &igwerr.UsageError{Msg: "required: --api-key (or IGNITION_API_TOKEN/config)"}
	}
	if common.timeout <= 0 {
		return &igwerr.UsageError{Msg: "--timeout must be positive"}
	}

	checks := make([]doctorCheck, 0, 4)
	stats := map[string]any{}

	parsedURL, err := url.Parse(resolved.GatewayURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		uerr := &igwerr.UsageError{Msg: "invalid gateway URL"}
		checks = append(checks, doctorCheck{
			Name:    "gateway_url",
			OK:      false,
			Message: uerr.Error(),
			Hint:    "Use a full URL like http://<windows-host-ip>:8088",
		})
		return c.printDoctorResult(common.jsonOutput, selectOpts, resolved.GatewayURL, checks, stats, uerr)
	}
	checks = append(checks, doctorCheck{
		Name:    "gateway_url",
		OK:      true,
		Message: "parsed",
	})

	addr, addrErr := dialAddress(parsedURL)
	if addrErr != nil {
		uerr := &igwerr.UsageError{Msg: addrErr.Error()}
		checks = append(checks, doctorCheck{
			Name:    "tcp_connect",
			OK:      false,
			Message: uerr.Error(),
			Hint:    "Gateway URL must include a valid host and scheme",
		})
		return c.printDoctorResult(common.jsonOutput, selectOpts, resolved.GatewayURL, checks, stats, uerr)
	}

	tcpStart := time.Now()
	conn, err := net.DialTimeout("tcp", addr, common.timeout)
	if err != nil {
		nerr := igwerr.NewTransportError(err)
		checks = append(checks, doctorCheck{
			Name:    "tcp_connect",
			OK:      false,
			Message: nerr.Error(),
			Hint:    doctorHintForError(nerr),
		})
		if common.timing || common.jsonStats {
			stats["tcpConnectMs"] = time.Since(tcpStart).Milliseconds()
		}
		return c.printDoctorResult(common.jsonOutput, selectOpts, resolved.GatewayURL, checks, stats, nerr)
	}
	_ = conn.Close()
	checks = append(checks, doctorCheck{
		Name:    "tcp_connect",
		OK:      true,
		Message: addr,
	})
	if common.timing || common.jsonStats {
		stats["tcpConnectMs"] = time.Since(tcpStart).Milliseconds()
	}

	client := &gateway.Client{
		BaseURL: resolved.GatewayURL,
		Token:   resolved.Token,
		HTTP:    c.runtimeHTTPClient(),
	}

	type doctorCallResult struct {
		resp      *gateway.CallResponse
		err       error
		elapsedMs int64
	}

	var (
		gatewayInfo doctorCallResult
		scanWrite   doctorCallResult
		wg          sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		resp, callErr := client.Call(context.Background(), gateway.CallRequest{
			Method:       http.MethodGet,
			Path:         "/data/api/v1/gateway-info",
			Timeout:      common.timeout,
			EnableTiming: common.timing || common.jsonStats,
		})
		gatewayInfo = doctorCallResult{resp: resp, err: callErr, elapsedMs: time.Since(start).Milliseconds()}
	}()

	if checkWrite {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			resp, callErr := client.Call(context.Background(), gateway.CallRequest{
				Method:       http.MethodPost,
				Path:         "/data/api/v1/scan/projects",
				Timeout:      common.timeout,
				EnableTiming: common.timing || common.jsonStats,
			})
			scanWrite = doctorCallResult{resp: resp, err: callErr, elapsedMs: time.Since(start).Milliseconds()}
		}()
	}

	wg.Wait()
	if common.timing || common.jsonStats {
		stats["gatewayInfoMs"] = gatewayInfo.elapsedMs
		if gatewayInfo.resp != nil && gatewayInfo.resp.Timing != nil {
			stats["gatewayInfoHTTP"] = gatewayInfo.resp.Timing
		}
		if checkWrite {
			stats["scanProjectsMs"] = scanWrite.elapsedMs
			if scanWrite.resp != nil && scanWrite.resp.Timing != nil {
				stats["scanProjectsHTTP"] = scanWrite.resp.Timing
			}
		}
	}

	if gatewayInfo.err != nil {
		checks = append(checks, doctorCheck{
			Name:    "gateway_info",
			OK:      false,
			Message: gatewayInfo.err.Error(),
			Hint:    doctorHintForError(gatewayInfo.err),
		})
		if checkWrite {
			checks = append(checks, doctorCheck{
				Name:    "scan_projects",
				OK:      false,
				Message: "skipped (gateway_info failed)",
			})
		} else {
			checks = append(checks, doctorCheck{
				Name:    "scan_projects",
				OK:      true,
				Message: "skipped (use --check-write)",
			})
		}
		return c.printDoctorResult(common.jsonOutput, selectOpts, resolved.GatewayURL, checks, stats, gatewayInfo.err)
	}
	checks = append(checks, doctorCheck{
		Name:    "gateway_info",
		OK:      true,
		Message: fmt.Sprintf("status %d", gatewayInfo.resp.StatusCode),
	})

	if checkWrite {
		if scanWrite.err != nil {
			checks = append(checks, doctorCheck{
				Name:    "scan_projects",
				OK:      false,
				Message: scanWrite.err.Error(),
				Hint:    doctorHintForError(scanWrite.err),
			})
			return c.printDoctorResult(common.jsonOutput, selectOpts, resolved.GatewayURL, checks, stats, scanWrite.err)
		}
		checks = append(checks, doctorCheck{
			Name:    "scan_projects",
			OK:      true,
			Message: fmt.Sprintf("status %d", scanWrite.resp.StatusCode),
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:    "scan_projects",
			OK:      true,
			Message: "skipped (use --check-write)",
		})
	}

	return c.printDoctorResult(common.jsonOutput, selectOpts, resolved.GatewayURL, checks, stats, nil)
}

type doctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type doctorEnvelope struct {
	OK         bool           `json:"ok"`
	Code       int            `json:"code,omitempty"`
	Error      string         `json:"error,omitempty"`
	GatewayURL string         `json:"gatewayURL"`
	Checks     []doctorCheck  `json:"checks"`
	Stats      map[string]any `json:"stats,omitempty"`
}

func (c *CLI) printDoctorResult(jsonOutput bool, selectOpts jsonSelectOptions, gatewayURL string, checks []doctorCheck, stats map[string]any, err error) error {
	if jsonOutput {
		payload := doctorEnvelope{
			OK:         err == nil,
			GatewayURL: gatewayURL,
			Checks:     checks,
			Stats:      stats,
		}
		if err != nil {
			payload.Code = igwerr.ExitCode(err)
			payload.Error = err.Error()
		}

		if selectWriteErr := printJSONSelection(c.Out, payload, selectOpts); selectWriteErr != nil {
			_ = writeJSONWithOptions(c.Out, jsonErrorPayload(selectWriteErr), selectOpts.compact)
			return selectWriteErr
		}
		return err
	}

	for _, check := range checks {
		state := "ok"
		if !check.OK {
			state = "fail"
		}
		if check.Hint != "" {
			fmt.Fprintf(c.Out, "%s\t%s\t%s\thint: %s\n", state, check.Name, check.Message, check.Hint)
			continue
		}
		fmt.Fprintf(c.Out, "%s\t%s\t%s\n", state, check.Name, check.Message)
	}

	if err != nil {
		fmt.Fprintln(c.Err, err.Error())
	}
	return err
}

func doctorHintForError(err error) string {
	var statusErr *igwerr.StatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusUnauthorized:
			return "401 indicates a missing or invalid token. Re-check your API key."
		case http.StatusForbidden:
			return "403 indicates permission mapping or secure-connection restrictions. Ensure token security levels are included in Gateway Read permissions."
		}
	}

	var transportErr *igwerr.TransportError
	if errors.As(err, &transportErr) && transportErr.Timeout {
		return "If this is WSL2 -> Windows, allow inbound TCP 8088 on interface alias \"vEthernet (WSL (Hyper-V firewall))\"."
	}

	if errors.As(err, &transportErr) {
		return "Check gateway host/port reachability and local firewall rules."
	}

	return ""
}

func dialAddress(gatewayURL *url.URL) (string, error) {
	host := strings.TrimSpace(gatewayURL.Hostname())
	if host == "" {
		return "", fmt.Errorf("gateway URL host is empty")
	}

	port := strings.TrimSpace(gatewayURL.Port())
	if port == "" {
		switch strings.ToLower(gatewayURL.Scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", fmt.Errorf("unsupported URL scheme %q", gatewayURL.Scheme)
		}
	}

	return net.JoinHostPort(host, port), nil
}
