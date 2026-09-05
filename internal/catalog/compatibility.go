package catalog

import (
	"sort"
	"strconv"
	"strings"
)

const compatibilityPolicy = "ignition-openapi/5"

// Compatibility describes the model adapter, not a claim that original vendor
// bytes satisfy the OAS schema. Receipts bind this policy to the raw SHA-256.
type Compatibility struct {
	Policy      string         `json:"policy"`
	Adjustments int            `json:"adjustments"`
	Rules       map[string]int `json:"rules"`
}

type Adjustment struct {
	Operation string `json:"operation"`
	Pointer   string `json:"pointer"`
	Rule      string `json:"rule"`
}

// normalizeIgnition handles reviewed structural defects captured from IA's
// 8.3 API generator. Supplied value constraints remain intact. Path presence
// follows the selected template; undocumented parameter values cannot be
// validated. Original bytes and descriptions are kept separately.
func normalizeIgnition(value any) []Adjustment {
	root, _ := value.(map[string]any)
	if !ignitionGenerator(root) {
		return nil
	}
	return normalizeIgnitionOperations(root)
}

func ignitionGenerator(root map[string]any) bool {
	info, _ := root["info"].(map[string]any)
	license, _ := info["license"].(map[string]any)
	// 8.3.0 points to the Gateway's own EULA; later captures use IA's site.
	knownLicense := license["url"] == "https://inductiveautomation.com/ignition/license" ||
		(license["url"] == "/res/sys/license.html" && license["name"] == "Inductive Automation EULA")
	return root["openapi"] == "3.1.0" && info["title"] == "Ignition HTTP API" && knownLicense
}

func keyboardDialect(root map[string]any) bool {
	dialect := root["jsonSchemaDialect"]
	return dialect == nil || dialect == "https://spec.openapis.org/oas/3.1/dialect/base"
}

func normalizeIgnitionOperations(root map[string]any) []Adjustment {
	paths, _ := root["paths"].(map[string]any)
	var adjustments []Adjustment
	for path, rawItem := range paths {
		item, _ := rawItem.(map[string]any)
		for _, method := range []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"} {
			op, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			key := strings.ToUpper(method) + " " + path
			pointer := "/paths/" + pointerEscape(path) + "/" + method
			params, _ := op["parameters"].([]any)
			for index, rawParam := range params {
				param, _ := rawParam.(map[string]any)
				if param["in"] == "path" && param["allowReserved"] == false {
					delete(param, "allowReserved")
					adjustments = append(adjustments, Adjustment{Operation: key, Pointer: pointer + "/parameters/" + strconv.Itoa(index) + "/allowReserved", Rule: "path-allow-reserved-false"})
				}
			}
			adjustments = append(adjustments, normalizeLegacyParameters(op, key, pointer)...)
			if keyboardDialect(root) {
				adjustments = append(adjustments, normalizeKeyboardDefinitions(op, key, pointer)...)
			}
			if responses, ok := op["responses"].(map[string]any); ok && len(responses) == 0 {
				responses["default"] = map[string]any{"description": "Response undocumented by the Gateway; placeholder for parser compatibility only."}
				adjustments = append(adjustments, Adjustment{Operation: key, Pointer: pointer + "/responses", Rule: "empty-responses"})
			}
			if method == "post" || method == "put" {
				adjustments = append(adjustments, normalizeResourceIDs(op, key, path, pointer)...)
			}
		}
	}
	sort.Slice(adjustments, func(i, j int) bool { return adjustments[i].Pointer < adjustments[j].Pointer })
	return adjustments
}

func pointerEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func (c *Catalog) Compatibility() *Compatibility {
	if len(c.adjustments) == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, adjustment := range c.adjustments {
		counts[adjustment.Rule]++
	}
	return &Compatibility{Policy: compatibilityPolicy, Adjustments: len(c.adjustments), Rules: counts}
}

func (c *Catalog) Adjustments() []Adjustment { return append([]Adjustment(nil), c.adjustments...) }
