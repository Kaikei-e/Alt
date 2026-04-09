from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class CreateReportRequest(_message.Message):
    __slots__ = ("title", "report_type", "scope")
    class ScopeEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    TITLE_FIELD_NUMBER: _ClassVar[int]
    REPORT_TYPE_FIELD_NUMBER: _ClassVar[int]
    SCOPE_FIELD_NUMBER: _ClassVar[int]
    title: str
    report_type: str
    scope: _containers.ScalarMap[str, str]
    def __init__(self, title: _Optional[str] = ..., report_type: _Optional[str] = ..., scope: _Optional[_Mapping[str, str]] = ...) -> None: ...

class CreateReportResponse(_message.Message):
    __slots__ = ("report_id",)
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    def __init__(self, report_id: _Optional[str] = ...) -> None: ...

class GetReportRequest(_message.Message):
    __slots__ = ("report_id",)
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    def __init__(self, report_id: _Optional[str] = ...) -> None: ...

class GetReportResponse(_message.Message):
    __slots__ = ("report", "sections")
    REPORT_FIELD_NUMBER: _ClassVar[int]
    SECTIONS_FIELD_NUMBER: _ClassVar[int]
    report: Report
    sections: _containers.RepeatedCompositeFieldContainer[ReportSection]
    def __init__(self, report: _Optional[_Union[Report, _Mapping]] = ..., sections: _Optional[_Iterable[_Union[ReportSection, _Mapping]]] = ...) -> None: ...

