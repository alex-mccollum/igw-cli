package referencebuild

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
)

// This policy names the exercised workflows and their expected outcomes.
// Failed/partial outcomes are required evidence for deliberate negative checks.
var workflowChecks = map[string]map[string][]string{
	"resource-workflows": {
		"completed": {"authenticated-catalog", "verify-created", "verify-updated", "verify-conflict-unchanged", "resource-types", "resource-describe", "resource-list", "resource-create", "resource-get", "resource-update", "resource-delete"},
		"accepted":  {"create", "update", "delete"},
		"preview":   {"preview-create", "resource-preview-create", "resource-preview-update", "resource-preview-delete"},
		"failed":    {"anonymous-denied", "bare-key-denied", "absence-after-preview", "stale-signature", "verify-deleted", "resource-preview-absence", "resource-duplicate-create", "resource-stale-update", "resource-verified-deleted"},
	},
	"project-tag-workflows": {
		"completed": {"catalog", "project-export", "project-get", "project-reexport", "tag-export", "tag-reexport", "workflow-project-list", "workflow-project-get", "workflow-project-export", "workflow-project-inspect", "workflow-project-import", "workflow-project-replace", "workflow-tag-preview-absence", "workflow-tag-import", "workflow-tag-export", "workflow-tag-preview-unchanged", "workflow-tag-overwrite", "workflow-tag-overwrite-export", "workflow-tag-abort-unchanged"},
		"accepted":  {"project-create", "project-import", "tag-import", "tag-overwrite", "tag-abort-conflict", "workflow-project-change", "workflow-project-delete", "project-delete-igw-transfer-source", "project-delete-igw-transfer-copy"},
		"preview":   {"project-preview", "workflow-project-preview", "workflow-project-replace-preview", "workflow-tag-preview", "workflow-tag-overwrite-preview"},
		"failed":    {"project-preview-absence", "project-existing-refused", "workflow-project-preview-absence", "workflow-project-stale-digest"},
		"partial":   {"workflow-tag-abort"},
	},
	"operational-workflows": {
		"completed": {"catalog", "gateway-info", "pending-restarts", "logs-list", "backup-export", "logs-download", "bundle-initial-status", "bundle-preview-unchanged", "bundle-status", "bundle-download", "bundle-after-download", "bundle-repeat-status", "workflow-logs-list", "workflow-backup-export", "workflow-logs-download", "workflow-bundle-status", "workflow-bundle-preview-unchanged", "workflow-bundle-collect", "workflow-bundle-download"},
		"accepted":  {"bundle-generate", "bundle-regenerate"},
		"preview":   {"bundle-preview", "workflow-bundle-preview"},
	},
}

