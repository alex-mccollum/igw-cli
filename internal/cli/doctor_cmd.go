package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var gatewayURL string
	var apiKey string
	var apiKeyStdin bool
	var profile string
	var timeout time.Duration
	var jsonOutput bool
	var checkWrite bool

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.StringVar(&profile, "profile", "", "Config profile name")
	fs.DurationVar(&timeout, "timeout", 5*time.Second, "Check timeout")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")
	fs.BoolVar(&checkWrite, "check-write", false, "Include mutating write-permission check (scan projects)")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	if apiKeyStdin {
		if apiKey != "" {
			return &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"}
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return igwerr.NewTransportError(err)
		}
		apiKey = strings.TrimSpace(string(tokenBytes))
	}

	resolved, err := c.resolveRuntimeConfig(profile, gatewayURL, apiKey)
	if err != nil {
		return err
	}

	if strings.TrimSpace(resolved.GatewayURL) == "" {
		return &igwerr.UsageError{Msg: "required: --gateway-url (or IGNITION_GATEWAY_URL/config)"}
	}
	if strings.TrimSpace(resolved.Token) == "" {
		return &igwerr.UsageError{Msg: "required: --api-key (or IGNITION_API_TOKEN/config)"}
	}
	if timeout <= 0 {
		return &igwerr.UsageError{Msg: "--timeout must be positive"}
	}

	checks := make([]doctorCheck, 0, 4)

	parsedURL, err := url.Parse(resolved.GatewayURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		uerr := &igwerr.UsageError{Msg: "invalid gateway URL"}
		checks = append(checks, doctorCheck{
			Name:    "gateway_url",
			OK:      false,
			Message: uerr.Error(),
			Hint:    "Use a full URL like http://<windows-host-ip>:8088",
		})
		return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, uerr)
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
		return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, uerr)
	}

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		nerr := igwerr.NewTransportError(err)
		checks = append(checks, doctorCheck{
			Name:    "tcp_connect",
			OK:      false,
			Message: nerr.Error(),
			Hint:    doctorHintForError(nerr),
		})
		return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, nerr)
	}
	_ = conn.Close()
	checks = append(checks, doctorCheck{
		Name:    "tcp_connect",
		OK:      true,
		Message: addr,
	})

	client := &gateway.Client{
		BaseURL: resolved.GatewayURL,
		Token:   resolved.Token,
		HTTP:    c.HTTPClient,
	}
	resp, err := client.Call(context.Background(), gateway.CallRequest{
		Method:  http.MethodGet,
		Path:    "/data/api/v1/gateway-info",
		Timeout: timeout,
	})
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:    "gateway_info",
			OK:      false,
			Message: err.Error(),
			Hint:    doctorHintForError(err),
		})
		return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, err)
	}

	checks = append(checks, doctorCheck{
		Name:    "gateway_info",
		OK:      true,
		Message: fmt.Sprintf("status %d", resp.StatusCode),
	})

	if checkWrite {
		writeResp, err := client.Call(context.Background(), gateway.CallRequest{
			Method:  http.MethodPost,
			Path:    "/data/api/v1/scan/projects",
			Timeout: timeout,
		})
		if err != nil {
			checks = append(checks, doctorCheck{
				Name:    "scan_projects",
				OK:      false,
				Message: err.Error(),
				Hint:    doctorHintForError(err),
			})
			return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, err)
		}
		checks = append(checks, doctorCheck{
			Name:    "scan_projects",
			OK:      true,
			Message: fmt.Sprintf("status %d", writeResp.StatusCode),
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:    "scan_projects",
			OK:      true,
			Message: "skipped (use --check-write)",
		})
	}

	return c.printDoctorResult(jsonOutput, resolved.GatewayURL, checks, nil)
}

type doctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type doctorEnvelope struct {
	OK         bool          `json:"ok"`
	Code       int           `json:"code,omitempty"`
	Error      string        `json:"error,omitempty"`
	GatewayURL string        `json:"gatewayURL"`
	Checks     []doctorCheck `json:"checks"`
}

func (c *CLI) printDoctorResult(jsonOutput bool, gatewayURL string, checks []doctorCheck, err error) error {
	if jsonOutput {
		payload := doctorEnvelope{
			OK:         err == nil,
			GatewayURL: gatewayURL,
			Checks:     checks,
		}
		if err != nil {
			payload.Code = igwerr.ExitCode(err)
			payload.Error = err.Error()
		}

		enc := json.NewEncoder(c.Out)
		enc.SetIndent("", "  ")
		if encodeErr := enc.Encode(payload); encodeErr != nil {
			return igwerr.NewTransportError(encodeErr)
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
