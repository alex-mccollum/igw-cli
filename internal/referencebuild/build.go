// Package referencebuild assembles contributor-qualified offline references.
// Container capture and workflow execution occur separately, before this step.
package referencebuild

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/imageref"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/moduleprofile"
	"github.com/alex-mccollum/igw-cli/internal/reference"
	"github.com/alex-mccollum/igw-cli/internal/testgateway"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

type Inputs struct {
	ResolutionDir string
	CaptureDir    string
	Lifecycle     string
	Resources     string
	Transfers     string
	Operations    string
	TestBinary    string
	Baseline      string
	Out           string
}

// Build refuses incomplete or mismatched evidence before creating the output
// directory. Every input is read locally; no Gateway or registry is contacted.
// The final reference manifest is published last and never replaces a bundle.
func Build(ctx context.Context, in Inputs) (reference.Manifest, error) {
	if in.ResolutionDir == "" || in.CaptureDir == "" || in.Lifecycle == "" || in.Resources == "" || in.Transfers == "" || in.Operations == "" || in.TestBinary == "" || in.Baseline == "" || in.Out == "" {
		return reference.Manifest{}, errors.New("reference qualification requires resolution, capture, lifecycle, all workflow receipts, test binary, baseline, and new output directory")
	}
	image, err := imageref.Load(ctx, in.ResolutionDir)
	if err != nil {
		return reference.Manifest{}, err
	}
	imageID, err := image.ConfigurationDigest()
	if err != nil {
		return reference.Manifest{}, err
	}
	binaryHash, err := hashBinary(ctx, in.TestBinary)
	if err != nil {
		return reference.Manifest{}, err
	}
	payloads := map[string][]byte{}
	paths := map[string]string{
		"capture.json":   filepath.Join(in.CaptureDir, "capture.json"),
		"lifecycle.json": in.Lifecycle, "resource-workflows.json": in.Resources,
		"project-tag-workflows.json": in.Transfers, "operational-workflows.json": in.Operations,
	}
	for name, path := range paths {
		b, err := reference.ReadFile(ctx, path, 4<<20)
		if err != nil {
			return reference.Manifest{}, err
		}
		if jsonvalue.Validate(b) != nil {
			return reference.Manifest{}, fmt.Errorf("invalid evidence JSON: %s", name)
		}
		payloads[name] = b
	}
	var capture testgateway.Evidence
	if json.Unmarshal(payloads["capture.json"], &capture) != nil {
		return reference.Manifest{}, errors.New("invalid capture receipt")
	}
	if err := validateCapture(capture, image.Resolution, imageID); err != nil {
		return reference.Manifest{}, err
	}
	profile, err := moduleprofile.FromWhitelist(capture.ModuleWhitelist)
	if err != nil {
		return reference.Manifest{}, err
	}
	if err := validateLifecycle(payloads["lifecycle.json"], capture, binaryHash); err != nil {
		return reference.Manifest{}, err
	}
	raw, err := reference.ReadFile(ctx, filepath.Join(in.CaptureDir, "openapi.json"), catalog.MaxDocumentBytes)
	if err != nil {
		return reference.Manifest{}, err
	}
	after, err := catalog.Parse(raw)
	if err != nil {
		return reference.Manifest{}, err
	}
	defer after.Close()
	if after.RawHash() != capture.RawSHA256 || after.DocumentHash() != capture.DocumentSHA256 || after.ContractHash() != capture.ContractSHA256 || after.OperationCount() != capture.Operations {
		return reference.Manifest{}, errors.New("capture identities do not match the original vendor document")
	}
	capabilities, err := workflow.AssessTags(after)
	if err != nil {
		return reference.Manifest{}, err
	}
	qualification, err := testgateway.NewQualification(binaryHash, capabilities)
	if err != nil {
		return reference.Manifest{}, err
	}
	for _, kind := range []string{"resource-workflows", "project-tag-workflows", "operational-workflows"} {
		if err := validateWorkflow(payloads[kind+".json"], kind, capture, binaryHash, capabilities); err != nil {
			return reference.Manifest{}, fmt.Errorf("%s: %w", kind, err)
		}
	}
	baseline, err := readBaseline(ctx, in.Baseline)
	if err != nil {
		return reference.Manifest{}, err
	}
	before, err := catalog.Parse(baseline)
	if err != nil {
		return reference.Manifest{}, err
	}
	defer before.Close()
	comparison := catalog.Compare(before, after)
	if err := ctx.Err(); err != nil {
		return reference.Manifest{}, err
	}
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write(raw); err != nil {
		return reference.Manifest{}, err
	}
	if err := w.Close(); err != nil {
		return reference.Manifest{}, err
	}
	payloads["openapi.json.gz"] = compressed.Bytes()
	payloads["registry-index.json"], payloads["registry-manifest.json"] = image.Index, image.Manifest
	// Canonical receipt encoding avoids a second read that could select newer
	// metadata than the exact manifests validated at the start of this build.
	payloads["resolution.json"], err = json.MarshalIndent(image.Resolution, "", "  ")
	if err != nil {
		return reference.Manifest{}, err
	}
	m := reference.Manifest{
		Version: reference.Version, CreatedAt: time.Now().UTC(), CapturedAt: &capture.CapturedAt,
		Name:                  "ignition-" + strings.Fields(capture.GatewayVersion)[0] + "-" + capture.ModuleInventory.SHA256[:12] + "-" + capture.ContractSHA256[:12],
		Image:                 reference.Image{Reference: capture.Image, ConfigurationDigest: capture.ImageID, Platform: capture.Platform, GatewayVersion: capture.GatewayVersion},
		ModuleInventorySHA256: capture.ModuleInventory.SHA256,
		ModuleProfile:         &profile,
		Catalog:               after.Identity(), ParserVersion: catalog.ParserVersion,
		Qualification: qualification,
	}
	for _, module := range capture.ModuleInventory.Modules {
		m.Modules = append(m.Modules, reference.Module{ID: module.ID, Version: module.Version, State: module.State, Collection: module.Collection})
	}
	qualification.ParserVersion, qualification.Catalog = catalog.ParserVersion, after.Identity()
	evidence := struct {
		Version    string             `json:"version"`
		Comparison catalog.Comparison `json:"comparison"`
		Files      []reference.File   `json:"files"`
	}{Version: "igw/reference-evidence/v1", Comparison: comparison}
	names := []string{"capture.json", "lifecycle.json", "operational-workflows.json", "project-tag-workflows.json", "registry-index.json", "registry-manifest.json", "resolution.json", "resource-workflows.json"}
	for _, name := range names {
		data := payloads[name]
		evidence.Files = append(evidence.Files, reference.File{Path: name, Bytes: int64(len(data)), SHA256: digest(data)})
	}
	evidenceBytes, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return reference.Manifest{}, err
	}
	qualification.Evidence = reference.Evidence{URI: "evidence/qualification.json", SHA256: digest(evidenceBytes)}
	m.Qualification = qualification
	for _, name := range reference.RequiredFiles() {
		b := payloads[name]
		m.Files = append(m.Files, reference.File{Path: name, Bytes: int64(len(b)), SHA256: digest(b)})
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return reference.Manifest{}, err
	}
	if len(b)+1 > reference.MaxManifestBytes {
		return reference.Manifest{}, errors.New("reference manifest exceeds size limit")
	}
	if err := os.Mkdir(in.Out, 0700); err != nil {
		return reference.Manifest{}, err
	}
	if err := os.Mkdir(filepath.Join(in.Out, "evidence"), 0700); err != nil {
		return reference.Manifest{}, err
	}
	for _, name := range names {
		if err := publish(filepath.Join(in.Out, "evidence", name), payloads[name]); err != nil {
			return reference.Manifest{}, err
		}
	}
	if err := publish(filepath.Join(in.Out, "evidence", "qualification.json"), evidenceBytes); err != nil {
		return reference.Manifest{}, err
	}
	for _, file := range m.Files {
		if err := ctx.Err(); err != nil {
			return reference.Manifest{}, err
		}
		if err := publish(filepath.Join(in.Out, file.Path), payloads[file.Path]); err != nil {
			return reference.Manifest{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return reference.Manifest{}, err
	}
	if err := publish(filepath.Join(in.Out, "reference.json"), append(b, '\n')); err != nil {
		return reference.Manifest{}, err
	}
	return reference.Read(ctx, in.Out)
}

func publish(path string, b []byte) error {
	w, err := artifact.New(path, false)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	if err == nil {
		_, err = w.Commit()
	}
	return errors.Join(err, w.Abort())
}

func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func hashBinary(ctx context.Context, path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 256<<20 {
		return "", errors.New("qualification requires a regular test binary of at most 256 MiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	b := make([]byte, 128<<10)
	var count int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := f.Read(b)
		count += int64(n)
		if count > 256<<20 {
			return "", errors.New("test binary exceeds size limit")
		}
		_, _ = h.Write(b[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if count != info.Size() {
		return "", errors.New("test binary changed during qualification")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readBaseline(ctx context.Context, path string) ([]byte, error) {
	b, err := reference.ReadFile(ctx, path, catalog.MaxDocumentBytes+(1<<20))
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(path, ".gz") {
		return b, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err = io.ReadAll(io.LimitReader(r, catalog.MaxDocumentBytes+1))
	if err != nil || len(b) > catalog.MaxDocumentBytes {
		return nil, errors.New("invalid or oversized baseline document")
	}
	return b, nil
}

var gatewayVersion = regexp.MustCompile(`^8\.3\.(0|[1-9][0-9]{0,4})( \(b[0-9]{1,32}\))?$`)

func validateCapture(c testgateway.Evidence, r imageref.Resolution, imageID string) error {
	if c.ParserVersion != catalog.ParserVersion {
		return errors.New("capture is incomplete or does not qualify the resolved image with the current parser")
	}
	return validateCaptureEvidence(c, r, imageID)
}

// Historical receipt checks preserve their recorded parser identity. Assembly
// must enter through validateCapture, which additionally requires today's parser.
func validateCaptureEvidence(c testgateway.Evidence, r imageref.Resolution, imageID string) error {
	if c.Version != 3 || !c.Validated || !c.Cleanup || c.ValidationError != "" || c.Image != r.Image || c.ImageID != imageID || c.Platform != r.Platform || !gatewayVersion.MatchString(c.GatewayVersion) || c.Source != "/openapi.json" || c.ParserVersion == "" || c.ContractPolicy != catalog.ContractPolicy || c.CapturedAt.IsZero() || c.Operations <= 0 {
		return errors.New("capture is incomplete or does not match the resolved image")
	}
	if r.Tag != "8.3" && strings.Fields(c.GatewayVersion)[0] != r.Tag {
		return errors.New("observed Gateway version does not match the resolved patch tag")
	}
	if err := validateInventory(c.ModuleInventory, c.ModuleWhitelist); err != nil {
		return err
	}
	if c.ModuleInventory.ObservedAt.After(c.CapturedAt) {
		return errors.New("module observation occurs after the recorded capture")
	}
	return nil
}

func validateInventory(inventory *testgateway.ModuleInventory, whitelist []string) error {
	profile, err := moduleprofile.FromWhitelist(whitelist)
	if err != nil {
		return err
	}
	return profile.ValidateInventory(inventory)
}
