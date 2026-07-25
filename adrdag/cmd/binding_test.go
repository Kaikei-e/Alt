package cmd

import (
	"encoding/json"
	"testing"
)

func TestBindingListsOnlyBindingADRs(t *testing.T) {
	// binding(A) ⇔ status=accepted ∧ no inbound supersedes edge
	stdout, _, code := run(t, "--adr-dir", fixture("ok"), "binding")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if want := "000003\tThird decision\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestBindingFanInSortedByID(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("fan-in"), "binding")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if want := "000002\tSplit successor A\n000003\tSplit successor B\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestBindingExcludesDriftedAcceptedWithInboundEdge(t *testing.T) {
	// 000001 is status=accepted but has an inbound edge from 000002:
	// it must NOT be listed as binding even though its status says accepted.
	stdout, _, code := run(t, "--adr-dir", fixture("status-drift"), "binding")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if want := "000002\tThe replacement\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestBindingEmptyResultIsSuccess(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("cycle"), "binding")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (empty binding set is not a binding-command failure)", code, exitOK)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestBindingJSON(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("fan-in"), "binding", "--format", "json")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	var docs []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(stdout), &docs); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(docs) != 2 || docs[0].ID != "000002" || docs[1].ID != "000003" {
		t.Errorf("json = %+v, want 000002 then 000003", docs)
	}
	if docs[0].Title != "Split successor A" {
		t.Errorf("title = %q, want %q", docs[0].Title, "Split successor A")
	}
}
