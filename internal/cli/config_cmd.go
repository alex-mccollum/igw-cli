package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runConfig(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw config <set|show|profile> [flags]")
		return &igwerr.UsageError{Msg: "required config subcommand"}
	}

	switch args[0] {
	case "set":
		return c.runConfigSet(args[1:])
	case "show":
		return c.runConfigShow(args[1:])
	case "profile":
		return c.runConfigProfile(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown config subcommand %q", args[0])}
	}
}

func (c *CLI) runGateway(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw gateway <info> [flags]")
		return &igwerr.UsageError{Msg: "required gateway subcommand"}
	}

	switch args[0] {
	case "info":
		return c.runGatewayInfo(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown gateway subcommand %q", args[0])}
	}
}

func (c *CLI) runGatewayInfo(args []string) error {
	fs := flag.NewFlagSet("gateway info", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var (
		gatewayURL     string
		apiKey         string
		profile        string
		apiKeyStdin    bool
		timeout        time.Duration
		jsonOutput     bool
		includeHeaders bool
		retry          int
		retryBackoff   time.Duration
		outPath        string
	)

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.StringVar(&profile, "profile", "", "Config profile name")
	fs.DurationVar(&timeout, "timeout", 8*time.Second, "Request timeout")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON envelope")
	fs.BoolVar(&includeHeaders, "include-headers", false, "Include response headers")
	fs.IntVar(&retry, "retry", 0, "Retry attempts for idempotent requests")
	fs.DurationVar(&retryBackoff, "retry-backoff", 250*time.Millisecond, "Retry backoff duration")
	fs.StringVar(&outPath, "out", "", "Write response body to file")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	callArgs := []string{
		"--method", "GET",
		"--path", "/data/api/v1/gateway-info",
		"--timeout", timeout.String(),
		"--retry", fmt.Sprintf("%d", retry),
		"--retry-backoff", retryBackoff.String(),
	}
	if gatewayURL != "" {
		callArgs = append(callArgs, "--gateway-url", gatewayURL)
	}
	if apiKey != "" {
		callArgs = append(callArgs, "--api-key", apiKey)
	}
	if apiKeyStdin {
		callArgs = append(callArgs, "--api-key-stdin")
	}
	if profile != "" {
		callArgs = append(callArgs, "--profile", profile)
	}
	if jsonOutput {
		callArgs = append(callArgs, "--json")
	}
	if includeHeaders {
		callArgs = append(callArgs, "--include-headers")
	}
	if outPath != "" {
		callArgs = append(callArgs, "--out", outPath)
	}

	return c.runCall(callArgs)
}

func (c *CLI) runScan(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw scan <projects> [flags]")
		return &igwerr.UsageError{Msg: "required scan subcommand"}
	}

	switch args[0] {
	case "projects":
		return c.runScanProjects(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown scan subcommand %q", args[0])}
	}
}

func (c *CLI) runScanProjects(args []string) error {
	fs := flag.NewFlagSet("scan projects", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var (
		gatewayURL     string
		apiKey         string
		profile        string
		apiKeyStdin    bool
		timeout        time.Duration
		jsonOutput     bool
		includeHeaders bool
		yes            bool
		dryRun         bool
	)

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.StringVar(&profile, "profile", "", "Config profile name")
	fs.DurationVar(&timeout, "timeout", 8*time.Second, "Request timeout")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON envelope")
	fs.BoolVar(&includeHeaders, "include-headers", false, "Include response headers")
	fs.BoolVar(&yes, "yes", false, "Confirm mutating request")
	fs.BoolVar(&dryRun, "dry-run", false, "Append dryRun=true query parameter")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	callArgs := []string{
		"--method", "POST",
		"--path", "/data/api/v1/scan/projects",
		"--timeout", timeout.String(),
	}
	if gatewayURL != "" {
		callArgs = append(callArgs, "--gateway-url", gatewayURL)
	}
	if apiKey != "" {
		callArgs = append(callArgs, "--api-key", apiKey)
	}
	if apiKeyStdin {
		callArgs = append(callArgs, "--api-key-stdin")
	}
	if profile != "" {
		callArgs = append(callArgs, "--profile", profile)
	}
	if jsonOutput {
		callArgs = append(callArgs, "--json")
	}
	if includeHeaders {
		callArgs = append(callArgs, "--include-headers")
	}
	if dryRun {
		callArgs = append(callArgs, "--dry-run")
	}
	if yes {
		callArgs = append(callArgs, "--yes")
	}

	return c.runCall(callArgs)
}

func (c *CLI) runConfigProfile(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw config profile <add|use|list> [flags]")
		return &igwerr.UsageError{Msg: "required config profile subcommand"}
	}

	switch args[0] {
	case "add":
		return c.runConfigProfileAdd(args[1:])
	case "use":
		return c.runConfigProfileUse(args[1:])
	case "list":
		return c.runConfigProfileList(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown config profile subcommand %q", args[0])}
	}
}

func (c *CLI) runConfigSet(args []string) error {
	fs := flag.NewFlagSet("config set", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var gatewayURL string
	var autoGateway bool
	var profileName string
	var apiKey string
	var apiKeyStdin bool

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.BoolVar(&autoGateway, "auto-gateway", false, "Detect Windows host IP from WSL and set gateway URL")
	fs.StringVar(&profileName, "profile", "", "Profile to update instead of default config")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")

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

	if autoGateway && strings.TrimSpace(gatewayURL) != "" {
		return &igwerr.UsageError{Msg: "use only one of --gateway-url or --auto-gateway"}
	}

	if autoGateway {
		if c.DetectWSLHostIP == nil {
			return &igwerr.UsageError{Msg: "auto-gateway is not available in this runtime"}
		}

		hostIP, source, detectErr := c.DetectWSLHostIP()
		if detectErr != nil {
			return &igwerr.UsageError{Msg: fmt.Sprintf("auto-gateway failed: %v", detectErr)}
		}

		gatewayURL = fmt.Sprintf("http://%s:8088", hostIP)
		fmt.Fprintf(c.Out, "auto-detected gateway URL from %s: %s\n", source, gatewayURL)
	}

	if strings.TrimSpace(gatewayURL) == "" && strings.TrimSpace(apiKey) == "" {
		return &igwerr.UsageError{Msg: "set at least one of --gateway-url or --api-key"}
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}

	profileName = strings.TrimSpace(profileName)
	if profileName != "" {
		if cfg.Profiles == nil {
			cfg.Profiles = map[string]config.Profile{}
		}

		profileCfg := cfg.Profiles[profileName]
		if strings.TrimSpace(gatewayURL) != "" {
			profileCfg.GatewayURL = strings.TrimSpace(gatewayURL)
		}
		if strings.TrimSpace(apiKey) != "" {
			profileCfg.Token = strings.TrimSpace(apiKey)
		}
		cfg.Profiles[profileName] = profileCfg
	} else {
		if strings.TrimSpace(gatewayURL) != "" {
			cfg.GatewayURL = strings.TrimSpace(gatewayURL)
		}
		if strings.TrimSpace(apiKey) != "" {
			cfg.Token = strings.TrimSpace(apiKey)
		}
	}

	if c.WriteConfig == nil {
		return &igwerr.UsageError{Msg: "config writer is not configured"}
	}
	if err := c.WriteConfig(cfg); err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("save config: %v", err)}
	}

	path, pathErr := config.Path()
	if pathErr == nil {
		fmt.Fprintf(c.Out, "saved config: %s\n", path)
	} else {
		fmt.Fprintln(c.Out, "saved config")
	}
	if profileName != "" {
		fmt.Fprintf(c.Out, "updated profile: %s\n", profileName)
	}

	return nil
}

func (c *CLI) runConfigShow(args []string) error {
	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var jsonOutput bool
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}

	if jsonOutput {
		type profileView struct {
			GatewayURL  string `json:"gatewayURL,omitempty"`
			TokenMasked string `json:"tokenMasked,omitempty"`
		}
		profiles := map[string]profileView{}
		for name, profile := range cfg.Profiles {
			profiles[name] = profileView{
				GatewayURL:  profile.GatewayURL,
				TokenMasked: config.MaskToken(profile.Token),
			}
		}
		payload := map[string]any{
			"gatewayURL":    cfg.GatewayURL,
			"tokenMasked":   config.MaskToken(cfg.Token),
			"activeProfile": cfg.ActiveProfile,
			"profiles":      profiles,
			"profileCount":  len(profiles),
		}
		enc := json.NewEncoder(c.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return igwerr.NewTransportError(err)
		}
		return nil
	}

	fmt.Fprintf(c.Out, "gateway_url\t%s\n", cfg.GatewayURL)
	fmt.Fprintf(c.Out, "token\t%s\n", config.MaskToken(cfg.Token))
	if strings.TrimSpace(cfg.ActiveProfile) != "" {
		fmt.Fprintf(c.Out, "active_profile\t%s\n", cfg.ActiveProfile)
	}
	if len(cfg.Profiles) > 0 {
		for name, profile := range cfg.Profiles {
			fmt.Fprintf(c.Out, "profile\t%s\t%s\t%s\n", name, profile.GatewayURL, config.MaskToken(profile.Token))
		}
	}
	return nil
}

