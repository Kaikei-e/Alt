# CLAUDE.md - Tag Generator Service

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- DO NOT use opus unless explicitly requested -->

## About this Service

The **tag-generator** is a Python-based ML microservice that automatically generates relevant tags for RSS feed articles. It operates as part of a larger mobile-first RSS reader ecosystem built with microservice architecture.

**Service Purpose:**
- Analyze article content and metadata to generate contextually relevant tags
- Enhance content discoverability and organization
- Support multi-language content processing
- Provide ML-powered content classification

**Architecture Role:**
- **Type:** Helper Application (ML/Data Processing)
- **Language:** Python 3.13
- **Pattern:** ML Pipeline Architecture
- **Integration:** Communicates with main backend via REST APIs

## Tech Stack

### Core Technologies
- **Language:** Python 3.13+
- **Package Manager:** UV (modern, fast Python package manager)
- **ML Framework:** scikit-learn, transformers, or similar
- **Text Processing:** NLTK, spaCy, or similar
- **HTTP Client:** requests or httpx
- **Data Processing:** pandas, numpy
- **Configuration:** pydantic for settings management
- **Testing:** pytest with fixtures (**TDD FIRST PRIORITY**)

### UV Commands for Development
```bash
# Project initialization
uv init tag-generator
cd tag-generator

# Install dependencies (TDD first!)
uv add --dev pytest pytest-asyncio pytest-cov pytest-mock
uv add --dev ruff mypy pre-commit

# Add ML dependencies
uv add transformers torch scikit-learn spacy nltk
uv add pandas numpy pydantic httpx fastapi uvicorn

# Run tests (TDD PRIORITY)
uv run pytest                           # Run all tests
uv run pytest --cov                    # With coverage
uv run pytest -v                       # Verbose output
uv run pytest tests/unit/              # Unit tests only
uv run pytest -k "test_tag_generation" # Specific tests
uv run pytest --lf                     # Last failed tests

# Code quality
uv run ruff check                       # Linting
uv run ruff format                      # Formatting
uv run mypy src/                        # Type checking

# Development server
uv run uvicorn src.tag_generator.main:app --reload

# Install pre-commit hooks
uv run pre-commit install
```

## Architecture Design

### Service Architecture Pattern
```
Input → Preprocessing → Feature Extraction → ML Models → Post-processing → Output
   ↓         ↓              ↓                ↓             ↓            ↓
Article   Clean Text    Vector/Features   Tag Scores   Filter/Rank   Tags
Content   Normalize     TF-IDF/Embeddings  Multi-class  Threshold    JSON
```

