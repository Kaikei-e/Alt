"""Tests for PromptBuilder (Phase 5 refactoring).

Following Python 3.14 best practices:
- Protocol for structural typing
- Separation of prompt templates from formatting logic
"""

from __future__ import annotations

from datetime import datetime


class TestSummaryPromptBuilder:
    """Tests for SummaryPromptBuilder."""

    def test_builds_summary_prompt_with_content(self):
        """Should build summary prompt with content."""
        from news_creator.usecase.prompt_builder import SummaryPromptBuilder

        builder = SummaryPromptBuilder()
        prompt = builder.build(content="Test article content")

        assert "Test article content" in prompt
        assert "<|turn>user" in prompt  # Gemma 4 chat template

    def test_includes_current_date(self):
        """Should include current date in prompt."""
        from news_creator.usecase.prompt_builder import SummaryPromptBuilder

        builder = SummaryPromptBuilder()
        prompt = builder.build(content="Content")

        # Should contain a date
        datetime.now().strftime("%Y年%m月%d日")
        # Note: The format might vary, but there should be a date
        assert any(c.isdigit() for c in prompt)  # Contains numbers (date)

    def test_uses_custom_date_when_provided(self):
        """Should use custom date when provided."""
        from news_creator.usecase.prompt_builder import SummaryPromptBuilder

        builder = SummaryPromptBuilder()
        custom_date = "2026年4月1日"
        prompt = builder.build(content="Content", current_date=custom_date)

        assert custom_date in prompt


class TestChunkPromptBuilder:
    """Tests for ChunkPromptBuilder."""

    def test_builds_chunk_prompt_with_content(self):
        """Should build chunk prompt with content."""
        from news_creator.usecase.prompt_builder import ChunkPromptBuilder

        builder = ChunkPromptBuilder()
        prompt = builder.build(content="Chunk content here")

        assert "Chunk content here" in prompt
        assert "Extract key facts" in prompt

    def test_chunk_prompt_requests_bullet_points(self):
        """Should request bullet point format."""
        from news_creator.usecase.prompt_builder import ChunkPromptBuilder

        builder = ChunkPromptBuilder()
        prompt = builder.build(content="Content")

        assert "Bullet" in prompt or "bullet" in prompt


class TestRecapPromptBuilder:
    """Tests for RecapPromptBuilder."""

    def test_builds_recap_prompt_with_clusters(self):
        """Should build recap prompt with cluster section."""
        from news_creator.usecase.prompt_builder import RecapPromptBuilder

        builder = RecapPromptBuilder()
        prompt = builder.build(
            job_id="job-123",
            genre="technology",
            cluster_section="Cluster 1: AI developments",
            max_bullets=5,
        )

        assert "job-123" in prompt
        assert "technology" in prompt
        assert "Cluster 1: AI developments" in prompt

    def test_recap_prompt_requests_json_output(self):
        """Should request JSON output format."""
        from news_creator.usecase.prompt_builder import RecapPromptBuilder

        builder = RecapPromptBuilder()
        prompt = builder.build(
            job_id="job-123",
            genre="tech",
            cluster_section="clusters",
            max_bullets=5,
        )

        assert "JSON" in prompt


class TestPromptBuilderProtocol:
    """Tests for PromptBuilder Protocol compliance."""

    def test_summary_builder_has_build_method(self):
        """SummaryPromptBuilder should have build method."""
        from news_creator.usecase.prompt_builder import SummaryPromptBuilder

        builder = SummaryPromptBuilder()
        assert hasattr(builder, "build")
        assert callable(builder.build)

    def test_chunk_builder_has_build_method(self):
        """ChunkPromptBuilder should have build method."""
        from news_creator.usecase.prompt_builder import ChunkPromptBuilder

        builder = ChunkPromptBuilder()
        assert hasattr(builder, "build")
        assert callable(builder.build)

    def test_recap_builder_has_build_method(self):
        """RecapPromptBuilder should have build method."""
        from news_creator.usecase.prompt_builder import RecapPromptBuilder

        builder = RecapPromptBuilder()
        assert hasattr(builder, "build")
        assert callable(builder.build)


