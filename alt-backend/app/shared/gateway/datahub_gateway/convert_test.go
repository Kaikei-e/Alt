package datahub_gateway

import (
	"math"
	"testing"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestTimeToProtoKeepsUnsetUnset: "no value" and "1970-01-01" mean different
// things to every column these timestamps land in, and a zero time.Time is how
// Go spells the first one.
func TestTimeToProtoKeepsUnsetUnset(t *testing.T) {
	assert.Nil(t, timeToProto(time.Time{}))
	assert.NotNil(t, timeToProto(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)))

	assert.True(t, timeFromProto(nil).IsZero())
	assert.Equal(t,
		time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		timeFromProto(timestamppb.New(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC))).UTC())
}

// TestOutboxStatusRoundTrip: every domain status must survive the wire, and an
// enum value this build does not recognise must not be guessed at. Mapping an
// unknown value to PENDING would hand a row back to the claim loop forever;
// mapping it to PROCESSED would silently declare an undelivered event
// delivered.
func TestOutboxStatusRoundTrip(t *testing.T) {
	for _, status := range []domain.OutboxEventStatus{
		domain.OutboxPending, domain.OutboxProcessing, domain.OutboxProcessed, domain.OutboxFailed,
	} {
		t.Run(string(status), func(t *testing.T) {
			assert.Equal(t, status, outboxStatusFromProto(outboxStatusToProto(status)))
		})
	}

	assert.Equal(t, domain.OutboxEventStatus(""), outboxStatusFromProto(datahubv1.OutboxEventStatus(99)))
	assert.Equal(t,
		datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_UNSPECIFIED,
		outboxStatusToProto(domain.OutboxEventStatus("RETRYING")))
}

func TestOutboxEventFromProtoHandlesNil(t *testing.T) {
	assert.Equal(t, domain.OutboxEvent{}, outboxEventFromProto(nil))
}

func TestArticleHeadFromProtoHandlesNil(t *testing.T) {
	assert.Nil(t, articleHeadFromProto(nil))
}

func TestImageProxyCacheEntryRoundTrip(t *testing.T) {
	entry := &domain.ImageProxyCacheEntry{
		URLHash:     "abc",
		OriginalURL: "https://cdn.example/og.png",
		Data:        []byte{0x52, 0x49, 0x46, 0x46},
		ContentType: "image/webp",
		Width:       600,
		Height:      315,
		SizeBytes:   4,
		ETag:        `W/"x"`,
		ExpiresAt:   time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
	}

	got := imageProxyCacheEntryFromProto(imageProxyCacheEntryToProto(entry))
	require.NotNil(t, got)
	assert.Equal(t, entry.Data, got.Data)
	assert.Equal(t, entry.Width, got.Width)
	assert.Equal(t, entry.SizeBytes, got.SizeBytes)
	assert.Equal(t, entry.ExpiresAt.UTC(), got.ExpiresAt.UTC())
	assert.True(t, got.CreatedAt.IsZero(), "an unset created_at must stay unset, not become the epoch")

	assert.Nil(t, imageProxyCacheEntryFromProto(nil))
	assert.Nil(t, imageProxyCacheEntryToProto(nil))
}

// TestScrapingDomainFromProtoDistinguishesAbsentFromZero is the whole reason
// those fields are `optional` in the proto. robots_crawl_delay_sec = 0 means
// "the publisher asked for no delay"; an absent one means "robots.txt said
// nothing about it", and the crawler treats them differently.
func TestScrapingDomainFromProtoDistinguishesAbsentFromZero(t *testing.T) {
	zero := int32(0)
	msg := &datahubv1.ScrapingDomain{
		Id:                  "2b1c3d4e-5f60-4711-8899-aabbccddeeff",
		Domain:              "example.com",
		RobotsCrawlDelaySec: &zero,
	}

	withZero, err := scrapingDomainFromProto(msg)
	require.NoError(t, err)
	require.NotNil(t, withZero.RobotsCrawlDelaySec)
	assert.Equal(t, 0, *withZero.RobotsCrawlDelaySec)

	msg.RobotsCrawlDelaySec = nil
	withAbsent, err := scrapingDomainFromProto(msg)
	require.NoError(t, err)
	assert.Nil(t, withAbsent.RobotsCrawlDelaySec)
}

// TestScrapingDomainFromProtoNeverReturnsNilDisallowPaths: the driver
// normalises a NULL text[] to an empty slice, and a nil arriving from the wire
// would reintroduce the ambiguity the driver removed.
func TestScrapingDomainFromProtoNeverReturnsNilDisallowPaths(t *testing.T) {
	sd, err := scrapingDomainFromProto(&datahubv1.ScrapingDomain{Domain: "example.com"})
	require.NoError(t, err)
	assert.NotNil(t, sd.RobotsDisallowPaths)
	assert.Empty(t, sd.RobotsDisallowPaths)
}

func TestScrapingDomainFromProtoRejectsMalformedID(t *testing.T) {
	_, err := scrapingDomainFromProto(&datahubv1.ScrapingDomain{Id: "not-a-uuid", Domain: "example.com"})
	require.Error(t, err)
}

func TestScrapingDomainFromProtoHandlesNil(t *testing.T) {
	sd, err := scrapingDomainFromProto(nil)
	require.NoError(t, err)
	assert.Nil(t, sd)
}

// TestScrapingDomainToProtoEmitsEmptyIDForNewRow: an unsaved domain has no id
// yet, and the provider assigns one. The zero UUID is what that looks like on
// this side; the handler treats it as "new" rather than as a row to overwrite.
func TestScrapingDomainToProtoEmitsEmptyIDForNewRow(t *testing.T) {
	msg := scrapingDomainToProto(&domain.ScrapingDomain{Domain: "example.com"})
	require.NotNil(t, msg)
	assert.Equal(t, uuid.Nil.String(), msg.GetId())
	assert.Nil(t, msg.GetCreatedAt())
}

