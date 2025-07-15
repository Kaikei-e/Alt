package rest

import (
	"testing"
)

func TestSafeExtractSearchHit(t *testing.T) {
	tests := []struct {
		name    string
		hit     interface{}
		want    SearchArticlesHit
		wantErr bool
	}{
		{
			name: "valid search hit",
			hit: map[string]interface{}{
				"id":      "123",
				"title":   "Test Title",
				"content": "Test Content",
				"tags":    []interface{}{"tag1", "tag2"},
			},
			want: SearchArticlesHit{
				ID:      "123",
				Title:   "Test Title",
				Content: "Test Content",
				Tags:    []string{"tag1", "tag2"},
			},
			wantErr: false,
		},
		{
			name: "missing id field",
			hit: map[string]interface{}{
				"title":   "Test Title",
				"content": "Test Content",
				"tags":    []interface{}{"tag1"},
			},
			want:    SearchArticlesHit{},
			wantErr: true,
		},
		{
			name: "missing title field",
			hit: map[string]interface{}{
				"id":      "123",
				"content": "Test Content",
				"tags":    []interface{}{"tag1"},
			},
			want:    SearchArticlesHit{},
			wantErr: true,
		},
		{
			name: "missing content field",
			hit: map[string]interface{}{
				"id":    "123",
				"title": "Test Title",
				"tags":  []interface{}{"tag1"},
			},
			want:    SearchArticlesHit{},
			wantErr: true,
		},
		{
			name: "id is not string",
			hit: map[string]interface{}{
				"id":      123,
				"title":   "Test Title",
				"content": "Test Content",
				"tags":    []interface{}{"tag1"},
			},
			want:    SearchArticlesHit{},
			wantErr: true,
		},
		{
			name: "title is not string",
			hit: map[string]interface{}{
				"id":      "123",
				"title":   123,
				"content": "Test Content",
				"tags":    []interface{}{"tag1"},
			},
			want:    SearchArticlesHit{},
			wantErr: true,
		},
		{
			name: "content is not string",
			hit: map[string]interface{}{
				"id":      "123",
				"title":   "Test Title",
				"content": 123,
				"tags":    []interface{}{"tag1"},
			},
			want:    SearchArticlesHit{},
			wantErr: true,
		},
		{
			name: "hit is not map",
			hit:  "invalid",
			want: SearchArticlesHit{},
			wantErr: true,
		},
		{
			name: "hit is nil",
			hit:  nil,
			want: SearchArticlesHit{},
			wantErr: true,
		},
		{
			name: "tags field missing (should use empty slice)",
			hit: map[string]interface{}{
				"id":      "123",
				"title":   "Test Title",
				"content": "Test Content",
			},
			want: SearchArticlesHit{
				ID:      "123",
				Title:   "Test Title",
				Content: "Test Content",
				Tags:    nil,
			},
			wantErr: false,
		},
		{
			name: "tags field invalid type (should use empty slice)",
			hit: map[string]interface{}{
				"id":      "123",
				"title":   "Test Title",
				"content": "Test Content",
				"tags":    "invalid",
			},
			want: SearchArticlesHit{
				ID:      "123",
				Title:   "Test Title",
				Content: "Test Content",
				Tags:    nil,
			},
			wantErr: false,
		},
		{
			name: "empty values should be preserved",
			hit: map[string]interface{}{
				"id":      "",
				"title":   "",
				"content": "",
				"tags":    []interface{}{},
			},
			want: SearchArticlesHit{
				ID:      "",
				Title:   "",
				Content: "",
				Tags:    []string{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := safeExtractSearchHit(tt.hit)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("safeExtractSearchHit() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("safeExtractSearchHit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got.ID != tt.want.ID {
				t.Errorf("safeExtractSearchHit() ID = %v, want %v", got.ID, tt.want.ID)
			}

			if got.Title != tt.want.Title {
				t.Errorf("safeExtractSearchHit() Title = %v, want %v", got.Title, tt.want.Title)
			}

			if got.Content != tt.want.Content {
				t.Errorf("safeExtractSearchHit() Content = %v, want %v", got.Content, tt.want.Content)
			}

			if len(got.Tags) != len(tt.want.Tags) {
				t.Errorf("safeExtractSearchHit() Tags length = %v, want %v", len(got.Tags), len(tt.want.Tags))
			} else {
				for i, tag := range got.Tags {
					if tag != tt.want.Tags[i] {
						t.Errorf("safeExtractSearchHit() Tags[%d] = %v, want %v", i, tag, tt.want.Tags[i])
					}
				}
			}
		})
	}
}