package datahubapi

import (
	"reflect"
	"testing"
	"time"

	"alt/dataplane/port/internal_tag_port"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestClampTagLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    int32
		expected int
	}{
		{name: "zero defaults to defaultTagLimit", input: 0, expected: defaultTagLimit},
		{name: "negative defaults to defaultTagLimit", input: -10, expected: defaultTagLimit},
		{name: "valid positive limit preserved", input: 50, expected: 50},
		{name: "exact maxTagLimit preserved", input: maxTagLimit, expected: maxTagLimit},
		{name: "exceeding maxTagLimit clamped to maxTagLimit", input: maxTagLimit + 100, expected: maxTagLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampTagLimit(tt.input)
			if got != tt.expected {
				t.Errorf("clampTagLimit(%d) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTagItemsFromProto(t *testing.T) {
	tests := []struct {
		name     string
		input    []*datahubv1.TagItem
		expected []internal_tag_port.TagItem
	}{
		{
			name:     "empty slice returns empty slice",
			input:    []*datahubv1.TagItem{},
			expected: []internal_tag_port.TagItem{},
		},
		{
			name: "single tag mapping",
			input: []*datahubv1.TagItem{
				{Name: "technology", Confidence: 0.95},
			},
			expected: []internal_tag_port.TagItem{
				{Name: "technology", Confidence: 0.95},
			},
		},
		{
			name: "multiple tags mapping preserves order and values",
			input: []*datahubv1.TagItem{
				{Name: "golang", Confidence: 0.88},
				{Name: "databases", Confidence: 0.72},
				{Name: "architecture", Confidence: 0.99},
			},
			expected: []internal_tag_port.TagItem{
				{Name: "golang", Confidence: 0.88},
				{Name: "databases", Confidence: 0.72},
				{Name: "architecture", Confidence: 0.99},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tagItemsFromProto(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("tagItemsFromProto() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCursorFromProto(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond).UTC()
	validTS := timestamppb.New(now)
	invalidTS := &timestamppb.Timestamp{Nanos: -1} // out-of-range timestamp: IsValid() is false

	tests := []struct {
		name      string
		ts        *timestamppb.Timestamp
		wantNil   bool
		checkTime bool
		wantTime  time.Time
	}{
		{
			name:    "nil timestamp returns nil",
			ts:      nil,
			wantNil: true,
		},
		{
			name:      "valid timestamp returns non-nil time",
			ts:        validTS,
			wantNil:   false,
			checkTime: true,
			wantTime:  now,
		},
		{
			name:    "invalid out-of-range timestamp returns non-nil time converting directly via AsTime",
			ts:      invalidTS,
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cursorFromProto(tt.ts)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("cursorFromProto(%v) = %v, want nil", tt.ts, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("cursorFromProto(%v) = nil, want non-nil", tt.ts)
			}
			if tt.checkTime && !got.Equal(tt.wantTime) {
				t.Errorf("cursorFromProto() = %v, want %v", got, tt.wantTime)
			}
		})
	}
}