class ListReportsRequest(_message.Message):
    __slots__ = ("cursor", "limit")
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    cursor: str
    limit: int
    def __init__(self, cursor: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class ListReportsResponse(_message.Message):
    __slots__ = ("reports", "next_cursor", "has_more")
    REPORTS_FIELD_NUMBER: _ClassVar[int]
    NEXT_CURSOR_FIELD_NUMBER: _ClassVar[int]
    HAS_MORE_FIELD_NUMBER: _ClassVar[int]
    reports: _containers.RepeatedCompositeFieldContainer[ReportSummary]
    next_cursor: str
    has_more: bool
    def __init__(self, reports: _Optional[_Iterable[_Union[ReportSummary, _Mapping]]] = ..., next_cursor: _Optional[str] = ..., has_more: _Optional[bool] = ...) -> None: ...

class GetReportVersionRequest(_message.Message):
    __slots__ = ("report_id", "version_no")
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    VERSION_NO_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    version_no: int
    def __init__(self, report_id: _Optional[str] = ..., version_no: _Optional[int] = ...) -> None: ...

class GetReportVersionResponse(_message.Message):
    __slots__ = ("version", "change_items", "section_versions")
    VERSION_FIELD_NUMBER: _ClassVar[int]
    CHANGE_ITEMS_FIELD_NUMBER: _ClassVar[int]
    SECTION_VERSIONS_FIELD_NUMBER: _ClassVar[int]
    version: ReportVersion
    change_items: _containers.RepeatedCompositeFieldContainer[ChangeItem]
    section_versions: _containers.RepeatedCompositeFieldContainer[SectionVersion]
    def __init__(self, version: _Optional[_Union[ReportVersion, _Mapping]] = ..., change_items: _Optional[_Iterable[_Union[ChangeItem, _Mapping]]] = ..., section_versions: _Optional[_Iterable[_Union[SectionVersion, _Mapping]]] = ...) -> None: ...

class ListReportVersionsRequest(_message.Message):
    __slots__ = ("report_id", "cursor", "limit")
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    cursor: str
    limit: int
    def __init__(self, report_id: _Optional[str] = ..., cursor: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class ListReportVersionsResponse(_message.Message):
    __slots__ = ("versions", "next_cursor", "has_more")
    VERSIONS_FIELD_NUMBER: _ClassVar[int]
    NEXT_CURSOR_FIELD_NUMBER: _ClassVar[int]
    HAS_MORE_FIELD_NUMBER: _ClassVar[int]
    versions: _containers.RepeatedCompositeFieldContainer[ReportVersionSummary]
    next_cursor: str
    has_more: bool
    def __init__(self, versions: _Optional[_Iterable[_Union[ReportVersionSummary, _Mapping]]] = ..., next_cursor: _Optional[str] = ..., has_more: _Optional[bool] = ...) -> None: ...

class DiffReportVersionsRequest(_message.Message):
    __slots__ = ("report_id", "from_version", "to_version")
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    FROM_VERSION_FIELD_NUMBER: _ClassVar[int]
    TO_VERSION_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    from_version: int
    to_version: int
    def __init__(self, report_id: _Optional[str] = ..., from_version: _Optional[int] = ..., to_version: _Optional[int] = ...) -> None: ...

class DiffReportVersionsResponse(_message.Message):
    __slots__ = ("change_items", "section_diffs")
    CHANGE_ITEMS_FIELD_NUMBER: _ClassVar[int]
    SECTION_DIFFS_FIELD_NUMBER: _ClassVar[int]
    change_items: _containers.RepeatedCompositeFieldContainer[ChangeItem]
    section_diffs: _containers.RepeatedCompositeFieldContainer[SectionDiff]
    def __init__(self, change_items: _Optional[_Iterable[_Union[ChangeItem, _Mapping]]] = ..., section_diffs: _Optional[_Iterable[_Union[SectionDiff, _Mapping]]] = ...) -> None: ...

class StartReportRunRequest(_message.Message):
    __slots__ = ("report_id",)
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    def __init__(self, report_id: _Optional[str] = ...) -> None: ...

class StartReportRunResponse(_message.Message):
    __slots__ = ("run_id",)
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    def __init__(self, run_id: _Optional[str] = ...) -> None: ...

class GetRunStatusRequest(_message.Message):
    __slots__ = ("run_id",)
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    def __init__(self, run_id: _Optional[str] = ...) -> None: ...

class GetRunStatusResponse(_message.Message):
    __slots__ = ("run", "jobs")
    RUN_FIELD_NUMBER: _ClassVar[int]
    JOBS_FIELD_NUMBER: _ClassVar[int]
    run: ReportRun
    jobs: _containers.RepeatedCompositeFieldContainer[ReportJob]
    def __init__(self, run: _Optional[_Union[ReportRun, _Mapping]] = ..., jobs: _Optional[_Iterable[_Union[ReportJob, _Mapping]]] = ...) -> None: ...

class StreamRunProgressRequest(_message.Message):
    __slots__ = ("run_id",)
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    def __init__(self, run_id: _Optional[str] = ...) -> None: ...

class StreamRunProgressResponse(_message.Message):
    __slots__ = ("kind", "step", "delta", "error_message", "done")
    KIND_FIELD_NUMBER: _ClassVar[int]
    STEP_FIELD_NUMBER: _ClassVar[int]
    DELTA_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    DONE_FIELD_NUMBER: _ClassVar[int]
    kind: str
    step: StepEvent
    delta: str
    error_message: str
    done: RunDoneEvent
    def __init__(self, kind: _Optional[str] = ..., step: _Optional[_Union[StepEvent, _Mapping]] = ..., delta: _Optional[str] = ..., error_message: _Optional[str] = ..., done: _Optional[_Union[RunDoneEvent, _Mapping]] = ...) -> None: ...

class StepEvent(_message.Message):
    __slots__ = ("step_name", "step_index", "total_steps")
    STEP_NAME_FIELD_NUMBER: _ClassVar[int]
    STEP_INDEX_FIELD_NUMBER: _ClassVar[int]
    TOTAL_STEPS_FIELD_NUMBER: _ClassVar[int]
    step_name: str
    step_index: int
    total_steps: int
    def __init__(self, step_name: _Optional[str] = ..., step_index: _Optional[int] = ..., total_steps: _Optional[int] = ...) -> None: ...

class RunDoneEvent(_message.Message):
    __slots__ = ("new_version_no", "run_status")
    NEW_VERSION_NO_FIELD_NUMBER: _ClassVar[int]
    RUN_STATUS_FIELD_NUMBER: _ClassVar[int]
    new_version_no: int
    run_status: str
    def __init__(self, new_version_no: _Optional[int] = ..., run_status: _Optional[str] = ...) -> None: ...

class RerunSectionRequest(_message.Message):
    __slots__ = ("report_id", "section_key")
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    SECTION_KEY_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    section_key: str
    def __init__(self, report_id: _Optional[str] = ..., section_key: _Optional[str] = ...) -> None: ...

class RerunSectionResponse(_message.Message):
    __slots__ = ("run_id",)
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    def __init__(self, run_id: _Optional[str] = ...) -> None: ...

class HealthCheckRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class HealthCheckResponse(_message.Message):
    __slots__ = ("status",)
    STATUS_FIELD_NUMBER: _ClassVar[int]
    status: str
    def __init__(self, status: _Optional[str] = ...) -> None: ...

class Report(_message.Message):
    __slots__ = ("report_id", "title", "report_type", "current_version", "latest_successful_run_id", "created_at")
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    REPORT_TYPE_FIELD_NUMBER: _ClassVar[int]
    CURRENT_VERSION_FIELD_NUMBER: _ClassVar[int]
    LATEST_SUCCESSFUL_RUN_ID_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    title: str
    report_type: str
    current_version: int
    latest_successful_run_id: str
    created_at: str
    def __init__(self, report_id: _Optional[str] = ..., title: _Optional[str] = ..., report_type: _Optional[str] = ..., current_version: _Optional[int] = ..., latest_successful_run_id: _Optional[str] = ..., created_at: _Optional[str] = ...) -> None: ...

class ReportSummary(_message.Message):
    __slots__ = ("report_id", "title", "report_type", "current_version", "latest_run_status", "created_at")
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    REPORT_TYPE_FIELD_NUMBER: _ClassVar[int]
    CURRENT_VERSION_FIELD_NUMBER: _ClassVar[int]
    LATEST_RUN_STATUS_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    title: str
    report_type: str
    current_version: int
    latest_run_status: str
    created_at: str
    def __init__(self, report_id: _Optional[str] = ..., title: _Optional[str] = ..., report_type: _Optional[str] = ..., current_version: _Optional[int] = ..., latest_run_status: _Optional[str] = ..., created_at: _Optional[str] = ...) -> None: ...

class ReportVersion(_message.Message):
    __slots__ = ("report_id", "version_no", "change_seq", "change_reason", "created_at", "prompt_template_version")
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    VERSION_NO_FIELD_NUMBER: _ClassVar[int]
    CHANGE_SEQ_FIELD_NUMBER: _ClassVar[int]
    CHANGE_REASON_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    PROMPT_TEMPLATE_VERSION_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    version_no: int
    change_seq: int
    change_reason: str
    created_at: str
    prompt_template_version: str
    def __init__(self, report_id: _Optional[str] = ..., version_no: _Optional[int] = ..., change_seq: _Optional[int] = ..., change_reason: _Optional[str] = ..., created_at: _Optional[str] = ..., prompt_template_version: _Optional[str] = ...) -> None: ...

class ReportVersionSummary(_message.Message):
    __slots__ = ("version_no", "change_reason", "created_at", "change_items")
    VERSION_NO_FIELD_NUMBER: _ClassVar[int]
    CHANGE_REASON_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    CHANGE_ITEMS_FIELD_NUMBER: _ClassVar[int]
    version_no: int
    change_reason: str
    created_at: str
    change_items: _containers.RepeatedCompositeFieldContainer[ChangeItem]
    def __init__(self, version_no: _Optional[int] = ..., change_reason: _Optional[str] = ..., created_at: _Optional[str] = ..., change_items: _Optional[_Iterable[_Union[ChangeItem, _Mapping]]] = ...) -> None: ...

class ChangeItem(_message.Message):
    __slots__ = ("field_name", "change_kind", "old_fingerprint", "new_fingerprint")
    FIELD_NAME_FIELD_NUMBER: _ClassVar[int]
    CHANGE_KIND_FIELD_NUMBER: _ClassVar[int]
    OLD_FINGERPRINT_FIELD_NUMBER: _ClassVar[int]
    NEW_FINGERPRINT_FIELD_NUMBER: _ClassVar[int]
    field_name: str
    change_kind: str
    old_fingerprint: str
    new_fingerprint: str
    def __init__(self, field_name: _Optional[str] = ..., change_kind: _Optional[str] = ..., old_fingerprint: _Optional[str] = ..., new_fingerprint: _Optional[str] = ...) -> None: ...

class ReportSection(_message.Message):
    __slots__ = ("section_key", "current_version", "display_order", "body", "citations_json")
    SECTION_KEY_FIELD_NUMBER: _ClassVar[int]
    CURRENT_VERSION_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_ORDER_FIELD_NUMBER: _ClassVar[int]
    BODY_FIELD_NUMBER: _ClassVar[int]
    CITATIONS_JSON_FIELD_NUMBER: _ClassVar[int]
    section_key: str
    current_version: int
    display_order: int
    body: str
    citations_json: str
    def __init__(self, section_key: _Optional[str] = ..., current_version: _Optional[int] = ..., display_order: _Optional[int] = ..., body: _Optional[str] = ..., citations_json: _Optional[str] = ...) -> None: ...

class SectionVersion(_message.Message):
    __slots__ = ("section_key", "version_no", "body", "citations_json", "created_at")
    SECTION_KEY_FIELD_NUMBER: _ClassVar[int]
    VERSION_NO_FIELD_NUMBER: _ClassVar[int]
    BODY_FIELD_NUMBER: _ClassVar[int]
    CITATIONS_JSON_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    section_key: str
    version_no: int
    body: str
    citations_json: str
    created_at: str
    def __init__(self, section_key: _Optional[str] = ..., version_no: _Optional[int] = ..., body: _Optional[str] = ..., citations_json: _Optional[str] = ..., created_at: _Optional[str] = ...) -> None: ...

class SectionDiff(_message.Message):
    __slots__ = ("section_key", "old_body", "new_body", "old_version", "new_version")
    SECTION_KEY_FIELD_NUMBER: _ClassVar[int]
    OLD_BODY_FIELD_NUMBER: _ClassVar[int]
    NEW_BODY_FIELD_NUMBER: _ClassVar[int]
    OLD_VERSION_FIELD_NUMBER: _ClassVar[int]
    NEW_VERSION_FIELD_NUMBER: _ClassVar[int]
    section_key: str
    old_body: str
    new_body: str
    old_version: int
    new_version: int
    def __init__(self, section_key: _Optional[str] = ..., old_body: _Optional[str] = ..., new_body: _Optional[str] = ..., old_version: _Optional[int] = ..., new_version: _Optional[int] = ...) -> None: ...

class ReportRun(_message.Message):
    __slots__ = ("run_id", "report_id", "target_version_no", "run_status", "planner_model", "writer_model", "critic_model", "started_at", "finished_at", "failure_code", "failure_message")
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    TARGET_VERSION_NO_FIELD_NUMBER: _ClassVar[int]
    RUN_STATUS_FIELD_NUMBER: _ClassVar[int]
    PLANNER_MODEL_FIELD_NUMBER: _ClassVar[int]
    WRITER_MODEL_FIELD_NUMBER: _ClassVar[int]
    CRITIC_MODEL_FIELD_NUMBER: _ClassVar[int]
    STARTED_AT_FIELD_NUMBER: _ClassVar[int]
    FINISHED_AT_FIELD_NUMBER: _ClassVar[int]
    FAILURE_CODE_FIELD_NUMBER: _ClassVar[int]
    FAILURE_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    run_id: str
    report_id: str
    target_version_no: int
    run_status: str
    planner_model: str
    writer_model: str
    critic_model: str
    started_at: str
    finished_at: str
    failure_code: str
    failure_message: str
    def __init__(self, run_id: _Optional[str] = ..., report_id: _Optional[str] = ..., target_version_no: _Optional[int] = ..., run_status: _Optional[str] = ..., planner_model: _Optional[str] = ..., writer_model: _Optional[str] = ..., critic_model: _Optional[str] = ..., started_at: _Optional[str] = ..., finished_at: _Optional[str] = ..., failure_code: _Optional[str] = ..., failure_message: _Optional[str] = ...) -> None: ...

class ReportJob(_message.Message):
    __slots__ = ("job_id", "run_id", "job_status", "attempt_no", "claimed_by", "claimed_at", "created_at")
    JOB_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    JOB_STATUS_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_NO_FIELD_NUMBER: _ClassVar[int]
    CLAIMED_BY_FIELD_NUMBER: _ClassVar[int]
    CLAIMED_AT_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    job_id: str
    run_id: str
    job_status: str
    attempt_no: int
    claimed_by: str
    claimed_at: str
    created_at: str
    def __init__(self, job_id: _Optional[str] = ..., run_id: _Optional[str] = ..., job_status: _Optional[str] = ..., attempt_no: _Optional[int] = ..., claimed_by: _Optional[str] = ..., claimed_at: _Optional[str] = ..., created_at: _Optional[str] = ...) -> None: ...