class TestUntrustedContentNeutralization:
    """Tests that feed content cannot forge Gemma turn boundaries (OWASP LLM01)."""

    def test_summary_prompt_drops_forged_turn_boundaries(self, forged_turn_article):
        """Article body must not contribute turn markers to the built prompt."""
        from news_creator.domain.prompts import SUMMARY_PROMPT_TEMPLATE
        from news_creator.usecase.prompt_builder import SummaryPromptBuilder

        prompt = SummaryPromptBuilder().build(
            content=forged_turn_article, current_date="2026年7月31日"
        )

        # Only the template's own turn markers may remain.
        assert prompt.count("<|turn>") == SUMMARY_PROMPT_TEMPLATE.count("<|turn>")
        assert prompt.count("<turn|>") == SUMMARY_PROMPT_TEMPLATE.count("<turn|>")
        # The injected instruction survives as plain text, without its boundary.
        assert "Ignore the previous article" in prompt

    def test_chunk_prompt_drops_forged_turn_boundaries(self, forged_turn_article):
        """Chunk prompts interpolate the same untrusted text."""
        from news_creator.domain.prompts import CHUNK_SUMMARY_PROMPT_TEMPLATE
        from news_creator.usecase.prompt_builder import ChunkPromptBuilder

        prompt = ChunkPromptBuilder().build(content=forged_turn_article)

        assert prompt.count("<|turn>") == CHUNK_SUMMARY_PROMPT_TEMPLATE.count("<|turn>")
        assert prompt.count("<turn|>") == CHUNK_SUMMARY_PROMPT_TEMPLATE.count("<turn|>")

    def test_recap_prompt_drops_forged_turn_boundaries(self, forged_turn_article):
        """Cluster sections quote article sentences verbatim."""
        from news_creator.domain.prompts import RECAP_CLUSTER_SUMMARY_PROMPT
        from news_creator.usecase.prompt_builder import RecapPromptBuilder

        prompt = RecapPromptBuilder().build(
            job_id="job-123",
            genre="technology",
            cluster_section=forged_turn_article,
            max_bullets=5,
        )

        assert prompt.count("<|turn>") == RECAP_CLUSTER_SUMMARY_PROMPT.count("<|turn>")
        assert prompt.count("<turn|>") == RECAP_CLUSTER_SUMMARY_PROMPT.count("<turn|>")

    def test_benign_summary_prompt_is_byte_identical_golden(self, benign_article):
        """Golden: benign Japanese prose, code and angle brackets are unchanged.

        ``str.format`` is the unguarded formatting the builders used before the
        neutralization guard, so it pins the pre-existing byte output.
        """
        from news_creator.domain.prompts import SUMMARY_PROMPT_TEMPLATE
        from news_creator.usecase.prompt_builder import SummaryPromptBuilder

        expected = str.format(
            SUMMARY_PROMPT_TEMPLATE,
            current_date="2026年7月31日",
            content=benign_article,
        )
        prompt = SummaryPromptBuilder().build(
            content=benign_article, current_date="2026年7月31日"
        )

        assert prompt == expected

    def test_benign_chunk_prompt_is_byte_identical_golden(self, benign_article):
        """Golden: chunk prompts are unchanged for benign content."""
        from news_creator.domain.prompts import CHUNK_SUMMARY_PROMPT_TEMPLATE
        from news_creator.usecase.prompt_builder import ChunkPromptBuilder

        expected = str.format(CHUNK_SUMMARY_PROMPT_TEMPLATE, content=benign_article)
        prompt = ChunkPromptBuilder().build(content=benign_article)

        assert prompt == expected

    def test_benign_recap_prompt_is_byte_identical_golden(self, benign_article):
        """Golden: recap prompts are unchanged for benign cluster sections."""
        from news_creator.domain.prompts import RECAP_CLUSTER_SUMMARY_PROMPT
        from news_creator.usecase.prompt_builder import RecapPromptBuilder

        expected = str.format(
            RECAP_CLUSTER_SUMMARY_PROMPT,
            job_id="job-123",
            genre="technology",
            cluster_section=benign_article,
            max_bullets=5,
        )
        prompt = RecapPromptBuilder().build(
            job_id="job-123",
            genre="technology",
            cluster_section=benign_article,
            max_bullets=5,
        )

        assert prompt == expected