### UV Project Structure (TDD-First Layout)
```
/tag-generator
├─ src/
│  └─ tag_generator/           # Main package
│     ├─ __init__.py
│     ├─ main.py              # Entry point
│     ├─ config/              # Configuration management
│     │  ├─ __init__.py
│     │  └─ settings.py       # Pydantic settings
│     ├─ core/                # Core business logic
│     │  ├─ __init__.py
│     │  ├─ tag_generator.py  # Main tag generation logic
│     │  └─ models.py         # Data models
│     ├─ ml/                  # Machine learning components
│     │  ├─ __init__.py
│     │  ├─ feature_extractor.py  # Feature engineering
│     │  ├─ classifiers.py    # ML model implementations
│     │  └─ embeddings.py     # Text embedding utilities
│     ├─ preprocessing/       # Text preprocessing
│     │  ├─ __init__.py
│     │  ├─ text_cleaner.py   # Text cleaning utilities
│     │  └─ language_detector.py # Language detection
│     ├─ api/                 # API layer (if exposing HTTP)
│     │  ├─ __init__.py
│     │  └─ routes.py         # FastAPI routes
│     └─ utils/               # Utilities
│        ├─ __init__.py
│        ├─ logger.py         # Logging configuration
│        └─ exceptions.py     # Custom exceptions
├─ tests/                    # Test suite (TDD PRIORITY)
│  ├─ __init__.py
│  ├─ conftest.py            # Pytest configuration and fixtures
│  ├─ unit/                  # Unit tests (RED-GREEN-REFACTOR)
│  │  ├─ __init__.py
│  │  ├─ test_tag_generator.py    # Core logic tests
│  │  ├─ test_feature_extractor.py # Feature extraction tests
│  │  ├─ test_text_cleaner.py     # Preprocessing tests
│  │  ├─ test_classifiers.py      # ML model tests
│  │  └─ test_models.py           # Data model tests
│  ├─ integration/           # Integration tests
│  │  ├─ __init__.py
│  │  ├─ test_api_endpoints.py    # API integration tests
│  │  ├─ test_ml_pipeline.py      # End-to-end ML tests
│  │  └─ test_external_apis.py    # External service tests
│  ├─ fixtures/              # Test fixtures and data
│  │  ├─ sample_articles.json     # Test articles
│  │  ├─ expected_tags.json       # Expected outputs
│  │  └─ mock_models/             # Mock ML models for testing
│  └─ performance/           # Performance tests
│     ├─ __init__.py
│     └─ test_benchmarks.py       # Performance benchmarks
├─ models/                   # Pre-trained models (UV ignored)
│  ├─ classifiers/          # Saved ML models
│  ├─ embeddings/           # Pre-trained embeddings
│  └─ cached_models/        # Downloaded model cache
├─ data/                    # Training and test data
│  ├─ training/             # Training datasets
│  ├─ validation/           # Validation sets
│  └─ sample/               # Sample data for testing
├─ scripts/                 # UV run scripts
│  ├─ train_model.py       # Model training
│  ├─ evaluate_model.py    # Model evaluation
│  ├─ setup_dev.py         # Development setup
│  └─ run_tests.py         # Test runner script
├─ config/                 # Configuration files
│  ├─ model_config.yaml    # ML model configurations
│  └─ tag_categories.yaml  # Tag taxonomy
├─ .github/                # GitHub workflows
│  └─ workflows/
│     ├─ ci.yml           # TDD CI pipeline
│     └─ release.yml      # Release pipeline
├─ pyproject.toml         # UV project configuration
├─ uv.lock               # UV lock file
├─ .env.example          # Environment variables template
├─ .gitignore            # Git ignore (includes UV cache)
├─ .pre-commit-config.yaml # Pre-commit hooks
├─ Dockerfile            # Container definition
└─ README.md             # Service documentation
```

## TDD Development Guidelines

### **CRITICAL: Test-Driven Development is MANDATORY**

**🔴 RED → 🟢 GREEN → 🔄 REFACTOR cycle MUST be followed for ALL code:**

1. **🔴 RED:** Write a FAILING test first
2. **🟢 GREEN:** Write MINIMAL code to make the test pass
3. **🔄 REFACTOR:** Improve code quality while keeping tests green
4. **REPEAT:** Continue cycle for next feature

### TDD Workflow with UV

#### 1. Start with a Failing Test (RED)
```bash
# Create test file first
uv run pytest tests/unit/test_tag_generator.py::test_generate_tags_for_article -v
# Expected: FAIL (function doesn't exist yet)
```

#### 2. Write Minimal Implementation (GREEN)
```python
# tests/unit/test_tag_generator.py (WRITE THIS FIRST)
import pytest
from tag_generator.core.tag_generator import TagGenerator
from tag_generator.core.models import ArticleContent, TagResult

class TestTagGenerator:
    @pytest.fixture
    def tag_generator(self):
        return TagGenerator()

    @pytest.fixture
    def sample_article(self):
        return ArticleContent(
            title="Machine Learning in Python",
            content="This article discusses ML algorithms and their implementation.",
            url="https://example.com/ml-article"
        )

    @pytest.mark.asyncio
    async def test_generate_tags_for_article(self, tag_generator, sample_article):
        # RED: This test MUST fail first
        tags = await tag_generator.generate_tags(sample_article)

        assert len(tags) > 0
        assert isinstance(tags[0], TagResult)
        assert tags[0].confidence > 0.0
        assert "machine learning" in [tag.tag.lower() for tag in tags]
```

