package hooks

import (
	"fmt"
	"log"
	"net/mail"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/mailer"
)

// SendLicenseEmail sends the welcome/purchase email with the new license key.
// It can also be used for the "Lost License" flow.
func SendLicenseEmail(app core.App, toEmail, toName, key string) {
	htmlBody := fmt.Sprintf(`
		<html>
			<body>
				<h2>Welcome to CursorClip Recorder!</h2>
				<p>Hello %s,</p>
				<p>Thank you for your interest in CursorClip Recorder!</p>
				<p>Your License Key is:</p>
				<pre style="background-color: #f5f5f5; padding: 10px; border-radius: 5px;">%s</pre>
				<p>Best regards,<br>The CursorClip Team</p>
			</body>
		</html>
	`, toName, key)

	message := &mailer.Message{
		From: mail.Address{
			Address: app.Settings().Meta.SenderAddress,
			Name:    app.Settings().Meta.SenderName,
		},
		To: []mail.Address{{
			Address: toEmail,
			Name:    toName,
		}},
		Subject: "Your CursorClip Recorder License Key",
		HTML:    htmlBody,
	}

	if err := app.NewMailClient().Send(message); err != nil {
		// Since this runs in a goroutine, we should log errors.
		log.Printf("ERROR: Failed to send license email to %s: %v", toEmail, err)
	}
}
