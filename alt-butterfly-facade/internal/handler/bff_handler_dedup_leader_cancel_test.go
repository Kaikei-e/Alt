package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestBFFHandler_Dedup_LeaderCancel_WaiterStillGets200 pins BFF-2: the
// singleflight leader runs the single upstream call and every other concurrent
// caller of the same key shares its result. If that upstream call rode on the
// leader's request context, a leader disconnect (context.Cancel) would cancel
// the in-flight backend request and fail every waiter with 502 — a client that
// never went away is punished for one that did.
//
// The upstream must be decoupled from the leader's cancellation
// (context.WithoutCancel + a server-side deadline), so cancelling the leader
// leaves the shared call running and the waiter still receives the backend's
// 200.
func TestBFFHandler_Dedup_LeaderCancel_WaiterStillGets200(t *testing.T) {
	const endpoint = "/alt.feeds.v2.FeedService/DoDedupThing"
	const okBody = `{"ok":true}`

	received := make(chan struct{}, 1)
	release := make(chan struct{})

	var hits int
	var hitsMu sync.Mutex
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsMu.Lock()
		hits++
		hitsMu.Unlock()
		// Signal that the leader's upstream call reached the backend, then hold
		// the connection open until the test releases it. While blocked here the
		// leader stays inside singleflight.Do, so the waiter joins as a
		// duplicate rather than starting its own upstream call.
		select {
		case received <- struct{}{}:
		default:
		}
		<-release
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(okBody))
	}))
	defer mockBackend.Close()

	secret := []byte("test-secret-key")
	config := BFFConfig{
		EnableDedup: true,
		DedupWindow: 500 * time.Millisecond,
	}
	handler := createTestBFFHandlerWithBackend(t, mockBackend.URL, secret, config)
	token := createTestToken(t, secret)

	// Leader request carries a cancellable context; cancelling it models the
	// leader's HTTP client hanging up mid-flight.
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderReq := httptest.NewRequest("POST", endpoint, nil).WithContext(leaderCtx)
	leaderReq.Header.Set("X-Alt-Backend-Token", token)
	leaderRec := httptest.NewRecorder()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		handler.ServeHTTP(leaderRec, leaderReq)
	}()

	// Wait until the leader's upstream call is in flight at the backend.
	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("leader upstream call never reached the backend")
	}

	// The waiter joins with the same dedup key while the leader is blocked.
	waiterReq := httptest.NewRequest("POST", endpoint, nil)
	waiterReq.Header.Set("X-Alt-Backend-Token", token)
	waiterRec := httptest.NewRecorder()

	wg.Add(1)
	go func() {
		defer wg.Done()
		handler.ServeHTTP(waiterRec, waiterReq)
	}()

	// Give the waiter time to enter singleflight.Do and dedup onto the leader.
	time.Sleep(150 * time.Millisecond)

	// The leader hangs up. With the bug this cancels the shared upstream call.
	cancelLeader()

	// Let the backend finish. In the fixed code the upstream is decoupled from
	// the leader's context, so this response still lands.
	close(release)

	wg.Wait()

	assert.Equal(t, http.StatusOK, waiterRec.Code,
		"a waiter must still receive the backend's 200 after the singleflight leader disconnects")
	assert.Equal(t, okBody, waiterRec.Body.String())

	hitsMu.Lock()
	defer hitsMu.Unlock()
	assert.Equal(t, 1, hits,
		"the leader and waiter must share a single upstream call (singleflight)")
}
