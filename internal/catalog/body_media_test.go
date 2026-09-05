package catalog

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestBodyCoverageAndInspection(t *testing.T) {
	const raw = `{"openapi":"3.1.0","info":{"title":"Synthetic body coverage","version":"test"},"paths":{"/body":{"post":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object"}},"text/plain":{"schema":{"type":"string"}},"application/xml":{"schema":{"type":"object"}},"application/*":{},"binary stream":{}}},"responses":{"200":{"description":"OK"}}}}}}`
	c, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, tt := range []struct {
		media, body, coverage string
		streaming             bool
	}{
		{"application/json", `{}`, ValidationSchema, false},
		{"text/plain; charset=utf-8", " exact \n日本 ", ValidationSchema, false},
		{"application/zip", "\x00\xff", ValidationTransport, true},
		{"application/xml", "<root/>", "", false},
		{"text/plain; charset=iso-8859-1", "text", "", false},
	} {
		req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tt.body))
		req.Header.Set("Content-Type", tt.media)
		checked, err := c.ValidateRequest("POST /body", req)
		if checked.Coverage != tt.coverage || len(checked.Issues) != 0 || (tt.coverage == "") != errors.Is(err, ErrUnsupportedBodyEncoding) {
			t.Fatalf("coverage for %s: %+v %v", tt.media, checked, err)
		}
		if c.OpaqueUpload("POST /body", tt.media) != tt.streaming {
			t.Fatalf("upload eligibility disagrees with actual media selection: %s", tt.media)
		}
	}
	description, err := c.Describe("POST /body")
	if err != nil || len(description.BodyInputs) != 5 || string(c.Raw()) != raw {
		t.Fatalf("body support inspection: %+v %v", description.BodyInputs, err)
	}
	want := []BodyInput{
		{MediaType: "application/*", Required: true, Encoding: "opaque", Validation: ValidationTransport, Streaming: "supported"},
		{MediaType: "application/json", Required: true, SchemaDeclared: true, Encoding: "json", Validation: ValidationSchema, Streaming: "unsupported"},
		{MediaType: "application/xml", Required: true, SchemaDeclared: true, Encoding: "unsupported", Validation: "unsupported", Streaming: "unsupported"},
		{MediaType: "binary stream", Required: true, Encoding: "unsupported", Validation: "unsupported", Streaming: "unsupported"},
		{MediaType: "text/plain", Required: true, SchemaDeclared: true, Encoding: "utf8", Validation: ValidationSchema, Streaming: "unsupported"},
	}
	for i, input := range want {
		if description.BodyInputs[i] != input {
			t.Fatalf("body input %d: %+v want %+v", i, description.BodyInputs[i], input)
		}
	}
}

func TestNonJSONBodyConstraints(t *testing.T) {
	for _, tt := range []struct {
		name, media, schema, body string
		valid                     bool
	}{
		{"text minimum", "text/plain", `{"type":"string","minLength":2}`, "a", false},
		{"text pattern", "text/plain", `{"type":"string","pattern":"^a"}`, "b", false},
		{"text exact enum", "text/plain", `{"type":"string","enum":[" text + & 日本 \n"]}`, " text + & 日本 \n", true},
		{"text enum mismatch", "text/plain", `{"type":"string","enum":[" text "]}`, "text", false},
		{"text character count", "text/plain", `{"type":"string","maxLength":1}`, "日本", false},
		{"text invalid UTF8", "text/plain", `{"type":"string"}`, "\xff", false},
		{"unsupported XML", "application/xml", `{"type":"object","required":["enabled"]}`, "<root/>", false},
		{"unsupported form", "application/x-www-form-urlencoded", `{"type":"object","required":["enabled"]}`, "name=one", false},
		{"unsupported multipart", "multipart/form-data; boundary=example", `{"type":"object","required":["enabled"]}`, "not multipart", false},
		{"unsupported binary schema", "application/octet-stream", `{"type":"integer"}`, "binary", false},
		{"binary constraints cannot disappear", "application/octet-stream", `{"type":"string","format":"binary","maxLength":1}`, "too long", false},
		{"unconstrained binary", "application/octet-stream", `{}`, "\x00\xff", true},
		{"legacy binary", "application/octet-stream", `{"type":"string","format":"binary"}`, "\x00\xff", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			declared, _, _ := strings.Cut(tt.media, ";")
			c := bodyValueCatalog(t, "3.1.0", tt.schema, declared, true)
			req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.media)
			issues, err := c.Validate("POST /body", req)
			if (len(issues) == 0 && err == nil) != tt.valid {
				t.Fatalf("valid=%t issues=%+v err=%v", tt.valid, issues, err)
			}
		})
	}
}

