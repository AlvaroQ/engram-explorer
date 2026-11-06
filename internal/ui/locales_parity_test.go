package ui

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// flattenLocale reads a locale catalog into dotted keys ("overview.kpi.spend")
// mapped to their string values. A value that is neither an object nor a
// string is reported as an error.
func flattenLocale(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := map[string]string{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				walk(prefix+"."+k, child)
			}
		case string:
			out[strings.TrimPrefix(prefix, ".")] = x
		default:
			t.Errorf("%s: key %s has a %T value, want an object or a string", path, strings.TrimPrefix(prefix, "."), v)
		}
	}
	walk("", tree)
	return out
}

// TestLocales_Parity: the server catalogs define the same keys and no empty
// value, so a key added to one language never falls back silently.
func TestLocales_Parity(t *testing.T) {
	en := flattenLocale(t, "locales/en.json")
	es := flattenLocale(t, "locales/es.json")
	var problems []string
	for k, v := range en {
		if _, ok := es[k]; !ok {
			problems = append(problems, "missing in es.json: "+k)
		}
		if strings.TrimSpace(v) == "" {
			problems = append(problems, "empty in en.json: "+k)
		}
	}
	for k, v := range es {
		if _, ok := en[k]; !ok {
			problems = append(problems, "missing in en.json: "+k)
		}
		if strings.TrimSpace(v) == "" {
			problems = append(problems, "empty in es.json: "+k)
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Errorf("%d locale parity problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
	if len(en) == 0 {
		t.Error("no keys read from locales/en.json")
	}
}
