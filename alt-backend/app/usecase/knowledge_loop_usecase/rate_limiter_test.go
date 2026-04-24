package knowledge_loop_usecase

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestObserveRateLimiter_AllowsFirstObserveForEntry(t *testing.T) {
	rl := NewLoopRateLimiter(time.Now)
	userID := uuid.New()
	ok, _ := rl.AllowObserve(userID, "default", "article:42", time.Unix(1000, 0))
	require.True(t, ok)
}

func TestObserveRateLimiter_RejectsRepeatWithin60s(t *testing.T) {
	rl := NewLoopRateLimiter(time.Now)
	userID := uuid.New()
	base := time.Unix(1000, 0)

	ok, _ := rl.AllowObserve(userID, "default", "article:42", base)
	require.True(t, ok)

	ok, reason := rl.AllowObserve(userID, "default", "article:42", base.Add(30*time.Second))
	require.False(t, ok)
	require.Equal(t, "observe_throttle", reason,
		"the reason must be distinguishable from the global-ceiling case so BFF can pick its retry-after")
}

func TestObserveRateLimiter_AllowsRepeatAfter60s(t *testing.T) {
	rl := NewLoopRateLimiter(time.Now)
	userID := uuid.New()
	base := time.Unix(1000, 0)

	_, _ = rl.AllowObserve(userID, "default", "article:42", base)
	ok, _ := rl.AllowObserve(userID, "default", "article:42", base.Add(61*time.Second))
	require.True(t, ok, "60s window elapsed — observe must be allowed again")
}

func TestObserveRateLimiter_IndependentKeysDoNotInterfere(t *testing.T) {
	rl := NewLoopRateLimiter(time.Now)
	userID := uuid.New()
	base := time.Unix(1000, 0)

	_, _ = rl.AllowObserve(userID, "default", "article:42", base)

	// different entry_key on the same user/lens: allowed.
	ok, _ := rl.AllowObserve(userID, "default", "article:43", base.Add(1*time.Second))
	require.True(t, ok)

	// different lens on the same user: allowed.
	ok, _ = rl.AllowObserve(userID, "research", "article:42", base.Add(2*time.Second))
	require.True(t, ok)

	// different user, same entry: allowed.
	ok, _ = rl.AllowObserve(uuid.New(), "default", "article:42", base.Add(3*time.Second))
	require.True(t, ok)
}

func TestGlobalLimiter_RejectsAboveGlobalCeiling(t *testing.T) {
	rl := NewLoopRateLimiter(time.Now)
	userID := uuid.New()
	base := time.Unix(1000, 0)

	// Within 1 minute, fire 600 permits for the global bucket. The 601st must be rejected.
	for i := 0; i < 600; i++ {
		ok, _ := rl.AllowGlobal(userID, base.Add(time.Duration(i)*time.Millisecond))
		require.True(t, ok, "unexpected rejection at call %d", i)
	}

	ok, reason := rl.AllowGlobal(userID, base.Add(700*time.Millisecond))
	require.False(t, ok)
	require.Equal(t, "user_global_ceiling", reason)
}

func TestGlobalLimiter_ResetsAcrossWindowBoundary(t *testing.T) {
	rl := NewLoopRateLimiter(time.Now)
	userID := uuid.New()
	base := time.Unix(1000, 0)

	for i := 0; i < 600; i++ {
		rl.AllowGlobal(userID, base.Add(time.Duration(i)*time.Millisecond))
	}
	// Step into the next minute.
	ok, _ := rl.AllowGlobal(userID, base.Add(61*time.Second))
	require.True(t, ok, "new minute window must reset the counter")
}

func TestLoopRateLimiter_AllowObserveAlsoCountsAgainstGlobal(t *testing.T) {
	// When the per-entry observe throttle allows an observe, it must still be
	// counted against the user global ceiling. Otherwise a pathological client
	// can stay under the global limit forever by rotating entry keys.
	rl := NewLoopRateLimiter(time.Now)
	userID := uuid.New()
	base := time.Unix(1000, 0)

	for i := 0; i < 600; i++ {
		entryKey := "article:"
		for _, c := range []byte{byte('a' + i%26), byte('a' + (i/26)%26), byte('a' + (i/676)%26)} {
			entryKey += string(c)
		}
		ok, _ := rl.AllowObserve(userID, "default", entryKey, base.Add(time.Duration(i)*time.Millisecond))
		require.True(t, ok)
	}
	ok, reason := rl.AllowObserve(userID, "default", "article:final", base.Add(700*time.Millisecond))
	require.False(t, ok, "601st distinct-entry observe in the same minute must be rejected by the global ceiling")
	require.Equal(t, "user_global_ceiling", reason)
}