func TestBinaryBodyInspectionAndUpload(t *testing.T) {
	for _, schema := range []string{`{}`, `{"type":"string","format":"binary"}`} {
		c := bodyValueCatalog(t, "3.1.0", schema, "application/octet-stream", true)
		if !c.OpaqueUpload("POST /body", "application/octet-stream") {
			t.Fatal("unconstrained binary input could not stream")
		}
		description, err := c.Describe("POST /body")
		if err != nil || len(description.BodyInputs) != 1 || !description.BodyInputs[0].SchemaDeclared || description.BodyInputs[0].Encoding != "binary" || description.BodyInputs[0].Validation != ValidationTransport || description.BodyInputs[0].Streaming != "supported" {
			t.Fatalf("binary coverage is not explicit: %+v %v", description.BodyInputs, err)
		}
		req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader("\x00\xff"))
		req.Header.Set("Content-Type", "application/octet-stream")
		checked, err := c.ValidateRequest("POST /body", req)
		if err != nil || len(checked.Issues) != 0 || checked.Coverage != ValidationTransport {
			t.Fatalf("binary validation misreported coverage: %+v %v", checked, err)
		}
	}
	limited := bodyValueCatalog(t, "3.1.0", `{"type":"string","format":"binary","maxLength":1}`, "application/octet-stream", true)
	if limited.OpaqueUpload("POST /body", "application/octet-stream") {
		t.Fatal("streaming discarded a declared binary constraint")
	}
}

func TestBodyDeclarationPresence(t *testing.T) {
	for _, tt := range []struct {
		name, declaration, media, body string
		valid                          bool
	}{
		{"undeclared body", ``, "application/json", `{}`, false},
		{"undeclared empty", ``, "", ``, true},
		{"opaque required empty", `"requestBody":{"required":true,"content":{"application/zip":{}}},`, "application/zip", ``, false},
		{"opaque required present", `"requestBody":{"required":true,"content":{"application/zip":{}}},`, "application/zip", `opaque`, true},
		{"optional body needs media type", `"requestBody":{"content":{"application/json":{"schema":{"type":"object"}}}},`, "", `{}`, false},
		{"optional omitted body", `"requestBody":{"content":{"application/json":{"schema":{"type":"object"}}}},`, "", ``, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"Synthetic body declaration","version":"test"},"paths":{"/body":{"post":{%s"responses":{"200":{"description":"OK"}}}}}}`, tt.declaration)
			c, err := Parse([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.media)
			issues, err := c.Validate("POST /body", req)
			if (len(issues) == 0 && err == nil) != tt.valid {
				t.Fatalf("valid=%t issues=%+v err=%v", tt.valid, issues, err)
			}
		})
	}
}

func TestBodyRangeInspectionRequiresSelectedMedia(t *testing.T) {
	for _, media := range []string{"application/*", "*/*"} {
		c := bodyValueCatalog(t, "3.1.0", `{"type":"string","format":"binary"}`, media, true)
		description, err := c.Describe("POST /body")
		want := BodyInput{MediaType: media, Required: true, SchemaDeclared: true,
			Encoding: "selected_media", Validation: "selected_media", Streaming: "selected_media"}
		if err != nil || len(description.BodyInputs) != 1 || description.BodyInputs[0] != want {
			t.Fatalf("range %s promised a decoder before media selection: %+v %v", media, description.BodyInputs, err)
		}
		for _, tt := range []struct {
			media, body, coverage string
			streaming             bool
		}{
			{"application/json", `"text"`, ValidationSchema, false},
			{"application/octet-stream", "\x00\xff", ValidationTransport, true},
		} {
			req, _ := http.NewRequest("POST", "http://gateway.test/body", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.media)
			checked, err := c.ValidateRequest("POST /body", req)
			if err != nil || len(checked.Issues) != 0 || checked.Coverage != tt.coverage || c.OpaqueUpload("POST /body", tt.media) != tt.streaming {
				t.Fatalf("selected media %s: %+v %v", tt.media, checked, err)
			}
		}
	}
}
