package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"knowledge-sovereign/config"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

type principalContextKeyType struct{}

var principalContextKey = principalContextKeyType{}

// Principal identifies the authenticated caller service and its granted capabilities.
type Principal struct {
	Name             string
	AllowedMethods   map[string]bool
	AllowedEvents    map[string]bool
	RequireUserToken bool
	// AllowSystemScope declares residual authority for data-plane replication and
	// background projector maintenance. These scopes are intentionally privileged
	// and MUST NOT be claimed as end-user bindings.
	AllowSystemScope bool
}

// ContextWithPrincipal attaches the authenticated service principal to the context.
func ContextWithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, p)
}

// PrincipalFromContext retrieves the authenticated service principal from the context.
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalContextKey).(*Principal)
	return p, ok
}

type backendClaimsContextKeyType string

const backendClaimsContextKey backendClaimsContextKeyType = "backend_claims"

func ContextWithBackendClaims(ctx context.Context, claims *config.BackendClaims) context.Context {
	return context.WithValue(ctx, backendClaimsContextKey, claims)
}

func BackendClaimsFromContext(ctx context.Context) (*config.BackendClaims, bool) {
	c, ok := ctx.Value(backendClaimsContextKey).(*config.BackendClaims)
	return c, ok
}

// --- Legacy Interceptor (Used when no AuthPolicy is loaded) ---

type eventAuthInterceptor struct {
	token   string
	enabled bool
}

// NewEventAuthInterceptor creates a Connect-RPC interceptor requiring
// Authorization: Bearer <token> on every incoming RPC.
// Pass-through happens only when enabled is false (EVENT_AUTH=disabled).
func NewEventAuthInterceptor(token string, enabled bool) connect.Interceptor {
	return &eventAuthInterceptor{
		token:   token,
		enabled: enabled,
	}
}

func (i *eventAuthInterceptor) checkAuth(header http.Header) error {
	if !i.enabled {
		return nil
	}
	if i.token == "" {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	const prefix = "Bearer "
	auth := header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) ||
		subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, prefix)), []byte(i.token)) != 1 {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	return nil
}

func (i *eventAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if err := i.checkAuth(req.Header()); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

func (i *eventAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *eventAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if err := i.checkAuth(conn.RequestHeader()); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}

// --- Policy Interceptor (Fail-closed per-service credential and capability enforcement) ---

type policyAuthInterceptor struct {
	policy       *config.AuthPolicy
	userVerifier config.TokenVerifier
	enabled      bool
}

// NewPolicyAuthInterceptor creates a Connect-RPC interceptor enforcing fail-closed
// per-service credentials, deny-by-default procedure & event permissions, and
// user delegation token verification (X-Alt-Backend-Token).
func NewPolicyAuthInterceptor(policy *config.AuthPolicy, verifier config.TokenVerifier, enabled bool) connect.Interceptor {
	return &policyAuthInterceptor{
		policy:       policy,
		userVerifier: verifier,
		enabled:      enabled,
	}
}

func (i *policyAuthInterceptor) authenticate(header http.Header) (*Principal, error) {
	if !i.enabled {
		return nil, nil
	}
	if i.policy == nil || len(i.policy.Services) == 0 {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized: policy not configured"))
	}

	const prefix = "Bearer "
	auth := header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized: missing or invalid Bearer token"))
	}
	rawToken := strings.TrimPrefix(auth, prefix)

	// Constant-time matching across all service principals
	var matchedSvc *config.ServicePolicy
	for idx := range i.policy.Services {
		svc := &i.policy.Services[idx]
		if subtle.ConstantTimeCompare([]byte(rawToken), []byte(svc.Token)) == 1 {
			matchedSvc = svc
		}
	}

	if matchedSvc == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized: invalid service credential"))
	}

	return &Principal{
		Name:             matchedSvc.Name,
		AllowedMethods:   matchedSvc.AllowedMethods,
		AllowedEvents:    matchedSvc.AllowedEvents,
		RequireUserToken: matchedSvc.RequireUserToken,
		AllowSystemScope: matchedSvc.AllowSystemScope,
	}, nil
}

