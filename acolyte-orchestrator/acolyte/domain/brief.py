"""ReportBrief — typed input specification for report generation.

Stored in report_briefs table with typed fields (not JSONB).
Converted from proto map<string, string> scope at the handler boundary.
"""

from __future__ import annotations

from dataclasses import dataclass, field


KNOWN_SCOPE_KEYS = {"topic", "report_type", "time_range", "entities", "exclude"}


@dataclass(frozen=True)
class ReportBrief:
    """Immutable report generation specification."""

    topic: str
    report_type: str
    time_range: str | None = None
    entities: list[str] = field(default_factory=list)
    exclude_topics: list[str] = field(default_factory=list)
    constraints: dict[str, str] = field(default_factory=dict)

    @classmethod
    def from_scope(cls, scope: dict[str, str], report_type: str) -> ReportBrief:
        """Construct from proto map<string, string> scope.

        Raises ValueError if topic is missing or blank.
        """
        topic = scope.get("topic", "").strip()
        if not topic:
            raise ValueError("scope must contain a non-empty 'topic' field")

        entities_raw = scope.get("entities", "")
        entities = [e.strip() for e in entities_raw.split(",") if e.strip()] if entities_raw else []

        exclude_raw = scope.get("exclude", "")
        exclude = [e.strip() for e in exclude_raw.split(",") if e.strip()] if exclude_raw else []

        constraints = {k: v for k, v in scope.items() if k not in KNOWN_SCOPE_KEYS}

        return cls(
            topic=topic,
            report_type=report_type,
            time_range=scope.get("time_range"),
            entities=entities,
            exclude_topics=exclude,
            constraints=constraints,
        )

    def to_dict(self) -> dict:
        """Serialize for scope_snapshot storage in report_versions."""
        return {
            "topic": self.topic,
            "report_type": self.report_type,
            "time_range": self.time_range,
            "entities": self.entities,
            "exclude_topics": self.exclude_topics,
            "constraints": self.constraints,
        }
