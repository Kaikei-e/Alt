package prometheus_client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeRoute struct {
	status int
	body   string
	delay  time.Duration
}

func newFakeProm(t *testing.T, routes map[string]fakeRoute) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, r := range routes {
		rc := r
		mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
			if rc.delay > 0 {
				select {
				case <-time.After(rc.delay):
				case <-req.Context().Done():
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(rc.status)
			_, _ = w.Write([]byte(rc.body))
		})
	}
	return httptest.NewServer(mux)
}

func TestClient_Query_InstantSuccess(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"alt-backend"},"value":[1700000000,"1"]}]}}`
	srv := newFakeProm(t, map[string]fakeRoute{
		"/api/v1/query": {status: http.StatusOK, body: body},
	})
	defer srv.Close()

	c, err := New(Config{URL: srv.URL, Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	res, err := c.QueryInstant(ctx, `up{job="alt-backend"}`, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatalf("QueryInstant: %v", err)
	}
	if res == nil || len(res.Vector) != 1 {
		t.Fatalf("want 1 sample, got %+v", res)
	}
	if got, want := res.Vector[0].Labels["job"], "alt-backend"; got != want {
		t.Fatalf("label job = %q want %q", got, want)
	}
	if got := res.Vector[0].Value; got != 1 {
		t.Fatalf("value = %v want 1", got)
	}
}

func TestClient_Query_ServerError_Classified(t *testing.T) {
	body := `{"status":"error","errorType":"execution","error":"query timed out"}`
	srv := newFakeProm(t, map[string]fakeRoute{
		"/api/v1/query": {status: http.StatusUnprocessableEntity, body: body},
	})
	defer srv.Close()

	c, _ := New(Config{URL: srv.URL, Timeout: 500 * time.Millisecond})

	_, err := c.QueryInstant(context.Background(), `up`, time.Now())
	if err == nil {
		t.Fatalf("expected error")
	}
	cerr, ok := err.(*QueryError)
	if !ok {
		t.Fatalf("want *QueryError got %T: %v", err, err)
	}
	if cerr.Kind != ErrKindExecution {
		t.Fatalf("Kind = %v want %v", cerr.Kind, ErrKindExecution)
	}
}

func TestClient_Query_BadData_Classified(t *testing.T) {
	body := `{"status":"error","errorType":"bad_data","error":"invalid parameter"}`
	srv := newFakeProm(t, map[string]fakeRoute{
		"/api/v1/query": {status: http.StatusBadRequest, body: body},
	})
	defer srv.Close()
	c, _ := New(Config{URL: srv.URL, Timeout: 500 * time.Millisecond})

	_, err := c.QueryInstant(context.Background(), `up`, time.Now())
	cerr, ok := err.(*QueryError)
	if !ok {
		t.Fatalf("want *QueryError got %T", err)
	}
	if cerr.Kind != ErrKindBadData {
		t.Fatalf("Kind = %v want BadData", cerr.Kind)
	}
}

func TestClient_Query_TimeoutIsClassified(t *testing.T) {
	srv := newFakeProm(t, map[string]fakeRoute{
		"/api/v1/query": {status: http.StatusOK, body: "ignored", delay: 200 * time.Millisecond},
	})
	defer srv.Close()
	c, _ := New(Config{URL: srv.URL, Timeout: 20 * time.Millisecond})

	_, err := c.QueryInstant(context.Background(), `up`, time.Now())
	if err == nil {
		t.Fatalf("expected timeout")
	}
	cerr, ok := err.(*QueryError)
	if !ok {
		t.Fatalf("want *QueryError got %T: %v", err, err)
	}
	if cerr.Kind != ErrKindTimeout {
		t.Fatalf("Kind = %v want Timeout", cerr.Kind)
	}
}

func TestClient_QueryRange_Success(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"job":"alt-backend"},"values":[[1700000000,"0.1"],[1700000015,"0.2"]]}]}}`
	srv := newFakeProm(t, map[string]fakeRoute{
		"/api/v1/query_range": {status: http.StatusOK, body: body},
	})
	defer srv.Close()
	c, _ := New(Config{URL: srv.URL, Timeout: 500 * time.Millisecond})

	start := time.Unix(1700000000, 0)
	end := start.Add(15 * time.Second)
	res, err := c.QueryRange(context.Background(), `up`, start, end, 15*time.Second)
	if err != nil {
		t.Fatalf("QueryRange: %v", err)
	}
	if res == nil || len(res.Matrix) != 1 {
		t.Fatalf("want 1 series")
	}
	if len(res.Matrix[0].Points) != 2 {
		t.Fatalf("want 2 points got %d", len(res.Matrix[0].Points))
	}
	if res.Matrix[0].Points[1].Value != 0.2 {
		t.Fatalf("point[1].Value = %v want 0.2", res.Matrix[0].Points[1].Value)
	}
}

func TestClient_Health_UpAndDown(t *testing.T) {
	up := newFakeProm(t, map[string]fakeRoute{"/-/ready": {status: http.StatusOK, body: "Prometheus is Ready."}})
	defer up.Close()
	c, _ := New(Config{URL: up.URL, Timeout: 200 * time.Millisecond})
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health up: %v", err)
	}

	down := newFakeProm(t, map[string]fakeRoute{"/-/ready": {status: http.StatusServiceUnavailable, body: "not ready"}})
	defer down.Close()
	c2, _ := New(Config{URL: down.URL, Timeout: 200 * time.Millisecond})
	if err := c2.Health(context.Background()); err == nil {
		t.Fatalf("Health down: want error")
	}
}

func TestClient_Query_Warnings_LoggedInResult(t *testing.T) {
	body := `{"status":"success","warnings":["label cardinality high"],"data":{"resultType":"vector","result":[]}}`
	srv := newFakeProm(t, map[string]fakeRoute{
		"/api/v1/query": {status: http.StatusOK, body: body},
	})
	defer srv.Close()
	c, _ := New(Config{URL: srv.URL, Timeout: 500 * time.Millisecond})

	res, err := c.QueryInstant(context.Background(), `up`, time.Now())
	if err != nil {
		t.Fatalf("QueryInstant: %v", err)
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != "label cardinality high" {
		t.Fatalf("warnings = %v", res.Warnings)
	}
}

// ensure JSON response decoder can handle tiny malformed body gracefully
func TestClient_Query_MalformedJSON(t *testing.T) {
	srv := newFakeProm(t, map[string]fakeRoute{
		"/api/v1/query": {status: http.StatusOK, body: `{"status":"success","data":`},
	})
	defer srv.Close()
	c, _ := New(Config{URL: srv.URL, Timeout: 500 * time.Millisecond})

	_, err := c.QueryInstant(context.Background(), `up`, time.Now())
	if err == nil {
		t.Fatalf("want error for malformed body")
	}
}

// sanity: New rejects empty URL
func TestClient_New_EmptyURL(t *testing.T) {
	if _, err := New(Config{URL: ""}); err == nil {
		t.Fatalf("want error")
	}
}

// internal: guard against silent JSON contract drift.
func TestClient_DecodesAllSupportedFields(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"a":"b"},"value":[1700000000.5,"3.14"]}]}}`
	var env promResponse
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatal(err)
	}
	if env.Status != "success" {
		t.Fatalf("status = %q", env.Status)
	}
}
