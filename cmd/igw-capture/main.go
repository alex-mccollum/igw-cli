// igw-capture is a contributor tool; it is not part of the released CLI surface.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/testgateway"
)

func main() {
	var cfg testgateway.Config
	var out, modules string
	var timeout time.Duration
	flag.StringVar(&cfg.Docker, "docker", "docker", "Docker executable")
	flag.StringVar(&cfg.Image, "image", "", "Pre-pulled official image pinned by digest")
	flag.StringVar(&modules, "modules", "", "Comma-separated first-party module whitelist; empty uses image defaults")
	flag.StringVar(&out, "out", "", "New directory for original document and capture evidence")
	flag.DurationVar(&timeout, "timeout", 5*time.Minute, "Startup and HTTP capture deadline")
	flag.Parse()
	if flag.NArg() != 0 || out == "" || cfg.Image == "" || timeout <= 0 || timeout > 8*time.Minute {
		fmt.Fprintln(os.Stderr, "required: --image and --out; timeout must be positive and at most eight minutes")
		os.Exit(2)
	}
	if modules != "" {
		cfg.Modules = strings.Split(modules, ",")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := run(ctx, cfg, out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg testgateway.Config, out string) (err error) {
	if err := os.Mkdir(out, 0700); err != nil {
		return err
	}
	session, err := testgateway.Start(ctx, cfg)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Waiting for disposable Gateway:", session.Name)
	defer func() {
		if cleanupErr := session.Close(); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("capture cleanup failed for %s: %w", session.Name, cleanupErr))
		}
	}()
	raw, err := session.WaitOpenAPI(ctx)
	if err != nil {
		return err
	}
	// Parsing can be expensive. Remove the container first so a killed or
	// memory-limited parser cannot leave a live Gateway behind.
	if err := session.Close(); err != nil {
		return err
	}
	evidence, err := session.Save(out, raw)
	if encodeErr := json.NewEncoder(os.Stdout).Encode(evidence); encodeErr != nil {
		return encodeErr
	}
	return err
}
