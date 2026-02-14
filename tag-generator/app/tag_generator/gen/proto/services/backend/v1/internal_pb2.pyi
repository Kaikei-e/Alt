import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ArticleWithTags(_message.Message):
    __slots__ = ("id", "title", "content", "tags", "created_at", "user_id")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    TAGS_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    id: str
    title: str
    content: str
    tags: _containers.RepeatedScalarFieldContainer[str]
    created_at: _timestamp_pb2.Timestamp
    user_id: str
    def __init__(self, id: _Optional[str] = ..., title: _Optional[str] = ..., content: _Optional[str] = ..., tags: _Optional[_Iterable[str]] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., user_id: _Optional[str] = ...) -> None: ...

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
    __slots__ = ("title", "url", "content", "feed_id", "user_id", "published_at")
    TITLE_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    FEED_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    title: str
    url: str
    content: str
    feed_id: str
    user_id: str
    published_at: _timestamp_pb2.Timestamp
    def __init__(self, title: _Optional[str] = ..., url: _Optional[str] = ..., content: _Optional[str] = ..., feed_id: _Optional[str] = ..., user_id: _Optional[str] = ..., published_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class CreateArticleResponse(_message.Message):
    __slots__ = ("article_id",)
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    def __init__(self, article_id: _Optional[str] = ...) -> None: ...

class SaveArticleSummaryRequest(_message.Message):
    __slots__ = ("article_id", "summary", "language")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    summary: str
    language: str
    def __init__(self, article_id: _Optional[str] = ..., summary: _Optional[str] = ..., language: _Optional[str] = ...) -> None: ...

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
    __slots__ = ("article_id", "title", "content", "url")
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    article_id: str
    title: str
    content: str
    url: str
    def __init__(self, article_id: _Optional[str] = ..., title: _Optional[str] = ..., content: _Optional[str] = ..., url: _Optional[str] = ...) -> None: ...

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
    __slots__ = ("limit", "offset")
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    OFFSET_FIELD_NUMBER: _ClassVar[int]
    limit: int
    offset: int
    def __init__(self, limit: _Optional[int] = ..., offset: _Optional[int] = ...) -> None: ...

class ListUntaggedArticlesResponse(_message.Message):
    __slots__ = ("articles", "total_count")
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    TOTAL_COUNT_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[ArticleWithTags]
    total_count: int
    def __init__(self, articles: _Optional[_Iterable[_Union[ArticleWithTags, _Mapping]]] = ..., total_count: _Optional[int] = ...) -> None: ...