```bash
# Run the failing test
uv run pytest tests/unit/test_tag_generator.py::test_generate_tags_for_article -v
# Result: FAIL ❌ (ImportError: No module named 'tag_generator.core.tag_generator')
```

#### 3. Create Minimal Implementation (GREEN)
```python
# src/tag_generator/core/models.py (Create this to make test importable)
from pydantic import BaseModel
from typing import List, Optional
from datetime import datetime

class TagResult(BaseModel):
    tag: str
    confidence: float
    source: str = "ml_model"

class ArticleContent(BaseModel):
    title: str
    content: str
    url: Optional[str] = None
```

```python
# src/tag_generator/core/tag_generator.py (Minimal implementation)
from typing import List
from .models import ArticleContent, TagResult

class TagGenerator:
    async def generate_tags(self, article: ArticleContent) -> List[TagResult]:
        # Minimal implementation to make test pass
        if "machine learning" in article.content.lower() or "ml" in article.content.lower():
            return [TagResult(tag="machine learning", confidence=0.8)]
        return []
```

```bash
# Run test again
uv run pytest tests/unit/test_tag_generator.py::test_generate_tags_for_article -v
# Result: PASS ✅
```

#### 4. Refactor (REFACTOR)
```python
# Now improve the implementation while keeping tests green
class TagGenerator:
    def __init__(self):
        self.keyword_patterns = {
            "machine learning": ["machine learning", "ml algorithms", "supervised learning"],
            "python": ["python", "pandas", "numpy", "scikit-learn"],
            "data science": ["data science", "data analysis", "statistics"]
        }

    async def generate_tags(self, article: ArticleContent) -> List[TagResult]:
        content_lower = f"{article.title} {article.content}".lower()
        tags = []

        for tag, patterns in self.keyword_patterns.items():
            confidence = self._calculate_confidence(content_lower, patterns)
            if confidence > 0.3:
                tags.append(TagResult(tag=tag, confidence=confidence))

        return sorted(tags, key=lambda x: x.confidence, reverse=True)

    def _calculate_confidence(self, content: str, patterns: List[str]) -> float:
        matches = sum(1 for pattern in patterns if pattern in content)
        return min(matches * 0.4, 1.0)
```

```bash
# Ensure tests still pass after refactoring
uv run pytest tests/unit/test_tag_generator.py -v
# Result: PASS ✅
```

### TDD Testing Strategy

#### Test Categories (All with UV)
```bash
# Unit tests (FASTEST - Run frequently during TDD)
uv run pytest tests/unit/ -v

# Integration tests (SLOWER - Run after unit tests pass)
uv run pytest tests/integration/ -v

# Performance tests (SLOWEST - Run before commits)
uv run pytest tests/performance/ -v

# All tests with coverage
uv run pytest --cov=src/tag_generator --cov-report=html

# Watch mode for TDD (auto-run tests on file changes)
uv run pytest-watch tests/unit/
```

#### TDD Test Fixtures (tests/conftest.py)
```python
import pytest
import json
from pathlib import Path
from typing import List, Dict, Any
from unittest.mock import Mock, AsyncMock

# Import your project's models here
# from your_project.models import InputModel, OutputModel

@pytest.fixture(scope="session")
def test_data_dir():
    """Path to test data directory"""
    return Path(__file__).parent / "fixtures"

@pytest.fixture(scope="session")
def sample_test_data(test_data_dir) -> List[Dict[str, Any]]:
    """Load sample data for testing"""
    with open(test_data_dir / "sample_data.json") as f:
        return json.load(f)

@pytest.fixture
def sample_input_data() -> Dict[str, Any]:
    """Sample input data for TDD"""
    return {
        "title": "Sample Input Title",
        "content": "Sample content for testing purposes with relevant keywords.",
        "metadata": {"source": "test", "category": "sample"}
    }

@pytest.fixture
def expected_output_data() -> List[Dict[str, Any]]:
    """Expected output data for TDD"""
    return [
        {"label": "category1", "confidence": 0.9, "source": "ml_model"},
        {"label": "category2", "confidence": 0.7, "source": "rule_based"}
    ]

@pytest.fixture
def mock_external_service():
    """Mock external service for testing"""
    mock = Mock()
    mock.process_data.return_value = {"status": "success", "data": []}
    mock.get_data.return_value = {"items": [], "total": 0}
    return mock

@pytest.fixture
async def mock_async_processor():
    """Mock async processor for integration tests"""
    mock = AsyncMock()
    mock.process_async.return_value = [
        {"result": "processed", "confidence": 0.8}
    ]
    return mock
```

