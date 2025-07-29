package hooks

import (
	"github.com/pocketbase/pocketbase/core"
)

// Register attaches all application hooks and API routes to the Pocketbase instance.
func Register(app core.App) error {
	// Register the API routes
	registerAPIRoutes(app)

	return nil
}