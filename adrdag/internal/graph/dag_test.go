package graph

import (
	"reflect"
	"testing"
)

func TestBuildReverse(t *testing.T) {
	cases := []struct {
		name string
		in   map[string][]string
		want map[string][]string
	}{
		{
			name: "empty",
			in:   map[string][]string{},
			want: map[string][]string{},
		},
		{
			name: "no edges",
			in:   map[string][]string{"000001": {}, "000002": {}},
			want: map[string][]string{"000001": {}, "000002": {}},
		},
		{
			name: "linear chain",
			in:   map[string][]string{"000001": {}, "000002": {"000001"}, "000003": {"000002"}},
			want: map[string][]string{"000001": {"000002"}, "000002": {"000003"}, "000003": {}},
		},
		{
			name: "fan-in: two successors of one node, ordered by successor id",
			in:   map[string][]string{"000001": {}, "000002": {"000001"}, "000003": {"000001"}},
			want: map[string][]string{"000001": {"000002", "000003"}, "000002": {}, "000003": {}},
		},
		{
			name: "edge target absent from node set still gets a reverse entry",
			in:   map[string][]string{"000002": {"000009"}},
			want: map[string][]string{"000002": {}, "000009": {"000002"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildReverse(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("BuildReverse(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestFindCycle(t *testing.T) {
	cases := []struct {
		name string
		in   map[string][]string
		want []string // nil means no cycle; otherwise a closed path like [A B A]
	}{
		{name: "empty", in: map[string][]string{}, want: nil},
		{name: "linear chain", in: map[string][]string{"000001": {}, "000002": {"000001"}, "000003": {"000002"}}, want: nil},
		{name: "diamond fan-in is acyclic", in: map[string][]string{"000001": {}, "000002": {"000001"}, "000003": {"000001"}, "000004": {"000002", "000003"}}, want: nil},
		{name: "self loop", in: map[string][]string{"000001": {"000001"}}, want: []string{"000001", "000001"}},
		{name: "two cycle", in: map[string][]string{"000001": {"000002"}, "000002": {"000001"}}, want: []string{"000001", "000002", "000001"}},
		{name: "three cycle reports closed path", in: map[string][]string{"000001": {"000002"}, "000002": {"000003"}, "000003": {"000001"}}, want: []string{"000001", "000002", "000003", "000001"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindCycle(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindCycle(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	// reverse maps: old id -> new ids that supersede it
	linear := map[string][]string{"000001": {"000002"}, "000002": {"000003"}, "000003": {}}
	fanIn := map[string][]string{"000001": {"000002", "000003"}, "000002": {}, "000003": {}}
	diamond := map[string][]string{"000001": {"000002", "000003"}, "000002": {"000004"}, "000003": {"000004"}, "000004": {}}
	cyclic := map[string][]string{"000001": {"000002"}, "000002": {"000001"}}

	cases := []struct {
		name    string
		id      string
		reverse map[string][]string
		want    []string
	}{
		{name: "terminal resolves to itself", id: "000003", reverse: linear, want: []string{"000003"}},
		{name: "linear depth-2 walk", id: "000001", reverse: linear, want: []string{"000003"}},
		{name: "fan-in yields both successors", id: "000001", reverse: fanIn, want: []string{"000002", "000003"}},
		{name: "diamond dedups converging leaf", id: "000001", reverse: diamond, want: []string{"000004"}},
		{name: "unknown id resolves to itself", id: "000099", reverse: linear, want: []string{"000099"}},
		{name: "cyclic input terminates", id: "000001", reverse: cyclic, want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.id, tc.reverse)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Resolve(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestIsBinding(t *testing.T) {
	reverse := map[string][]string{"000001": {"000002"}, "000002": {}}
	cases := []struct {
		name   string
		status string
		id     string
		want   bool
	}{
		{name: "accepted with no inbound is binding", status: "accepted", id: "000002", want: true},
		{name: "accepted with inbound edge is not binding", status: "accepted", id: "000001", want: false},
		{name: "proposed with no inbound is not binding", status: "proposed", id: "000002", want: false},
		{name: "superseded with inbound is not binding", status: "superseded", id: "000001", want: false},
		{name: "superseded with no inbound is not binding", status: "superseded", id: "000002", want: false},
		{name: "deprecated is not binding", status: "deprecated", id: "000002", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBinding(tc.status, tc.id, reverse); got != tc.want {
				t.Errorf("IsBinding(%q, %q) = %v, want %v", tc.status, tc.id, got, tc.want)
			}
		})
	}
}