#### TDD Test Examples

**Unit Test Pattern (TDD RED-GREEN-REFACTOR):**
```python
# tests/unit/test_data_processor.py
import pytest
from your_project.processors.data_processor import DataProcessor

class TestDataProcessor:
    @pytest.fixture
    def data_processor(self):
        return DataProcessor()

    # RED: Write failing test first
    def test_process_data_extracts_features(self, data_processor):
        input_data = {"text": "Sample text with some features.", "metadata": {}}
        result = data_processor.extract_features(input_data)

        assert isinstance(result, dict)
        assert "feature_count" in result
        assert result["feature_count"] > 0
        assert "feature_values" in result
        assert len(result["feature_values"]) > 0

    # Continue TDD cycle...
    def test_process_empty_input(self, data_processor):
        result = data_processor.extract_features({})

        assert result["feature_count"] == 0
        assert result["feature_values"] == []

    def test_process_data_handles_special_cases(self, data_processor):
        special_input = {"text": "Special@#$%characters!", "metadata": {"type": "special"}}
        result = data_processor.extract_features(special_input)

        assert isinstance(result, dict)
        assert result["feature_count"] >= 0  # Should handle gracefully
```

**Integration Test Pattern:**
```python
# tests/integration/test_processing_pipeline.py
import pytest
from your_project.core.pipeline import ProcessingPipeline
from your_project.models import InputModel

@pytest.mark.integration
class TestProcessingPipeline:
    @pytest.fixture
    async def real_pipeline(self):
        # Use real components for integration testing
        return ProcessingPipeline()

    @pytest.mark.asyncio
    async def test_end_to_end_processing(self, real_pipeline, sample_input_data):
        # Test complete pipeline with real components
        input_model = InputModel(**sample_input_data)
        results = await real_pipeline.process(input_model)

        assert len(results) > 0
        assert all(result.confidence > 0.0 for result in results)
        # Verify domain-specific expectations
        assert any(hasattr(result, 'category') for result in results)
```

### UV Testing Commands Integration

#### Pre-commit Hooks (.pre-commit-config.yaml)
```yaml
repos:
  - repo: local
    hooks:
      - id: tests
        name: Run TDD Tests
        entry: uv run pytest tests/unit/ --tb=short
        language: system
        pass_filenames: false
        always_run: true

      - id: ruff-check
        name: Ruff Linting
        entry: uv run ruff check
        language: system
        types: [python]

      - id: ruff-format
        name: Ruff Formatting
        entry: uv run ruff format
        language: system
        types: [python]

      - id: mypy
        name: Type Checking
        entry: uv run mypy src/
        language: system
        types: [python]
```

### Coding Standards

#### Python Development with UV
- **UV First:** All dependencies managed through `uv add` and `pyproject.toml`
- **TDD Mandatory:** Write failing tests before ANY implementation code
- **Type Hints:** Use comprehensive type hints for all functions
- **Error Handling:** Implement proper exception handling with custom exceptions
- **Logging:** Use structured logging with context
- **Documentation:** Write comprehensive docstrings
- **Code Quality:** Use ruff for linting/formatting, mypy for type checking