// TestScrapingPolicyUpdateToProtoKeepsAbsentFieldsAbsent: the update is
// applied with COALESCE, so materialising an omitted field as false or 0 would
// reset a policy the caller never mentioned.
func TestScrapingPolicyUpdateToProtoKeepsAbsentFieldsAbsent(t *testing.T) {
	allow := false
	msg := scrapingPolicyUpdateToProto(&domain.ScrapingPolicyUpdate{AllowFetchBody: &allow})

	require.NotNil(t, msg.AllowFetchBody)
	assert.False(t, msg.GetAllowFetchBody())
	assert.Nil(t, msg.AllowMlTraining)
	assert.Nil(t, msg.AllowCacheDays)
	assert.Nil(t, msg.ForceRespectRobots)

	assert.NotNil(t, scrapingPolicyUpdateToProto(nil), "a nil update must become an empty message, not a nil one")
}

func TestParseUUIDTreatsEmptyAsNew(t *testing.T) {
	id, err := parseUUID("")
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, id)

	_, err = parseUUID("nope")
	require.Error(t, err)
}

func TestImageProxyCacheEntryToProto_PixelDimsDoNotWrapOnOverflow(t *testing.T) {
	got := imageProxyCacheEntryToProto(&domain.ImageProxyCacheEntry{
		Width:  math.MaxInt32 + 1,
		Height: math.MaxInt32 + 1,
	})
	require.NotNil(t, got)
	assert.Equal(t, int32(math.MaxInt32), got.Width, "Width must clamp to MaxInt32, not wrap negative")
	assert.Equal(t, int32(math.MaxInt32), got.Height, "Height must clamp to MaxInt32, not wrap negative")
	assert.GreaterOrEqual(t, got.Width, int32(0))
	assert.GreaterOrEqual(t, got.Height, int32(0))
}

// Clamping, not zeroing: 0 means "caching disallowed", so coercing an
// overflowing value to 0 would silently invert the recorded policy.
func TestScrapingDomainToProto_AllowCacheDaysClampsOnOverflow(t *testing.T) {
	got := scrapingDomainToProto(&domain.ScrapingDomain{
		AllowCacheDays: math.MaxInt32 + 1,
	})
	require.NotNil(t, got)
	assert.Equal(t, int32(math.MaxInt32), got.AllowCacheDays, "out-of-range days must clamp, not flip to 'caching disallowed'")
}

func TestScrapingDomainToProto_HTTPStatusOutOfRangeIsOmitted(t *testing.T) {
	overflow := math.MaxInt32 + 1
	got := scrapingDomainToProto(&domain.ScrapingDomain{RobotsTxtLastStatus: &overflow})
	require.NotNil(t, got)
	assert.Nil(t, got.RobotsTxtLastStatus, "overflowing HTTP status must not wrap onto the wire")

	tooBig := 1000
	got = scrapingDomainToProto(&domain.ScrapingDomain{RobotsTxtLastStatus: &tooBig})
	assert.Nil(t, got.RobotsTxtLastStatus, "HTTP status outside 0..999 must be omitted")

	negative := -1
	got = scrapingDomainToProto(&domain.ScrapingDomain{RobotsTxtLastStatus: &negative})
	assert.Nil(t, got.RobotsTxtLastStatus)

	ok := 404
	got = scrapingDomainToProto(&domain.ScrapingDomain{RobotsTxtLastStatus: &ok})
	require.NotNil(t, got.RobotsTxtLastStatus)
	assert.Equal(t, int32(404), *got.RobotsTxtLastStatus)
}

// Clamping, not omitting: an absent optional field reads as "no crawl delay
// known", which would let the crawler speed up exactly when the recorded
// delay was huge.
func TestScrapingDomainToProto_CrawlDelayClampsOnOverflow(t *testing.T) {
	overflow := math.MaxInt32 + 1
	got := scrapingDomainToProto(&domain.ScrapingDomain{RobotsCrawlDelaySec: &overflow})
	require.NotNil(t, got)
	require.NotNil(t, got.RobotsCrawlDelaySec, "overflowing crawl delay must stay on the wire")
	assert.Equal(t, int32(math.MaxInt32), *got.RobotsCrawlDelaySec)

	ok := 10
	got = scrapingDomainToProto(&domain.ScrapingDomain{RobotsCrawlDelaySec: &ok})
	require.NotNil(t, got.RobotsCrawlDelaySec)
	assert.Equal(t, int32(10), *got.RobotsCrawlDelaySec)
}

// Clamping, not omitting: in an update message an absent optional field means
// "do not change", so dropping the field would silently discard the caller's
// requested change while the RPC still reports success.
func TestScrapingPolicyUpdateToProto_AllowCacheDaysClampsOnOverflow(t *testing.T) {
	overflow := math.MaxInt32 + 1
	got := scrapingPolicyUpdateToProto(&domain.ScrapingPolicyUpdate{AllowCacheDays: &overflow})
	require.NotNil(t, got)
	require.NotNil(t, got.AllowCacheDays, "a requested change must never be dropped silently")
	assert.Equal(t, int32(math.MaxInt32), *got.AllowCacheDays)

	ok := 7
	got = scrapingPolicyUpdateToProto(&domain.ScrapingPolicyUpdate{AllowCacheDays: &ok})
	require.NotNil(t, got.AllowCacheDays)
	assert.Equal(t, int32(7), *got.AllowCacheDays)
}
