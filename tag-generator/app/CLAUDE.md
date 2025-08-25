# CLAUDE.md - Tag Generator Service

## About this Service

The **tag-generator** is a Python-based ML microservice that automatically generates relevant tags for RSS feed articles. It is built with a focus on testability, maintainability, and performance.

- **Language**: Python 3.13+
- **Package Manager**: `uv`
- **Frameworks**: FastAPI, Pydantic
- **Testing**: `pytest` with a TDD-first approach

## TDD and Testing Philosophy

**Test-Driven Development (TDD) is mandatory.** It is the foundation of our development process, ensuring that every component is testable and that our application logic is decoupled from the non-deterministic nature of ML models.

### The TDD Cycle: Red-Green-Refactor

1.  **Red**: Write a failing test that defines the desired functionality.
2.  **Green**: Write the absolute minimum amount of code to make the test pass.
3.  **Refactor**: Improve the code's design and readability while keeping all tests green.

## Testing Strategy

### 1. Unit Testing

#### Testing FastAPI Endpoints

Use FastAPI's `TestClient` to test your endpoints. Use dependency injection to provide mock services to your endpoints, allowing you to test the API layer in isolation.

```python
# tests/unit/test_api.py
from fastapi.testclient import TestClient
from unittest.mock import Mock
from app.main import app, get_tag_generator_service

# Mock the service dependency
mock_service = Mock()
mock_service.generate_tags.return_value = [{"tag": "test", "confidence": 0.9}]

# Override the dependency in the app
def override_get_tag_generator_service():
    return mock_service

app.dependency_overrides[get_tag_generator_service] = override_get_tag_generator_service

client = TestClient(app)

def test_generate_tags_endpoint():
    response = client.post("/tags/", json={"content": "some text"})
    assert response.status_code == 200
    assert response.json() == [{"tag": "test", "confidence": 0.9}]
    mock_service.generate_tags.assert_called_once()
```

#### Testing Pydantic Models

Focus on testing your custom validation logic, not Pydantic's built-in features.

```python
# tests/unit/test_models.py
import pytest
from pydantic import ValidationError
from app.models import Article

def test_article_with_short_content():
    with pytest.raises(ValidationError) as excinfo:
        Article(title="A", content="short") # Assumes a validator checks for min length
    assert "Content must be at least 10 characters" in str(excinfo.value)
```

### 2. Testing ML Components

#### Testing the Model's Integration

For unit and component tests, use a small, deterministic, pre-trained model. The goal is to test that your application correctly preprocesses data, calls the model, and handles its output, not to test the model's predictive accuracy.

#### Testing for Model Quality (Advanced)

Beyond functional testing, ML models require a specialized testing approach:
-   **Bias and Fairness**: Write tests to ensure your model does not produce biased results for different demographic groups.
-   **Robustness**: Test the model's performance against adversarial inputs (e.g., text with typos, irrelevant information).
-   **Performance**: Benchmark the model's inference speed and memory usage to ensure it meets performance requirements.

These tests are often part of a separate model evaluation pipeline, but it's important to consider them as part of the overall quality assurance process.

### 3. Integration Testing

Integration tests should verify the interactions between your service and its external dependencies, such as a database or other APIs. Use containerized versions of these dependencies to ensure a consistent and isolated testing environment.

## Development with `uv`

-   **Dependencies**: Manage all dependencies with `uv add` and `pyproject.toml`.
-   **Testing**: Use `uv run pytest` to execute tests.
-   **Code Quality**: Use `uv run ruff check` for linting, `uv run ruff format` for formatting, and `uv run mypy src/` for type checking.

## References

-   [Testing FastAPI Applications](https://fastapi.tiangolo.com/tutorial/testing/)
-   [Testing in Python with `pytest`](https://realpython.com/pytest-python-testing/)
-   [Testing Machine Learning Systems](https://www.eugeneyan.com/writing/testing-ml/)
