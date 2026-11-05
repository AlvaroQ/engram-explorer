package doctor

import (
	"context"
	"strconv"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// tr resolves an i18n key in the language of the request being rendered
// (ui.LangFromContext), so the doctor partials need no lang parameter.
func tr(ctx context.Context, key string, args ...string) string {
	return ui.T(ui.LangFromContext(ctx), key, args...)
}

// trCount resolves key+"One" when n is 1 and key otherwise, passing n as
// {{count}} (the catalogs have no plural rules; two keys keep both languages
// grammatical).
func trCount(ctx context.Context, key string, n int, args ...string) string {
	if n == 1 {
		key += "One"
	}
	return tr(ctx, key, append([]string{"count", strconv.Itoa(n)}, args...)...)
}
