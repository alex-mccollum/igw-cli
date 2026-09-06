package testgateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

func TestQualificationRequiresReviewedCapabilityShapes(t *testing.T) {
	for _, shape := range []string{"both", "neither", "import only", "export only"} {
		paths := map[string]any{}
		for _, route := range []string{"import", "export"} {
			if shape != "both" && shape != route+" only" {
				continue
			}
			method := "get"
			if route == "import" {
				method = "post"
			}
			paths["/data/api/v1/tags/"+route] = map[string]any{method: map[string]any{"responses": map[string]any{"200": map[string]any{"description": "OK"}}}}
		}
		b, _ := json.Marshal(map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": "Shape", "version": "test"}, "paths": paths})
		c, err := catalog.Parse(b)
		if err != nil {
			t.Fatal(err)
		}
		caps, err := workflow.AssessTags(c)
		c.Close()
		if err != nil {
			t.Fatal(err)
		}
		_, err = NewQualification(strings.Repeat("a", 64), caps)
		if shape == "import only" || shape == "export only" {
			if err == nil {
				t.Fatal("partial API silently treated as a qualified shape")
			}
			continue
		}
		if err != nil {
			t.Fatalf("reviewed shape rejected: %s %v", shape, err)
		}
	}
}
