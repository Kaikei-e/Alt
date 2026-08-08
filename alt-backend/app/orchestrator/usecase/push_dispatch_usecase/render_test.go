package push_dispatch_usecase

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"alt/domain"
)

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("rendered payload is not JSON: %v", err)
	}
	return out
}

func notification(t *testing.T, body []byte) map[string]any {
	t.Helper()
	block, ok := decode(t, body)["notification"].(map[string]any)
	if !ok {
		t.Fatalf("rendered payload has no notification block: %s", body)
	}
	return block
}

func TestRenderNotification_UsesTheDeclarativeEnvelope(t *testing.T) {
	// Safari renders this shape without waking a service worker, and uses it as
	// a fallback if the worker fails to show anything — which is what stops it
	// revoking the site's notification permission. Chrome and Firefox ignore
	// the envelope and read the same JSON in their own push handler, so one
	// payload serves every browser.
	body, err := renderNotification(domain.NotificationKindRecapReady, []byte(`{"url":"/recap"}`))
	if err != nil {
		t.Fatalf("renderNotification: %v", err)
	}

	if got := decode(t, body)["web_push"]; got != float64(8030) {
		t.Fatalf(`web_push = %v, want 8030 (the RFC number Safari keys off)`, got)
	}

	block := notification(t, body)
	if block["navigate"] != "/recap" {
		t.Fatalf("navigate = %v, want /recap", block["navigate"])
	}
	if block["kind"] != domain.NotificationKindRecapReady {
		t.Fatalf("kind = %v, want %s", block["kind"], domain.NotificationKindRecapReady)
	}
	if s, _ := block["title"].(string); s == "" {
		t.Fatal("a notification with no title is one Safari will punish us for")
	}
}

func TestRenderNotification_CarriesACountWhenTheProducerSuppliedOne(t *testing.T) {
	body, err := renderNotification(
		domain.NotificationKindTodayEntranceReady,
		[]byte(`{"url":"/home","count":3}`),
	)
	if err != nil {
		t.Fatalf("renderNotification: %v", err)
	}

	block := notification(t, body)
	text, _ := block["body"].(string)
	if !strings.Contains(text, "3") {
		t.Fatalf("body = %q, want it to mention the count", text)
	}
}

func TestRenderNotification_OmitsTheCountWhenThereIsNone(t *testing.T) {
	body, err := renderNotification(
		domain.NotificationKindTodayEntranceReady,
		[]byte(`{"url":"/home"}`),
	)
	if err != nil {
		t.Fatalf("renderNotification: %v", err)
	}

	if text, _ := notification(t, body)["body"].(string); strings.Contains(text, "0") {
		t.Fatalf("body = %q, want no fabricated zero", text)
	}
}

func TestRenderNotification_FallsBackToTheRootForAMissingURL(t *testing.T) {
	body, err := renderNotification(domain.NotificationKindSummaryReady, []byte(`{}`))
	if err != nil {
		t.Fatalf("renderNotification: %v", err)
	}

	if got := notification(t, body)["navigate"]; got != "/" {
		t.Fatalf("navigate = %v, want /", got)
	}
}

func TestRenderNotification_RefusesAnOffOriginNavigateTarget(t *testing.T) {
	// The payload reaches the device and a tap follows it. The service worker
	// checks the resolved origin too, but a target that could never be right
	// should not be put on the wire in the first place.
	for _, target := range []string{
		"https://evil.example/x",
		"//evil.example/x",
		`/\evil.example/x`,
	} {
		body, err := renderNotification(
			domain.NotificationKindRecapReady,
			[]byte(`{"url":`+strconv.Quote(target)+`}`),
		)
		if err != nil {
			t.Fatalf("%s: renderNotification: %v", target, err)
		}
		if got := notification(t, body)["navigate"]; got != "/" {
			t.Fatalf("%s: navigate = %v, want /", target, got)
		}
	}
}

func TestRenderNotification_StillProducesSomethingDisplayableForJunk(t *testing.T) {
	// A push that displays nothing costs the site its permission on Safari, so
	// a payload we cannot parse must degrade rather than abort the send.
	body, err := renderNotification(domain.NotificationKindSummaryReady, []byte(`not json`))
	if err != nil {
		t.Fatalf("renderNotification must not fail on a bad payload: %v", err)
	}

	block := notification(t, body)
	if s, _ := block["title"].(string); s == "" {
		t.Fatal("expected a generic but displayable title")
	}
	if got := block["navigate"]; got != "/" {
		t.Fatalf("navigate = %v, want /", got)
	}
}

func TestRenderNotification_StaysUnderThePayloadCeiling(t *testing.T) {
	// RFC 8291 leaves at most 3993 octets of plaintext once the aes128gcm
	// header, padding delimiter and AEAD tag are accounted for; Firefox over
	// the FCM bridge is stricter still. Nothing here is user-supplied text, so
	// this is a guard against a future field rather than against input.
	body, err := renderNotification(
		domain.NotificationKindTodayEntranceReady,
		[]byte(`{"url":"/home","count":999999}`),
	)
	if err != nil {
		t.Fatalf("renderNotification: %v", err)
	}
	if len(body) > 2744 {
		t.Fatalf("rendered payload is %d bytes, over the Firefox-via-FCM ceiling", len(body))
	}
}
