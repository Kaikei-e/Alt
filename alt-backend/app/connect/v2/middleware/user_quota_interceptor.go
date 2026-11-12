package middleware

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"alt/config"
	"alt/domain"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
)

type OperationClass string

const (
	OpSummarize   OperationClass = "summarize"
	OpAugurChat   OperationClass = "augur_chat"
	OpMorningChat OperationClass = "morning_chat"
)

var ErrQuotaUnauthenticated = errors.New("authentication required")

type QuotaExceededError struct {
	RetryAfter time.Duration
}

func (e *QuotaExceededError) Error() string {
	return "rate limit exceeded"
}

type userBucket struct {
	userID     uuid.UUID
	tokens     float64
	lastUpdate time.Time
	inFlight   int
}

type operationLimiter struct {
	mu              sync.Mutex
	burst           float64
	refillPerSec    float64
	maxInFlight     int
	maxTrackedUsers int
	buckets         map[uuid.UUID]*list.Element
	lru             *list.List
}

func newOperationLimiter(cfg config.OperationQuotaConfig, maxTrackedUsers int) *operationLimiter {
	return &operationLimiter{
		burst:           float64(cfg.Burst),
		refillPerSec:    float64(cfg.Rate) / cfg.Window.Seconds(),
		maxInFlight:     cfg.MaxInFlight,
		maxTrackedUsers: maxTrackedUsers,
		buckets:         make(map[uuid.UUID]*list.Element),
		lru:             list.New(),
	}
}

func (o *operationLimiter) acquire(userID uuid.UUID, now time.Time) (func(), error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	b := o.getOrCreateBucketLocked(userID, now)
	if elapsed := now.Sub(b.lastUpdate); elapsed > 0 {
		b.tokens = math.Min(o.burst, b.tokens+elapsed.Seconds()*o.refillPerSec)
		b.lastUpdate = now
	}

	if b.inFlight >= o.maxInFlight {
		return nil, &QuotaExceededError{RetryAfter: time.Second}
	}

	if b.tokens < 1.0 {
		waitSec := (1.0 - b.tokens) / o.refillPerSec
		wait := time.Duration(math.Ceil(waitSec * float64(time.Second)))
		if wait < time.Second {
			wait = time.Second
		}
		return nil, &QuotaExceededError{RetryAfter: wait}
	}

	b.tokens -= 1.0
	b.inFlight++

	var once sync.Once
	return func() {
		once.Do(func() {
			o.mu.Lock()
			if b.inFlight > 0 {
				b.inFlight--
			}
			o.evictIdleAboveLocked(o.maxTrackedUsers)
			o.mu.Unlock()
		})
	}, nil
}

func (o *operationLimiter) getOrCreateBucketLocked(userID uuid.UUID, now time.Time) *userBucket {
	if elem, ok := o.buckets[userID]; ok {
		o.lru.MoveToFront(elem)
		return elem.Value.(*userBucket)
	}
	o.evictIdleAboveLocked(o.maxTrackedUsers - 1)
	b := &userBucket{
		userID:     userID,
		tokens:     o.burst,
		lastUpdate: now,
	}
	elem := o.lru.PushFront(b)
	o.buckets[userID] = elem
	return b
}

func (o *operationLimiter) evictIdleAboveLocked(maxLen int) {
	for elem := o.lru.Back(); elem != nil && len(o.buckets) > maxLen; {
		prev := elem.Prev()
		b := elem.Value.(*userBucket)
		if b.inFlight == 0 {
			delete(o.buckets, b.userID)
			o.lru.Remove(elem)
		}
		elem = prev
	}
}

func (o *operationLimiter) trackedUsers() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.buckets)
}

type UserQuotaLimiter struct {
	ops map[OperationClass]*operationLimiter
	now func() time.Time
}

