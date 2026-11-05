package doctor

import (
	"context"

	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// tr resolves an i18n key in the language of the request being rendered
// (ui.LangFromContext), so the doctor partials need no lang parameter.
func tr(ctx context.Context, key string, args ...string) string {
	return ui.T(ui.LangFromContext(ctx), key, args...)
}
