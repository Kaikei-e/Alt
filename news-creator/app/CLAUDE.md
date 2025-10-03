# CLAUDE.md - News Creator Service

## About This Service

The news-creator is a Python FastAPI service that generates summaries and derivative content from processed articles using a Large Language Model (LLM). **This service has been refactored to follow Clean Architecture principles**, ensuring maintainability, testability, and scalability.

- **Language**: Python 3.11+
- **Framework**: FastAPI
- **HTTP Client**: `aiohttp`
- **Testing**: `pytest`, `pytest-asyncio`
- **Architecture**: Clean Architecture (5-layer)

## Clean Architecture

This service follows a strict 5-layer Clean Architecture pattern, consistent with other services in the Alt project:

```
REST Handler → Usecase → Port → Gateway (ACL) → Driver
```

### Layer Responsibilities

#### 1. **Handler Layer** (`news_creator/handler/`)
- **Responsibility**: HTTP request/response handling, validation, error mapping
- **Files**:
  - `summarize_handler.py`: Article summarization endpoint
  - `generate_handler.py`: Generic LLM generation endpoint
  - `health_handler.py`: Health check endpoint
- **Dependencies**: Uses Usecase layer via dependency injection
- **Testing**: FastAPI TestClient with mocked Usecases

#### 2. **Usecase Layer** (`news_creator/usecase/`)
- **Responsibility**: Business logic orchestration, no external dependencies
- **Files**:
  - `summarize_usecase.py`: Summarization business logic
- **Dependencies**: Uses Port interfaces (not concrete implementations)
- **Testing**: Unit tests with mocked Ports

#### 3. **Port Layer** (`news_creator/port/`)
- **Responsibility**: Abstract interfaces for external dependencies
- **Files**:
  - `llm_provider_port.py`: LLM provider interface (ABC)
  - `auth_port.py`: Authentication service interface (ABC)
  - `user_preferences_port.py`: User preferences repository interface (ABC)
- **Testing**: No tests needed (pure interfaces)

#### 4. **Gateway Layer** (`news_creator/gateway/`)
- **Responsibility**: Anti-Corruption Layer - implements Port interfaces, translates between domain models and external formats
- **Files**:
  - `ollama_gateway.py`: Implements `LLMProviderPort` for Ollama
- **Dependencies**: Uses Driver layer
- **Testing**: Unit tests with mocked Drivers

#### 5. **Driver Layer** (`news_creator/driver/`)
- **Responsibility**: Direct communication with external APIs/services
- **Files**:
  - `ollama_driver.py`: HTTP client for Ollama API
- **Dependencies**: Only `aiohttp` and configuration
- **Testing**: Unit tests with mocked HTTP responses

### Supporting Layers

#### **Domain Layer** (`news_creator/domain/`)
- **Responsibility**: Domain models and business entities
- **Files**:
  - `models.py`: Pydantic models for requests/responses
  - `prompts.py`: LLM prompt templates
- **Testing**: Unit tests for validation logic

#### **Config Layer** (`news_creator/config/`)
- **Responsibility**: Environment-based configuration
- **Files**:
  - `config.py`: Configuration class with environment variable loading
- **Testing**: Unit tests for config validation

## Project Structure

```
news-creator/app/
├── main.py                          # FastAPI app + Dependency Injection Container
├── news_creator/                    # Main package
│   ├── __init__.py
│   ├── config/                      # Configuration layer
│   │   ├── __init__.py
│   │   └── config.py
│   ├── domain/                      # Domain models
│   │   ├── __init__.py
│   │   ├── models.py
│   │   └── prompts.py
│   ├── port/                        # Port interfaces (ABC)
│   │   ├── __init__.py
│   │   ├── llm_provider_port.py
│   │   ├── auth_port.py
│   │   └── user_preferences_port.py
│   ├── driver/                      # External API clients
│   │   ├── __init__.py
│   │   └── ollama_driver.py
│   ├── gateway/                     # Anti-Corruption Layer
│   │   ├── __init__.py
│   │   └── ollama_gateway.py
│   ├── usecase/                     # Business logic
│   │   ├── __init__.py
│   │   └── summarize_usecase.py
│   └── handler/                     # REST endpoints
│       ├── __init__.py
│       ├── summarize_handler.py
│       ├── generate_handler.py
│       └── health_handler.py
└── tests/                           # Test suite (mirrors structure)
    ├── config/
    ├── domain/
    ├── driver/
    ├── gateway/
    ├── usecase/
    └── handler/
```

## Testing Strategy for LLM Applications

Testing an LLM application requires a multi-layered approach. We separate the testing of the application's deterministic logic from the evaluation of the non-deterministic LLM outputs.

### 1. TDD for Each Layer

Each layer is developed using strict TDD:

**Example: Testing Usecase with Mocked Port**

```python
# tests/usecase/test_summarize_usecase.py
import pytest
from unittest.mock import Mock, AsyncMock
from news_creator.usecase.summarize_usecase import SummarizeUsecase
from news_creator.domain.models import LLMGenerateResponse

@pytest.fixture
def mock_llm_provider():
    mock = Mock()
    mock.generate = AsyncMock(return_value=LLMGenerateResponse(
        response="これはテスト要約です。",
        model="test-model"
    ))
    return mock

@pytest.fixture
def mock_config():
    config = Mock()
    config.summary_num_predict = 500
    return config

@pytest.mark.asyncio
async def test_generate_summary_success(mock_config, mock_llm_provider):
    usecase = SummarizeUsecase(mock_config, mock_llm_provider)

    summary, metadata = await usecase.generate_summary(
        article_id="test-123",
        content="Test article content"
    )

    assert summary == "これはテスト要約です。"
    assert metadata["model"] == "test-model"
    mock_llm_provider.generate.assert_called_once()
```

