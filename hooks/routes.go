package hooks

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/forms"
)

// registerAPIRoutes attaches all our custom API endpoints to the Pocketbase app.
func registerAPIRoutes(app core.App) {
	// The OnServe hook is recommended for attaching routes.
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Grouping routes is a good practice.
		api := e.Router.Group("/api/v1")

		api.POST("/app_check", handleAppCheck(app))
		api.POST("/activate", handleActivate(app))
		api.POST("/request_license", handleRequestLicense(app))

		// Webhook can be registered separately or within the group.
		e.Router.POST("/api/hooks/dodo_purchase", handleDodoPurchase(app))

		return nil
	})
}

// --- HANDLER IMPLEMENTATIONS ---

// handleDodoPurchase has been updated to the new handler signature.
func handleDodoPurchase(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		// In a real app, you MUST verify a secret signature from the webhook header.
		// Example: if e.Request.Header.Get("X-Dodo-Signature") != "YOUR_SECRET" { return apis.NewForbiddenError(...) }

		payload := struct {
			CustomerEmail string `json:"customer_email"`
			CustomerName  string `json:"customer_name"`
			TransactionID string `json:"transaction_id"`
		}{}

		// Use e.BindBody() to parse the request data.
		if err := e.BindBody(&payload); err != nil {
			return apis.NewBadRequestError("Invalid payload", err)
		}

		// Sanitize email
		sanitizedEmail := strings.ToLower(strings.TrimSpace(payload.CustomerEmail))

		userRecord, err := e.App.FindFirstRecordByFilter("users", "email = {:email}", map[string]any{"email": sanitizedEmail})
		if err != nil {
			if err != sql.ErrNoRows {
				return apis.NewApiError(http.StatusInternalServerError, "Database error finding user", err)
			}
			// User doesn't exist, create them
			userCollection, _ := e.App.FindCollectionByNameOrId("users")
			userRecord = core.NewRecord(userCollection)
			userForm := forms.NewRecordUpsert(e.App, userRecord)
			userForm.Load(map[string]any{"email": sanitizedEmail, "name": payload.CustomerName})
			if err := userForm.Submit(); err != nil {
				return apis.NewApiError(http.StatusInternalServerError, "Failed to create user", err)
			}
		}

		// Generate a unique license key
		newKey, err := GenerateUniqueKey(e.App) // Assuming GenerateUniqueKey exists and takes core.App
		if err != nil {
			return apis.NewApiError(http.StatusInternalServerError, "Failed to generate license key", err)
		}

		// Create the license record
		licenseCollection, _ := e.App.FindCollectionByNameOrId("licenses")
		licenseRecord := core.NewRecord(licenseCollection)
		form := forms.NewRecordUpsert(e.App, licenseRecord)
		form.Load(map[string]any{
			"key":         newKey,
			"user":        userRecord.Id,
			"status":      "active",
			"tier":        "pro",
			"purchase_id": payload.TransactionID,
		})

		if err := form.Submit(); err != nil {
			return apis.NewApiError(http.StatusInternalServerError, "Failed to create license", err)
		}

		// Send the email in a goroutine so it doesn't block the webhook response
		go SendLicenseEmail(e.App, sanitizedEmail, payload.CustomerName, newKey) // Assuming SendLicenseEmail exists

		// Use e.NoContent() to send the response.
		return e.NoContent(http.StatusOK)
	}
}

// handleActivate updated to the new handler signature.
func handleActivate(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		payload := struct {
			Email    string `json:"email"`
			Key      string `json:"key"`
			DeviceID string `json:"deviceId"`
		}{}
		if err := e.BindBody(&payload); err != nil {
			return apis.NewBadRequestError("Invalid request body", err)
		}
		if payload.DeviceID == "" {
			return apis.NewBadRequestError("Device ID is required", nil)
		}

		sanitizedEmail := strings.ToLower(strings.TrimSpace(payload.Email))

		// 1. Find the license by key
		license, err := e.App.FindFirstRecordByFilter("licenses", "key = {:key}", map[string]any{"key": payload.Key})
		if err != nil {
			return apis.NewNotFoundError("License not found or invalid.", nil)
		}

		// 2. Validate the user associated with the license
		user, err := e.App.FindRecordById("users", license.GetString("user"))
		if err != nil || user.GetString("email") != sanitizedEmail {
			return apis.NewNotFoundError("License not found or invalid.", nil)
		}

		// 3. Check license status
		if license.GetString("status") != "active" {
			return apis.NewForbiddenError("This license is not active.", nil)
		}

		// 4. Activate the device
		ok, err := activateDeviceIfNeeded(e.App, license, payload.DeviceID) // Assuming this function exists
		if err != nil {
			return apis.NewApiError(http.StatusInternalServerError, "Could not activate device.", err)
		}
		if !ok {
			return apis.NewForbiddenError("Activation limit reached.", nil)
		}

		return e.JSON(http.StatusOK, map[string]string{
			"status": "success",
			"tier":   license.GetString("tier"),
		})
	}
}

