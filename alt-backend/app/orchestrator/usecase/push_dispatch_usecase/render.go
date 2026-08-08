package push_dispatch_usecase

import (
	"encoding/json"
	"fmt"
	"strings"

	"alt/domain"
)

// declarativeWebPushVersion is the RFC number Safari keys the envelope off.
// Anything else and it falls back to waking the service worker, losing the
// property this format exists for.
const declarativeWebPushVersion = 8030

// producerPayload is what a producer wrote into its own notification_outbox.
//
// It is deliberately thin. The notification tells the user something is ready
// and where to look; the thing itself is fetched by the app afterwards, over
// an authenticated navigation load. Putting the content here would make the
// push a delivery channel — and would put it on a lock screen.
type producerPayload struct {
	URL   string `json:"url"`
	Count int    `json:"count"`
}

type declarativeNotification struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Navigate string `json:"navigate"`
	// Kind rides along so the service worker can pick the notification `tag`,
	// which collapses an already-displayed notification of the same kind. The
	// server's RFC 8030 Topic header collapses the still-queued ones; neither
	// covers the other's window.
	Kind string `json:"kind"`
}

type declarativeEnvelope struct {
	WebPush      int                     `json:"web_push"`
	Notification declarativeNotification `json:"notification"`
}

var notificationCopy = map[string]struct{ title, body string }{
	domain.NotificationKindSummaryReady: {
		title: "Summary ready",
		body:  "The summary you asked for has finished.",
	},
	domain.NotificationKindAcolyteReportReady: {
		title: "Report ready",
		body:  "The report you asked for has finished.",
	},
	domain.NotificationKindRecapReady: {
		title: "Recap ready",
		body:  "The recap you asked for has finished.",
	},
	domain.NotificationKindTodayEntranceReady: {
		title: "Today's entrance is ready",
		body:  "There is something new to look at.",
	},
}

// safeNavigate keeps a notification tap on this origin.
//
// A prefix check is enough only because it *rejects* rather than accepts: a
// leading backslash is normalised to a slash by WHATWG URL parsing, so `/\host`
// resolves off-origin and has to go. The service worker re-checks the resolved
// origin, but a target that could never be right should not reach the device.
func safeNavigate(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") {
		return "/"
	}
	if strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, `/\`) {
		return "/"
	}
	return raw
}

// renderNotification turns a producer's outbox payload into the body that goes
// on the wire.
//
// It never returns an empty title and never fails on a malformed payload: a
// push that displays nothing costs the site its notification permission on
// Safari outright, and draws Chrome's "updated in the background" notice. So a
// payload we cannot read degrades to the generic copy for its kind.
func renderNotification(kind string, payload []byte) ([]byte, error) {
	text, known := notificationCopy[kind]
	if !known {
		text = struct{ title, body string }{
			title: "Alt",
			body:  "Something is ready.",
		}
	}

	var parsed producerPayload
	// A parse failure is not fatal — see above. The zero value gives the
	// generic copy and a root navigate target, which is displayable.
	_ = json.Unmarshal(payload, &parsed)

	body := text.body
	if parsed.Count > 0 {
		body = fmt.Sprintf("%d new since yesterday.", parsed.Count)
	}

	return json.Marshal(declarativeEnvelope{
		WebPush: declarativeWebPushVersion,
		Notification: declarativeNotification{
			Title:    text.title,
			Body:     body,
			Navigate: safeNavigate(parsed.URL),
			Kind:     kind,
		},
	})
}
