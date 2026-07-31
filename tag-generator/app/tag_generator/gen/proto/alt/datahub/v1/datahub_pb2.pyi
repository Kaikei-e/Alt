import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

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
    __slots__ = ("article_id", "summary", "language", "user_id")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    summary: str
    language: str
    user_id: str
    def __init__(self, article_id: _Optional[str] = ..., summary: _Optional[str] = ..., language: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

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
