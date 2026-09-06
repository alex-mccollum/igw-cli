package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/resource"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

// Human formatting never changes Data or the JSON result contract. Buffer a
// view before publication so an unknown response shape can fall back intact.
func humanLogs(out io.Writer, r result.Result) error {
	raw, ok := r.Data.(json.RawMessage)
	var page map[string]json.RawMessage
	if !ok || jsonvalue.Validate(raw) != nil || json.Unmarshal(raw, &page) != nil {
		return human(out, r)
	}
	var rows []map[string]json.RawMessage
	if len(page["items"]) == 0 || string(page["items"]) == "null" || json.Unmarshal(page["items"], &rows) != nil {
		return human(out, r)
	}
	var view bytes.Buffer
	for _, row := range rows {
		timestamp, err := strconv.ParseInt(string(row["timestamp"]), 10, 64)
		var level, logger, message *string
		if err != nil || timestamp < 0 || timestamp > 253402300799999 || json.Unmarshal(row["level"], &level) != nil || level == nil || json.Unmarshal(row["loggerName"], &logger) != nil || logger == nil || json.Unmarshal(row["message"], &message) != nil || message == nil {
			return human(out, r)
		}
		fmt.Fprintf(&view, "%s  %s  %s\n", time.UnixMilli(timestamp).UTC().Format(time.RFC3339Nano), terminalText(*level), terminalText(*logger))
		humanLines(&view, *message)
		delete(row, "timestamp")
		delete(row, "level")
		delete(row, "loggerName")
		delete(row, "message")
		if stack, exists := row["stack"]; exists {
			var frames []json.RawMessage
			if string(stack) != "null" && json.Unmarshal(stack, &frames) != nil {
				return human(out, r)
			}
			for _, frame := range frames {
				var text string
				if json.Unmarshal(frame, &text) == nil && string(frame) != "null" {
					humanLines(&view, text)
				} else {
					humanLines(&view, string(frame))
				}
			}
			delete(row, "stack")
		}
		if len(row) > 0 {
			extra, _ := json.Marshal(row)
			fmt.Fprintf(&view, "  Context: %s\n", terminalText(string(extra)))
		}
	}
	if len(rows) == 0 {
		fmt.Fprintln(&view, "No log events on this page.")
	}
	var metadata map[string]json.RawMessage
	if json.Unmarshal(page["metadata"], &metadata) == nil && metadata != nil {
		values := map[string]int64{}
		valid := true
		for _, key := range []string{"total", "matching", "limit", "offset"} {
			value, err := strconv.ParseInt(string(metadata[key]), 10, 64)
			if err != nil || value < 0 {
				valid = false
			}
			values[key] = value
		}
		if valid {
			fmt.Fprintf(&view, "%d events; offset %d, limit %d; %d matching, %d total.\n", len(rows), values["offset"], values["limit"], values["matching"], values["total"])
			// Subtraction avoids overflow with an unexpected large server offset.
			if len(rows) > 0 && values["matching"] > values["offset"] && int64(len(rows)) < values["matching"]-values["offset"] {
				fmt.Fprintf(&view, "Next page: keep the same filters and use --offset %d.\n", values["offset"]+int64(len(rows)))
			}
			for key := range values {
				delete(metadata, key)
			}
			if len(metadata) == 0 {
				delete(page, "metadata")
			} else {
				page["metadata"], _ = json.Marshal(metadata)
			}
		}
	}
	delete(page, "items")
	if len(page) > 0 {
		extra, _ := json.Marshal(page)
		fmt.Fprintf(&view, "Page details: %s\n", terminalText(string(extra)))
	}
	_, err := out.Write(view.Bytes())
	return err
}

func humanDoctor(out io.Writer, r result.Result) error {
	var view bytes.Buffer
	fmt.Fprintln(&view, "Gateway information request succeeded.")
	fmt.Fprintln(&view, "Scope: API connectivity and Gateway information. Check resource state and logs for module, device, or project problems.")
	if err := human(&view, r); err != nil {
		return err
	}
	_, err := out.Write(view.Bytes())
	return err
}

