package datahub_gateway

import (
	"fmt"

	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"

	"github.com/google/uuid"
)

// The mapping for the two versioned artifacts (capability catalog §2.K).
//
// Both directions parse rather than coerce. A version row is the thing a
// reprojection resolves against, so a malformed id is a failure to report, not
// a uuid.Nil to carry forward: the zero UUID is a valid-looking value that
// would quietly become "some other article's version" the moment it was used
// as a key.

func summaryVersionToProto(sv domain.SummaryVersion) *datahubv1.SummaryVersion {
	out := &datahubv1.SummaryVersion{
		SummaryVersionId: sv.SummaryVersionID.String(),
		ArticleId:        sv.ArticleID.String(),
		UserId:           sv.UserID.String(),
		GeneratedAt:      timeToProto(sv.GeneratedAt),
		Model:            sv.Model,
		PromptVersion:    sv.PromptVersion,
		InputHash:        sv.InputHash,
		SummaryText:      sv.SummaryText,
	}
	// Absent rather than 0.0. A summary that scored zero and one that was never
	// scored are different facts, and the column is nullable for that reason.
	if sv.QualityScore != nil {
		out.QualityScore = sv.QualityScore
	}
	if sv.SupersededBy != nil {
		s := sv.SupersededBy.String()
		out.SupersededBy = &s
	}
	return out
}

func summaryVersionFromProto(msg *datahubv1.SummaryVersion) (domain.SummaryVersion, error) {
	if msg == nil {
		return domain.SummaryVersion{}, fmt.Errorf("summary version: response carried none")
	}

	versionID, err := uuid.Parse(msg.GetSummaryVersionId())
	if err != nil {
		return domain.SummaryVersion{}, fmt.Errorf("summary_version_id %q: %w", msg.GetSummaryVersionId(), err)
	}
	articleID, err := uuid.Parse(msg.GetArticleId())
	if err != nil {
		return domain.SummaryVersion{}, fmt.Errorf("summary version article_id %q: %w", msg.GetArticleId(), err)
	}
	userID, err := uuid.Parse(msg.GetUserId())
	if err != nil {
		return domain.SummaryVersion{}, fmt.Errorf("summary version user_id %q: %w", msg.GetUserId(), err)
	}

	sv := domain.SummaryVersion{
		SummaryVersionID: versionID,
		ArticleID:        articleID,
		UserID:           userID,
		GeneratedAt:      timeFromProto(msg.GetGeneratedAt()),
		Model:            msg.GetModel(),
		PromptVersion:    msg.GetPromptVersion(),
		InputHash:        msg.GetInputHash(),
		SummaryText:      msg.GetSummaryText(),
	}
	if msg.QualityScore != nil {
		score := msg.GetQualityScore()
		sv.QualityScore = &score
	}
	if msg.SupersededBy != nil {
		supersededBy, parseErr := uuid.Parse(msg.GetSupersededBy())
		if parseErr != nil {
			return domain.SummaryVersion{}, fmt.Errorf("summary version superseded_by %q: %w", msg.GetSupersededBy(), parseErr)
		}
		sv.SupersededBy = &supersededBy
	}
	return sv, nil
}

func tagSetVersionToProto(tsv domain.TagSetVersion) *datahubv1.TagSetVersion {
	out := &datahubv1.TagSetVersion{
		TagSetVersionId: tsv.TagSetVersionID.String(),
		ArticleId:       tsv.ArticleID.String(),
		UserId:          tsv.UserID.String(),
		GeneratedAt:     timeToProto(tsv.GeneratedAt),
		Generator:       tsv.Generator,
		InputHash:       tsv.InputHash,
		// Sent as the generator wrote it. json.RawMessage is already bytes;
		// round-tripping it through a decode would reorder keys and change what
		// a later read returns.
		TagsJson: tsv.TagsJSON,
	}
	if tsv.SupersededBy != nil {
		s := tsv.SupersededBy.String()
		out.SupersededBy = &s
	}
	return out
}

func tagSetVersionFromProto(msg *datahubv1.TagSetVersion) (domain.TagSetVersion, error) {
	if msg == nil {
		return domain.TagSetVersion{}, fmt.Errorf("tag set version: response carried none")
	}

	versionID, err := uuid.Parse(msg.GetTagSetVersionId())
	if err != nil {
		return domain.TagSetVersion{}, fmt.Errorf("tag_set_version_id %q: %w", msg.GetTagSetVersionId(), err)
	}
	articleID, err := uuid.Parse(msg.GetArticleId())
	if err != nil {
		return domain.TagSetVersion{}, fmt.Errorf("tag set version article_id %q: %w", msg.GetArticleId(), err)
	}
	userID, err := uuid.Parse(msg.GetUserId())
	if err != nil {
		return domain.TagSetVersion{}, fmt.Errorf("tag set version user_id %q: %w", msg.GetUserId(), err)
	}

	tsv := domain.TagSetVersion{
		TagSetVersionID: versionID,
		ArticleID:       articleID,
		UserID:          userID,
		GeneratedAt:     timeFromProto(msg.GetGeneratedAt()),
		Generator:       msg.GetGenerator(),
		InputHash:       msg.GetInputHash(),
		TagsJSON:        msg.GetTagsJson(),
	}
	if msg.SupersededBy != nil {
		supersededBy, parseErr := uuid.Parse(msg.GetSupersededBy())
		if parseErr != nil {
			return domain.TagSetVersion{}, fmt.Errorf("tag set version superseded_by %q: %w", msg.GetSupersededBy(), parseErr)
		}
		tsv.SupersededBy = &supersededBy
	}
	return tsv, nil
}