#### Logging Standards
```python
import structlog
from typing import Any, Dict

# Configure structured logging
structlog.configure(
    processors=[
        structlog.stdlib.filter_by_level,
        structlog.stdlib.add_logger_name,
        structlog.stdlib.add_log_level,
        structlog.stdlib.PositionalArgumentsFormatter(),
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.StackInfoRenderer(),
        structlog.processors.format_exc_info,
        structlog.processors.UnicodeDecoder(),
        structlog.processors.JSONRenderer()
    ],
    context_class=dict,
    logger_factory=structlog.stdlib.LoggerFactory(),
    wrapper_class=structlog.stdlib.BoundLogger,
    cache_logger_on_first_use=True,
)

logger = structlog.get_logger()

# Usage example
async def generate_tags(article: ArticleContent) -> List[TagResult]:
    logger.info("Starting tag generation",
                article_url=article.url,
                content_length=len(article.content))

    try:
        # Processing logic
        tags = await process_article(article)

        logger.info("Tag generation completed",
                   article_url=article.url,
                   tags_generated=len(tags))
        return tags

    except Exception as e:
        logger.error("Tag generation failed",
                    article_url=article.url,
                    error=str(e),
                    exc_info=True)
        raise
```

## TDD Implementation Examples

### TDD Example: Core Component Development

#### Step 1: RED (Failing Test First)
```python
# tests/unit/test_core_processor.py
import pytest
from your_project.core.processor import CoreProcessor  # This doesn't exist yet!

class TestCoreProcessor:
    @pytest.fixture
    def processor(self):
        return CoreProcessor()

    def test_process_input_returns_expected_structure(self, processor):
        # RED: This test MUST fail first
        input_data = {"text": "Sample input for processing", "metadata": {"type": "test"}}
        result = processor.process(input_data)

        assert isinstance(result, dict)
        assert "processed_data" in result
        assert "confidence" in result
        assert result["confidence"] > 0.0
        assert len(result["processed_data"]) > 0
```

```bash
# Run the failing test
uv run pytest tests/unit/test_core_processor.py::test_process_input_returns_expected_structure -v
# Result: FAIL ❌ (ImportError: No module named 'your_project.core.processor')
```

#### Step 2: GREEN (Minimal Implementation)
```python
# src/your_project/core/__init__.py
# (Create empty file)

# src/your_project/core/processor.py
from typing import Dict, Any

class CoreProcessor:
    def process(self, input_data: Dict[str, Any]) -> Dict[str, Any]:
        """Process input data - minimal implementation"""
        if not input_data or "text" not in input_data:
            return {"processed_data": [], "confidence": 0.0}

        # Minimal processing logic
        processed_items = [{"item": "basic_processing", "value": len(input_data["text"])}]

        return {
            "processed_data": processed_items,
            "confidence": 0.8
        }
```

```bash
# Run test again
uv run pytest tests/unit/test_core_processor.py::test_process_input_returns_expected_structure -v
# Result: PASS ✅
```

#### Step 3: REFACTOR (Improve Implementation)
```python
# Add more tests first (RED)
def test_process_handles_empty_input(self, processor):
    result = processor.process({})
    assert result["processed_data"] == []
    assert result["confidence"] == 0.0

def test_process_handles_complex_input(self, processor):
    complex_input = {
        "text": "Complex input with multiple features and metadata",
        "metadata": {"type": "complex", "priority": "high"}
    }
    result = processor.process(complex_input)
    # Should handle complex cases appropriately
    assert len(result["processed_data"]) > 1
    assert result["confidence"] > 0.5
```