// handleAppCheck updated to the new handler signature and correct file URL generation.
func handleAppCheck(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		payload := struct {
			Key                string `json:"key"`
			DeviceID           string `json:"deviceId"`
			CurrentBuildNumber int    `json:"current_build_number"`
		}{}
		if err := e.BindBody(&payload); err != nil {
			return apis.NewBadRequestError("Invalid request body", err)
		}

		// --- Activation Status Check ---
		activationStatus := map[string]string{"status": "free", "tier": "free"}
		if payload.Key != "" {
			license, err := e.App.FindFirstRecordByFilter("licenses", "key = {:key}", map[string]any{"key": payload.Key})
			if err == nil { // License exists
				isValidOnDevice := false
				for _, id := range license.GetStringSlice("activated_devices") {
					if id == payload.DeviceID {
						isValidOnDevice = true
						break
					}
				}

				if license.GetString("status") == "active" && isValidOnDevice {
					activationStatus["status"] = "active"
					activationStatus["tier"] = license.GetString("tier")
					license.Set("last_checked_at", time.Now().UTC().Format(time.RFC3339))
					_ = e.App.Save(license)
				} else {
					activationStatus["status"] = "invalid"
				}
			} else {
				activationStatus["status"] = "invalid"
			}
		}

		baseURL := os.Getenv("PB_PUBLIC_URL") // e.g. https://api.example.com
		if baseURL == "" {
			log.Fatal("PB_PUBLIC_URL env var not set")
		}
		// --- Update Check ---
		var updateInfo map[string]any = nil

		latestVersions, err := app.FindRecordsByFilter(
			"versions",
			"is_published = true AND build_number > {:build}",
			"-build_number", // Sort by build_number descending
			1, 0,
			dbx.Params{"build": payload.CurrentBuildNumber},
		)

		if err == nil && len(latestVersions) > 0 { // A newer version was found
			latestVersion := latestVersions[0]
			minRequiredBuild := latestVersion.GetInt("min_required_build")
			isForceUpdate := false
			if minRequiredBuild > 0 && payload.CurrentBuildNumber < minRequiredBuild {
				isForceUpdate = true
			}

			// example.com/api/files/COLLECTION_ID_OR_NAME/RECORD_ID/FILENAME
			fileUrl := fmt.Sprintf("%s/api/files/%s/%s/%s", baseURL, "versions", latestVersion.Id, latestVersion.GetString("download_url"))

			updateInfo = map[string]any{
				"force_update":    isForceUpdate,
				"version_string":  latestVersion.GetString("version_string"),
				"release_notes":   latestVersion.GetString("release_notes"),
				"download_url":    fileUrl,
				"signature_eddsa": latestVersion.GetString("signature_eddsa"),
			}
		}

		// --- Final Response ---
		return e.JSON(http.StatusOK, map[string]any{
			"activation": activationStatus,
			"update":     updateInfo,
		})
	}
}

// handleRequestLicense updated to the new handler signature.
func handleRequestLicense(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		payload := struct {
			Email string `json:"email"`
		}{}
		if err := e.BindBody(&payload); err != nil {
			return apis.NewBadRequestError("Invalid request body", err)
		}

		sanitizedEmail := strings.ToLower(strings.TrimSpace(payload.Email))

		user, err := e.App.FindFirstRecordByFilter("users", "email = {:email}", map[string]any{"email": sanitizedEmail})
		if err != nil {
			return e.JSON(http.StatusOK, map[string]string{"status": "ok"})
		}

		licenses, err := e.App.FindRecordsByFilter("licenses", "user = {:id}", "-created", 0, 0, map[string]any{"id": user.Id})
		if err != nil || len(licenses) == 0 {
			return e.JSON(http.StatusOK, map[string]string{"status": "ok"})
		}

		var keys []string
		for _, lic := range licenses {
			keys = append(keys, lic.GetString("key"))
		}

		go SendLicenseEmail(e.App, sanitizedEmail, user.GetString("name"), strings.Join(keys, "\n"))

		return e.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}
}
