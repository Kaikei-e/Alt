import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SummaryVersioning(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SUMMARY_VERSIONING_UNSPECIFIED: _ClassVar[SummaryVersioning]
    SUMMARY_VERSIONING_SKIP: _ClassVar[SummaryVersioning]

class OutboxEventStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    OUTBOX_EVENT_STATUS_UNSPECIFIED: _ClassVar[OutboxEventStatus]
    OUTBOX_EVENT_STATUS_PENDING: _ClassVar[OutboxEventStatus]
    OUTBOX_EVENT_STATUS_PROCESSING: _ClassVar[OutboxEventStatus]
    OUTBOX_EVENT_STATUS_PROCESSED: _ClassVar[OutboxEventStatus]
    OUTBOX_EVENT_STATUS_FAILED: _ClassVar[OutboxEventStatus]

class FeedScope(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    FEED_SCOPE_UNSPECIFIED: _ClassVar[FeedScope]
    FEED_SCOPE_ALL: _ClassVar[FeedScope]
    FEED_SCOPE_UNREAD: _ClassVar[FeedScope]
    FEED_SCOPE_READ: _ClassVar[FeedScope]
    FEED_SCOPE_FAVORITE: _ClassVar[FeedScope]

class TrendWindow(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    TREND_WINDOW_UNSPECIFIED: _ClassVar[TrendWindow]
    TREND_WINDOW_4H: _ClassVar[TrendWindow]
    TREND_WINDOW_24H: _ClassVar[TrendWindow]
    TREND_WINDOW_3D: _ClassVar[TrendWindow]
    TREND_WINDOW_7D: _ClassVar[TrendWindow]

class TrendGranularity(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    TREND_GRANULARITY_UNSPECIFIED: _ClassVar[TrendGranularity]
    TREND_GRANULARITY_HOURLY: _ClassVar[TrendGranularity]
    TREND_GRANULARITY_DAILY: _ClassVar[TrendGranularity]

class NotificationState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    NOTIFICATION_STATE_UNSPECIFIED: _ClassVar[NotificationState]
    NOTIFICATION_STATE_PENDING: _ClassVar[NotificationState]
    NOTIFICATION_STATE_SENDING: _ClassVar[NotificationState]
    NOTIFICATION_STATE_SENT: _ClassVar[NotificationState]
    NOTIFICATION_STATE_DEAD: _ClassVar[NotificationState]
    NOTIFICATION_STATE_EXPIRED: _ClassVar[NotificationState]
SUMMARY_VERSIONING_UNSPECIFIED: SummaryVersioning
SUMMARY_VERSIONING_SKIP: SummaryVersioning
OUTBOX_EVENT_STATUS_UNSPECIFIED: OutboxEventStatus
OUTBOX_EVENT_STATUS_PENDING: OutboxEventStatus
OUTBOX_EVENT_STATUS_PROCESSING: OutboxEventStatus
OUTBOX_EVENT_STATUS_PROCESSED: OutboxEventStatus
OUTBOX_EVENT_STATUS_FAILED: OutboxEventStatus
FEED_SCOPE_UNSPECIFIED: FeedScope
FEED_SCOPE_ALL: FeedScope
FEED_SCOPE_UNREAD: FeedScope
FEED_SCOPE_READ: FeedScope
FEED_SCOPE_FAVORITE: FeedScope
TREND_WINDOW_UNSPECIFIED: TrendWindow
TREND_WINDOW_4H: TrendWindow
TREND_WINDOW_24H: TrendWindow
TREND_WINDOW_3D: TrendWindow
TREND_WINDOW_7D: TrendWindow
TREND_GRANULARITY_UNSPECIFIED: TrendGranularity
TREND_GRANULARITY_HOURLY: TrendGranularity
TREND_GRANULARITY_DAILY: TrendGranularity
NOTIFICATION_STATE_UNSPECIFIED: NotificationState
NOTIFICATION_STATE_PENDING: NotificationState
NOTIFICATION_STATE_SENDING: NotificationState
NOTIFICATION_STATE_SENT: NotificationState
NOTIFICATION_STATE_DEAD: NotificationState
NOTIFICATION_STATE_EXPIRED: NotificationState

class ArticleWithTags(_message.Message):
    __slots__ = ("id", "title", "content", "tags", "created_at", "user_id", "feed_id", "language", "published_at")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    TAGS_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    id: str
    title: str
    content: str
    tags: _containers.RepeatedScalarFieldContainer[str]
    created_at: _timestamp_pb2.Timestamp
    user_id: str
    feed_id: str
    language: str
    published_at: _timestamp_pb2.Timestamp
    def __init__(self, id: _Optional[str] = ..., title: _Optional[str] = ..., content: _Optional[str] = ..., tags: _Optional[_Iterable[str]] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., user_id: _Optional[str] = ..., feed_id: _Optional[str] = ..., language: _Optional[str] = ..., published_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class DeletedArticle(_message.Message):
    __slots__ = ("id", "deleted_at")
    ID_FIELD_NUMBER: _ClassVar[int]
    DELETED_AT_FIELD_NUMBER: _ClassVar[int]
    id: str
    deleted_at: _timestamp_pb2.Timestamp
    def __init__(self, id: _Optional[str] = ..., deleted_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ListArticlesWithTagsRequest(_message.Message):
    __slots__ = ("last_created_at", "last_id", "limit")
    LAST_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_ID_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    last_created_at: _timestamp_pb2.Timestamp
    last_id: str
    limit: int
    def __init__(self, last_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_id: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class ListArticlesWithTagsResponse(_message.Message):
    __slots__ = ("articles", "next_created_at", "next_id")
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    NEXT_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    NEXT_ID_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[ArticleWithTags]
    next_created_at: _timestamp_pb2.Timestamp
    next_id: str
    def __init__(self, articles: _Optional[_Iterable[_Union[ArticleWithTags, _Mapping]]] = ..., next_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., next_id: _Optional[str] = ...) -> None: ...

class ListArticlesWithTagsForwardRequest(_message.Message):
    __slots__ = ("incremental_mark", "last_created_at", "last_id", "limit")
    INCREMENTAL_MARK_FIELD_NUMBER: _ClassVar[int]
    LAST_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_ID_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    incremental_mark: _timestamp_pb2.Timestamp
    last_created_at: _timestamp_pb2.Timestamp
    last_id: str
    limit: int
    def __init__(self, incremental_mark: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_id: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class ListArticlesWithTagsForwardResponse(_message.Message):
    __slots__ = ("articles", "next_created_at", "next_id")
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    NEXT_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    NEXT_ID_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[ArticleWithTags]
    next_created_at: _timestamp_pb2.Timestamp
    next_id: str
    def __init__(self, articles: _Optional[_Iterable[_Union[ArticleWithTags, _Mapping]]] = ..., next_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., next_id: _Optional[str] = ...) -> None: ...

class ListDeletedArticlesRequest(_message.Message):
    __slots__ = ("last_deleted_at", "limit")
    LAST_DELETED_AT_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    last_deleted_at: _timestamp_pb2.Timestamp
    limit: int
    def __init__(self, last_deleted_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., limit: _Optional[int] = ...) -> None: ...

class ListDeletedArticlesResponse(_message.Message):
    __slots__ = ("articles", "next_deleted_at")
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    NEXT_DELETED_AT_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[DeletedArticle]
    next_deleted_at: _timestamp_pb2.Timestamp
    def __init__(self, articles: _Optional[_Iterable[_Union[DeletedArticle, _Mapping]]] = ..., next_deleted_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class GetLatestArticleTimestampRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetLatestArticleTimestampResponse(_message.Message):
    __slots__ = ("latest_created_at",)
    LATEST_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    latest_created_at: _timestamp_pb2.Timestamp
    def __init__(self, latest_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class GetArticleByIDRequest(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class GetArticleByIDResponse(_message.Message):
    __slots__ = ("article",)
    ARTICLE_FIELD_NUMBER: _ClassVar[int]
    article: ArticleWithTags
    def __init__(self, article: _Optional[_Union[ArticleWithTags, _Mapping]] = ...) -> None: ...

class CheckArticleExistsRequest(_message.Message):
    __slots__ = ("url", "feed_id")
    URL_FIELD_NUMBER: _ClassVar[int]
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    url: str
    feed_id: str
    def __init__(self, url: _Optional[str] = ..., feed_id: _Optional[str] = ...) -> None: ...

class CheckArticleExistsResponse(_message.Message):
    __slots__ = ("exists", "article_id")
    EXISTS_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    exists: bool
    article_id: str
    def __init__(self, exists: _Optional[bool] = ..., article_id: _Optional[str] = ...) -> None: ...

class CreateArticleRequest(_message.Message):
    __slots__ = ("title", "url", "content", "feed_id", "user_id", "published_at", "language")
    TITLE_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    title: str
    url: str
    content: str
    feed_id: str
    user_id: str
    published_at: _timestamp_pb2.Timestamp
    language: str
    def __init__(self, title: _Optional[str] = ..., url: _Optional[str] = ..., content: _Optional[str] = ..., feed_id: _Optional[str] = ..., user_id: _Optional[str] = ..., published_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., language: _Optional[str] = ...) -> None: ...

class CreateArticleResponse(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class SaveArticleSummaryRequest(_message.Message):
    __slots__ = ("article_id", "summary", "language", "user_id", "article_title", "summary_versioning")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_TITLE_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_VERSIONING_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    summary: str
    language: str
    user_id: str
    article_title: str
    summary_versioning: SummaryVersioning
    def __init__(self, article_id: _Optional[str] = ..., summary: _Optional[str] = ..., language: _Optional[str] = ..., user_id: _Optional[str] = ..., article_title: _Optional[str] = ..., summary_versioning: _Optional[_Union[SummaryVersioning, str]] = ...) -> None: ...

class SaveArticleSummaryResponse(_message.Message):
    __slots__ = ("success",)
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    success: bool
    def __init__(self, success: _Optional[bool] = ...) -> None: ...

class GetArticleContentRequest(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class GetArticleContentResponse(_message.Message):
    __slots__ = ("article_id", "title", "content", "url", "user_id")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    title: str
    content: str
    url: str
    user_id: str
    def __init__(self, article_id: _Optional[str] = ..., title: _Optional[str] = ..., content: _Optional[str] = ..., url: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class GetFeedIDRequest(_message.Message):
    __slots__ = ("feed_url",)
    FEED_URL_FIELD_NUMBER: _ClassVar[int]
    feed_url: str
    def __init__(self, feed_url: _Optional[str] = ...) -> None: ...

class GetFeedIDResponse(_message.Message):
    __slots__ = ("feed_id",)
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    feed_id: str
    def __init__(self, feed_id: _Optional[str] = ...) -> None: ...

class ListFeedURLsRequest(_message.Message):
    __slots__ = ("cursor", "limit")
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    cursor: str
    limit: int
    def __init__(self, cursor: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class ListFeedURLsResponse(_message.Message):
    __slots__ = ("feeds", "next_cursor", "has_more")
    FEEDS_FIELD_NUMBER: _ClassVar[int]
    NEXT_CURSOR_FIELD_NUMBER: _ClassVar[int]
    HAS_MORE_FIELD_NUMBER: _ClassVar[int]
    feeds: _containers.RepeatedCompositeFieldContainer[FeedURL]
    next_cursor: str
    has_more: bool
    def __init__(self, feeds: _Optional[_Iterable[_Union[FeedURL, _Mapping]]] = ..., next_cursor: _Optional[str] = ..., has_more: _Optional[bool] = ...) -> None: ...

class FeedURL(_message.Message):
    __slots__ = ("feed_id", "url")
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    feed_id: str
    url: str
    def __init__(self, feed_id: _Optional[str] = ..., url: _Optional[str] = ...) -> None: ...

class UpsertArticleTagsRequest(_message.Message):
    __slots__ = ("article_id", "feed_id", "tags")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    TAGS_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    feed_id: str
    tags: _containers.RepeatedCompositeFieldContainer[TagItem]
    def __init__(self, article_id: _Optional[str] = ..., feed_id: _Optional[str] = ..., tags: _Optional[_Iterable[_Union[TagItem, _Mapping]]] = ...) -> None: ...

class TagItem(_message.Message):
    __slots__ = ("name", "confidence")
    NAME_FIELD_NUMBER: _ClassVar[int]
    CONFIDENCE_FIELD_NUMBER: _ClassVar[int]
    name: str
    confidence: float
    def __init__(self, name: _Optional[str] = ..., confidence: _Optional[float] = ...) -> None: ...

class UpsertArticleTagsResponse(_message.Message):
    __slots__ = ("success", "upserted_count")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    UPSERTED_COUNT_FIELD_NUMBER: _ClassVar[int]
    success: bool
    upserted_count: int
    def __init__(self, success: _Optional[bool] = ..., upserted_count: _Optional[int] = ...) -> None: ...

class BatchUpsertArticleTagsRequest(_message.Message):
    __slots__ = ("items",)
    ITEMS_FIELD_NUMBER: _ClassVar[int]
    items: _containers.RepeatedCompositeFieldContainer[UpsertArticleTagsRequest]
    def __init__(self, items: _Optional[_Iterable[_Union[UpsertArticleTagsRequest, _Mapping]]] = ...) -> None: ...

class BatchUpsertArticleTagsResponse(_message.Message):
    __slots__ = ("success", "total_upserted")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    TOTAL_UPSERTED_FIELD_NUMBER: _ClassVar[int]
    success: bool
    total_upserted: int
    def __init__(self, success: _Optional[bool] = ..., total_upserted: _Optional[int] = ...) -> None: ...

class ListUntaggedArticlesRequest(_message.Message):
    __slots__ = ("limit", "offset", "last_created_at", "last_id")
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    OFFSET_FIELD_NUMBER: _ClassVar[int]
    LAST_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_ID_FIELD_NUMBER: _ClassVar[int]
    limit: int
    offset: int
    last_created_at: _timestamp_pb2.Timestamp
    last_id: str
    def __init__(self, limit: _Optional[int] = ..., offset: _Optional[int] = ..., last_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_id: _Optional[str] = ...) -> None: ...

class ListUntaggedArticlesResponse(_message.Message):
    __slots__ = ("articles", "total_count", "next_created_at", "next_id")
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    TOTAL_COUNT_FIELD_NUMBER: _ClassVar[int]
    NEXT_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    NEXT_ID_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[ArticleWithTags]
    total_count: int
    next_created_at: _timestamp_pb2.Timestamp
    next_id: str
    def __init__(self, articles: _Optional[_Iterable[_Union[ArticleWithTags, _Mapping]]] = ..., total_count: _Optional[int] = ..., next_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., next_id: _Optional[str] = ...) -> None: ...

class BatchGetTagsByArticleIDsRequest(_message.Message):
    __slots__ = ("article_ids",)
    ARTICLE_IDS_FIELD_NUMBER: _ClassVar[int]
    article_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, article_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class ArticleTagEntry(_message.Message):
    __slots__ = ("tag_name", "confidence", "source", "updated_at")
    TAG_NAME_FIELD_NUMBER: _ClassVar[int]
    CONFIDENCE_FIELD_NUMBER: _ClassVar[int]
    SOURCE_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    tag_name: str
    confidence: float
    source: str
    updated_at: _timestamp_pb2.Timestamp
    def __init__(self, tag_name: _Optional[str] = ..., confidence: _Optional[float] = ..., source: _Optional[str] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ArticleTagsEntry(_message.Message):
    __slots__ = ("article_id", "tags")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    TAGS_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    tags: _containers.RepeatedCompositeFieldContainer[ArticleTagEntry]
    def __init__(self, article_id: _Optional[str] = ..., tags: _Optional[_Iterable[_Union[ArticleTagEntry, _Mapping]]] = ...) -> None: ...

class BatchGetTagsByArticleIDsResponse(_message.Message):
    __slots__ = ("items",)
    ITEMS_FIELD_NUMBER: _ClassVar[int]
    items: _containers.RepeatedCompositeFieldContainer[ArticleTagsEntry]
    def __init__(self, items: _Optional[_Iterable[_Union[ArticleTagsEntry, _Mapping]]] = ...) -> None: ...

class DeleteArticleSummaryRequest(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class DeleteArticleSummaryResponse(_message.Message):
    __slots__ = ("success",)
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    success: bool
    def __init__(self, success: _Optional[bool] = ...) -> None: ...

class CheckArticleSummaryExistsRequest(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class CheckArticleSummaryExistsResponse(_message.Message):
    __slots__ = ("exists", "summary_id")
    EXISTS_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_ID_FIELD_NUMBER: _ClassVar[int]
    exists: bool
    summary_id: str
    def __init__(self, exists: _Optional[bool] = ..., summary_id: _Optional[str] = ...) -> None: ...

class ArticleWithSummaryItem(_message.Message):
    __slots__ = ("article_id", "article_content", "article_url", "summary_id", "summary_japanese", "created_at")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_CONTENT_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_URL_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_ID_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_JAPANESE_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    article_content: str
    article_url: str
    summary_id: str
    summary_japanese: str
    created_at: _timestamp_pb2.Timestamp
    def __init__(self, article_id: _Optional[str] = ..., article_content: _Optional[str] = ..., article_url: _Optional[str] = ..., summary_id: _Optional[str] = ..., summary_japanese: _Optional[str] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class FindArticlesWithSummariesRequest(_message.Message):
    __slots__ = ("last_created_at", "last_id", "limit")
    LAST_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_ID_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    last_created_at: _timestamp_pb2.Timestamp
    last_id: str
    limit: int
    def __init__(self, last_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_id: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class FindArticlesWithSummariesResponse(_message.Message):
    __slots__ = ("articles", "next_created_at", "next_id")
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    NEXT_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    NEXT_ID_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[ArticleWithSummaryItem]
    next_created_at: _timestamp_pb2.Timestamp
    next_id: str
    def __init__(self, articles: _Optional[_Iterable[_Union[ArticleWithSummaryItem, _Mapping]]] = ..., next_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., next_id: _Optional[str] = ...) -> None: ...

class UnsummarizedArticle(_message.Message):
    __slots__ = ("id", "title", "content", "url", "created_at", "user_id")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    id: str
    title: str
    content: str
    url: str
    created_at: _timestamp_pb2.Timestamp
    user_id: str
    def __init__(self, id: _Optional[str] = ..., title: _Optional[str] = ..., content: _Optional[str] = ..., url: _Optional[str] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., user_id: _Optional[str] = ...) -> None: ...

class ListUnsummarizedArticlesRequest(_message.Message):
    __slots__ = ("last_created_at", "last_id", "limit")
    LAST_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_ID_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    last_created_at: _timestamp_pb2.Timestamp
    last_id: str
    limit: int
    def __init__(self, last_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_id: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class ListUnsummarizedArticlesResponse(_message.Message):
    __slots__ = ("articles", "next_created_at", "next_id")
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    NEXT_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    NEXT_ID_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[UnsummarizedArticle]
    next_created_at: _timestamp_pb2.Timestamp
    next_id: str
    def __init__(self, articles: _Optional[_Iterable[_Union[UnsummarizedArticle, _Mapping]]] = ..., next_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., next_id: _Optional[str] = ...) -> None: ...

class HasUnsummarizedArticlesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class HasUnsummarizedArticlesResponse(_message.Message):
    __slots__ = ("has_unsummarized",)
    HAS_UNSUMMARIZED_FIELD_NUMBER: _ClassVar[int]
    has_unsummarized: bool
    def __init__(self, has_unsummarized: _Optional[bool] = ...) -> None: ...

class GetEmptyFeedIDRequest(_message.Message):
    __slots__ = ("feed_url",)
    FEED_URL_FIELD_NUMBER: _ClassVar[int]
    feed_url: str
    def __init__(self, feed_url: _Optional[str] = ...) -> None: ...

class GetEmptyFeedIDResponse(_message.Message):
    __slots__ = ("feed_id",)
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    feed_id: str
    def __init__(self, feed_id: _Optional[str] = ...) -> None: ...

class FetchTagCloudRequest(_message.Message):
    __slots__ = ("limit",)
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    limit: int
    def __init__(self, limit: _Optional[int] = ...) -> None: ...

class FetchTagCloudResponse(_message.Message):
    __slots__ = ("tags",)
    TAGS_FIELD_NUMBER: _ClassVar[int]
    tags: _containers.RepeatedCompositeFieldContainer[TagCloudItem]
    def __init__(self, tags: _Optional[_Iterable[_Union[TagCloudItem, _Mapping]]] = ...) -> None: ...

class TagCloudItem(_message.Message):
    __slots__ = ("tag_name", "article_count")
    TAG_NAME_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_COUNT_FIELD_NUMBER: _ClassVar[int]
    tag_name: str
    article_count: int
    def __init__(self, tag_name: _Optional[str] = ..., article_count: _Optional[int] = ...) -> None: ...

class FetchArticlesByTagRequest(_message.Message):
    __slots__ = ("tag_name", "limit")
    TAG_NAME_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    tag_name: str
    limit: int
    def __init__(self, tag_name: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class FetchArticlesByTagResponse(_message.Message):
    __slots__ = ("articles",)
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[ArticleByTagItem]
    def __init__(self, articles: _Optional[_Iterable[_Union[ArticleByTagItem, _Mapping]]] = ...) -> None: ...

class ArticleByTagItem(_message.Message):
    __slots__ = ("id", "title", "url", "published_at")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    id: str
    title: str
    url: str
    published_at: str
    def __init__(self, id: _Optional[str] = ..., title: _Optional[str] = ..., url: _Optional[str] = ..., published_at: _Optional[str] = ...) -> None: ...

class ListRecapArticlesRequest(_message.Message):
    __slots__ = ("to", "page", "page_size", "fields", "lang_hint")
    FROM_FIELD_NUMBER: _ClassVar[int]
    TO_FIELD_NUMBER: _ClassVar[int]
    PAGE_FIELD_NUMBER: _ClassVar[int]
    PAGE_SIZE_FIELD_NUMBER: _ClassVar[int]
    FIELDS_FIELD_NUMBER: _ClassVar[int]
    LANG_HINT_FIELD_NUMBER: _ClassVar[int]
    to: str
    page: int
    page_size: int
    fields: _containers.RepeatedScalarFieldContainer[str]
    lang_hint: str
    def __init__(self, to: _Optional[str] = ..., page: _Optional[int] = ..., page_size: _Optional[int] = ..., fields: _Optional[_Iterable[str]] = ..., lang_hint: _Optional[str] = ..., **kwargs) -> None: ...

class RecapArticleRange(_message.Message):
    __slots__ = ("to",)
    FROM_FIELD_NUMBER: _ClassVar[int]
    TO_FIELD_NUMBER: _ClassVar[int]
    to: str
    def __init__(self, to: _Optional[str] = ..., **kwargs) -> None: ...

class RecapArticleItem(_message.Message):
    __slots__ = ("article_id", "title", "fulltext", "published_at", "source_url", "lang_hint")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    FULLTEXT_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    SOURCE_URL_FIELD_NUMBER: _ClassVar[int]
    LANG_HINT_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    title: str
    fulltext: str
    published_at: str
    source_url: str
    lang_hint: str
    def __init__(self, article_id: _Optional[str] = ..., title: _Optional[str] = ..., fulltext: _Optional[str] = ..., published_at: _Optional[str] = ..., source_url: _Optional[str] = ..., lang_hint: _Optional[str] = ...) -> None: ...

class ListRecapArticlesResponse(_message.Message):
    __slots__ = ("range", "total", "page", "page_size", "has_more", "articles")
    RANGE_FIELD_NUMBER: _ClassVar[int]
    TOTAL_FIELD_NUMBER: _ClassVar[int]
    PAGE_FIELD_NUMBER: _ClassVar[int]
    PAGE_SIZE_FIELD_NUMBER: _ClassVar[int]
    HAS_MORE_FIELD_NUMBER: _ClassVar[int]
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    range: RecapArticleRange
    total: int
    page: int
    page_size: int
    has_more: bool
    articles: _containers.RepeatedCompositeFieldContainer[RecapArticleItem]
    def __init__(self, range: _Optional[_Union[RecapArticleRange, _Mapping]] = ..., total: _Optional[int] = ..., page: _Optional[int] = ..., page_size: _Optional[int] = ..., has_more: _Optional[bool] = ..., articles: _Optional[_Iterable[_Union[RecapArticleItem, _Mapping]]] = ...) -> None: ...

class GetSystemUserRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetSystemUserResponse(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class RecentArticleItem(_message.Message):
    __slots__ = ("id", "title", "url", "published_at", "feed_id", "tags")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    TAGS_FIELD_NUMBER: _ClassVar[int]
    id: str
    title: str
    url: str
    published_at: str
    feed_id: str
    tags: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, id: _Optional[str] = ..., title: _Optional[str] = ..., url: _Optional[str] = ..., published_at: _Optional[str] = ..., feed_id: _Optional[str] = ..., tags: _Optional[_Iterable[str]] = ...) -> None: ...

class ListRecentArticlesRequest(_message.Message):
    __slots__ = ("within_hours", "limit")
    WITHIN_HOURS_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    within_hours: int
    limit: int
    def __init__(self, within_hours: _Optional[int] = ..., limit: _Optional[int] = ...) -> None: ...

class ListRecentArticlesResponse(_message.Message):
    __slots__ = ("articles", "since", "until", "count")
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    SINCE_FIELD_NUMBER: _ClassVar[int]
    UNTIL_FIELD_NUMBER: _ClassVar[int]
    COUNT_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[RecentArticleItem]
    since: str
    until: str
    count: int
    def __init__(self, articles: _Optional[_Iterable[_Union[RecentArticleItem, _Mapping]]] = ..., since: _Optional[str] = ..., until: _Optional[str] = ..., count: _Optional[int] = ...) -> None: ...

class OutboxEvent(_message.Message):
    __slots__ = ("id", "event_type", "payload", "status", "created_at")
    ID_FIELD_NUMBER: _ClassVar[int]
    EVENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    id: str
    event_type: str
    payload: bytes
    status: OutboxEventStatus
    created_at: _timestamp_pb2.Timestamp
    def __init__(self, id: _Optional[str] = ..., event_type: _Optional[str] = ..., payload: _Optional[bytes] = ..., status: _Optional[_Union[OutboxEventStatus, str]] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ClaimOutboxBatchRequest(_message.Message):
    __slots__ = ("limit",)
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    limit: int
    def __init__(self, limit: _Optional[int] = ...) -> None: ...

class ClaimOutboxBatchResponse(_message.Message):
    __slots__ = ("events",)
    EVENTS_FIELD_NUMBER: _ClassVar[int]
    events: _containers.RepeatedCompositeFieldContainer[OutboxEvent]
    def __init__(self, events: _Optional[_Iterable[_Union[OutboxEvent, _Mapping]]] = ...) -> None: ...

class MarkOutboxProcessedRequest(_message.Message):
    __slots__ = ("id", "status", "error_message")
    ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    id: str
    status: OutboxEventStatus
    error_message: str
    def __init__(self, id: _Optional[str] = ..., status: _Optional[_Union[OutboxEventStatus, str]] = ..., error_message: _Optional[str] = ...) -> None: ...

class MarkOutboxProcessedResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ReleaseOutboxEventRequest(_message.Message):
    __slots__ = ("id",)
    ID_FIELD_NUMBER: _ClassVar[int]
    id: str
    def __init__(self, id: _Optional[str] = ...) -> None: ...

class ReleaseOutboxEventResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class PruneOutboxEventsRequest(_message.Message):
    __slots__ = ("older_than_seconds",)
    OLDER_THAN_SECONDS_FIELD_NUMBER: _ClassVar[int]
    older_than_seconds: int
    def __init__(self, older_than_seconds: _Optional[int] = ...) -> None: ...

class PruneOutboxEventsResponse(_message.Message):
    __slots__ = ("pruned_count",)
    PRUNED_COUNT_FIELD_NUMBER: _ClassVar[int]
    pruned_count: int
    def __init__(self, pruned_count: _Optional[int] = ...) -> None: ...

class ArticleHead(_message.Message):
    __slots__ = ("id", "article_id", "head_html", "og_image_url")
    ID_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    HEAD_HTML_FIELD_NUMBER: _ClassVar[int]
    OG_IMAGE_URL_FIELD_NUMBER: _ClassVar[int]
    id: str
    article_id: str
    head_html: str
    og_image_url: str
    def __init__(self, id: _Optional[str] = ..., article_id: _Optional[str] = ..., head_html: _Optional[str] = ..., og_image_url: _Optional[str] = ...) -> None: ...

class GetArticleHeadRequest(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class GetArticleHeadResponse(_message.Message):
    __slots__ = ("head",)
    HEAD_FIELD_NUMBER: _ClassVar[int]
    head: ArticleHead
    def __init__(self, head: _Optional[_Union[ArticleHead, _Mapping]] = ...) -> None: ...

class BatchGetOgImageURLsRequest(_message.Message):
    __slots__ = ("article_ids",)
    ARTICLE_IDS_FIELD_NUMBER: _ClassVar[int]
    article_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, article_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class BatchGetOgImageURLsResponse(_message.Message):
    __slots__ = ("og_image_urls",)
    class OgImageUrlsEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    OG_IMAGE_URLS_FIELD_NUMBER: _ClassVar[int]
    og_image_urls: _containers.ScalarMap[str, str]
    def __init__(self, og_image_urls: _Optional[_Mapping[str, str]] = ...) -> None: ...

class OgImageBackfillCandidate(_message.Message):
    __slots__ = ("article_id", "url")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    url: str
    def __init__(self, article_id: _Optional[str] = ..., url: _Optional[str] = ...) -> None: ...

class ListFeedsMissingOgImageRequest(_message.Message):
    __slots__ = ("limit",)
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    limit: int
    def __init__(self, limit: _Optional[int] = ...) -> None: ...

class ListFeedsMissingOgImageResponse(_message.Message):
    __slots__ = ("candidates",)
    CANDIDATES_FIELD_NUMBER: _ClassVar[int]
    candidates: _containers.RepeatedCompositeFieldContainer[OgImageBackfillCandidate]
    def __init__(self, candidates: _Optional[_Iterable[_Union[OgImageBackfillCandidate, _Mapping]]] = ...) -> None: ...

class ListUnwarmedOgImageURLsRequest(_message.Message):
    __slots__ = ("limit",)
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    limit: int
    def __init__(self, limit: _Optional[int] = ...) -> None: ...

class ListUnwarmedOgImageURLsResponse(_message.Message):
    __slots__ = ("urls",)
    URLS_FIELD_NUMBER: _ClassVar[int]
    urls: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, urls: _Optional[_Iterable[str]] = ...) -> None: ...

class PurgeExpiredArticleHeadsRequest(_message.Message):
    __slots__ = ("ttl_seconds",)
    TTL_SECONDS_FIELD_NUMBER: _ClassVar[int]
    ttl_seconds: int
    def __init__(self, ttl_seconds: _Optional[int] = ...) -> None: ...

class PurgeExpiredArticleHeadsResponse(_message.Message):
    __slots__ = ("purged_count",)
    PURGED_COUNT_FIELD_NUMBER: _ClassVar[int]
    purged_count: int
    def __init__(self, purged_count: _Optional[int] = ...) -> None: ...

class ImageProxyCacheEntry(_message.Message):
    __slots__ = ("url_hash", "original_url", "data", "content_type", "width", "height", "size_bytes", "etag", "created_at", "expires_at")
    URL_HASH_FIELD_NUMBER: _ClassVar[int]
    ORIGINAL_URL_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    CONTENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    WIDTH_FIELD_NUMBER: _ClassVar[int]
    HEIGHT_FIELD_NUMBER: _ClassVar[int]
    SIZE_BYTES_FIELD_NUMBER: _ClassVar[int]
    ETAG_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    url_hash: str
    original_url: str
    data: bytes
    content_type: str
    width: int
    height: int
    size_bytes: int
    etag: str
    created_at: _timestamp_pb2.Timestamp
    expires_at: _timestamp_pb2.Timestamp
    def __init__(self, url_hash: _Optional[str] = ..., original_url: _Optional[str] = ..., data: _Optional[bytes] = ..., content_type: _Optional[str] = ..., width: _Optional[int] = ..., height: _Optional[int] = ..., size_bytes: _Optional[int] = ..., etag: _Optional[str] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class GetImageProxyCacheRequest(_message.Message):
    __slots__ = ("url_hash",)
    URL_HASH_FIELD_NUMBER: _ClassVar[int]
    url_hash: str
    def __init__(self, url_hash: _Optional[str] = ...) -> None: ...

class GetImageProxyCacheResponse(_message.Message):
    __slots__ = ("entry",)
    ENTRY_FIELD_NUMBER: _ClassVar[int]
    entry: ImageProxyCacheEntry
    def __init__(self, entry: _Optional[_Union[ImageProxyCacheEntry, _Mapping]] = ...) -> None: ...

class PutImageProxyCacheRequest(_message.Message):
    __slots__ = ("entry",)
    ENTRY_FIELD_NUMBER: _ClassVar[int]
    entry: ImageProxyCacheEntry
    def __init__(self, entry: _Optional[_Union[ImageProxyCacheEntry, _Mapping]] = ...) -> None: ...

class PutImageProxyCacheResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class EvictExpiredImageProxyCacheRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class EvictExpiredImageProxyCacheResponse(_message.Message):
    __slots__ = ("evicted_count",)
    EVICTED_COUNT_FIELD_NUMBER: _ClassVar[int]
    evicted_count: int
    def __init__(self, evicted_count: _Optional[int] = ...) -> None: ...

class PurgeImageProxyCacheOlderThanRequest(_message.Message):
    __slots__ = ("ttl_seconds",)
    TTL_SECONDS_FIELD_NUMBER: _ClassVar[int]
    ttl_seconds: int
    def __init__(self, ttl_seconds: _Optional[int] = ...) -> None: ...

class PurgeImageProxyCacheOlderThanResponse(_message.Message):
    __slots__ = ("purged_count",)
    PURGED_COUNT_FIELD_NUMBER: _ClassVar[int]
    purged_count: int
    def __init__(self, purged_count: _Optional[int] = ...) -> None: ...

class ScrapingDomain(_message.Message):
    __slots__ = ("id", "domain", "scheme", "allow_fetch_body", "allow_ml_training", "allow_cache_days", "force_respect_robots", "robots_txt_url", "robots_txt_content", "robots_txt_fetched_at", "robots_txt_last_status", "robots_crawl_delay_sec", "robots_disallow_paths", "created_at", "updated_at")
    ID_FIELD_NUMBER: _ClassVar[int]
    DOMAIN_FIELD_NUMBER: _ClassVar[int]
    SCHEME_FIELD_NUMBER: _ClassVar[int]
    ALLOW_FETCH_BODY_FIELD_NUMBER: _ClassVar[int]
    ALLOW_ML_TRAINING_FIELD_NUMBER: _ClassVar[int]
    ALLOW_CACHE_DAYS_FIELD_NUMBER: _ClassVar[int]
    FORCE_RESPECT_ROBOTS_FIELD_NUMBER: _ClassVar[int]
    ROBOTS_TXT_URL_FIELD_NUMBER: _ClassVar[int]
    ROBOTS_TXT_CONTENT_FIELD_NUMBER: _ClassVar[int]
    ROBOTS_TXT_FETCHED_AT_FIELD_NUMBER: _ClassVar[int]
    ROBOTS_TXT_LAST_STATUS_FIELD_NUMBER: _ClassVar[int]
    ROBOTS_CRAWL_DELAY_SEC_FIELD_NUMBER: _ClassVar[int]
    ROBOTS_DISALLOW_PATHS_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    id: str
    domain: str
    scheme: str
    allow_fetch_body: bool
    allow_ml_training: bool
    allow_cache_days: int
    force_respect_robots: bool
    robots_txt_url: str
    robots_txt_content: str
    robots_txt_fetched_at: _timestamp_pb2.Timestamp
    robots_txt_last_status: int
    robots_crawl_delay_sec: int
    robots_disallow_paths: _containers.RepeatedScalarFieldContainer[str]
    created_at: _timestamp_pb2.Timestamp
    updated_at: _timestamp_pb2.Timestamp
    def __init__(self, id: _Optional[str] = ..., domain: _Optional[str] = ..., scheme: _Optional[str] = ..., allow_fetch_body: _Optional[bool] = ..., allow_ml_training: _Optional[bool] = ..., allow_cache_days: _Optional[int] = ..., force_respect_robots: _Optional[bool] = ..., robots_txt_url: _Optional[str] = ..., robots_txt_content: _Optional[str] = ..., robots_txt_fetched_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., robots_txt_last_status: _Optional[int] = ..., robots_crawl_delay_sec: _Optional[int] = ..., robots_disallow_paths: _Optional[_Iterable[str]] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class GetScrapingDomainByDomainRequest(_message.Message):
    __slots__ = ("domain",)
    DOMAIN_FIELD_NUMBER: _ClassVar[int]
    domain: str
    def __init__(self, domain: _Optional[str] = ...) -> None: ...

class GetScrapingDomainByDomainResponse(_message.Message):
    __slots__ = ("scraping_domain",)
    SCRAPING_DOMAIN_FIELD_NUMBER: _ClassVar[int]
    scraping_domain: ScrapingDomain
    def __init__(self, scraping_domain: _Optional[_Union[ScrapingDomain, _Mapping]] = ...) -> None: ...

class GetScrapingDomainByIDRequest(_message.Message):
    __slots__ = ("id",)
    ID_FIELD_NUMBER: _ClassVar[int]
    id: str
    def __init__(self, id: _Optional[str] = ...) -> None: ...

class GetScrapingDomainByIDResponse(_message.Message):
    __slots__ = ("scraping_domain",)
    SCRAPING_DOMAIN_FIELD_NUMBER: _ClassVar[int]
    scraping_domain: ScrapingDomain
    def __init__(self, scraping_domain: _Optional[_Union[ScrapingDomain, _Mapping]] = ...) -> None: ...

class SaveScrapingDomainRequest(_message.Message):
    __slots__ = ("scraping_domain",)
    SCRAPING_DOMAIN_FIELD_NUMBER: _ClassVar[int]
    scraping_domain: ScrapingDomain
    def __init__(self, scraping_domain: _Optional[_Union[ScrapingDomain, _Mapping]] = ...) -> None: ...

class SaveScrapingDomainResponse(_message.Message):
    __slots__ = ("scraping_domain",)
    SCRAPING_DOMAIN_FIELD_NUMBER: _ClassVar[int]
    scraping_domain: ScrapingDomain
    def __init__(self, scraping_domain: _Optional[_Union[ScrapingDomain, _Mapping]] = ...) -> None: ...

class ListScrapingDomainsRequest(_message.Message):
    __slots__ = ("offset", "limit")
    OFFSET_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    offset: int
    limit: int
    def __init__(self, offset: _Optional[int] = ..., limit: _Optional[int] = ...) -> None: ...

class ListScrapingDomainsResponse(_message.Message):
    __slots__ = ("scraping_domains",)
    SCRAPING_DOMAINS_FIELD_NUMBER: _ClassVar[int]
    scraping_domains: _containers.RepeatedCompositeFieldContainer[ScrapingDomain]
    def __init__(self, scraping_domains: _Optional[_Iterable[_Union[ScrapingDomain, _Mapping]]] = ...) -> None: ...

class ScrapingPolicyUpdate(_message.Message):
    __slots__ = ("allow_fetch_body", "allow_ml_training", "allow_cache_days", "force_respect_robots")
    ALLOW_FETCH_BODY_FIELD_NUMBER: _ClassVar[int]
    ALLOW_ML_TRAINING_FIELD_NUMBER: _ClassVar[int]
    ALLOW_CACHE_DAYS_FIELD_NUMBER: _ClassVar[int]
    FORCE_RESPECT_ROBOTS_FIELD_NUMBER: _ClassVar[int]
    allow_fetch_body: bool
    allow_ml_training: bool
    allow_cache_days: int
    force_respect_robots: bool
    def __init__(self, allow_fetch_body: _Optional[bool] = ..., allow_ml_training: _Optional[bool] = ..., allow_cache_days: _Optional[int] = ..., force_respect_robots: _Optional[bool] = ...) -> None: ...

class UpdateScrapingDomainPolicyRequest(_message.Message):
    __slots__ = ("id", "update")
    ID_FIELD_NUMBER: _ClassVar[int]
    UPDATE_FIELD_NUMBER: _ClassVar[int]
    id: str
    update: ScrapingPolicyUpdate
    def __init__(self, id: _Optional[str] = ..., update: _Optional[_Union[ScrapingPolicyUpdate, _Mapping]] = ...) -> None: ...

class UpdateScrapingDomainPolicyResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class SaveDeclinedDomainRequest(_message.Message):
    __slots__ = ("user_id", "domain")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    DOMAIN_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    domain: str
    def __init__(self, user_id: _Optional[str] = ..., domain: _Optional[str] = ...) -> None: ...

class SaveDeclinedDomainResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class IsDomainDeclinedRequest(_message.Message):
    __slots__ = ("user_id", "domain")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    DOMAIN_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    domain: str
    def __init__(self, user_id: _Optional[str] = ..., domain: _Optional[str] = ...) -> None: ...

class IsDomainDeclinedResponse(_message.Message):
    __slots__ = ("declined",)
    DECLINED_FIELD_NUMBER: _ClassVar[int]
    declined: bool
    def __init__(self, declined: _Optional[bool] = ...) -> None: ...

class ListSubscribedUserIDsByFeedLinkIDRequest(_message.Message):
    __slots__ = ("feed_link_id",)
    FEED_LINK_ID_FIELD_NUMBER: _ClassVar[int]
    feed_link_id: str
    def __init__(self, feed_link_id: _Optional[str] = ...) -> None: ...

class ListSubscribedUserIDsByFeedLinkIDResponse(_message.Message):
    __slots__ = ("user_ids",)
    USER_IDS_FIELD_NUMBER: _ClassVar[int]
    user_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, user_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class CheckArticleExistsByURLForUserRequest(_message.Message):
    __slots__ = ("url", "user_id")
    URL_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    url: str
    user_id: str
    def __init__(self, url: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class CheckArticleExistsByURLForUserResponse(_message.Message):
    __slots__ = ("exists", "article_id")
    EXISTS_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    exists: bool
    article_id: str
    def __init__(self, exists: _Optional[bool] = ..., article_id: _Optional[str] = ...) -> None: ...

class ArchiveArticleRequest(_message.Message):
    __slots__ = ("url", "title", "content", "user_id")
    URL_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    url: str
    title: str
    content: str
    user_id: str
    def __init__(self, url: _Optional[str] = ..., title: _Optional[str] = ..., content: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class ArchiveArticleResponse(_message.Message):
    __slots__ = ("article_id", "created")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    CREATED_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    created: bool
    def __init__(self, article_id: _Optional[str] = ..., created: _Optional[bool] = ...) -> None: ...

class SaveArticleHeadRequest(_message.Message):
    __slots__ = ("article_id", "head_html", "og_image_url")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    HEAD_HTML_FIELD_NUMBER: _ClassVar[int]
    OG_IMAGE_URL_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    head_html: str
    og_image_url: str
    def __init__(self, article_id: _Optional[str] = ..., head_html: _Optional[str] = ..., og_image_url: _Optional[str] = ...) -> None: ...

class SaveArticleHeadResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ArticleContent(_message.Message):
    __slots__ = ("id", "title", "content", "url", "feed_id")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    id: str
    title: str
    content: str
    url: str
    feed_id: str
    def __init__(self, id: _Optional[str] = ..., title: _Optional[str] = ..., content: _Optional[str] = ..., url: _Optional[str] = ..., feed_id: _Optional[str] = ...) -> None: ...

class UserArticle(_message.Message):
    __slots__ = ("id", "feed_id", "title", "content", "url", "tags", "published_at", "created_at")
    ID_FIELD_NUMBER: _ClassVar[int]
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    TAGS_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    id: str
    feed_id: str
    title: str
    content: str
    url: str
    tags: _containers.RepeatedScalarFieldContainer[str]
    published_at: _timestamp_pb2.Timestamp
    created_at: _timestamp_pb2.Timestamp
    def __init__(self, id: _Optional[str] = ..., feed_id: _Optional[str] = ..., title: _Optional[str] = ..., content: _Optional[str] = ..., url: _Optional[str] = ..., tags: _Optional[_Iterable[str]] = ..., published_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class GetArticleByURLRequest(_message.Message):
    __slots__ = ("url", "user_id")
    URL_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    url: str
    user_id: str
    def __init__(self, url: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class GetArticleByURLResponse(_message.Message):
    __slots__ = ("article",)
    ARTICLE_FIELD_NUMBER: _ClassVar[int]
    article: ArticleContent
    def __init__(self, article: _Optional[_Union[ArticleContent, _Mapping]] = ...) -> None: ...

class BatchGetArticlesByURLsRequest(_message.Message):
    __slots__ = ("urls", "user_id")
    URLS_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    urls: _containers.RepeatedScalarFieldContainer[str]
    user_id: str
    def __init__(self, urls: _Optional[_Iterable[str]] = ..., user_id: _Optional[str] = ...) -> None: ...

class BatchGetArticlesByURLsResponse(_message.Message):
    __slots__ = ("articles",)
    class ArticlesEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: ArticleContent
        def __init__(self, key: _Optional[str] = ..., value: _Optional[_Union[ArticleContent, _Mapping]] = ...) -> None: ...
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.MessageMap[str, ArticleContent]
    def __init__(self, articles: _Optional[_Mapping[str, ArticleContent]] = ...) -> None: ...

class GetArticleContentByIDRequest(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class GetArticleContentByIDResponse(_message.Message):
    __slots__ = ("article",)
    ARTICLE_FIELD_NUMBER: _ClassVar[int]
    article: ArticleContent
    def __init__(self, article: _Optional[_Union[ArticleContent, _Mapping]] = ...) -> None: ...

class ListArticlesCursorRequest(_message.Message):
    __slots__ = ("user_id", "cursor", "limit")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    cursor: _timestamp_pb2.Timestamp
    limit: int
    def __init__(self, user_id: _Optional[str] = ..., cursor: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., limit: _Optional[int] = ...) -> None: ...

class ListArticlesCursorResponse(_message.Message):
    __slots__ = ("articles",)
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[UserArticle]
    def __init__(self, articles: _Optional[_Iterable[_Union[UserArticle, _Mapping]]] = ...) -> None: ...

class ListArticleIDsCursorRequest(_message.Message):
    __slots__ = ("user_id", "cursor", "limit")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    cursor: _timestamp_pb2.Timestamp
    limit: int
    def __init__(self, user_id: _Optional[str] = ..., cursor: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., limit: _Optional[int] = ...) -> None: ...

class ListArticleIDsCursorResponse(_message.Message):
    __slots__ = ("article_ids",)
    ARTICLE_IDS_FIELD_NUMBER: _ClassVar[int]
    article_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, article_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class BatchGetArticlesByIDsRequest(_message.Message):
    __slots__ = ("article_ids",)
    ARTICLE_IDS_FIELD_NUMBER: _ClassVar[int]
    article_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, article_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class BatchGetArticlesByIDsResponse(_message.Message):
    __slots__ = ("articles",)
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[UserArticle]
    def __init__(self, articles: _Optional[_Iterable[_Union[UserArticle, _Mapping]]] = ...) -> None: ...

class GetLatestArticleByFeedIDRequest(_message.Message):
    __slots__ = ("feed_id",)
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    feed_id: str
    def __init__(self, feed_id: _Optional[str] = ...) -> None: ...

class GetLatestArticleByFeedIDResponse(_message.Message):
    __slots__ = ("article",)
    ARTICLE_FIELD_NUMBER: _ClassVar[int]
    article: ArticleContent
    def __init__(self, article: _Optional[_Union[ArticleContent, _Mapping]] = ...) -> None: ...

class LookupArticleURLRequest(_message.Message):
    __slots__ = ("article_id", "user_id")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    user_id: str
    def __init__(self, article_id: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class LookupArticleURLResponse(_message.Message):
    __slots__ = ("url",)
    URL_FIELD_NUMBER: _ClassVar[int]
    url: str
    def __init__(self, url: _Optional[str] = ...) -> None: ...

class BackfillArticle(_message.Message):
    __slots__ = ("article_id", "user_id", "created_at", "published_at", "title", "url")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    user_id: str
    created_at: _timestamp_pb2.Timestamp
    published_at: _timestamp_pb2.Timestamp
    title: str
    url: str
    def __init__(self, article_id: _Optional[str] = ..., user_id: _Optional[str] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., published_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., title: _Optional[str] = ..., url: _Optional[str] = ...) -> None: ...

class CountBackfillArticlesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class CountBackfillArticlesResponse(_message.Message):
    __slots__ = ("count",)
    COUNT_FIELD_NUMBER: _ClassVar[int]
    count: int
    def __init__(self, count: _Optional[int] = ...) -> None: ...

class ListBackfillArticlesRequest(_message.Message):
    __slots__ = ("last_created_at", "last_article_id", "limit")
    LAST_CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    last_created_at: _timestamp_pb2.Timestamp
    last_article_id: str
    limit: int
    def __init__(self, last_created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_article_id: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class ListBackfillArticlesResponse(_message.Message):
    __slots__ = ("articles",)
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[BackfillArticle]
    def __init__(self, articles: _Optional[_Iterable[_Union[BackfillArticle, _Mapping]]] = ...) -> None: ...

class BackfillSummaryTitle(_message.Message):
    __slots__ = ("summary_version_id", "article_id", "user_id", "tenant_id", "title", "generated_at")
    SUMMARY_VERSION_ID_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    GENERATED_AT_FIELD_NUMBER: _ClassVar[int]
    summary_version_id: str
    article_id: str
    user_id: str
    tenant_id: str
    title: str
    generated_at: _timestamp_pb2.Timestamp
    def __init__(self, summary_version_id: _Optional[str] = ..., article_id: _Optional[str] = ..., user_id: _Optional[str] = ..., tenant_id: _Optional[str] = ..., title: _Optional[str] = ..., generated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class CountBackfillSummaryTitlesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class CountBackfillSummaryTitlesResponse(_message.Message):
    __slots__ = ("count",)
    COUNT_FIELD_NUMBER: _ClassVar[int]
    count: int
    def __init__(self, count: _Optional[int] = ...) -> None: ...

class ListBackfillSummaryTitlesRequest(_message.Message):
    __slots__ = ("last_generated_at", "last_summary_version_id", "limit")
    LAST_GENERATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_SUMMARY_VERSION_ID_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    last_generated_at: _timestamp_pb2.Timestamp
    last_summary_version_id: str
    limit: int
    def __init__(self, last_generated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_summary_version_id: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class ListBackfillSummaryTitlesResponse(_message.Message):
    __slots__ = ("entries",)
    ENTRIES_FIELD_NUMBER: _ClassVar[int]
    entries: _containers.RepeatedCompositeFieldContainer[BackfillSummaryTitle]
    def __init__(self, entries: _Optional[_Iterable[_Union[BackfillSummaryTitle, _Mapping]]] = ...) -> None: ...

class FeedLink(_message.Message):
    __slots__ = ("id", "url")
    ID_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    id: str
    url: str
    def __init__(self, id: _Optional[str] = ..., url: _Optional[str] = ...) -> None: ...

class FeedLinkAvailability(_message.Message):
    __slots__ = ("feed_link_id", "is_active", "consecutive_failures", "last_failure_at", "last_failure_reason")
    FEED_LINK_ID_FIELD_NUMBER: _ClassVar[int]
    IS_ACTIVE_FIELD_NUMBER: _ClassVar[int]
    CONSECUTIVE_FAILURES_FIELD_NUMBER: _ClassVar[int]
    LAST_FAILURE_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_FAILURE_REASON_FIELD_NUMBER: _ClassVar[int]
    feed_link_id: str
    is_active: bool
    consecutive_failures: int
    last_failure_at: _timestamp_pb2.Timestamp
    last_failure_reason: str
    def __init__(self, feed_link_id: _Optional[str] = ..., is_active: _Optional[bool] = ..., consecutive_failures: _Optional[int] = ..., last_failure_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_failure_reason: _Optional[str] = ...) -> None: ...

class FeedLinkWithHealth(_message.Message):
    __slots__ = ("feed_link", "availability")
    FEED_LINK_FIELD_NUMBER: _ClassVar[int]
    AVAILABILITY_FIELD_NUMBER: _ClassVar[int]
    feed_link: FeedLink
    availability: FeedLinkAvailability
    def __init__(self, feed_link: _Optional[_Union[FeedLink, _Mapping]] = ..., availability: _Optional[_Union[FeedLinkAvailability, _Mapping]] = ...) -> None: ...

class FeedLinkDomain(_message.Message):
    __slots__ = ("domain", "scheme")
    DOMAIN_FIELD_NUMBER: _ClassVar[int]
    SCHEME_FIELD_NUMBER: _ClassVar[int]
    domain: str
    scheme: str
    def __init__(self, domain: _Optional[str] = ..., scheme: _Optional[str] = ...) -> None: ...

class FeedLinkExportEntry(_message.Message):
    __slots__ = ("url", "title")
    URL_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    url: str
    title: str
    def __init__(self, url: _Optional[str] = ..., title: _Optional[str] = ...) -> None: ...

class Feed(_message.Message):
    __slots__ = ("id", "title", "description", "website_url", "pub_date", "created_at", "updated_at", "article_id", "is_read", "feed_link_id", "og_image_url")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    WEBSITE_URL_FIELD_NUMBER: _ClassVar[int]
    PUB_DATE_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    IS_READ_FIELD_NUMBER: _ClassVar[int]
    FEED_LINK_ID_FIELD_NUMBER: _ClassVar[int]
    OG_IMAGE_URL_FIELD_NUMBER: _ClassVar[int]
    id: str
    title: str
    description: str
    website_url: str
    pub_date: _timestamp_pb2.Timestamp
    created_at: _timestamp_pb2.Timestamp
    updated_at: _timestamp_pb2.Timestamp
    article_id: str
    is_read: bool
    feed_link_id: str
    og_image_url: str
    def __init__(self, id: _Optional[str] = ..., title: _Optional[str] = ..., description: _Optional[str] = ..., website_url: _Optional[str] = ..., pub_date: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., article_id: _Optional[str] = ..., is_read: _Optional[bool] = ..., feed_link_id: _Optional[str] = ..., og_image_url: _Optional[str] = ...) -> None: ...

class FeedRegistration(_message.Message):
    __slots__ = ("title", "description", "website_url", "pub_date", "created_at", "updated_at", "feed_link_id", "og_image_url")
    TITLE_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    WEBSITE_URL_FIELD_NUMBER: _ClassVar[int]
    PUB_DATE_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    FEED_LINK_ID_FIELD_NUMBER: _ClassVar[int]
    OG_IMAGE_URL_FIELD_NUMBER: _ClassVar[int]
    title: str
    description: str
    website_url: str
    pub_date: _timestamp_pb2.Timestamp
    created_at: _timestamp_pb2.Timestamp
    updated_at: _timestamp_pb2.Timestamp
    feed_link_id: str
    og_image_url: str
    def __init__(self, title: _Optional[str] = ..., description: _Optional[str] = ..., website_url: _Optional[str] = ..., pub_date: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., feed_link_id: _Optional[str] = ..., og_image_url: _Optional[str] = ...) -> None: ...

class FeedRegistrationResult(_message.Message):
    __slots__ = ("feed_id", "created")
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    CREATED_FIELD_NUMBER: _ClassVar[int]
    feed_id: str
    created: bool
    def __init__(self, feed_id: _Optional[str] = ..., created: _Optional[bool] = ...) -> None: ...

class FeedSummary(_message.Message):
    __slots__ = ("summary",)
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    summary: str
    def __init__(self, summary: _Optional[str] = ...) -> None: ...

class FeedAndArticle(_message.Message):
    __slots__ = ("feed_id", "article_id", "url", "feed_title", "article_title")
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    FEED_TITLE_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_TITLE_FIELD_NUMBER: _ClassVar[int]
    feed_id: str
    article_id: str
    url: str
    feed_title: str
    article_title: str
    def __init__(self, feed_id: _Optional[str] = ..., article_id: _Optional[str] = ..., url: _Optional[str] = ..., feed_title: _Optional[str] = ..., article_title: _Optional[str] = ...) -> None: ...

class InoreaderSummary(_message.Message):
    __slots__ = ("article_url", "title", "author", "content", "content_type", "published_at", "fetched_at", "inoreader_id")
    ARTICLE_URL_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    AUTHOR_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    CONTENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    FETCHED_AT_FIELD_NUMBER: _ClassVar[int]
    INOREADER_ID_FIELD_NUMBER: _ClassVar[int]
    article_url: str
    title: str
    author: str
    content: str
    content_type: str
    published_at: _timestamp_pb2.Timestamp
    fetched_at: _timestamp_pb2.Timestamp
    inoreader_id: str
    def __init__(self, article_url: _Optional[str] = ..., title: _Optional[str] = ..., author: _Optional[str] = ..., content: _Optional[str] = ..., content_type: _Optional[str] = ..., published_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., fetched_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., inoreader_id: _Optional[str] = ...) -> None: ...

class RegisterFeedLinkRequest(_message.Message):
    __slots__ = ("url",)
    URL_FIELD_NUMBER: _ClassVar[int]
    url: str
    def __init__(self, url: _Optional[str] = ...) -> None: ...

class RegisterFeedLinkResponse(_message.Message):
    __slots__ = ("already_existed",)
    ALREADY_EXISTED_FIELD_NUMBER: _ClassVar[int]
    already_existed: bool
    def __init__(self, already_existed: _Optional[bool] = ...) -> None: ...

class BulkRegisterFeedLinksRequest(_message.Message):
    __slots__ = ("urls",)
    URLS_FIELD_NUMBER: _ClassVar[int]
    urls: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, urls: _Optional[_Iterable[str]] = ...) -> None: ...

class BulkRegisterFeedLinksResponse(_message.Message):
    __slots__ = ("registered", "skipped", "failed_urls")
    REGISTERED_FIELD_NUMBER: _ClassVar[int]
    SKIPPED_FIELD_NUMBER: _ClassVar[int]
    FAILED_URLS_FIELD_NUMBER: _ClassVar[int]
    registered: int
    skipped: int
    failed_urls: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, registered: _Optional[int] = ..., skipped: _Optional[int] = ..., failed_urls: _Optional[_Iterable[str]] = ...) -> None: ...

class ListFeedLinksRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListFeedLinksResponse(_message.Message):
    __slots__ = ("feed_links",)
    FEED_LINKS_FIELD_NUMBER: _ClassVar[int]
    feed_links: _containers.RepeatedCompositeFieldContainer[FeedLink]
    def __init__(self, feed_links: _Optional[_Iterable[_Union[FeedLink, _Mapping]]] = ...) -> None: ...

class ListFeedLinksWithHealthRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListFeedLinksWithHealthResponse(_message.Message):
    __slots__ = ("feed_links",)
    FEED_LINKS_FIELD_NUMBER: _ClassVar[int]
    feed_links: _containers.RepeatedCompositeFieldContainer[FeedLinkWithHealth]
    def __init__(self, feed_links: _Optional[_Iterable[_Union[FeedLinkWithHealth, _Mapping]]] = ...) -> None: ...

class DeleteFeedLinkRequest(_message.Message):
    __slots__ = ("id",)
    ID_FIELD_NUMBER: _ClassVar[int]
    id: str
    def __init__(self, id: _Optional[str] = ...) -> None: ...

class DeleteFeedLinkResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ResolveFeedLinkIDByURLRequest(_message.Message):
    __slots__ = ("feed_url",)
    FEED_URL_FIELD_NUMBER: _ClassVar[int]
    feed_url: str
    def __init__(self, feed_url: _Optional[str] = ...) -> None: ...

class ResolveFeedLinkIDByURLResponse(_message.Message):
    __slots__ = ("feed_link_id",)
    FEED_LINK_ID_FIELD_NUMBER: _ClassVar[int]
    feed_link_id: str
    def __init__(self, feed_link_id: _Optional[str] = ...) -> None: ...

class ListFeedLinkDomainsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListFeedLinkDomainsResponse(_message.Message):
    __slots__ = ("domains",)
    DOMAINS_FIELD_NUMBER: _ClassVar[int]
    domains: _containers.RepeatedCompositeFieldContainer[FeedLinkDomain]
    def __init__(self, domains: _Optional[_Iterable[_Union[FeedLinkDomain, _Mapping]]] = ...) -> None: ...

class ListRSSFeedURLsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListRSSFeedURLsResponse(_message.Message):
    __slots__ = ("feed_links",)
    FEED_LINKS_FIELD_NUMBER: _ClassVar[int]
    feed_links: _containers.RepeatedCompositeFieldContainer[FeedLink]
    def __init__(self, feed_links: _Optional[_Iterable[_Union[FeedLink, _Mapping]]] = ...) -> None: ...

class ListFeedLinksForExportRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListFeedLinksForExportResponse(_message.Message):
    __slots__ = ("entries",)
    ENTRIES_FIELD_NUMBER: _ClassVar[int]
    entries: _containers.RepeatedCompositeFieldContainer[FeedLinkExportEntry]
    def __init__(self, entries: _Optional[_Iterable[_Union[FeedLinkExportEntry, _Mapping]]] = ...) -> None: ...

class RecordFeedLinkFailureRequest(_message.Message):
    __slots__ = ("feed_url", "reason", "disable_after_failures")
    FEED_URL_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    DISABLE_AFTER_FAILURES_FIELD_NUMBER: _ClassVar[int]
    feed_url: str
    reason: str
    disable_after_failures: int
    def __init__(self, feed_url: _Optional[str] = ..., reason: _Optional[str] = ..., disable_after_failures: _Optional[int] = ...) -> None: ...

class RecordFeedLinkFailureResponse(_message.Message):
    __slots__ = ("availability", "disabled_now")
    AVAILABILITY_FIELD_NUMBER: _ClassVar[int]
    DISABLED_NOW_FIELD_NUMBER: _ClassVar[int]
    availability: FeedLinkAvailability
    disabled_now: bool
    def __init__(self, availability: _Optional[_Union[FeedLinkAvailability, _Mapping]] = ..., disabled_now: _Optional[bool] = ...) -> None: ...

class ResetFeedLinkFailuresRequest(_message.Message):
    __slots__ = ("feed_url",)
    FEED_URL_FIELD_NUMBER: _ClassVar[int]
    feed_url: str
    def __init__(self, feed_url: _Optional[str] = ...) -> None: ...

class ResetFeedLinkFailuresResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class RegisterFeedsRequest(_message.Message):
    __slots__ = ("feeds",)
    FEEDS_FIELD_NUMBER: _ClassVar[int]
    feeds: _containers.RepeatedCompositeFieldContainer[FeedRegistration]
    def __init__(self, feeds: _Optional[_Iterable[_Union[FeedRegistration, _Mapping]]] = ...) -> None: ...

class RegisterFeedsResponse(_message.Message):
    __slots__ = ("results",)
    RESULTS_FIELD_NUMBER: _ClassVar[int]
    results: _containers.RepeatedCompositeFieldContainer[FeedRegistrationResult]
    def __init__(self, results: _Optional[_Iterable[_Union[FeedRegistrationResult, _Mapping]]] = ...) -> None: ...

class ListFeedsCursorRequest(_message.Message):
    __slots__ = ("scope", "user_id", "cursor", "limit", "exclude_feed_link_ids")
    SCOPE_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    EXCLUDE_FEED_LINK_IDS_FIELD_NUMBER: _ClassVar[int]
    scope: FeedScope
    user_id: str
    cursor: _timestamp_pb2.Timestamp
    limit: int
    exclude_feed_link_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, scope: _Optional[_Union[FeedScope, str]] = ..., user_id: _Optional[str] = ..., cursor: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., limit: _Optional[int] = ..., exclude_feed_link_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class ListFeedsCursorResponse(_message.Message):
    __slots__ = ("feeds",)
    FEEDS_FIELD_NUMBER: _ClassVar[int]
    feeds: _containers.RepeatedCompositeFieldContainer[Feed]
    def __init__(self, feeds: _Optional[_Iterable[_Union[Feed, _Mapping]]] = ...) -> None: ...

class ListFeedsPageRequest(_message.Message):
    __slots__ = ("page", "unread_only", "user_id")
    PAGE_FIELD_NUMBER: _ClassVar[int]
    UNREAD_ONLY_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    page: int
    unread_only: bool
    user_id: str
    def __init__(self, page: _Optional[int] = ..., unread_only: _Optional[bool] = ..., user_id: _Optional[str] = ...) -> None: ...

class ListFeedsPageResponse(_message.Message):
    __slots__ = ("feeds",)
    FEEDS_FIELD_NUMBER: _ClassVar[int]
    feeds: _containers.RepeatedCompositeFieldContainer[Feed]
    def __init__(self, feeds: _Optional[_Iterable[_Union[Feed, _Mapping]]] = ...) -> None: ...

class ListFeedsLimitRequest(_message.Message):
    __slots__ = ("limit",)
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    limit: int
    def __init__(self, limit: _Optional[int] = ...) -> None: ...

class ListFeedsLimitResponse(_message.Message):
    __slots__ = ("feeds",)
    FEEDS_FIELD_NUMBER: _ClassVar[int]
    feeds: _containers.RepeatedCompositeFieldContainer[Feed]
    def __init__(self, feeds: _Optional[_Iterable[_Union[Feed, _Mapping]]] = ...) -> None: ...

class GetSingleFeedRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetSingleFeedResponse(_message.Message):
    __slots__ = ("feed",)
    FEED_FIELD_NUMBER: _ClassVar[int]
    feed: Feed
    def __init__(self, feed: _Optional[_Union[Feed, _Mapping]] = ...) -> None: ...

class ListFeedsByFeedLinkIDRequest(_message.Message):
    __slots__ = ("feed_link_id",)
    FEED_LINK_ID_FIELD_NUMBER: _ClassVar[int]
    feed_link_id: str
    def __init__(self, feed_link_id: _Optional[str] = ...) -> None: ...

class ListFeedsByFeedLinkIDResponse(_message.Message):
    __slots__ = ("feeds",)
    FEEDS_FIELD_NUMBER: _ClassVar[int]
    feeds: _containers.RepeatedCompositeFieldContainer[Feed]
    def __init__(self, feeds: _Optional[_Iterable[_Union[Feed, _Mapping]]] = ...) -> None: ...

class GetFeedSummaryRequest(_message.Message):
    __slots__ = ("feed_url", "user_id")
    FEED_URL_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    feed_url: str
    user_id: str
    def __init__(self, feed_url: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class GetFeedSummaryResponse(_message.Message):
    __slots__ = ("summary",)
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    summary: FeedSummary
    def __init__(self, summary: _Optional[_Union[FeedSummary, _Mapping]] = ...) -> None: ...

class GetArticleSummaryByArticleIDRequest(_message.Message):
    __slots__ = ("article_id", "user_id")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    user_id: str
    def __init__(self, article_id: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class GetArticleSummaryByArticleIDResponse(_message.Message):
    __slots__ = ("summary",)
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    summary: FeedSummary
    def __init__(self, summary: _Optional[_Union[FeedSummary, _Mapping]] = ...) -> None: ...

class SearchFeedsByTitleRequest(_message.Message):
    __slots__ = ("query", "user_id")
    QUERY_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    query: str
    user_id: str
    def __init__(self, query: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class SearchFeedsByTitleResponse(_message.Message):
    __slots__ = ("feeds",)
    FEEDS_FIELD_NUMBER: _ClassVar[int]
    feeds: _containers.RepeatedCompositeFieldContainer[Feed]
    def __init__(self, feeds: _Optional[_Iterable[_Union[Feed, _Mapping]]] = ...) -> None: ...

class GetRandomFeedRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetRandomFeedResponse(_message.Message):
    __slots__ = ("feed",)
    FEED_FIELD_NUMBER: _ClassVar[int]
    feed: Feed
    def __init__(self, feed: _Optional[_Union[Feed, _Mapping]] = ...) -> None: ...

class GetFeedURLsByArticleIDsRequest(_message.Message):
    __slots__ = ("article_ids",)
    ARTICLE_IDS_FIELD_NUMBER: _ClassVar[int]
    article_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, article_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class GetFeedURLsByArticleIDsResponse(_message.Message):
    __slots__ = ("pairs",)
    PAIRS_FIELD_NUMBER: _ClassVar[int]
    pairs: _containers.RepeatedCompositeFieldContainer[FeedAndArticle]
    def __init__(self, pairs: _Optional[_Iterable[_Union[FeedAndArticle, _Mapping]]] = ...) -> None: ...

class BatchGetFeedTitlesByIDsRequest(_message.Message):
    __slots__ = ("feed_ids",)
    FEED_IDS_FIELD_NUMBER: _ClassVar[int]
    feed_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, feed_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class BatchGetFeedTitlesByIDsResponse(_message.Message):
    __slots__ = ("titles",)
    class TitlesEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    TITLES_FIELD_NUMBER: _ClassVar[int]
    titles: _containers.ScalarMap[str, str]
    def __init__(self, titles: _Optional[_Mapping[str, str]] = ...) -> None: ...

class GetInoreaderSummariesByURLsRequest(_message.Message):
    __slots__ = ("urls",)
    URLS_FIELD_NUMBER: _ClassVar[int]
    urls: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, urls: _Optional[_Iterable[str]] = ...) -> None: ...

class GetInoreaderSummariesByURLsResponse(_message.Message):
    __slots__ = ("summaries",)
    SUMMARIES_FIELD_NUMBER: _ClassVar[int]
    summaries: _containers.RepeatedCompositeFieldContainer[InoreaderSummary]
    def __init__(self, summaries: _Optional[_Iterable[_Union[InoreaderSummary, _Mapping]]] = ...) -> None: ...

class FeedSubscription(_message.Message):
    __slots__ = ("feed_link_id", "url", "is_subscribed", "subscribed_at")
    FEED_LINK_ID_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    IS_SUBSCRIBED_FIELD_NUMBER: _ClassVar[int]
    SUBSCRIBED_AT_FIELD_NUMBER: _ClassVar[int]
    feed_link_id: str
    url: str
    is_subscribed: bool
    subscribed_at: _timestamp_pb2.Timestamp
    def __init__(self, feed_link_id: _Optional[str] = ..., url: _Optional[str] = ..., is_subscribed: _Optional[bool] = ..., subscribed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class MarkFeedReadRequest(_message.Message):
    __slots__ = ("feed_url", "user_id")
    FEED_URL_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    feed_url: str
    user_id: str
    def __init__(self, feed_url: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class MarkFeedReadResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class MarkArticleReadRequest(_message.Message):
    __slots__ = ("article_url", "user_id")
    ARTICLE_URL_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    article_url: str
    user_id: str
    def __init__(self, article_url: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class MarkArticleReadResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetReadFeedIDsRequest(_message.Message):
    __slots__ = ("user_id", "feed_ids")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    FEED_IDS_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    feed_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, user_id: _Optional[str] = ..., feed_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class GetReadFeedIDsResponse(_message.Message):
    __slots__ = ("read_feed_ids",)
    READ_FEED_IDS_FIELD_NUMBER: _ClassVar[int]
    read_feed_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, read_feed_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class GetAllReadFeedIDsRequest(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class GetAllReadFeedIDsResponse(_message.Message):
    __slots__ = ("read_feed_ids",)
    READ_FEED_IDS_FIELD_NUMBER: _ClassVar[int]
    read_feed_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, read_feed_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class GetUserSubscribedFeedLinkIDsRequest(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class GetUserSubscribedFeedLinkIDsResponse(_message.Message):
    __slots__ = ("feed_link_ids",)
    FEED_LINK_IDS_FIELD_NUMBER: _ClassVar[int]
    feed_link_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, feed_link_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class ListSubscriptionsRequest(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class ListSubscriptionsResponse(_message.Message):
    __slots__ = ("subscriptions",)
    SUBSCRIPTIONS_FIELD_NUMBER: _ClassVar[int]
    subscriptions: _containers.RepeatedCompositeFieldContainer[FeedSubscription]
    def __init__(self, subscriptions: _Optional[_Iterable[_Union[FeedSubscription, _Mapping]]] = ...) -> None: ...

class SubscribeRequest(_message.Message):
    __slots__ = ("user_id", "feed_link_id")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    FEED_LINK_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    feed_link_id: str
    def __init__(self, user_id: _Optional[str] = ..., feed_link_id: _Optional[str] = ...) -> None: ...

class SubscribeResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class UnsubscribeRequest(_message.Message):
    __slots__ = ("user_id", "feed_link_id")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    FEED_LINK_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    feed_link_id: str
    def __init__(self, user_id: _Optional[str] = ..., feed_link_id: _Optional[str] = ...) -> None: ...

class UnsubscribeResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class AddFavoriteFeedRequest(_message.Message):
    __slots__ = ("feed_url", "user_id")
    FEED_URL_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    feed_url: str
    user_id: str
    def __init__(self, feed_url: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class AddFavoriteFeedResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class RemoveFavoriteFeedRequest(_message.Message):
    __slots__ = ("feed_url", "user_id")
    FEED_URL_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    feed_url: str
    user_id: str
    def __init__(self, feed_url: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class RemoveFavoriteFeedResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class FeedTag(_message.Message):
    __slots__ = ("id", "feed_id", "tag_name", "confidence", "tag_type", "created_at", "updated_at")
    ID_FIELD_NUMBER: _ClassVar[int]
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    TAG_NAME_FIELD_NUMBER: _ClassVar[int]
    CONFIDENCE_FIELD_NUMBER: _ClassVar[int]
    TAG_TYPE_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    id: str
    feed_id: str
    tag_name: str
    confidence: float
    tag_type: str
    created_at: _timestamp_pb2.Timestamp
    updated_at: _timestamp_pb2.Timestamp
    def __init__(self, id: _Optional[str] = ..., feed_id: _Optional[str] = ..., tag_name: _Optional[str] = ..., confidence: _Optional[float] = ..., tag_type: _Optional[str] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class GetArticleTagsRequest(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class GetArticleTagsResponse(_message.Message):
    __slots__ = ("tags",)
    TAGS_FIELD_NUMBER: _ClassVar[int]
    tags: _containers.RepeatedCompositeFieldContainer[FeedTag]
    def __init__(self, tags: _Optional[_Iterable[_Union[FeedTag, _Mapping]]] = ...) -> None: ...

class GetFeedTagsRequest(_message.Message):
    __slots__ = ("feed_id", "cursor", "limit")
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    feed_id: str
    cursor: _timestamp_pb2.Timestamp
    limit: int
    def __init__(self, feed_id: _Optional[str] = ..., cursor: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., limit: _Optional[int] = ...) -> None: ...

class GetFeedTagsResponse(_message.Message):
    __slots__ = ("tags",)
    TAGS_FIELD_NUMBER: _ClassVar[int]
    tags: _containers.RepeatedCompositeFieldContainer[FeedTag]
    def __init__(self, tags: _Optional[_Iterable[_Union[FeedTag, _Mapping]]] = ...) -> None: ...

class TagCooccurrence(_message.Message):
    __slots__ = ("tag_name_a", "tag_name_b", "shared_count")
    TAG_NAME_A_FIELD_NUMBER: _ClassVar[int]
    TAG_NAME_B_FIELD_NUMBER: _ClassVar[int]
    SHARED_COUNT_FIELD_NUMBER: _ClassVar[int]
    tag_name_a: str
    tag_name_b: str
    shared_count: int
    def __init__(self, tag_name_a: _Optional[str] = ..., tag_name_b: _Optional[str] = ..., shared_count: _Optional[int] = ...) -> None: ...

class GetTagCooccurrencesRequest(_message.Message):
    __slots__ = ("tag_names",)
    TAG_NAMES_FIELD_NUMBER: _ClassVar[int]
    tag_names: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, tag_names: _Optional[_Iterable[str]] = ...) -> None: ...

class GetTagCooccurrencesResponse(_message.Message):
    __slots__ = ("cooccurrences",)
    COOCCURRENCES_FIELD_NUMBER: _ClassVar[int]
    cooccurrences: _containers.RepeatedCompositeFieldContainer[TagCooccurrence]
    def __init__(self, cooccurrences: _Optional[_Iterable[_Union[TagCooccurrence, _Mapping]]] = ...) -> None: ...

class TagPrefixHit(_message.Message):
    __slots__ = ("tag_name", "article_count")
    TAG_NAME_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_COUNT_FIELD_NUMBER: _ClassVar[int]
    tag_name: str
    article_count: int
    def __init__(self, tag_name: _Optional[str] = ..., article_count: _Optional[int] = ...) -> None: ...

class SearchTagsByPrefixRequest(_message.Message):
    __slots__ = ("prefix", "limit")
    PREFIX_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    prefix: str
    limit: int
    def __init__(self, prefix: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class SearchTagsByPrefixResponse(_message.Message):
    __slots__ = ("hits",)
    HITS_FIELD_NUMBER: _ClassVar[int]
    hits: _containers.RepeatedCompositeFieldContainer[TagPrefixHit]
    def __init__(self, hits: _Optional[_Iterable[_Union[TagPrefixHit, _Mapping]]] = ...) -> None: ...

class TagArticleCount(_message.Message):
    __slots__ = ("tag_name", "article_count")
    TAG_NAME_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_COUNT_FIELD_NUMBER: _ClassVar[int]
    tag_name: str
    article_count: int
    def __init__(self, tag_name: _Optional[str] = ..., article_count: _Optional[int] = ...) -> None: ...

class GetTagArticleCountsRequest(_message.Message):
    __slots__ = ("user_id", "since")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    SINCE_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    since: _timestamp_pb2.Timestamp
    def __init__(self, user_id: _Optional[str] = ..., since: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class GetTagArticleCountsResponse(_message.Message):
    __slots__ = ("counts",)
    COUNTS_FIELD_NUMBER: _ClassVar[int]
    counts: _containers.RepeatedCompositeFieldContainer[TagArticleCount]
    def __init__(self, counts: _Optional[_Iterable[_Union[TagArticleCount, _Mapping]]] = ...) -> None: ...

class TagTrailArticle(_message.Message):
    __slots__ = ("id", "title", "url", "published_at", "feed_id", "feed_title")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    FEED_TITLE_FIELD_NUMBER: _ClassVar[int]
    id: str
    title: str
    url: str
    published_at: _timestamp_pb2.Timestamp
    feed_id: str
    feed_title: str
    def __init__(self, id: _Optional[str] = ..., title: _Optional[str] = ..., url: _Optional[str] = ..., published_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., feed_id: _Optional[str] = ..., feed_title: _Optional[str] = ...) -> None: ...

class ListArticlesByTagIDRequest(_message.Message):
    __slots__ = ("tag_id", "cursor", "limit")
    TAG_ID_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    tag_id: str
    cursor: _timestamp_pb2.Timestamp
    limit: int
    def __init__(self, tag_id: _Optional[str] = ..., cursor: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., limit: _Optional[int] = ...) -> None: ...

class ListArticlesByTagIDResponse(_message.Message):
    __slots__ = ("articles",)
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[TagTrailArticle]
    def __init__(self, articles: _Optional[_Iterable[_Union[TagTrailArticle, _Mapping]]] = ...) -> None: ...

class ListArticlesByTagNameRequest(_message.Message):
    __slots__ = ("tag_name", "cursor", "limit")
    TAG_NAME_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    tag_name: str
    cursor: _timestamp_pb2.Timestamp
    limit: int
    def __init__(self, tag_name: _Optional[str] = ..., cursor: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., limit: _Optional[int] = ...) -> None: ...

class ListArticlesByTagNameResponse(_message.Message):
    __slots__ = ("articles",)
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[TagTrailArticle]
    def __init__(self, articles: _Optional[_Iterable[_Union[TagTrailArticle, _Mapping]]] = ...) -> None: ...

class GetArticleTitleAndLinkRequest(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class GetArticleTitleAndLinkResponse(_message.Message):
    __slots__ = ("found", "title", "url", "published_at")
    FOUND_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    found: bool
    title: str
    url: str
    published_at: _timestamp_pb2.Timestamp
    def __init__(self, found: _Optional[bool] = ..., title: _Optional[str] = ..., url: _Optional[str] = ..., published_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class SummaryVersion(_message.Message):
    __slots__ = ("summary_version_id", "article_id", "user_id", "generated_at", "model", "prompt_version", "input_hash", "quality_score", "summary_text", "superseded_by")
    SUMMARY_VERSION_ID_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    GENERATED_AT_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    PROMPT_VERSION_FIELD_NUMBER: _ClassVar[int]
    INPUT_HASH_FIELD_NUMBER: _ClassVar[int]
    QUALITY_SCORE_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_TEXT_FIELD_NUMBER: _ClassVar[int]
    SUPERSEDED_BY_FIELD_NUMBER: _ClassVar[int]
    summary_version_id: str
    article_id: str
    user_id: str
    generated_at: _timestamp_pb2.Timestamp
    model: str
    prompt_version: str
    input_hash: str
    quality_score: float
    summary_text: str
    superseded_by: str
    def __init__(self, summary_version_id: _Optional[str] = ..., article_id: _Optional[str] = ..., user_id: _Optional[str] = ..., generated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., model: _Optional[str] = ..., prompt_version: _Optional[str] = ..., input_hash: _Optional[str] = ..., quality_score: _Optional[float] = ..., summary_text: _Optional[str] = ..., superseded_by: _Optional[str] = ...) -> None: ...

class CreateSummaryVersionRequest(_message.Message):
    __slots__ = ("version",)
    VERSION_FIELD_NUMBER: _ClassVar[int]
    version: SummaryVersion
    def __init__(self, version: _Optional[_Union[SummaryVersion, _Mapping]] = ...) -> None: ...

class CreateSummaryVersionResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class MarkSummaryVersionSupersededRequest(_message.Message):
    __slots__ = ("article_id", "new_version_id")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    NEW_VERSION_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    new_version_id: str
    def __init__(self, article_id: _Optional[str] = ..., new_version_id: _Optional[str] = ...) -> None: ...

class MarkSummaryVersionSupersededResponse(_message.Message):
    __slots__ = ("previous_version",)
    PREVIOUS_VERSION_FIELD_NUMBER: _ClassVar[int]
    previous_version: SummaryVersion
    def __init__(self, previous_version: _Optional[_Union[SummaryVersion, _Mapping]] = ...) -> None: ...

class GetSummaryVersionByIDRequest(_message.Message):
    __slots__ = ("summary_version_id",)
    SUMMARY_VERSION_ID_FIELD_NUMBER: _ClassVar[int]
    summary_version_id: str
    def __init__(self, summary_version_id: _Optional[str] = ...) -> None: ...

class GetSummaryVersionByIDResponse(_message.Message):
    __slots__ = ("version",)
    VERSION_FIELD_NUMBER: _ClassVar[int]
    version: SummaryVersion
    def __init__(self, version: _Optional[_Union[SummaryVersion, _Mapping]] = ...) -> None: ...

class GetLatestSummaryVersionRequest(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class GetLatestSummaryVersionResponse(_message.Message):
    __slots__ = ("version",)
    VERSION_FIELD_NUMBER: _ClassVar[int]
    version: SummaryVersion
    def __init__(self, version: _Optional[_Union[SummaryVersion, _Mapping]] = ...) -> None: ...

class TagSetVersion(_message.Message):
    __slots__ = ("tag_set_version_id", "article_id", "user_id", "generated_at", "generator", "input_hash", "tags_json", "superseded_by")
    TAG_SET_VERSION_ID_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    GENERATED_AT_FIELD_NUMBER: _ClassVar[int]
    GENERATOR_FIELD_NUMBER: _ClassVar[int]
    INPUT_HASH_FIELD_NUMBER: _ClassVar[int]
    TAGS_JSON_FIELD_NUMBER: _ClassVar[int]
    SUPERSEDED_BY_FIELD_NUMBER: _ClassVar[int]
    tag_set_version_id: str
    article_id: str
    user_id: str
    generated_at: _timestamp_pb2.Timestamp
    generator: str
    input_hash: str
    tags_json: bytes
    superseded_by: str
    def __init__(self, tag_set_version_id: _Optional[str] = ..., article_id: _Optional[str] = ..., user_id: _Optional[str] = ..., generated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., generator: _Optional[str] = ..., input_hash: _Optional[str] = ..., tags_json: _Optional[bytes] = ..., superseded_by: _Optional[str] = ...) -> None: ...

class CreateTagSetVersionRequest(_message.Message):
    __slots__ = ("version",)
    VERSION_FIELD_NUMBER: _ClassVar[int]
    version: TagSetVersion
    def __init__(self, version: _Optional[_Union[TagSetVersion, _Mapping]] = ...) -> None: ...

class CreateTagSetVersionResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class MarkTagSetVersionSupersededRequest(_message.Message):
    __slots__ = ("article_id", "new_version_id")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    NEW_VERSION_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    new_version_id: str
    def __init__(self, article_id: _Optional[str] = ..., new_version_id: _Optional[str] = ...) -> None: ...

class MarkTagSetVersionSupersededResponse(_message.Message):
    __slots__ = ("previous_version",)
    PREVIOUS_VERSION_FIELD_NUMBER: _ClassVar[int]
    previous_version: TagSetVersion
    def __init__(self, previous_version: _Optional[_Union[TagSetVersion, _Mapping]] = ...) -> None: ...

class GetTagSetVersionByIDRequest(_message.Message):
    __slots__ = ("tag_set_version_id",)
    TAG_SET_VERSION_ID_FIELD_NUMBER: _ClassVar[int]
    tag_set_version_id: str
    def __init__(self, tag_set_version_id: _Optional[str] = ...) -> None: ...

class GetTagSetVersionByIDResponse(_message.Message):
    __slots__ = ("version",)
    VERSION_FIELD_NUMBER: _ClassVar[int]
    version: TagSetVersion
    def __init__(self, version: _Optional[_Union[TagSetVersion, _Mapping]] = ...) -> None: ...

class GetFeedAmountRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetFeedAmountResponse(_message.Message):
    __slots__ = ("count",)
    COUNT_FIELD_NUMBER: _ClassVar[int]
    count: int
    def __init__(self, count: _Optional[int] = ...) -> None: ...

class GetTotalArticlesCountRequest(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class GetTotalArticlesCountResponse(_message.Message):
    __slots__ = ("count",)
    COUNT_FIELD_NUMBER: _ClassVar[int]
    count: int
    def __init__(self, count: _Optional[int] = ...) -> None: ...

class GetSummarizedArticlesCountRequest(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class GetSummarizedArticlesCountResponse(_message.Message):
    __slots__ = ("count",)
    COUNT_FIELD_NUMBER: _ClassVar[int]
    count: int
    def __init__(self, count: _Optional[int] = ...) -> None: ...

class GetUnsummarizedArticlesCountRequest(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class GetUnsummarizedArticlesCountResponse(_message.Message):
    __slots__ = ("count",)
    COUNT_FIELD_NUMBER: _ClassVar[int]
    count: int
    def __init__(self, count: _Optional[int] = ...) -> None: ...

class GetTodayUnreadArticlesCountRequest(_message.Message):
    __slots__ = ("user_id", "since")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    SINCE_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    since: _timestamp_pb2.Timestamp
    def __init__(self, user_id: _Optional[str] = ..., since: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class GetTodayUnreadArticlesCountResponse(_message.Message):
    __slots__ = ("count",)
    COUNT_FIELD_NUMBER: _ClassVar[int]
    count: int
    def __init__(self, count: _Optional[int] = ...) -> None: ...

class TrendDataPoint(_message.Message):
    __slots__ = ("bucket", "articles", "summarized", "feed_activity")
    BUCKET_FIELD_NUMBER: _ClassVar[int]
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    SUMMARIZED_FIELD_NUMBER: _ClassVar[int]
    FEED_ACTIVITY_FIELD_NUMBER: _ClassVar[int]
    bucket: _timestamp_pb2.Timestamp
    articles: int
    summarized: int
    feed_activity: int
    def __init__(self, bucket: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., articles: _Optional[int] = ..., summarized: _Optional[int] = ..., feed_activity: _Optional[int] = ...) -> None: ...

class GetTrendStatsRequest(_message.Message):
    __slots__ = ("user_id", "window")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    WINDOW_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    window: TrendWindow
    def __init__(self, user_id: _Optional[str] = ..., window: _Optional[_Union[TrendWindow, str]] = ...) -> None: ...

class GetTrendStatsResponse(_message.Message):
    __slots__ = ("points", "granularity")
    POINTS_FIELD_NUMBER: _ClassVar[int]
    GRANULARITY_FIELD_NUMBER: _ClassVar[int]
    points: _containers.RepeatedCompositeFieldContainer[TrendDataPoint]
    granularity: TrendGranularity
    def __init__(self, points: _Optional[_Iterable[_Union[TrendDataPoint, _Mapping]]] = ..., granularity: _Optional[_Union[TrendGranularity, str]] = ...) -> None: ...

class ListUserFeedIDsRequest(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class ListUserFeedIDsResponse(_message.Message):
    __slots__ = ("feed_ids",)
    FEED_IDS_FIELD_NUMBER: _ClassVar[int]
    feed_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, feed_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class NotificationPreferences(_message.Message):
    __slots__ = ("summary_ready", "acolyte_report_ready", "recap_ready", "today_entrance_ready")
    SUMMARY_READY_FIELD_NUMBER: _ClassVar[int]
    ACOLYTE_REPORT_READY_FIELD_NUMBER: _ClassVar[int]
    RECAP_READY_FIELD_NUMBER: _ClassVar[int]
    TODAY_ENTRANCE_READY_FIELD_NUMBER: _ClassVar[int]
    summary_ready: bool
    acolyte_report_ready: bool
    recap_ready: bool
    today_entrance_ready: bool
    def __init__(self, summary_ready: _Optional[bool] = ..., acolyte_report_ready: _Optional[bool] = ..., recap_ready: _Optional[bool] = ..., today_entrance_ready: _Optional[bool] = ...) -> None: ...

class PushSubscription(_message.Message):
    __slots__ = ("user_id", "endpoint", "p256dh", "auth", "preferences", "vapid_key_fingerprint", "created_at", "updated_at", "last_success_at", "last_failure_at")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    P256DH_FIELD_NUMBER: _ClassVar[int]
    AUTH_FIELD_NUMBER: _ClassVar[int]
    PREFERENCES_FIELD_NUMBER: _ClassVar[int]
    VAPID_KEY_FINGERPRINT_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_SUCCESS_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_FAILURE_AT_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    endpoint: str
    p256dh: str
    auth: str
    preferences: NotificationPreferences
    vapid_key_fingerprint: str
    created_at: _timestamp_pb2.Timestamp
    updated_at: _timestamp_pb2.Timestamp
    last_success_at: _timestamp_pb2.Timestamp
    last_failure_at: _timestamp_pb2.Timestamp
    def __init__(self, user_id: _Optional[str] = ..., endpoint: _Optional[str] = ..., p256dh: _Optional[str] = ..., auth: _Optional[str] = ..., preferences: _Optional[_Union[NotificationPreferences, _Mapping]] = ..., vapid_key_fingerprint: _Optional[str] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_success_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_failure_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class UpsertPushSubscriptionRequest(_message.Message):
    __slots__ = ("subscription",)
    SUBSCRIPTION_FIELD_NUMBER: _ClassVar[int]
    subscription: PushSubscription
    def __init__(self, subscription: _Optional[_Union[PushSubscription, _Mapping]] = ...) -> None: ...

class UpsertPushSubscriptionResponse(_message.Message):
    __slots__ = ("created",)
    CREATED_FIELD_NUMBER: _ClassVar[int]
    created: bool
    def __init__(self, created: _Optional[bool] = ...) -> None: ...

class GetPushSubscriptionRequest(_message.Message):
    __slots__ = ("user_id", "endpoint")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    endpoint: str
    def __init__(self, user_id: _Optional[str] = ..., endpoint: _Optional[str] = ...) -> None: ...

class GetPushSubscriptionResponse(_message.Message):
    __slots__ = ("subscription",)
    SUBSCRIPTION_FIELD_NUMBER: _ClassVar[int]
    subscription: PushSubscription
    def __init__(self, subscription: _Optional[_Union[PushSubscription, _Mapping]] = ...) -> None: ...

class UpdatePushSubscriptionPreferencesRequest(_message.Message):
    __slots__ = ("user_id", "endpoint", "preferences")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    PREFERENCES_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    endpoint: str
    preferences: NotificationPreferences
    def __init__(self, user_id: _Optional[str] = ..., endpoint: _Optional[str] = ..., preferences: _Optional[_Union[NotificationPreferences, _Mapping]] = ...) -> None: ...

class UpdatePushSubscriptionPreferencesResponse(_message.Message):
    __slots__ = ("updated",)
    UPDATED_FIELD_NUMBER: _ClassVar[int]
    updated: bool
    def __init__(self, updated: _Optional[bool] = ...) -> None: ...

class DeletePushSubscriptionRequest(_message.Message):
    __slots__ = ("user_id", "endpoint")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    endpoint: str
    def __init__(self, user_id: _Optional[str] = ..., endpoint: _Optional[str] = ...) -> None: ...

class DeletePushSubscriptionResponse(_message.Message):
    __slots__ = ("deleted",)
    DELETED_FIELD_NUMBER: _ClassVar[int]
    deleted: bool
    def __init__(self, deleted: _Optional[bool] = ...) -> None: ...

class ListPushSubscriptionsForUserRequest(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class ListPushSubscriptionsForUserResponse(_message.Message):
    __slots__ = ("subscriptions",)
    SUBSCRIPTIONS_FIELD_NUMBER: _ClassVar[int]
    subscriptions: _containers.RepeatedCompositeFieldContainer[PushSubscription]
    def __init__(self, subscriptions: _Optional[_Iterable[_Union[PushSubscription, _Mapping]]] = ...) -> None: ...

class PushDelivery(_message.Message):
    __slots__ = ("id", "dedupe_key", "subscription_id", "user_id", "kind", "payload", "occurred_at", "state", "attempts", "next_attempt_at", "expires_at", "endpoint", "p256dh", "auth")
    ID_FIELD_NUMBER: _ClassVar[int]
    DEDUPE_KEY_FIELD_NUMBER: _ClassVar[int]
    SUBSCRIPTION_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    KIND_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    OCCURRED_AT_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    ATTEMPTS_FIELD_NUMBER: _ClassVar[int]
    NEXT_ATTEMPT_AT_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    P256DH_FIELD_NUMBER: _ClassVar[int]
    AUTH_FIELD_NUMBER: _ClassVar[int]
    id: str
    dedupe_key: str
    subscription_id: str
    user_id: str
    kind: str
    payload: bytes
    occurred_at: _timestamp_pb2.Timestamp
    state: NotificationState
    attempts: int
    next_attempt_at: _timestamp_pb2.Timestamp
    expires_at: _timestamp_pb2.Timestamp
    endpoint: str
    p256dh: str
    auth: str
    def __init__(self, id: _Optional[str] = ..., dedupe_key: _Optional[str] = ..., subscription_id: _Optional[str] = ..., user_id: _Optional[str] = ..., kind: _Optional[str] = ..., payload: _Optional[bytes] = ..., occurred_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., state: _Optional[_Union[NotificationState, str]] = ..., attempts: _Optional[int] = ..., next_attempt_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., endpoint: _Optional[str] = ..., p256dh: _Optional[str] = ..., auth: _Optional[str] = ...) -> None: ...

class EnqueueNotificationRequest(_message.Message):
    __slots__ = ("dedupe_key", "user_id", "kind", "payload", "occurred_at", "expires_at")
    DEDUPE_KEY_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    KIND_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    OCCURRED_AT_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    dedupe_key: str
    user_id: str
    kind: str
    payload: bytes
    occurred_at: _timestamp_pb2.Timestamp
    expires_at: _timestamp_pb2.Timestamp
    def __init__(self, dedupe_key: _Optional[str] = ..., user_id: _Optional[str] = ..., kind: _Optional[str] = ..., payload: _Optional[bytes] = ..., occurred_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class EnqueueNotificationResponse(_message.Message):
    __slots__ = ("delivery_count", "superseded_count")
    DELIVERY_COUNT_FIELD_NUMBER: _ClassVar[int]
    SUPERSEDED_COUNT_FIELD_NUMBER: _ClassVar[int]
    delivery_count: int
    superseded_count: int
    def __init__(self, delivery_count: _Optional[int] = ..., superseded_count: _Optional[int] = ...) -> None: ...

class ClaimNotificationBatchRequest(_message.Message):
    __slots__ = ("locked_by", "limit", "lease_seconds")
    LOCKED_BY_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    LEASE_SECONDS_FIELD_NUMBER: _ClassVar[int]
    locked_by: str
    limit: int
    lease_seconds: int
    def __init__(self, locked_by: _Optional[str] = ..., limit: _Optional[int] = ..., lease_seconds: _Optional[int] = ...) -> None: ...

class ClaimNotificationBatchResponse(_message.Message):
    __slots__ = ("deliveries",)
    DELIVERIES_FIELD_NUMBER: _ClassVar[int]
    deliveries: _containers.RepeatedCompositeFieldContainer[PushDelivery]
    def __init__(self, deliveries: _Optional[_Iterable[_Union[PushDelivery, _Mapping]]] = ...) -> None: ...

class MarkNotificationSentRequest(_message.Message):
    __slots__ = ("id", "status_code")
    ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_CODE_FIELD_NUMBER: _ClassVar[int]
    id: str
    status_code: int
    def __init__(self, id: _Optional[str] = ..., status_code: _Optional[int] = ...) -> None: ...

class MarkNotificationSentResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ReleaseNotificationRequest(_message.Message):
    __slots__ = ("id", "next_attempt_at", "error_message")
    ID_FIELD_NUMBER: _ClassVar[int]
    NEXT_ATTEMPT_AT_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    id: str
    next_attempt_at: _timestamp_pb2.Timestamp
    error_message: str
    def __init__(self, id: _Optional[str] = ..., next_attempt_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., error_message: _Optional[str] = ...) -> None: ...

class ReleaseNotificationResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class MarkNotificationDeadRequest(_message.Message):
    __slots__ = ("id", "status_code", "error_message")
    ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_CODE_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    id: str
    status_code: int
    error_message: str
    def __init__(self, id: _Optional[str] = ..., status_code: _Optional[int] = ..., error_message: _Optional[str] = ...) -> None: ...

class MarkNotificationDeadResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...