func (c *CLI) runConfigProfileAdd(args []string) error {
	if len(args) == 0 {
		return &igwerr.UsageError{Msg: "usage: igw config profile add <name> [flags]"}
	}
	name := strings.TrimSpace(args[0])
	if strings.HasPrefix(name, "-") || name == "" {
		return &igwerr.UsageError{Msg: "usage: igw config profile add <name> [flags]"}
	}

	fs := flag.NewFlagSet("config profile add", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var gatewayURL string
	var autoGateway bool
	var apiKey string
	var apiKeyStdin bool
	var makeActive bool

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.BoolVar(&autoGateway, "auto-gateway", false, "Detect Windows host IP from WSL and set gateway URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.BoolVar(&makeActive, "use", false, "Set added profile as active profile")

	if err := fs.Parse(args[1:]); err != nil {
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

	if autoGateway && strings.TrimSpace(gatewayURL) != "" {
		return &igwerr.UsageError{Msg: "use only one of --gateway-url or --auto-gateway"}
	}
	if autoGateway {
		if c.DetectWSLHostIP == nil {
			return &igwerr.UsageError{Msg: "auto-gateway is not available in this runtime"}
		}
		hostIP, _, detectErr := c.DetectWSLHostIP()
		if detectErr != nil {
			return &igwerr.UsageError{Msg: fmt.Sprintf("auto-gateway failed: %v", detectErr)}
		}
		gatewayURL = fmt.Sprintf("http://%s:8088", hostIP)
	}

	if strings.TrimSpace(gatewayURL) == "" && strings.TrimSpace(apiKey) == "" {
		return &igwerr.UsageError{Msg: "set at least one of --gateway-url or --api-key"}
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.Profile{}
	}
	profileCountBefore := len(cfg.Profiles)

	profile := cfg.Profiles[name]
	if strings.TrimSpace(gatewayURL) != "" {
		profile.GatewayURL = strings.TrimSpace(gatewayURL)
	}
	if strings.TrimSpace(apiKey) != "" {
		profile.Token = strings.TrimSpace(apiKey)
	}
	cfg.Profiles[name] = profile

	if makeActive {
		cfg.ActiveProfile = name
	} else if strings.TrimSpace(cfg.ActiveProfile) == "" && profileCountBefore == 0 {
		// First profile becomes active by default to reduce first-run friction.
		cfg.ActiveProfile = name
	}

	if c.WriteConfig == nil {
		return &igwerr.UsageError{Msg: "config writer is not configured"}
	}
	if err := c.WriteConfig(cfg); err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("save config: %v", err)}
	}

	fmt.Fprintf(c.Out, "saved profile: %s\n", name)
	if makeActive {
		fmt.Fprintf(c.Out, "active profile: %s\n", name)
	}
	return nil
}

func (c *CLI) runConfigProfileUse(args []string) error {
	if len(args) == 0 {
		return &igwerr.UsageError{Msg: "usage: igw config profile use <name>"}
	}
	name := strings.TrimSpace(args[0])
	if strings.HasPrefix(name, "-") || name == "" {
		return &igwerr.UsageError{Msg: "usage: igw config profile use <name>"}
	}

	fs := flag.NewFlagSet("config profile use", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	if err := fs.Parse(args[1:]); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}
	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return &igwerr.UsageError{Msg: fmt.Sprintf("profile %q not found", name)}
	}
	cfg.ActiveProfile = name

	if c.WriteConfig == nil {
		return &igwerr.UsageError{Msg: "config writer is not configured"}
	}
	if err := c.WriteConfig(cfg); err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("save config: %v", err)}
	}

	fmt.Fprintf(c.Out, "active profile: %s\n", name)
	return nil
}

