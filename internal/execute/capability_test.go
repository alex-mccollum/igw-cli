package execute

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
)

func TestScopeRequiresCapabilitiesBeforeOperations(t *testing.T) {
	var specs, calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi.json" {
			specs.Add(1)
			_, _ = io.WriteString(w, scopeSpec)
			return
		}
		calls.Add(1)
	}))
	defer srv.Close()
	target, _ := catalog.NewTarget("test", srv.URL)
	e := Engine{Catalog: catalog.Service{Store: catalog.Store{Dir: t.TempDir()}, HTTP: srv.Client()}, HTTP: srv.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scope, err := e.Open(ctx, target, "private-token", catalog.Policy{ForWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	requirement := catalog.Capability{ID: "test", RequiredOperations: []string{"PUT /state", "GET /missing"}}
	got := scope.Require(requirement)
	if got.OK || got.Error.Kind != "capability" || got.Error.Code != 2 || got.Meta.Catalog == nil || *got.Meta.Target != target || got.Meta.HTTPStatus != 0 || len(got.Error.Details.(catalog.CapabilityAssessment).MissingOperations) != 1 {
		t.Fatalf("missing prerequisite was not reported with provenance: %+v", got)
	}
	requirement.RequiredOperations = []string{"GET /state", "PUT /state"}
	if got := scope.Require(requirement); !got.OK {
		t.Fatalf("advertised prerequisites rejected: %+v", got)
	}
	cancel()
	if got := scope.Require(requirement); got.OK || got.Error.Kind != "canceled" {
		t.Fatal("canceled scope assessed capability")
	}
	scope.Close()
	if got := scope.Require(requirement); got.OK || got.Error.Code != 2 {
		t.Fatal("closed scope assessed capability")
	}
	if specs.Load() != 1 || calls.Load() != 0 {
		t.Fatal("capability checks sent an operation or refreshed the shared catalog")
	}
}