func (i *policyAuthInterceptor) authorize(ctx context.Context, principal *Principal, procedure string, header http.Header, msg any) error {
	if !i.enabled || principal == nil {
		return nil
	}

	// 1. Deny-by-default procedure check
	if !principal.AllowedMethods[procedure] {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("service %q is not authorized to call procedure %q", principal.Name, procedure))
	}

	// 2. Request body inspection for event types, global scans, and user delegation.
	//    SECURITY: The default case denies any unrecognized typed message when the
	//    principal has RequireUserToken=true, ensuring new RPC types fail-closed
	//    until explicitly authorized in the switch.
	switch m := msg.(type) {
	case *sovereignv1.AppendKnowledgeEventRequest:
		if m.Event == nil {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("event is required"))
		}
		if !principal.AllowedEvents[m.Event.EventType] {
			return connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("service %q is not authorized to append event type %q", principal.Name, m.Event.EventType))
		}
		// B-02 fix: RequireUserToken ALWAYS enforces user delegation for
		// AppendKnowledgeEvent, regardless of whether UserId is empty.
		// Empty UserId with RequireUserToken means the caller must still
		// present a valid delegation token — the subject becomes the
		// authoritative user identity.
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.Event.UserId, m.Event.TenantId); err != nil {
				return err
			}
		}

	case *sovereignv1.AppendKnowledgeUserEventRequest:
		if m.Event == nil {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("event is required"))
		}
		// B-02 fix: enforce event-type capability allowlist for user events.
		if !principal.AllowedEvents[m.Event.EventType] {
			return connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("service %q is not authorized to append user event type %q", principal.Name, m.Event.EventType))
		}
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.Event.UserId, m.Event.TenantId); err != nil {
				return err
			}
		}

	case *sovereignv1.ListKnowledgeEventsRequest:
		if m.UserId == "" {
			if !principal.AllowSystemScope {
				return connect.NewError(connect.CodePermissionDenied,
					fmt.Errorf("service %q lacks system scope: global event log scan requires system replication scope", principal.Name))
			}
		} else {
			if principal.RequireUserToken {
				if err := i.verifyUserDelegation(ctx, header, m.UserId, m.TenantId); err != nil {
					return err
				}
			}
		}

	case *sovereignv1.GetLatestEventSeqRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, m.TenantId); err != nil {
				return err
			}
		}

	case *sovereignv1.GetKnowledgeHomeItemsRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.GetTodayDigestRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.GetRecallCandidatesRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.GetTrailFootprintsRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.GetTrailBranchesForAnchorRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.ListRecallSignalsRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.AppendRecallSignalRequest:
		if m.Signal == nil {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("signal is required"))
		}
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.Signal.UserId, ""); err != nil {
				return err
			}
		}

	// --- B-02 fix: Mutations require user delegation when RequireUserToken=true ---
	// Mutations carry raw JSON payload without explicit user_id/tenant_id fields.
	// When a principal requires user delegation, the delegation token subject
	// becomes the authoritative user identity for the mutation.
	case *sovereignv1.ApplyProjectionMutationRequest:
		if principal.RequireUserToken {
			if err := i.verifyMutationDelegation(ctx, header, []byte(m.Payload), m.MutationType); err != nil {
				return err
			}
		}

	case *sovereignv1.ApplyRecallMutationRequest:
		if principal.RequireUserToken {
			if err := i.verifyMutationDelegation(ctx, header, []byte(m.Payload), m.MutationType); err != nil {
				return err
			}
		}

	case *sovereignv1.ApplyCurationMutationRequest:
		if principal.RequireUserToken {
			if err := i.verifyMutationDelegation(ctx, header, []byte(m.Payload), m.MutationType); err != nil {
				return err
			}
		}

	// --- B-02 fix: Allow projection state reading for delegated claims ---
	case *sovereignv1.GetActiveProjectionVersionRequest:
		if principal.RequireUserToken {
			if _, ok := BackendClaimsFromContext(ctx); !ok {
				return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or missing user delegation token context"))
			}
		}

	case *sovereignv1.GetProjectionFreshnessRequest:
		if principal.RequireUserToken {
			if _, ok := BackendClaimsFromContext(ctx); !ok {
				return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or missing user delegation token context"))
			}
		}

	// --- B-02 fix: Lens RPCs require user delegation ---
	case *sovereignv1.AreArticlesVisibleInLensRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, m.TenantId); err != nil {
				return err
			}
		}

	case *sovereignv1.ListLensesRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.GetLensRequest:
		// GetLens only carries lens_id (no user_id).
		// Empty user/tenant is passed because ownership is checked in the handler.
		// But verifyUserDelegation will fail on empty expectedUserID now.
		// Instead, we just check that a valid token is present in the context!
		if principal.RequireUserToken {
			if _, ok := BackendClaimsFromContext(ctx); !ok {
				return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or missing user delegation token context"))
			}
		}

	case *sovereignv1.GetCurrentLensSelectionRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.ResolveLensFilterRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.CreateLensRequest:
		if m.Lens == nil {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("lens is required"))
		}
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.Lens.UserId, m.Lens.TenantId); err != nil {
				return err
			}
		}

	case *sovereignv1.CreateLensVersionRequest:
		if m.Version == nil {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("version is required"))
		}
		if principal.RequireUserToken {
			if _, ok := BackendClaimsFromContext(ctx); !ok {
				return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or missing user delegation token context"))
			}
		}

	case *sovereignv1.SelectCurrentLensRequest:
		if m.Selection == nil {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("selection is required"))
		}
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.Selection.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.ClearCurrentLensRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	case *sovereignv1.ArchiveLensRequest:
		if principal.RequireUserToken {
			if _, ok := BackendClaimsFromContext(ctx); !ok {
				return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or missing user delegation token context"))
			}
		}

	// --- B-02 fix: System-scope-only RPCs ---
	case *sovereignv1.ListDistinctUserIDsRequest:
		if !principal.AllowSystemScope {
			return connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("service %q lacks system scope: ListDistinctUserIDs requires system scope", principal.Name))
		}

	case *sovereignv1.CountNeedToKnowItemsRequest:
		if principal.RequireUserToken {
			if err := i.verifyUserDelegation(ctx, header, m.UserId, ""); err != nil {
				return err
			}
		}

	// --- B-02 fix: Streaming RPCs (msg is nil in WrapStreamingHandler) ---
	// WatchProjectorEvents requires system scope since it receives all
	// projector events across tenants. When msg is nil (streaming path),
	// the system scope check is enforced via the nil case below.
	case *sovereignv1.WatchProjectorEventsRequest:
		if !principal.AllowSystemScope {
			return connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("service %q lacks system scope: WatchProjectorEvents requires system scope", principal.Name))
		}

	case nil:
		// Streaming handler passes nil msg. For procedures that require system
		// scope (WatchProjectorEvents), enforce it here based on procedure name.
		if procedure == "/services.sovereign.v1.KnowledgeSovereignService/WatchProjectorEvents" {
			if !principal.AllowSystemScope {
				return connect.NewError(connect.CodePermissionDenied,
					fmt.Errorf("service %q lacks system scope: WatchProjectorEvents requires system scope", principal.Name))
			}
		}

	default:
		// SECURITY: Deny unrecognized typed message when the principal requires
		// user delegation. This ensures new RPC types fail-closed until their
		// authorization shape is explicitly added to this switch.
		if principal.RequireUserToken {
			return connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("service %q: unrecognized request type in authorization — deny by default", principal.Name))
		}
	}

	return nil
}