func (c *CLI) runConfigProfileList(args []string) error {
	fs := flag.NewFlagSet("config profile list", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var jsonOutput bool
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return &igwerr.UsageError{Msg: err.Error()}
	}
	if fs.NArg() > 0 {
		return &igwerr.UsageError{Msg: "unexpected positional arguments"}
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}

	type profileView struct {
		Name        string `json:"name"`
		Active      bool   `json:"active"`
		GatewayURL  string `json:"gatewayURL,omitempty"`
		TokenMasked string `json:"tokenMasked,omitempty"`
	}

	views := make([]profileView, 0, len(cfg.Profiles))
	for name, profile := range cfg.Profiles {
		views = append(views, profileView{
			Name:        name,
			Active:      name == cfg.ActiveProfile,
			GatewayURL:  profile.GatewayURL,
			TokenMasked: config.MaskToken(profile.Token),
		})
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].Name < views[j].Name
	})

	if jsonOutput {
		return writeJSON(c.Out, map[string]any{
			"activeProfile": cfg.ActiveProfile,
			"count":         len(views),
			"profiles":      views,
		})
	}

	fmt.Fprintln(c.Out, "ACTIVE\tNAME\tGATEWAY_URL\tTOKEN")
	for _, view := range views {
		active := ""
		if view.Active {
			active = "*"
		}
		fmt.Fprintf(c.Out, "%s\t%s\t%s\t%s\n", active, view.Name, view.GatewayURL, view.TokenMasked)
	}

	return nil
}

func (c *CLI) resolveRuntimeConfig(profile string, gatewayURL string, apiKey string) (config.Effective, error) {
	cfg, err := c.ReadConfig()
	if err != nil {
		return config.Effective{}, &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}

	resolved, err := config.ResolveWithProfile(cfg, c.Getenv, gatewayURL, apiKey, profile)
	if err != nil {
		return config.Effective{}, &igwerr.UsageError{Msg: err.Error()}
	}

	return resolved, nil
}