func NewUserQuotaLimiter(cfg config.UserQuotaConfig) (*UserQuotaLimiter, error) {
	if err := config.ValidateUserQuotaConfig(cfg); err != nil {
		return nil, err
	}
	return &UserQuotaLimiter{
		ops: map[OperationClass]*operationLimiter{
			OpSummarize:   newOperationLimiter(cfg.Summarize, cfg.MaxTrackedUsers),
			OpAugurChat:   newOperationLimiter(cfg.AugurChat, cfg.MaxTrackedUsers),
			OpMorningChat: newOperationLimiter(cfg.MorningChat, cfg.MaxTrackedUsers),
		},
		now: time.Now,
	}, nil
}

func (l *UserQuotaLimiter) Acquire(ctx context.Context, op OperationClass) (func(), error) {
	if l == nil {
		panic("user quota limiter is not wired")
	}
	opLimiter, ok := l.ops[op]
	if !ok {
		panic(fmt.Sprintf("unknown user quota operation class: %q", op))
	}
	user, err := domain.GetUserFromContext(ctx)
	if err != nil || user == nil || user.UserID == uuid.Nil {
		return nil, ErrQuotaUnauthenticated
	}
	return opLimiter.acquire(user.UserID, l.now())
}

func (l *UserQuotaLimiter) TrackedUsers(op OperationClass) int {
	opLimiter, ok := l.ops[op]
	if !ok {
		return 0
	}
	return opLimiter.trackedUsers()
}

var connectProcedureClasses = map[string]OperationClass{
	feedsv2connect.FeedServiceStreamSummarizeProcedure:             OpSummarize,
	augurv2connect.AugurServiceStreamChatProcedure:                 OpAugurChat,
	morningletterv2connect.MorningLetterServiceStreamChatProcedure: OpMorningChat,
}

type UserQuotaInterceptor struct {
	limiter *UserQuotaLimiter
}

func NewUserQuotaInterceptor(limiter *UserQuotaLimiter) *UserQuotaInterceptor {
	return &UserQuotaInterceptor{limiter: limiter}
}

func (u *UserQuotaInterceptor) Interceptor() connect.Interceptor {
	return &userQuotaConnectInterceptor{limiter: u.limiter}
}

type userQuotaConnectInterceptor struct {
	limiter *UserQuotaLimiter
}

func (i *userQuotaConnectInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		op, gated := connectProcedureClasses[req.Spec().Procedure]
		if !gated {
			return next(ctx, req)
		}
		release, err := i.limiter.Acquire(ctx, op)
		if err != nil {
			return nil, toConnectQuotaError(err)
		}
		defer release()
		return next(ctx, req)
	}
}

func (i *userQuotaConnectInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *userQuotaConnectInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		op, gated := connectProcedureClasses[conn.Spec().Procedure]
		if !gated {
			return next(ctx, conn)
		}
		release, err := i.limiter.Acquire(ctx, op)
		if err != nil {
			return toConnectQuotaError(err)
		}
		defer release()
		return next(ctx, conn)
	}
}

func toConnectQuotaError(err error) *connect.Error {
	if errors.Is(err, ErrQuotaUnauthenticated) {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	var qErr *QuotaExceededError
	if errors.As(err, &qErr) {
		cErr := connect.NewError(connect.CodeResourceExhausted, errors.New("rate limit exceeded"))
		cErr.Meta().Set("Retry-After", strconv.Itoa(retryAfterSeconds(qErr.RetryAfter)))
		return cErr
	}
	return connect.NewError(connect.CodeResourceExhausted, errors.New("rate limit exceeded"))
}

func EchoUserQuotaMiddleware(limiter *UserQuotaLimiter, op OperationClass) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			release, err := limiter.Acquire(c.Request().Context(), op)
			if err != nil {
				if errors.Is(err, ErrQuotaUnauthenticated) {
					return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
				}
				var qErr *QuotaExceededError
				retrySec := 1
				if errors.As(err, &qErr) {
					retrySec = retryAfterSeconds(qErr.RetryAfter)
				}
				c.Response().Header().Set("Retry-After", strconv.Itoa(retrySec))
				return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			}
			defer release()
			return next(c)
		}
	}
}

func retryAfterSeconds(d time.Duration) int {
	sec := int(math.Ceil(d.Seconds()))
	if sec < 1 {
		return 1
	}
	return sec
}
