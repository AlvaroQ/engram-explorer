package doctor

import (
	"context"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// CloudController abstracts the cloud-control operations used by the sync
// handlers.  The production implementation is *services.CloudControlService;
// tests can inject a lightweight fake via Deps.Cloud.
type CloudController interface {
	Enroll(ctx context.Context, project string) (services.CloudResult, error)
	Unenroll(ctx context.Context, project string) (services.CloudResult, error)
	Sync(ctx context.Context, project string) (services.CloudResult, error)
	// Capabilities probes which cloud subcommands the CLI supports so the
	// projects table can gate the Enroll/Unenroll buttons (parity with the
	// React capabilities check).
	Capabilities(ctx context.Context) (services.CloudCapabilities, error)
}
