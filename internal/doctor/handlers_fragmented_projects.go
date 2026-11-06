package doctor

import (
	"errors"
	"net/http"

	"github.com/AlvaroQ/engram-explorer/internal/services"
	"github.com/AlvaroQ/engram-explorer/internal/ui"
)

// renderFragmentedProjects loads the suggestion groups and renders the section
// body with an optional result notice or error message.
func renderFragmentedProjects(w http.ResponseWriter, r *http.Request, d Deps, notice, errMsg string) {
	groups := []services.FragmentedGroup{}
	if d.RoDB != nil {
		var err error
		groups, err = services.FindFragmentedProjects(d.RoDB)
		if err != nil {
			ui.RenderError(w, r, "errors.loadFragmentedProjects", err)
			return
		}
	}
	ui.Render(w, r, FragmentedProjectsPartial(groups, d.RWDB != nil, notice, errMsg))
}

// handleFragmentedProjectsList serves GET /doctor/maintenance/fragmented-projects.
func handleFragmentedProjectsList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderFragmentedProjects(w, r, d, "", "")
	}
}

// handleFragmentedProjectsCombine serves POST
// /doctor/maintenance/fragmented-projects/combine (form: source, target).
// It only combines a pair the detection still relates (services.SameFragment), then merges
// with services.RenameProject (mode "merge"), which refuses cloud-enrolled
// projects. Wrapped in writeGuard, so RWDB is non-nil here.
func handleFragmentedProjectsCombine(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			ui.RenderError(w, r, "errors.badRequest", err)
			return
		}
		source, target := r.PostFormValue("source"), r.PostFormValue("target")
		if source == "" || target == "" || source == target || !services.SameFragment(source, target) {
			renderFragmentedProjects(w, r, d, "", trReq(r, "maintenance.fragmented.error.notSuggested"))
			return
		}
		_, err := services.RenameProject(r.Context(), d.RWDB, services.RenameProjectParams{
			Source: source, Target: target, Mode: "merge",
		})
		if err != nil {
			renderFragmentedProjects(w, r, d, "", combineErrorMessage(r, err))
			return
		}
		renderFragmentedProjects(w, r, d,
			trReq(r, "maintenance.fragmented.combined", "source", source, "target", target), "")
	}
}

// combineErrorMessage maps the RenameProject merge codes to the same
// plain-language messages the project page uses.
func combineErrorMessage(r *http.Request, err error) string {
	var we *services.WriteError
	if !errors.As(err, &we) {
		return ui.ErrorMessage(r, "", err)
	}
	key := map[string]string{
		"SOURCE_NOT_FOUND":            "projectDetail.combine.error.sourceNotFound",
		"TARGET_NOT_FOUND":            "projectDetail.combine.error.targetNotFound",
		"ENROLLED_SOURCE_UNSUPPORTED": "projectDetail.combine.error.enrolledSource",
		"ENROLLED_TARGET_UNSUPPORTED": "projectDetail.combine.error.enrolledTarget",
		"SAME_NAME":                   "projectDetail.combine.error.sameName",
	}[we.Code]
	if key == "" {
		return trReq(r, "projectDetail.combine.error.generic", "message", we.Code)
	}
	return trReq(r, key)
}

// trReq resolves key in the language of request r. Handlers must use it
// instead of tr(r.Context(), ...): the request context only carries the
// language once ui.Render wraps it.
func trReq(r *http.Request, key string, args ...string) string {
	return ui.T(ui.LangForRequest(r), key, args...)
}
