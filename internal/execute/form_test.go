package execute

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
)

func TestFormConstructionBoundsAndOwnership(t *testing.T) {
	for _, fields := range []url.Values{
		{"": {"a"}}, {"name": nil}, {"name": {"\xff"}}, {"\xff": {"a"}},
		{"name": {strings.Repeat("+", catalog.MaxJSONBodyBytes/3)}},
		{"name": make([]string, catalog.MaxFormFields+1)},
	} {
		if _, err := encodeForm(fields); err == nil {
			t.Fatal("invalid or excessive form accepted")
		}
	}
	fields := url.Values{"z": {"a,b", "+%2F 日本"}, "a": {""}}
	e := Engine{}
	target, _ := catalog.NewTarget("test", "http://gateway.test")
	p, err := e.Prepare(context.Background(), target, "", Request{Method: "POST", Path: "/form", Form: fields, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "a=&z=a%2Cb&z=%2B%252F+%E6%97%A5%E6%9C%AC"
	fields["z"][0] = "changed"
	if string(p.input.Body) != want || p.input.Form != nil || p.preview.BodyBytes != int64(len(want)) || p.input.ContentType != "application/x-www-form-urlencoded" {
		t.Fatal("form snapshot changed or has incorrect wire bytes")
	}
	for _, in := range []Request{
		{Form: url.Values{}, Body: []byte{}},
		{Form: url.Values{}, ContentType: "application/json"},
	} {
		if _, err := e.Prepare(context.Background(), target, "", in); err == nil {
			t.Fatal("conflicting typed body input accepted")
		}
	}
	empty, err := e.Prepare(context.Background(), target, "", Request{Method: "POST", Path: "/form", Form: url.Values{}, DryRun: true})
	if err != nil || !empty.preview.BodyPresent || empty.preview.BodyBytes != 0 {
		t.Fatalf("explicit empty form: %+v %v", empty, err)
	}
}
