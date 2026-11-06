package doctor

import (
	"context"
	"net/http"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// handleDataQuality serves GET /doctor/maintenance/data-quality. The checks
// only read, so the section is the same in read-only mode.
func handleDataQuality(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rep := services.DataQualityReport{}
		if d.RoDB != nil {
			var err error
			rep, err = services.CheckDataQuality(d.RoDB, services.DataQualityDefaultLimit)
			if err != nil {
				ui.RenderError(w, r, "errors.loadDataQuality", err)
				return
			}
		}
		ui.Render(w, r, DataQualityPartial(rep))
	}
}

// dataQualityTitle returns title, or the localized "(no title)" label when it
// is blank.
func dataQualityTitle(ctx context.Context, title string) string {
	if strings.TrimSpace(title) == "" {
		return tr(ctx, "maintenance.dataQuality.noTitle")
	}
	return title
}
