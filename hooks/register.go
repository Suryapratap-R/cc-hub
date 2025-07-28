package hooks

import (
	"github.com/pocketbase/pocketbase/core"
)

// Register attaches all application hooks and API routes to the Pocketbase instance.
func Register(app core.App) error {
	// Register the API routes
	registerAPIRoutes(app)

	// Here you could register other hooks in the future, for example:
	// app.OnRecordBeforeCreateRequest("licenses").Add(func(e *core.RecordCreateEvent) error {
	// 	 // Your logic here
	// 	 return nil
	// })

	return nil
}