```python
# Refactor implementation (GREEN)
from typing import Dict, Any, List
import re

class CoreProcessor:
    def __init__(self):
        self.processing_rules = {
            "simple": {"confidence_boost": 0.1, "min_features": 1},
            "complex": {"confidence_boost": 0.3, "min_features": 3}
        }

    def process(self, input_data: Dict[str, Any]) -> Dict[str, Any]:
        """Process input data with enhanced logic"""
        if not input_data or "text" not in input_data:
            return {"processed_data": [], "confidence": 0.0}

        text = input_data["text"]
        metadata = input_data.get("metadata", {})

        # Extract features
        features = self._extract_features(text, metadata)

        # Calculate confidence
        confidence = self._calculate_confidence(features, metadata)

        return {
            "processed_data": features,
            "confidence": confidence,
            "metadata": {"feature_count": len(features)}
        }

    def _extract_features(self, text: str, metadata: Dict[str, Any]) -> List[Dict[str, Any]]:
        """Extract features from text and metadata"""
        features = []

        # Basic text features
        features.append({"type": "text_length", "value": len(text)})
        features.append({"type": "word_count", "value": len(text.split())})

        # Pattern-based features
        if re.search(r'\b\w+ing\b', text):
            features.append({"type": "pattern_gerund", "value": 1.0})

        # Metadata-based features
        if metadata.get("type") == "complex":
            features.append({"type": "metadata_complex", "value": 1.0})

        return features

    def _calculate_confidence(self, features: List[Dict[str, Any]], metadata: Dict[str, Any]) -> float:
        """Calculate processing confidence"""
        base_confidence = 0.5
        feature_boost = len(features) * 0.1

        # Metadata boost
        processing_type = metadata.get("type", "simple")
        type_boost = self.processing_rules.get(processing_type, {}).get("confidence_boost", 0.0)

        return min(base_confidence + feature_boost + type_boost, 1.0)
```

```bash
# Ensure all tests pass after refactoring
uv run pytest tests/unit/test_core_processor.py -v
# Result: ALL PASS ✅
```

### TDD Example: Async Service Integration

#### Complete TDD Cycle for Async Operations
```python
# tests/unit/test_async_service.py (RED PHASE)
import pytest
from unittest.mock import Mock, AsyncMock
from your_project.services.async_service import AsyncService
from your_project.models import RequestModel, ResponseModel

class TestAsyncService:
    @pytest.fixture
    def mock_external_client(self):
        mock = AsyncMock()
        mock.fetch_data.return_value = {
            "status": "success",
            "items": [{"id": 1, "data": "sample"}]
        }
        return mock

    @pytest.fixture
    def mock_processor(self):
        mock = Mock()
        mock.process_items.return_value = [
            {"id": 1, "processed": True, "score": 0.9}
        ]
        return mock

    @pytest.fixture
    def async_service(self, mock_external_client, mock_processor):
        service = AsyncService()
        service.external_client = mock_external_client
        service.processor = mock_processor
        return service

    @pytest.mark.asyncio
    async def test_process_request_returns_valid_response(self, async_service):
        # RED: This will fail because AsyncService.process_request doesn't exist
        request = RequestModel(query="test query", options={"limit": 10})

        response = await async_service.process_request(request)

        assert isinstance(response, ResponseModel)
        assert response.status == "success"
        assert len(response.items) > 0
        assert all(item.score > 0 for item in response.items)

    @pytest.mark.asyncio
    async def test_process_request_handles_external_failure(self, async_service):
        # Configure mock to simulate failure
        async_service.external_client.fetch_data.side_effect = Exception("External API error")

        request = RequestModel(query="test query")
        response = await async_service.process_request(request)

        # Should handle errors gracefully
        assert response.status == "error"
        assert "External API error" in response.error_message
```

```python
# src/your_project/services/async_service.py (GREEN PHASE - Minimal)
from typing import Optional
from ..models import RequestModel, ResponseModel, ResponseItem

class AsyncService:
    def __init__(self):
        self.external_client = None
        self.processor = None

    async def process_request(self, request: RequestModel) -> ResponseModel:
        """Process async request - minimal implementation"""
        try:
            if not self.external_client or not self.processor:
                return ResponseModel(status="error", error_message="Service not configured")

            # Fetch external data
            external_data = await self.external_client.fetch_data(request.query)

            if external_data.get("status") != "success":
                return ResponseModel(status="error", error_message="External fetch failed")

            # Process items
            processed_items = self.processor.process_items(external_data["items"])

            # Convert to response items
            response_items = [
                ResponseItem(id=item["id"], score=item["score"], data=item)
                for item in processed_items
            ]

            return ResponseModel(status="success", items=response_items)

        except Exception as e:
            return ResponseModel(
                status="error",
                error_message=str(e),
                items=[]
            )
```

