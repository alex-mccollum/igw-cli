package catalog

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

const ReadTTL = 24 * time.Hour

type Policy struct {
	ForWrite   bool
	Offline    bool
	AllowStale bool
	Refresh    bool
	Pin        string
}

type Service struct {
	Store Store
	HTTP  *http.Client
	Now   func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) Acquire(ctx context.Context, target Target, token string, policy Policy) (*Snapshot, error) {
	if policy.Pin != "" && !validDigest(policy.Pin) {
		return nil, &igwerr.UsageError{Msg: "spec pin must be a SHA-256 hex digest"}
	}
	if policy.Offline && policy.Refresh {
		return nil, &igwerr.UsageError{Msg: "offline and refresh cannot be combined"}
	}
	cached, loadErr := s.Store.Load(ctx, target)
	if cached != nil {
		age := s.now().Sub(cached.Metadata.VerifiedAt)
		cached.Stale = age < 0 || age >= ReadTTL || cached.Metadata.SourceKind != "gateway" || len(cached.Warnings) > 0
	}
	if policy.Offline {
		if loadErr != nil {
			return nil, &igwerr.UsageError{Msg: "offline catalog unavailable for this target; sync or import a snapshot first"}
		}
		if policy.ForWrite && (!policy.AllowStale || cached.Metadata.SourceKind != "gateway") {
			cached.Close()
			return nil, &igwerr.UsageError{Msg: "writes require fresh Gateway verification; --allow-stale-spec can explicitly use a previously fetched target snapshot"}
		}
		if policy.ForWrite {
			cached.Stale = true
		}
		return checkPin(cached, policy.Pin)
	}
	if !policy.ForWrite && !policy.Refresh && cached != nil && !cached.Stale {
		return checkPin(cached, policy.Pin)
	}
	fresh, err := s.fetch(ctx, target, token, cached)
	if err == nil {
		cached.Close()
		return checkPin(fresh, policy.Pin)
	}
	if cached != nil && !policy.Refresh && (!policy.ForWrite || (policy.AllowStale && cached.Metadata.SourceKind == "gateway")) {
		cached.Stale = true
		cached.Warnings = append(cached.Warnings, "Gateway contract refresh failed; using a cached target snapshot.")
		return checkPin(cached, policy.Pin)
	}
	cached.Close()
	return nil, err
}

func checkPin(snapshot *Snapshot, pin string) (*Snapshot, error) {
	if pin != "" && snapshot.Metadata.ContractSHA256 != pin {
		snapshot.Close()
		return nil, &igwerr.UsageError{Msg: "Gateway contract differs from the pinned digest; inspect the spec diff"}
	}
	return snapshot, nil
}

// fetch preserves cached on failure. After successful publication it may
// transfer cached.Catalog into the returned snapshot, leaving cached consumed.
func (s Service) fetch(ctx context.Context, target Target, token string, cached *Snapshot) (*Snapshot, error) {
	endpoint, err := target.Endpoint("/openapi.json")
	if err != nil {
		return nil, &igwerr.UsageError{Msg: err.Error()}
	}
	headers := []string{"Accept: application/json"}
	conditional := false
	if cached != nil && cached.Metadata.SourceKind == "gateway" {
		if cached.Metadata.ETag != "" {
			headers = append(headers, "If-None-Match: "+cached.Metadata.ETag)
			conditional = true
		} else if cached.Metadata.LastModified != "" {
			headers = append(headers, "If-Modified-Since: "+cached.Metadata.LastModified)
			conditional = true
		}
	}
	client := &gateway.Client{BaseURL: target.URL, Token: token, HTTP: s.HTTP}
	resp, err := client.Call(ctx, gateway.CallRequest{Method: http.MethodGet, Path: endpoint, Headers: headers, MaxBodyBytes: MaxDocumentBytes})
	verifiedAt := s.now()
	var metadata Metadata
	var parsed *Catalog
	var status *igwerr.StatusError
	reused := false
	if errors.As(err, &status) && status.StatusCode == http.StatusNotModified && conditional {
		// A 304 only verifies a document when we sent that cached representation's
		// validator. Store.Load already validated its bytes with the current parser.
		parsed, err, reused = cached.Catalog, nil, true
		metadata = cached.Metadata
		metadata.VerifiedAt = verifiedAt
	} else if err == nil {
		// Equal contract hashes can conceal documentation or representation
		// changes. Reuse requires identical vendor bytes from this fresh response.
		if cached != nil && bytes.Equal(cached.Catalog.raw, resp.Body) {
			parsed, reused = cached.Catalog, true
		} else {
			parsed, err = Parse(resp.Body)
		}
		metadata = Metadata{Version: SnapshotVersion, Target: target, Source: endpoint, SourceKind: "gateway",
			FetchedAt: verifiedAt, VerifiedAt: verifiedAt, ETag: resp.Headers.Get("ETag"), LastModified: resp.Headers.Get("Last-Modified")}
	}
	if err != nil {
		return nil, err
	}
	metadata.RawSHA256, metadata.ContractSHA256 = parsed.RawHash(), parsed.ContractHash()
	metadata.ParserVersion = ParserVersion
	snapshot := &Snapshot{Metadata: metadata, Catalog: parsed}
	if err := s.Store.Save(ctx, snapshot); err != nil {
		if !reused {
			snapshot.Close()
		}
		return nil, err
	}
	if reused {
		cached.Catalog = nil
	}
	return snapshot, nil
}