func validateWorkflow(raw []byte, kind string, capture testgateway.Evidence, binaryHash string) error {
	if _, ok := workflowChecks[kind]; !ok {
		return errors.New("unsupported workflow qualification policy")
	}
	var r struct {
		Version        int                          `json:"version"`
		Kind           string                       `json:"kind"`
		Image          string                       `json:"image"`
		ImageID        string                       `json:"imageId"`
		Platform       string                       `json:"platform"`
		GatewayVersion string                       `json:"gatewayVersion"`
		BinarySHA256   string                       `json:"testBinarySha256"`
		StartedAt      time.Time                    `json:"startedAt"`
		FinishedAt     time.Time                    `json:"finishedAt"`
		Inventory      *testgateway.ModuleInventory `json:"moduleInventory"`
		Catalog        *catalog.Metadata            `json:"catalog"`
		Checks         []struct {
			Name    string `json:"name"`
			Outcome string `json:"outcome"`
		} `json:"checks"`
		Cleanup bool `json:"cleanup"`
		Passed  bool `json:"passed"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Version != 2 || r.Kind != kind || !r.Passed || !r.Cleanup || r.Image != capture.Image || r.ImageID != capture.ImageID || r.Platform != capture.Platform || r.GatewayVersion != capture.GatewayVersion || r.BinarySHA256 != binaryHash || r.StartedAt.IsZero() || !r.FinishedAt.After(r.StartedAt) || r.FinishedAt.Sub(r.StartedAt) > 8*time.Minute {
		return errors.New("workflow receipt does not qualify this image and test binary")
	}
	if err := validateInventory(r.Inventory); err != nil {
		return err
	}
	if r.Inventory.SHA256 != capture.ModuleInventory.SHA256 || r.Inventory.ObservedAt.Before(r.StartedAt) || r.Inventory.ObservedAt.After(r.FinishedAt) {
		return errors.New("workflow module inventory does not match the capture or observation window")
	}
	c := r.Catalog
	if c == nil || c.Version != catalog.SnapshotVersion || c.SourceKind != "gateway" || c.ParserVersion != catalog.ParserVersion || c.ContractPolicy != catalog.ContractPolicy || c.ContractSHA256 != capture.ContractSHA256 || c.FetchedAt.Before(r.StartedAt) || c.VerifiedAt.Before(c.FetchedAt) || c.VerifiedAt.After(r.FinishedAt) {
		return errors.New("workflow did not validate the captured contract with the current parser")
	}
	u, err := url.Parse(c.Target.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || c.Source != c.Target.URL+"/openapi.json" {
		return errors.New("workflow catalog did not come from a disposable loopback Gateway")
	}
	seen := map[string]string{}
	for _, check := range r.Checks {
		if check.Name == "" || seen[check.Name] != "" || check.Outcome == "" {
			return errors.New("workflow receipt contains invalid or duplicate checks")
		}
		seen[check.Name] = check.Outcome
	}
	for outcome, names := range workflowChecks[kind] {
		for _, name := range names {
			if seen[name] != outcome {
				return fmt.Errorf("required workflow check missing or changed: %s", name)
			}
		}
	}
	return nil
}

func validateLifecycle(raw []byte, capture testgateway.Evidence, binaryHash string) error {
	var r struct {
		Version         int       `json:"version"`
		Kind            string    `json:"kind"`
		Image           string    `json:"image"`
		ImageID         string    `json:"imageId"`
		Platform        string    `json:"platform"`
		BinarySHA256    string    `json:"testBinarySha256"`
		StartedAt       time.Time `json:"startedAt"`
		FinishedAt      time.Time `json:"finishedAt"`
		LifetimeSeconds int       `json:"lifetimeSeconds"`
		ElapsedSeconds  float64   `json:"elapsedSeconds"`
		ExitCode        int       `json:"exitCode"`
		Checks          []string  `json:"checks"`
		Cleanup         bool      `json:"cleanup"`
		Passed          bool      `json:"passed"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Version != 1 || r.Kind != "capture-lifecycle" || !r.Passed || !r.Cleanup || r.Image != capture.Image || r.ImageID != capture.ImageID || r.Platform != capture.Platform || r.BinarySHA256 != binaryHash || r.StartedAt.IsZero() || !r.FinishedAt.After(r.StartedAt) || r.FinishedAt.Sub(r.StartedAt) > 90*time.Second || r.LifetimeSeconds != 5 || r.ElapsedSeconds < 4 || r.ElapsedSeconds > 25 || (r.ExitCode != 124 && r.ExitCode != 137) {
		return errors.New("lifecycle receipt does not qualify this image and test binary")
	}
	seen := map[string]bool{}
	for _, name := range r.Checks {
		if name == "" || seen[name] {
			return errors.New("lifecycle receipt contains duplicate checks")
		}
		seen[name] = true
	}
	for _, name := range []string{"image-platform", "container-image-identity", "configured-limits", "kernel-limits", "loopback-only", "exclusive-admission", "lifetime-termination", "no-oom", "owned-cleanup", "absence-after-cleanup"} {
		if !seen[name] {
			return fmt.Errorf("required lifecycle check missing: %s", name)
		}
	}
	return nil
}