### TDD Integration Test Pattern

```python
# tests/integration/test_complete_pipeline.py
import pytest
from pathlib import Path
from your_project.core.pipeline import CompletePipeline
from your_project.models import InputModel

@pytest.mark.integration
class TestCompletePipeline:
    @pytest.fixture(scope="class")
    async def production_pipeline(self):
        """Real pipeline with actual components for integration testing"""
        pipeline = CompletePipeline()
        await pipeline.initialize()  # Load real dependencies
        return pipeline

    @pytest.mark.asyncio
    async def test_end_to_end_processing_workflow(self, production_pipeline):
        """Test complete workflow with realistic data"""
        input_data = InputModel(
            content="Comprehensive test content with multiple aspects to process",
            metadata={
                "source": "integration_test",
                "priority": "high",
                "features": ["feature1", "feature2"]
            }
        )

        # Process through complete pipeline
        result = await production_pipeline.process_complete(input_data)

        # Comprehensive assertions
        assert result.success is True, "Pipeline should complete successfully"
        assert len(result.outputs) > 0, "Should generate at least one output"
        assert result.processing_time_ms < 5000, "Should complete within 5 seconds"

        # Check output quality
        outputs = result.outputs
        assert all(0.0 <= output.confidence <= 1.0 for output in outputs)
        assert outputs[0].confidence >= outputs[-1].confidence, "Should be sorted by confidence"

        # Verify business logic
        expected_categories = ["category1", "category2", "category3"]
        found_categories = [output.category for output in outputs]
        assert any(cat in found_categories for cat in expected_categories)
```

### UV TDD Workflow Scripts

```python
# scripts/run_tdd_cycle.py - UV script for TDD automation
#!/usr/bin/env python3
"""Automated TDD cycle script for UV projects"""

import subprocess
import sys
from pathlib import Path

def run_command(cmd: str) -> bool:
    """Run UV command and return success status"""
    print(f"🔄 Running: {cmd}")
    result = subprocess.run(cmd, shell=True, capture_output=True, text=True)

    if result.returncode == 0:
        print(f"✅ Success: {cmd}")
        return True
    else:
        print(f"❌ Failed: {cmd}")
        print(result.stdout)
        print(result.stderr)
        return False

def tdd_cycle():
    """Run complete TDD cycle with UV"""
    print("🚀 Starting TDD Cycle...")

    # RED: Run tests, expect some to fail
    print("\n🔴 RED PHASE: Running tests (expecting failures)")
    run_command("uv run pytest tests/unit/ --tb=short -x")

    input("📝 Write minimal code to make tests pass, then press Enter...")

    # GREEN: Run tests again, expect them to pass
    print("\n🟢 GREEN PHASE: Running tests (expecting success)")
    if not run_command("uv run pytest tests/unit/ --tb=short"):
        print("❌ Tests still failing. Fix implementation and try again.")
        return False

    # REFACTOR: Run full test suite with coverage
    print("\n🔄 REFACTOR PHASE: Running full test suite with coverage")
    if not run_command("uv run pytest --cov=src --cov-fail-under=80"):
        print("❌ Coverage too low or tests failing after refactor.")
        return False

    # Quality checks
    print("\n🔍 QUALITY CHECKS")
    if not run_command("uv run ruff check"):
        return False
    if not run_command("uv run ruff format --diff"):
        return False
    if not run_command("uv run mypy src/"):
        return False

    print("\n🎉 TDD Cycle completed successfully!")
    return True

if __name__ == "__main__":
    success = tdd_cycle()
    sys.exit(0 if success else 1)
```

```bash
# Make script executable and run
chmod +x scripts/run_tdd_cycle.py
uv run python scripts/run_tdd_cycle.py
```