func (i *policyAuthInterceptor) verifyUserDelegation(ctx context.Context, header http.Header, expectedUserID, expectedTenantID string) error {
	userToken := header.Get("X-Alt-Backend-Token")
	if userToken == "" {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("missing user delegation token (X-Alt-Backend-Token)"))
	}

	claims, ok := BackendClaimsFromContext(ctx)
	if !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or missing user delegation token context"))
	}

	if expectedUserID == "" {
		return connect.NewError(connect.CodePermissionDenied, errors.New("missing explicit user_id in request for user delegation"))
	}
	if claims.Subject != expectedUserID {
		return connect.NewError(connect.CodePermissionDenied, errors.New("delegation token subject/tenant does not match requested user/tenant"))
	}

	if expectedTenantID != "" {
		if claims.TenantID == "" {
			return connect.NewError(connect.CodePermissionDenied, errors.New("delegation token has empty tenant_id but request specifies a tenant"))
		}
		if claims.TenantID != expectedTenantID {
			return connect.NewError(connect.CodePermissionDenied, errors.New("delegation token subject/tenant does not match requested user/tenant"))
		}
	}

	return nil
}

func (i *policyAuthInterceptor) verifyMutationDelegation(ctx context.Context, header http.Header, rawPayload []byte, mutationType string) error {
	userToken := header.Get("X-Alt-Backend-Token")
	if userToken == "" {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("missing user delegation token (X-Alt-Backend-Token)"))
	}

	claims, ok := BackendClaimsFromContext(ctx)
	if !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or missing user delegation token context"))
	}
	if claims.Subject == "" || claims.TenantID == "" {
		return connect.NewError(connect.CodePermissionDenied, errors.New("delegation token missing subject or tenant_id"))
	}

	var rawMap map[string]any
	if err := json.Unmarshal(rawPayload, &rawMap); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid json payload for authorization"))
	}

	var explicitUserID, explicitTenantID string
	var hasExplicitTenantKey bool
	for k, v := range rawMap {
		normKey := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(k, "_", ""), "-", ""))
		if normKey == "tenantid" || normKey == "tenant" {
			hasExplicitTenantKey = true
			if v != nil {
				strVal := strings.TrimSpace(fmt.Sprintf("%v", v))
				if strVal != "" && strVal != "<nil>" {
					explicitTenantID = strVal
				}
			}
		} else if normKey == "userid" || normKey == "user" {
			if v != nil {
				strVal := strings.TrimSpace(fmt.Sprintf("%v", v))
				if strVal != "" && strVal != "<nil>" {
					explicitUserID = strVal
				}
			}
		}
	}

	if mutationType == MutationUpsertHomeItem && (!hasExplicitTenantKey || explicitTenantID == "") {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("missing tenant_id in UpsertHomeItem payload"))
	}

	if explicitUserID != "" && explicitUserID != claims.Subject {
		return connect.NewError(connect.CodePermissionDenied, errors.New("delegation token subject/tenant does not match requested user/tenant"))
	}

	if explicitTenantID != "" && explicitTenantID != claims.TenantID {
		return connect.NewError(connect.CodePermissionDenied, errors.New("delegation token subject/tenant does not match requested user/tenant"))
	}

	return nil
}

func (i *policyAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		principal, err := i.authenticate(req.Header())
		if err != nil {
			return nil, err
		}
		if principal != nil {
			ctx = ContextWithPrincipal(ctx, principal)
			if principal.RequireUserToken {
				userToken := req.Header().Get("X-Alt-Backend-Token")
				if userToken != "" && i.userVerifier != nil {
					if claims, err := i.userVerifier.ValidateToken(ctx, userToken); err == nil {
						ctx = ContextWithBackendClaims(ctx, claims)
					}
				}
			}
		}
		if err := i.authorize(ctx, principal, req.Spec().Procedure, req.Header(), req.Any()); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

func (i *policyAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *policyAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		principal, err := i.authenticate(conn.RequestHeader())
		if err != nil {
			return err
		}
		if principal != nil {
			ctx = ContextWithPrincipal(ctx, principal)
		}
		if err := i.authorize(ctx, principal, conn.Spec().Procedure, conn.RequestHeader(), nil); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}
