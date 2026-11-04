package ui_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestLocales_NoStaleCloudUnenrollClaims covers S17: cloud is read-only and
// `engram cloud unenroll <project>` is the supported way to stop replicating
// a project, so no locale may claim the CLI lacks unenroll or that the
// dashboard writes sync_enrolled_projects itself.
func TestLocales_NoStaleCloudUnenrollClaims(t *testing.T) {
	banned := []string{"does not expose", "no expone", "sync_enrolled_projects"}
	for _, file := range []string{"locales/en.json", "locales/es.json"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var tree map[string]any
		if err := json.Unmarshal(raw, &tree); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var walk func(prefix string, v any)
		walk = func(prefix string, v any) {
			switch v := v.(type) {
			case map[string]any:
				for k, child := range v {
					walk(prefix+"."+k, child)
				}
			case string:
				for _, b := range banned {
					if strings.Contains(v, b) {
						t.Errorf("%s%s mentions %q: %q", file, prefix, b, v)
					}
				}
			}
		}
		walk("", tree)
	}
}
