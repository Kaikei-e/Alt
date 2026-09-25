package datahub_capability_gateway

import (
	"reflect"
	"testing"
	"time"

	"alt/domain"
	"alt/shared/driver/alt_db"
)

func TestOutboxEventsFromDriver(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		rows []alt_db.OutboxEvent
		want []domain.OutboxEvent
	}{
		{
			name: "empty rows",
			rows: []alt_db.OutboxEvent{},
			want: []domain.OutboxEvent{},
		},
		{
			name: "single event mapped to processing status",
			rows: []alt_db.OutboxEvent{
				{
					ID:        "evt-1",
					EventType: "ARTICLE_CREATED",
					Payload:   []byte(`{"id":"123"}`),
					CreatedAt: now,
				},
			},
			want: []domain.OutboxEvent{
				{
					ID:        "evt-1",
					EventType: "ARTICLE_CREATED",
					Payload:   []byte(`{"id":"123"}`),
					Status:    domain.OutboxProcessing,
					CreatedAt: now,
				},
			},
		},
		{
			name: "multiple events",
			rows: []alt_db.OutboxEvent{
				{
					ID:        "evt-1",
					EventType: "ARTICLE_CREATED",
					Payload:   []byte(`{"id":"1"}`),
					CreatedAt: now,
				},
				{
					ID:        "evt-2",
					EventType: "TAG_SET_CREATED",
					Payload:   []byte(`{"id":"2"}`),
					CreatedAt: now.Add(time.Minute),
				},
			},
			want: []domain.OutboxEvent{
				{
					ID:        "evt-1",
					EventType: "ARTICLE_CREATED",
					Payload:   []byte(`{"id":"1"}`),
					Status:    domain.OutboxProcessing,
					CreatedAt: now,
				},
				{
					ID:        "evt-2",
					EventType: "TAG_SET_CREATED",
					Payload:   []byte(`{"id":"2"}`),
					Status:    domain.OutboxProcessing,
					CreatedAt: now.Add(time.Minute),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outboxEventsFromDriver(tt.rows)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("outboxEventsFromDriver() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOutboxErrorPtr(t *testing.T) {
	msg := "connection refused"

	tests := []struct {
		name    string
		input   string
		wantNil bool
		wantVal string
	}{
		{
			name:    "empty string returns nil",
			input:   "",
			wantNil: true,
		},
		{
			name:    "non-empty string returns pointer",
			input:   msg,
			wantNil: false,
			wantVal: msg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outboxErrorPtr(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("outboxErrorPtr(%q) = %v, want nil", tt.input, *got)
				}
			} else {
				if got == nil || *got != tt.wantVal {
					t.Errorf("outboxErrorPtr(%q) = %v, want %q", tt.input, got, tt.wantVal)
				}
			}
		})
	}
}
