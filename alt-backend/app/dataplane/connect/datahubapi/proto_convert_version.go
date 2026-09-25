package datahubapi

import (
	"errors"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// Conversions
// ---------------------------------------------------------------------------

func summaryVersionFromProto(msg *datahubv1.SummaryVersion) (domain.SummaryVersion, error) {
	if msg == nil {
		return domain.SummaryVersion{}, connect.NewError(connect.CodeInvalidArgument, errors.New("version is required"))
	}

	versionID, err := requiredUUID(msg.GetSummaryVersionId(), "version.summary_version_id")
	if err != nil {
		return domain.SummaryVersion{}, err
	}
	articleID, err := requiredUUID(msg.GetArticleId(), "version.article_id")
	if err != nil {
		return domain.SummaryVersion{}, err
	}
	userID, err := requiredUUID(msg.GetUserId(), "version.user_id")
	if err != nil {
		return domain.SummaryVersion{}, err
	}
	if msg.GetSummaryText() == "" {
		return domain.SummaryVersion{}, connect.NewError(connect.CodeInvalidArgument, errors.New("version.summary_text is required"))
	}

	sv := domain.SummaryVersion{
		SummaryVersionID: versionID,
		ArticleID:        articleID,
		UserID:           userID,
		GeneratedAt:      msg.GetGeneratedAt().AsTime(),
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
		supersededBy, parseErr := requiredUUID(msg.GetSupersededBy(), "version.superseded_by")
		if parseErr != nil {
			return domain.SummaryVersion{}, parseErr
		}
		sv.SupersededBy = &supersededBy
	}
	return sv, nil
}

func summaryVersionToProto(sv domain.SummaryVersion) *datahubv1.SummaryVersion {
	out := &datahubv1.SummaryVersion{
		SummaryVersionId: sv.SummaryVersionID.String(),
		ArticleId:        sv.ArticleID.String(),
		UserId:           sv.UserID.String(),
		Model:            sv.Model,
		PromptVersion:    sv.PromptVersion,
		InputHash:        sv.InputHash,
		SummaryText:      sv.SummaryText,
	}
	// A zero generated_at stays absent rather than becoming 1970: consumers
	// order versions by it, and an epoch timestamp sorts first.
	if !sv.GeneratedAt.IsZero() {
		out.GeneratedAt = timestamppb.New(sv.GeneratedAt)
	}
	if sv.QualityScore != nil {
		out.QualityScore = sv.QualityScore
	}
	if sv.SupersededBy != nil {
		s := sv.SupersededBy.String()
		out.SupersededBy = &s
	}
	return out
}

func tagSetVersionFromProto(msg *datahubv1.TagSetVersion) (domain.TagSetVersion, error) {
	if msg == nil {
		return domain.TagSetVersion{}, connect.NewError(connect.CodeInvalidArgument, errors.New("version is required"))
	}

	versionID, err := requiredUUID(msg.GetTagSetVersionId(), "version.tag_set_version_id")
	if err != nil {
		return domain.TagSetVersion{}, err
	}
	articleID, err := requiredUUID(msg.GetArticleId(), "version.article_id")
	if err != nil {
		return domain.TagSetVersion{}, err
	}
	userID, err := requiredUUID(msg.GetUserId(), "version.user_id")
	if err != nil {
		return domain.TagSetVersion{}, err
	}

	tsv := domain.TagSetVersion{
		TagSetVersionID: versionID,
		ArticleID:       articleID,
		UserID:          userID,
		GeneratedAt:     msg.GetGeneratedAt().AsTime(),
		Generator:       msg.GetGenerator(),
		InputHash:       msg.GetInputHash(),
		// Stored as the generator wrote it. Not decoded and re-encoded here:
		// the column is jsonb, and a round trip would reorder keys, so a later
		// read would return bytes the generator never produced.
		TagsJSON: msg.GetTagsJson(),
	}
	if msg.SupersededBy != nil {
		supersededBy, parseErr := requiredUUID(msg.GetSupersededBy(), "version.superseded_by")
		if parseErr != nil {
			return domain.TagSetVersion{}, parseErr
		}
		tsv.SupersededBy = &supersededBy
	}
	return tsv, nil
}

func tagSetVersionToProto(tsv domain.TagSetVersion) *datahubv1.TagSetVersion {
	out := &datahubv1.TagSetVersion{
		TagSetVersionId: tsv.TagSetVersionID.String(),
		ArticleId:       tsv.ArticleID.String(),
		UserId:          tsv.UserID.String(),
		Generator:       tsv.Generator,
		InputHash:       tsv.InputHash,
		TagsJson:        tsv.TagsJSON,
	}
	if !tsv.GeneratedAt.IsZero() {
		out.GeneratedAt = timestamppb.New(tsv.GeneratedAt)
	}
	if tsv.SupersededBy != nil {
		s := tsv.SupersededBy.String()
		out.SupersededBy = &s
	}
	return out
}
