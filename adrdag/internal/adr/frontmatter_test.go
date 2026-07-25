package adr

import (
	"reflect"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	cases := []struct {
		name          string
		content       string
		wantFields    map[string]string
		wantLists     map[string][]string
		wantEmptyList map[string]bool
	}{
		{
			name: "scalar fields with quotes stripped",
			content: "---\ntitle: \"Quoted title\"\ndate: 2026-01-01\nstatus: accepted\n---\n# body\n",
			wantFields: map[string]string{"title": "Quoted title", "date": "2026-01-01", "status": "accepted"},
			wantLists:  map[string][]string{},
			wantEmptyList: map[string]bool{},
		},
		{
			name: "inline flow list",
			content: "---\ntitle: t\nstatus: accepted\nsupersedes: [\"000219\", \"000220\", \"000221\"]\n---\nbody",
			wantFields: map[string]string{"title": "t", "status": "accepted"},
			wantLists:  map[string][]string{"supersedes": {"000219", "000220", "000221"}},
			wantEmptyList: map[string]bool{},
		},
		{
			name: "block list",
			content: "---\ntitle: t\nstatus: accepted\nsupersedes:\n  - \"000486\"\n  - \"000488\"\ntags:\n  - backend\n---\nbody",
			wantFields: map[string]string{"title": "t", "status": "accepted"},
			wantLists:  map[string][]string{"supersedes": {"000486", "000488"}, "tags": {"backend"}},
			wantEmptyList: map[string]bool{},
		},
		{
			name: "empty dash stub is flagged",
			content: "---\ntitle: t\nstatus: accepted\nsupersedes:\n  -\n---\nbody",
			wantFields: map[string]string{"title": "t", "status": "accepted"},
			wantLists:  map[string][]string{"supersedes": {}},
			wantEmptyList: map[string]bool{"supersedes": true},
		},
		{
			name: "unquoted scalar containing colon-space and backticks parses line-based",
			content: "---\ntitle: t\nstatus: accepted\naffected_services:\n  - \"compose (rag.yaml) — `setup-uv@v6` を `enable-cache: true` に\"\n---\nbody",
			wantFields: map[string]string{"title": "t", "status": "accepted"},
			wantLists:  map[string][]string{"affected_services": {"compose (rag.yaml) — `setup-uv@v6` を `enable-cache: true` に"}},
			wantEmptyList: map[string]bool{},
		},
		{
			name:          "missing frontmatter yields empty result",
			content:       "# ADR without frontmatter\n",
			wantFields:    map[string]string{},
			wantLists:     map[string][]string{},
			wantEmptyList: map[string]bool{},
		},
		{
			name:          "unterminated frontmatter yields empty result",
			content:       "---\ntitle: t\nstatus: accepted\n",
			wantFields:    map[string]string{},
			wantLists:     map[string][]string{},
			wantEmptyList: map[string]bool{},
		},
		{
			name: "unicode title survives",
			content: "---\ntitle: 推論ワークロードを分離しローカル完結の RAG トポロジを確立する\nstatus: accepted\n---\nbody",
			wantFields: map[string]string{"title": "推論ワークロードを分離しローカル完結の RAG トポロジを確立する", "status": "accepted"},
			wantLists:  map[string][]string{},
			wantEmptyList: map[string]bool{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseFrontmatter(tc.content)
			if !reflect.DeepEqual(got.Fields, tc.wantFields) {
				t.Errorf("Fields = %#v, want %#v", got.Fields, tc.wantFields)
			}
			if !reflect.DeepEqual(got.Lists, tc.wantLists) {
				t.Errorf("Lists = %#v, want %#v", got.Lists, tc.wantLists)
			}
			if !reflect.DeepEqual(got.EmptyListKeys, tc.wantEmptyList) {
				t.Errorf("EmptyListKeys = %#v, want %#v", got.EmptyListKeys, tc.wantEmptyList)
			}
		})
	}
}

func TestNormalizeID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"339", "000339"},
		{"ADR-339", "000339"},
		{"000339", "000339"},
		{"ADR-000339", "000339"},
		{" 42 ", "000042"},
	}
	for _, tc := range cases {
		if got := NormalizeID(tc.in); got != tc.want {
			t.Errorf("NormalizeID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
