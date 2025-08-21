# CLAUDE.md - News Creator Service

## About This Service

The news-creator is a Python FastAPI service that generates summaries and derivative content from processed articles using a Large Language Model (LLM). It is designed with a test-first methodology, emphasizing the unique challenges of testing LLM-based applications.

- **Language**: Python 3.11+
- **Framework**: FastAPI
- **HTTP Client**: `aiohttp`
- **Testing**: `pytest`, `pytest-asyncio`

## Testing Strategy for LLM Applications

Testing an LLM application requires a multi-layered approach. We separate the testing of the application's deterministic logic from the evaluation of the non-deterministic LLM outputs.

### 1. TDD for the FastAPI Application

The core application logic (API endpoints, data validation, business logic) is developed using a strict TDD cycle. In this phase, the LLM provider is **always mocked**.

**Example: Testing an Endpoint with a Mocked LLM Service**

We use FastAPI's dependency injection system to replace the real LLM service with a mock during tests.

```python
# tests/test_api.py
from fastapi.testclient import TestClient
from unittest.mock import Mock
from app.main import app, get_llm_provider

# 1. Mock the LLM provider
mock_provider = Mock()
mock_provider.generate_summary.return_value = "This is a mocked summary."

# 2. Define the dependency override
def override_get_llm_provider():
    return mock_provider

# 3. Apply the override to the app
app.dependency_overrides[get_llm_provider] = override_get_llm_provider

client = TestClient(app)

def test_generate_summary_endpoint():
    # 4. Call the endpoint
    response = client.post("/v1/generate/summary", json={"content": "some text"})

    # 5. Assert the results
    assert response.status_code == 200
    assert response.json() == {"summary": "This is a mocked summary."}
    mock_provider.generate_summary.assert_called_once()

# Remember to clean up overrides after tests if needed
app.dependency_overrides = {}
```

### 2. Testing and Evaluating Prompts

Prompt engineering is an iterative process that requires its own testing and evaluation pipeline. The goal is to ensure prompts consistently produce high-quality and safe responses.

-   **Golden Datasets**: Create a dataset of diverse inputs and their ideal outputs. Run your prompts against this dataset to measure performance and detect regressions.
-   **Metric-Based Evaluation**: Use quantitative metrics to assess LLM outputs. For summarization, this could be ROUGE scores. For classification, F1-score. For generation, check for JSON format correctness or the presence of keywords.
-   **LLM-as-Judge**: Use a powerful LLM (like GPT-4) to evaluate the output of your application's LLM. You can ask it to score a response based on criteria like helpfulness, coherence, and adherence to instructions.
-   **Frameworks**: Use frameworks like `DeepEval` to integrate LLM quality checks into your testing pipeline.

### 3. Security and Safety Testing

LLM applications introduce new security vulnerabilities. We test for these based on the **OWASP Top 10 for LLM Applications**.

-   **Prompt Injection**: We maintain a suite of adversarial prompts to test if the LLM can be manipulated to bypass its instructions.
-   **Insecure Output Handling**: We test that the application properly sanitizes and validates any output from the LLM, especially if it could be interpreted as code (e.g., Markdown, HTML).
-   **Sensitive Information Disclosure**: We test to ensure the LLM does not reveal sensitive information from its training data or context.

## FastAPI Best Practices

-   **Dependency Injection**: Use `Depends` for all external services, including the LLM provider and configuration.
-   **Response Models**: Use explicit Pydantic response models to prevent leaking internal data.
-   **Exception Handling**: Implement custom exception handlers to map provider errors (e.g., API timeouts, content moderation flags) to appropriate HTTP status codes.

## References

-   [Testing FastAPI Applications](https://fastapi.tiangolo.com/tutorial/testing/)
-   [OWASP Top 10 for LLM Applications](https://owasp.org/www-project-top-10-for-large-language-model-applications/)
-   [DeepEval for LLM Evaluation](https://github.com/confident-ai/deepeval)