**Example: Testing Handler with Mocked Usecase**

```python
# tests/handler/test_summarize_handler.py
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock
from news_creator.handler.summarize_handler import create_summarize_router

def test_summarize_endpoint_success():
    # Mock usecase
    mock_usecase = Mock()
    mock_usecase.generate_summary = AsyncMock(return_value=(
        "Test summary",
        {"model": "test-model", "prompt_tokens": 100}
    ))

    # Create router with mocked usecase
    router = create_summarize_router(mock_usecase)
    app = FastAPI()
    app.include_router(router)
    client = TestClient(app)

    # Test endpoint
    response = client.post("/api/v1/summarize", json={
        "article_id": "test-123",
        "content": "Test content"
    })

    assert response.status_code == 200
    assert response.json()["success"] is True
    assert response.json()["summary"] == "Test summary"
```

### 2. Testing and Evaluating Prompts

Prompt engineering is an iterative process that requires its own testing and evaluation pipeline. The goal is to ensure prompts consistently produce high-quality and safe responses.

- **Golden Datasets**: Create a dataset of diverse inputs and their ideal outputs. Run your prompts against this dataset to measure performance and detect regressions.
- **Metric-Based Evaluation**: Use quantitative metrics to assess LLM outputs. For summarization, this could be ROUGE scores. For classification, F1-score. For generation, check for JSON format correctness or the presence of keywords.
- **LLM-as-Judge**: Use a powerful LLM (like GPT-4) to evaluate the output of your application's LLM. You can ask it to score a response based on criteria like helpfulness, coherence, and adherence to instructions.
- **Frameworks**: Use frameworks like `DeepEval` to integrate LLM quality checks into your testing pipeline.

### 3. Security and Safety Testing

LLM applications introduce new security vulnerabilities. We test for these based on the **OWASP Top 10 for LLM Applications**.

- **Prompt Injection**: We maintain a suite of adversarial prompts to test if the LLM can be manipulated to bypass its instructions.
- **Insecure Output Handling**: We test that the application properly sanitizes and validates any output from the LLM, especially if it could be interpreted as code (e.g., Markdown, HTML).
- **Sensitive Information Disclosure**: We test to ensure the LLM does not reveal sensitive information from its training data or context.

## Dependency Injection

The service uses a `DependencyContainer` class in `main.py` to wire all layers together:

```python
class DependencyContainer:
    def __init__(self):
        # Config layer
        self.config = NewsCreatorConfig()

        # Gateway layer (implements Ports)
        self.ollama_gateway = OllamaGateway(self.config)

        # Usecase layer (depends on Ports)
        self.summarize_usecase = SummarizeUsecase(
            config=self.config,
            llm_provider=self.ollama_gateway,
        )
```

Handlers are created with factory functions that accept dependencies:

```python
app.include_router(
    create_summarize_router(container.summarize_usecase),
    tags=["summarization"]
)
```

## Configuration

All configuration is loaded from environment variables via `NewsCreatorConfig`:

```bash
# Required
SERVICE_SECRET=your-secret-key

# LLM Service
LLM_SERVICE_URL=http://localhost:11434
LLM_MODEL=gemma3:4b
LLM_TIMEOUT_SECONDS=60

# LLM Parameters
LLM_TEMPERATURE=0.0
LLM_TOP_P=0.9
LLM_NUM_PREDICT=500
SUMMARY_NUM_PREDICT=500

# Authentication
AUTH_SERVICE_URL=http://auth-service:8080
```

## Development Workflow

### 1. Adding a New Feature

Follow the TDD approach, working from the outside in:

1. **Handler**: Write tests for the new endpoint
2. **Usecase**: Write tests for business logic with mocked Ports
3. **Gateway/Driver** (if needed): Write tests with mocked HTTP responses
4. **Implement** each layer to make tests pass

### 2. Adding a New External Service

1. Define a new **Port** interface (ABC) in `port/`
2. Implement the **Driver** in `driver/` with tests
3. Implement the **Gateway** in `gateway/` with tests
4. Use the Port in **Usecase** layer
5. Update **DependencyContainer** to wire everything together

### 3. Running Tests

```bash
# All tests
SERVICE_SECRET=test-secret pytest

# Specific layer
SERVICE_SECRET=test-secret pytest tests/usecase/

# With coverage
SERVICE_SECRET=test-secret pytest --cov=news_creator
```

## API Endpoints

### POST /api/v1/summarize
Generate Japanese summary for an article.

**Request:**
```json
{
  "article_id": "article-123",
  "content": "Full article text in English..."
}
```

**Response:**
```json
{
  "success": true,
  "article_id": "article-123",
  "summary": "日本語の要約...",
  "model": "gemma3:4b",
  "prompt_tokens": 1234,
  "completion_tokens": 456,
  "total_duration_ms": 1500.5
}
```

### POST /api/generate
Generic LLM generation (Ollama-compatible).

**Request:**
```json
{
  "prompt": "Your prompt here",
  "model": "gemma3:4b",
  "stream": false,
  "options": {
    "temperature": 0.7
  }
}
```

### GET /health
Health check endpoint.

## References

- [Testing FastAPI Applications](https://fastapi.tiangolo.com/tutorial/testing/)
- [OWASP Top 10 for LLM Applications](https://owasp.org/www-project-top-10-for-large-language-model-applications/)
- [DeepEval for LLM Evaluation](https://github.com/confident-ai/deepeval)
- [Clean Architecture by Robert C. Martin](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
