package datahubapi

import (
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRequiredUUID(t *testing.T) {
	validStr := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	validUUID := uuid.MustParse(validStr)

	tests := []struct {
		name      string
		raw       string
		field     string
		wantID    uuid.UUID
		wantErr   bool
		errorCode connect.Code
	}{
		{
			name:    "valid uuid",
			raw:     validStr,
			field:   "article_id",
			wantID:  validUUID,
			wantErr: false,
		},
		{
			name:      "empty string",
			raw:       "",
			field:     "article_id",
			wantErr:   true,
			errorCode: connect.CodeInvalidArgument,
		},
		{
			name:      "invalid format",
			raw:       "not-a-uuid",
			field:     "user_id",
			wantErr:   true,
			errorCode: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := requiredUUID(tt.raw, tt.field)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("requiredUUID(%q, %q) expected error, got nil", tt.raw, tt.field)
				}
				var connectErr *connect.Error
				if !errors.As(err, &connectErr) {
					t.Fatalf("expected *connect.Error, got %T: %v", err, err)
				}
				if connectErr.Code() != tt.errorCode {
					t.Errorf("error code = %v, want %v", connectErr.Code(), tt.errorCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("requiredUUID(%q, %q) unexpected error: %v", tt.raw, tt.field, err)
			}
			if got != tt.wantID {
				t.Errorf("requiredUUID() = %v, want %v", got, tt.wantID)
			}
		})
	}
}

func TestOptionalUUID(t *testing.T) {
	validStr := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	validUUID := uuid.MustParse(validStr)

	tests := []struct {
		name      string
		raw       string
		field     string
		wantID    *uuid.UUID
		wantErr   bool
		errorCode connect.Code
	}{
		{
			name:    "empty raw string returns nil without error",
			raw:     "",
			field:   "feed_id",
			wantID:  nil,
			wantErr: false,
		},
		{
			name:    "valid uuid returns pointer",
			raw:     validStr,
			field:   "feed_id",
			wantID:  &validUUID,
			wantErr: false,
		},
		{
			name:      "malformed uuid returns invalid argument",
			raw:       "malformed-uuid",
			field:     "feed_id",
			wantID:    nil,
			wantErr:   true,
			errorCode: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := optionalUUID(tt.raw, tt.field)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("optionalUUID(%q, %q) expected error, got nil", tt.raw, tt.field)
				}
				var connectErr *connect.Error
				if !errors.As(err, &connectErr) {
					t.Fatalf("expected *connect.Error, got %T: %v", err, err)
				}
				if connectErr.Code() != tt.errorCode {
					t.Errorf("error code = %v, want %v", connectErr.Code(), tt.errorCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("optionalUUID(%q, %q) unexpected error: %v", tt.raw, tt.field, err)
			}
			if tt.wantID == nil {
				if got != nil {
					t.Errorf("optionalUUID() = %v, want nil", got)
				}
				return
			}
			if got == nil || *got != *tt.wantID {
				t.Errorf("optionalUUID() = %v, want %v", got, tt.wantID)
			}
		})
	}
}

func TestKeysetCursor(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond).UTC()
	ts := timestamppb.New(now)
	validUUIDStr := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	validUUID := uuid.MustParse(validUUIDStr)

	tests := []struct {
		name      string
		ts        *timestamppb.Timestamp
		rawID     string
		wantNil   bool
		wantErr   bool
		errorCode connect.Code
	}{
		{
			name:    "both empty returns nil, nil, nil",
			ts:      nil,
			rawID:   "",
			wantNil: true,
			wantErr: false,
		},
		{
			name:    "both set and valid returns parsed values",
			ts:      ts,
			rawID:   validUUIDStr,
			wantNil: false,
			wantErr: false,
		},
		{
			name:      "timestamp set but id empty returns invalid argument",
			ts:        ts,
			rawID:     "",
			wantNil:   true,
			wantErr:   true,
			errorCode: connect.CodeInvalidArgument,
		},
		{
			name:      "id set but timestamp nil returns invalid argument",
			ts:        nil,
			rawID:     validUUIDStr,
			wantNil:   true,
			wantErr:   true,
			errorCode: connect.CodeInvalidArgument,
		},
		{
			name:      "both set but id invalid returns invalid argument",
			ts:        ts,
			rawID:     "bad-id",
			wantNil:   true,
			wantErr:   true,
			errorCode: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTime, gotID, err := keysetCursor(tt.ts, tt.rawID, "last_created_at", "last_id")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("keysetCursor() expected error, got nil")
				}
				var connectErr *connect.Error
				if !errors.As(err, &connectErr) {
					t.Fatalf("expected *connect.Error, got %T: %v", err, err)
				}
				if connectErr.Code() != tt.errorCode {
					t.Errorf("error code = %v, want %v", connectErr.Code(), tt.errorCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("keysetCursor() unexpected error: %v", err)
			}
			if tt.wantNil {
				if gotTime != nil || gotID != nil {
					t.Errorf("keysetCursor() expected nil outputs, got time=%v, id=%v", gotTime, gotID)
				}
				return
			}
			if gotTime == nil || !gotTime.Equal(now) {
				t.Errorf("keysetCursor() gotTime = %v, want %v", gotTime, now)
			}
			if gotID == nil || *gotID != validUUID {
				t.Errorf("keysetCursor() gotID = %v, want %v", gotID, validUUID)
			}
		})
	}
}
