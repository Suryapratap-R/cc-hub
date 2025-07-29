package hooks

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/pocketbase/pocketbase/core"
)

const (
	licenseChars  = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	licensePrefix = "C1P"
)

// generateSegment creates a single random 3-character string.
func generateSegment() (string, error) {
	segment := make([]byte, 3)
	for i := range segment {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(licenseChars))))
		if err != nil {
			return "", err
		}
		segment[i] = licenseChars[num.Int64()]
	}
	return string(segment), nil
}

// GenerateUniqueKey creates a new C1P-XXX-XXX key and guarantees it's unique in the DB.
func GenerateUniqueKey(app core.App) (string, error) {
	for i := 0; i < 10; i++ { // Safety break after 10 attempts
		seg1, err := generateSegment()
		if err != nil {
			return "", err
		}
		seg2, err := generateSegment()
		if err != nil {
			return "", err
		}

		key := fmt.Sprintf("%s-%s-%s", licensePrefix, seg1, seg2)

		// Check for uniqueness. A "no rows in result set" error is the success case.
		if _, err := app.FindFirstRecordByData("licenses", "key", key); err != nil {
			return key, nil // The key is unique
		}
		
		// Key already exists, we loop again
	}
	return "", fmt.Errorf("failed to generate a unique license key after 10 attempts")
}

// activateDeviceIfNeeded checks the device limit and adds the new device if a slot is available.
// It returns a boolean indicating if the activation was successful, and an error if one occurred.
func activateDeviceIfNeeded(app core.App, license *core.Record, deviceID string) (bool, error) {
	activatedDevices := license.GetStringSlice("activated_devices")

	// Check if device is already activated
	for _, id := range activatedDevices {
		if id == deviceID {
			return true, nil // Already active, success.
		}
	}

	// Device is new, check if there is a free slot
	activationLimit := license.GetInt("activation_limit")
	if len(activatedDevices) >= activationLimit {
		return false, nil // Limit reached, not an error but a business rule failure.
	}

	// Add the new device and save
	activatedDevices = append(activatedDevices, deviceID)
	license.Set("activated_devices", activatedDevices)

	if err := app.Save(license); err != nil {
		return false, err // Database error on save
	}

	return true, nil // Activation successful
}

func GenerateSalt(length int) (string, error) {
    const saltChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    salt := make([]byte, length)
    for i := range salt {
        num, err := rand.Int(rand.Reader, big.NewInt(int64(len(saltChars))))
        if err != nil {
            return "", err
        }
        salt[i] = saltChars[num.Int64()]
    }
    return string(salt), nil
}