func humanResource(out io.Writer, r result.Result, e resource.Evidence) error {
	var view bytes.Buffer
	fmt.Fprintf(&view, "Resource %s: %s\n", terminalText(e.Action), terminalText(r.Outcome))
	if r.Meta.Target != nil {
		fmt.Fprintf(&view, "Target: %s\n", terminalText(r.Meta.Target.URL))
	}
	fmt.Fprintf(&view, "Type: %s\nCollection: %s\n", terminalText(e.Type), terminalText(e.Collection))
	if e.Singleton {
		fmt.Fprintln(&view, "Identity: singleton")
	} else {
		fmt.Fprintf(&view, "Name: %s\n", terminalText(e.Name))
	}
	if e.State != "" {
		fmt.Fprintf(&view, "State: %s\n", terminalText(e.State))
	}
	if len(e.ChangedFields) > 0 {
		fmt.Fprintf(&view, "Changed fields: %s\n", terminalText(strings.Join(e.ChangedFields, ", ")))
	}
	if e.BeforeSignature != "" {
		fmt.Fprintf(&view, "Reviewed signature: %s\n", terminalText(e.BeforeSignature))
	}
	if e.AfterSignature != "" {
		fmt.Fprintf(&view, "Readback signature: %s\n", terminalText(e.AfterSignature))
	}
	if e.Request != nil {
		fmt.Fprintf(&view, "Request: %s %s\n", terminalText(e.Request.Method), terminalText(e.Request.Path))
		if e.Request.BodyPresent {
			fmt.Fprintf(&view, "Body: %d bytes, SHA-256 %s\n", e.Request.BodyBytes, terminalText(e.Request.BodySHA256))
		}
	}
	if c := e.Checks; c != nil {
		fmt.Fprintf(&view, "Checks: acknowledged=%t, matching changes=%d, valid readback=%t\n", c.Acknowledged, c.MatchingChanges, c.ReadbackValid)
		if c.SignatureMatched != nil {
			fmt.Fprintf(&view, "Signature matched: %t\n", *c.SignatureMatched)
		}
		if c.FieldsMatched != nil {
			fmt.Fprintf(&view, "Fields matched: %t\n", *c.FieldsMatched)
		}
		if len(c.MismatchedFields) > 0 {
			fmt.Fprintf(&view, "Mismatched fields: %s\n", terminalText(strings.Join(c.MismatchedFields, ", ")))
		}
	}
	if r.Outcome == "preview" {
		fmt.Fprintln(&view, "Preview: no proposed mutation was sent. Review the body before adding --yes.")
		if e.Action != "create" {
			fmt.Fprintln(&view, "Pass the reviewed signature with --if-signature when applying this change.")
		}
	} else if r.Meta.Verification != "" {
		fmt.Fprintf(&view, "Verification: %s\n", terminalText(r.Meta.Verification))
	}
	_, err := out.Write(view.Bytes())
	return err
}

// Escape terminal controls from Gateway/user text, retaining readable Unicode.
func terminalText(text string) string {
	var safe strings.Builder
	for _, r := range text {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			quoted := strconv.QuoteRuneToGraphic(r)
			safe.WriteString(quoted[1 : len(quoted)-1])
		} else {
			safe.WriteRune(r)
		}
	}
	return safe.String()
}

func humanLines(out io.Writer, text string) {
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintln(out, "  "+terminalText(line))
	}
}

func recoveryHint(r result.Result) string {
	if r.Outcome == "uncertain" {
		return "Inspect current state and recent logs before deciding on another change; do not automatically retry the write."
	}
	if r.Error == nil {
		return ""
	}
	switch r.Error.Kind {
	case "auth":
		return "Use igw profile show with the same target selection; verify token presence and the full name:key token's permissions on the Gateway."
	case "transport", "timeout":
		return "Check the selected Gateway address, port, TLS, and proxy path with igw profile show; use igw gateway doctor for a read-only connection check."
	case "catalog", "catalog_schema":
		return "Inspect igw spec sync --help and the selected target. Offline references describe APIs but do not establish live state."
	case "capability", "unsupported_input", "validation":
		return "Inspect igw api describe --help for the operation's input contract and igw api capabilities for supported workflow routes."
	case "conflict", "verification", "resource_rejected":
		return "Read the current resource or exported state, review the differences, and prepare a new preview before applying another change."
	case "http":
		return "Inspect the operation contract and igw logs list --min-level WARN --since 1h on the same target."
	case "usage", "config":
		return "Check this command's --help; use igw profile show to inspect the selected configuration without displaying the token."
	}
	return ""
}
