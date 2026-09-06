// Package reference reads independently distributable offline API references.
// A reference never establishes the contract or authorization of a live target.
package reference

import (
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/moduleprofile"
)

const Version = "igw/reference/v2"
const QualificationPolicy = "igw-reference-workflows/2"
const MaxManifestBytes = 256 << 10

func QualificationScopes() []string {
	return []string{"catalog/model", "container/lifecycle", "operations/backup-logs-diagnostics", "projects/disabled-project", "resources/basic-schedule", "tags/memory-json"}
}

type Image struct {
	Reference           string `json:"reference"`
	ConfigurationDigest string `json:"configurationDigest"`
	Platform            string `json:"platform"`
	GatewayVersion      string `json:"gatewayVersion"`
}

type Module struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	State      string `json:"state"`
	Collection string `json:"collection"`
}

type Qualification struct {
	ParserVersion     string                         `json:"parserVersion"`
	Catalog           catalog.Identity               `json:"catalog"`
	Evidence          Evidence                       `json:"evidence"`
	Policy            string                         `json:"policy"`
	TestBinarySHA256  string                         `json:"testBinarySha256"`
	Scopes            []string                       `json:"scopes"`
	UnavailableScopes []string                       `json:"unavailableScopes,omitempty"`
	Capabilities      []catalog.CapabilityAssessment `json:"capabilities,omitempty"`
}

type Evidence struct {
	URI    string `json:"uri"`
	SHA256 string `json:"sha256"`
}

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Manifest struct {
	Version               string                   `json:"version"`
	Name                  string                   `json:"name"`
	CreatedAt             time.Time                `json:"createdAt"`
	Image                 Image                    `json:"image"`
	ModuleInventorySHA256 string                   `json:"moduleInventorySha256"`
	ModuleProfile         *moduleprofile.Selection `json:"moduleProfile,omitempty"`
	Modules               []Module                 `json:"modules"`
	Catalog               catalog.Identity         `json:"catalog"`
	ParserVersion         string                   `json:"parserVersion"`
	Qualification         Qualification            `json:"qualification"`
	Files                 []File                   `json:"files"`
}

// RequiredFiles is deliberately fixed in this format version. No input can
// direct a loader to read a path outside its selected reference directory.
func RequiredFiles() []string {
	return []string{"openapi.json.gz"}
}
