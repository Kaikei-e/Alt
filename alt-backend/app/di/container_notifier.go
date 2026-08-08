package di

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"time"

	"alt/config"
	"alt/orchestrator/gateway/push_dispatch_gateway"
	"alt/orchestrator/usecase/push_dispatch_usecase"
	"alt/shared/driver/webpush"
	"alt/shared/gateway/datahub_gateway"
)

const (
	// pushClientTimeout bounds a single request to a push service. Left at the
	// zero value an http.Client waits forever, and a wedged connection to one
	// device would hold the whole pass hostage.
	//
	// The dispatcher's own interval, timeout and batch size live beside the job
	// registration in orchestrator/job, not here: registering the job from this
	// file would close an import cycle, so the schedule belongs where the
	// schedule is actually declared. Keeping a second copy here was how they
	// came to disagree in the first place.
	pushClientTimeout = 10 * time.Second
)

// NotifierComponents is cmd/notifier's composition root.
//
// It holds the usecase rather than a scheduler because orchestrator/job already
// imports di — registering the job here would close an import cycle. main wires
// the two together, exactly as cmd/harvester does.
type NotifierComponents struct {
	Dispatch *push_dispatch_usecase.Usecase
}

// NewNotifierComponents wires the dispatcher, or refuses to start.
//
// Every failure below is a panic rather than a degraded mode. A notifier that
// starts without a signing key, or without the data-hub client, is not a
// reduced service — it is a process that reports healthy on its ops listener
// while the queue drains into nothing, which is indistinguishable from "nobody
// enabled notifications". That ambiguity is the four-week outage in
// PM-2026-045, and CLAUDE.md rule 8 exists because of it.
func NewNotifierComponents(ctx context.Context, cfg *config.Config) *NotifierComponents {
	client := newDataHubClient("alt-notifier")

	signer, err := webpush.NewVAPIDSigner(cfg.WebPush.Subject, cfg.WebPush.PrivateKey)
	if err != nil {
		slog.Error("web_push_signer_invalid", "error", err)
		panic("alt-notifier: VAPID signer could not be built — refusing to start with a dispatcher that would 401 on every send: " + err.Error())
	}

	pushClient, err := webpush.NewClient(signer, &http.Client{Timeout: pushClientTimeout})
	if err != nil {
		panic("alt-notifier: web push client could not be built: " + err.Error())
	}

	// The fingerprint, never the key. It is enough to answer "is the running
	// dispatcher signing with the key the browsers subscribed under", which is
	// the question a rotation raises.
	slog.Info("web_push_dispatch_enabled",
		"vapid_public_key_fingerprint", fingerprintVAPIDKey(pushClient.PublicKey()),
		"subject", cfg.WebPush.Subject,
	)

	usecase := push_dispatch_usecase.New(
		push_dispatch_gateway.NewDeliveryGateway(
			datahub_gateway.NewPushDeliveryGateway(client),
			dispatcherIdentity(),
		),
		push_dispatch_gateway.NewSubscriptionGateway(
			datahub_gateway.NewPushSubscriptionGateway(client),
		),
		push_dispatch_gateway.NewSender(pushClient),
	)

	return &NotifierComponents{Dispatch: usecase}
}

// dispatcherIdentity names this instance in the claim, so a lease that never
// clears can be attributed to a container rather than guessed at.
func dispatcherIdentity() string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return "alt-notifier"
}

// fingerprintVAPIDKey turns the public key into something safe to log.
//
// The startup line has to answer one question — is this dispatcher signing with
// the key the browsers subscribed under — because rotating the keypair
// invalidates every existing subscription and there is no server-side migration
// for it (RFC 8292 §4.2). The key itself is not secret, but logging it in full
// makes the line unreadable and invites copy-paste into places it should not be.
func fingerprintVAPIDKey(publicKey string) string {
	sum := sha256.Sum256([]byte(publicKey))
	return hex.EncodeToString(sum[:4])
}
