package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

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
	jsonRequested := argsWantJSON(args)
	fs := flag.NewFlagSet("config set", flag.ContinueOnError)
	fs.SetOutput(c.Err)
	if jsonRequested {
		fs.SetOutput(io.Discard)
	}

	var gatewayURL string
	var autoGateway bool
	var profileName string
	var apiKey string
	var apiKeyStdin bool
	var jsonOutput bool

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.BoolVar(&autoGateway, "auto-gateway", false, "Detect Windows host IP from WSL and set gateway URL")
	fs.StringVar(&profileName, "profile", "", "Profile to update instead of default config")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	if fs.NArg() > 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}

	if apiKeyStdin {
		if apiKey != "" {
			return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"})
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return c.printJSONCommandError(jsonOutput, igwerr.NewTransportError(err))
		}
		apiKey = strings.TrimSpace(string(tokenBytes))
	}

	if autoGateway && strings.TrimSpace(gatewayURL) != "" {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "use only one of --gateway-url or --auto-gateway"})
	}

	autoGatewaySource := ""
	if autoGateway {
		if c.DetectWSLHostIP == nil {
			return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "auto-gateway is not available in this runtime"})
		}

		hostIP, source, detectErr := c.DetectWSLHostIP()
		if detectErr != nil {
			return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("auto-gateway failed: %v", detectErr)})
		}

		gatewayURL = fmt.Sprintf("http://%s:8088", hostIP)
		autoGatewaySource = source
		if !jsonOutput {
			fmt.Fprintf(c.Out, "auto-detected gateway URL from %s: %s\n", source, gatewayURL)
		}
	}

	if strings.TrimSpace(gatewayURL) == "" && strings.TrimSpace(apiKey) == "" {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "set at least one of --gateway-url or --api-key"})
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)})
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
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "config writer is not configured"})
	}
	if err := c.WriteConfig(cfg); err != nil {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("save config: %v", err)})
	}

	path, pathErr := config.Path()
	pathValue := ""
	if pathErr == nil {
		pathValue = path
	}

	if jsonOutput {
		payload := map[string]any{
			"ok":           true,
			"configPath":   pathValue,
			"profile":      profileName,
			"gatewayURL":   strings.TrimSpace(gatewayURL),
			"tokenUpdated": strings.TrimSpace(apiKey) != "",
		}
		if autoGatewaySource != "" {
			payload["autoGatewaySource"] = autoGatewaySource
		}
		return writeJSON(c.Out, payload)
	}

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
		names := make([]string, 0, len(cfg.Profiles))
		for name := range cfg.Profiles {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			profile := cfg.Profiles[name]
			fmt.Fprintf(c.Out, "profile\t%s\t%s\t%s\n", name, profile.GatewayURL, config.MaskToken(profile.Token))
		}
	}
	return nil
}

func (c *CLI) runConfigProfileAdd(args []string) error {
	jsonRequested := argsWantJSON(args)
	if len(args) == 0 {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: "usage: igw config profile add <name> [flags]"})
	}
	name := strings.TrimSpace(args[0])
	if strings.HasPrefix(name, "-") || name == "" {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: "usage: igw config profile add <name> [flags]"})
	}

	fs := flag.NewFlagSet("config profile add", flag.ContinueOnError)
	fs.SetOutput(c.Err)
	if jsonRequested {
		fs.SetOutput(io.Discard)
	}

	var gatewayURL string
	var autoGateway bool
	var apiKey string
	var apiKeyStdin bool
	var makeActive bool
	var jsonOutput bool

	fs.StringVar(&gatewayURL, "gateway-url", "", "Gateway base URL")
	fs.BoolVar(&autoGateway, "auto-gateway", false, "Detect Windows host IP from WSL and set gateway URL")
	fs.StringVar(&apiKey, "api-key", "", "Ignition API token")
	fs.BoolVar(&apiKeyStdin, "api-key-stdin", false, "Read API token from stdin")
	fs.BoolVar(&makeActive, "use", false, "Set added profile as active profile")
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args[1:]); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	if fs.NArg() > 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}

	if apiKeyStdin {
		if apiKey != "" {
			return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"})
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return c.printJSONCommandError(jsonOutput, igwerr.NewTransportError(err))
		}
		apiKey = strings.TrimSpace(string(tokenBytes))
	}

	if autoGateway && strings.TrimSpace(gatewayURL) != "" {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "use only one of --gateway-url or --auto-gateway"})
	}
	autoGatewaySource := ""
	if autoGateway {
		if c.DetectWSLHostIP == nil {
			return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "auto-gateway is not available in this runtime"})
		}
		hostIP, source, detectErr := c.DetectWSLHostIP()
		if detectErr != nil {
			return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("auto-gateway failed: %v", detectErr)})
		}
		gatewayURL = fmt.Sprintf("http://%s:8088", hostIP)
		autoGatewaySource = source
	}

	if strings.TrimSpace(gatewayURL) == "" && strings.TrimSpace(apiKey) == "" {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "set at least one of --gateway-url or --api-key"})
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)})
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
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "config writer is not configured"})
	}
	if err := c.WriteConfig(cfg); err != nil {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("save config: %v", err)})
	}

	if jsonOutput {
		payload := map[string]any{
			"ok":           true,
			"name":         name,
			"active":       cfg.ActiveProfile == name,
			"gatewayURL":   strings.TrimSpace(gatewayURL),
			"tokenUpdated": strings.TrimSpace(apiKey) != "",
		}
		if autoGatewaySource != "" {
			payload["autoGatewaySource"] = autoGatewaySource
		}
		return writeJSON(c.Out, payload)
	}

	fmt.Fprintf(c.Out, "saved profile: %s\n", name)
	if cfg.ActiveProfile == name {
		fmt.Fprintf(c.Out, "active profile: %s\n", name)
	}
	return nil
}

func (c *CLI) runConfigProfileUse(args []string) error {
	jsonRequested := argsWantJSON(args)
	if len(args) == 0 {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: "usage: igw config profile use <name> [flags]"})
	}
	name := strings.TrimSpace(args[0])
	if strings.HasPrefix(name, "-") || name == "" {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: "usage: igw config profile use <name> [flags]"})
	}

	fs := flag.NewFlagSet("config profile use", flag.ContinueOnError)
	fs.SetOutput(c.Err)
	if jsonRequested {
		fs.SetOutput(io.Discard)
	}
	var jsonOutput bool
	fs.BoolVar(&jsonOutput, "json", false, "Print JSON output")

	if err := fs.Parse(args[1:]); err != nil {
		return c.printJSONCommandError(jsonRequested, &igwerr.UsageError{Msg: err.Error()})
	}
	if fs.NArg() > 0 {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}
	cfg, err := c.ReadConfig()
	if err != nil {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)})
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("profile %q not found", name)})
	}
	cfg.ActiveProfile = name

	if c.WriteConfig == nil {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: "config writer is not configured"})
	}
	if err := c.WriteConfig(cfg); err != nil {
		return c.printJSONCommandError(jsonOutput, &igwerr.UsageError{Msg: fmt.Sprintf("save config: %v", err)})
	}

	if jsonOutput {
		return writeJSON(c.Out, map[string]any{
			"ok":     true,
			"active": name,
		})
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
