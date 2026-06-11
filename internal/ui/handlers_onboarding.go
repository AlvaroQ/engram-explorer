package ui

// OnboardingProviderInfo is a view-model for one provider row on the
// onboarding zero-state page.
type OnboardingProviderInfo struct {
	ID          string
	DisplayName string
	// Detected is true when the source was found at its default path at startup.
	Detected bool
	// Path is the detected path (populated only when Detected == true).
	Path string
	// PathError is a validation error from a prior path-entry attempt.
	PathError string
}

// onboardingData bundles everything needed to render the onboarding page.
type onboardingData struct {
	Providers []OnboardingProviderInfo
	Lang      string
	Theme     string
}

// buildOnboardingData constructs the onboarding view-model from Deps.
// It lists only Tier-1 providers (both detected and undetected) so the user can
// choose to activate or enter a path. Tier-2 providers are excluded per spec.
func buildOnboardingData(d Deps, lang, theme string) onboardingData {
	var providers []OnboardingProviderInfo

	if d.Modules != nil {
		for _, m := range d.Modules() {
			if m.Tier != ModuleTier1 {
				continue
			}
			detected := m.State == ModuleDetected || m.State == ModuleEnabled
			providers = append(providers, OnboardingProviderInfo{
				ID:          m.ID,
				DisplayName: m.DisplayName,
				Detected:    detected,
				Path:        m.Path,
			})
		}
	}

	return onboardingData{
		Providers: providers,
		Lang:      lang,
		Theme:     theme,
	}
}
