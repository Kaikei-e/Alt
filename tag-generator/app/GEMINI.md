# GEMINI.md: Tag Generator Service

This document outlines the best practices for the `tag-generator` service, adhering to Gemini standards as of July 2025. This Python-based ML microservice automatically generates tags for RSS feed articles.

## 1. Core Responsibilities

*   Analyzes article content and metadata to generate relevant tags.
*   Enhances content discoverability and organization.
*   Supports multi-language content processing.

## 2. Architecture

### 2.1. ML Pipeline Architecture

**Input → Preprocessing → Feature Extraction → ML Models → Post-processing → Output**

### 2.2. Tech Stack

*   **Language**: Python 3.13+
*   **Package Manager**: `uv`
*   **ML Frameworks**: scikit-learn, transformers
*   **Testing**: `pytest` with fixtures

## 3. Development Guidelines

### 3.1. Test-Driven Development (TDD)

*   The Red-Green-Refactor cycle is **mandatory** for all code changes.
*   Use `uv run pytest` to execute tests.

#### TDD Workflow with `uv`

1.  **Red**: Write a failing test.
2.  **Green**: Write the minimal code to pass the test.
3.  **Refactor**: Improve the code while keeping tests green.

### 3.2. Project Structure

*   The project is organized into `src`, `tests`, `models`, and `data` directories.
*   Tests are further divided into `unit`, `integration`, and `performance` tests.

### 3.3. Coding Standards

*   All dependencies are managed through `uv` and `pyproject.toml`.
*   Use comprehensive type hints for all functions.
*   Use `ruff` for linting and formatting, and `mypy` for type checking.
*   Use `structlog` for structured logging.

## 4. TDD Implementation Examples

### 4.1. Test Fixtures

*   Use `tests/conftest.py` to define reusable test fixtures, such as sample data and mock objects.

### 4.2. Unit Tests

*   Focus on testing individual functions and classes in isolation.
*   Mock external dependencies.

### 4.3. Integration Tests

*   Test the interaction between different components of the service.
*   Use real components where possible to test the end-to-end pipeline.

## 5. Gemini Model Interaction

*   **TDD First**: Instruct Gemini to write failing tests before implementing any code.
*   **Incremental Changes**: Work in small, testable increments.
*   **Quality Control**: Use pre-commit hooks and CI to enforce testing, linting, and formatting standards.

## 6. Common Patterns

### 6.1. Pydantic Settings Management

*   Use `pydantic-settings` to manage application settings from environment variables and `.env` files.

### 6.2. FastAPI Endpoint

*   Use FastAPI to expose an API for tag generation.
*   Use dependency injection to provide the `TagGenerator` to the API endpoint.

### 6.3. ML Model Loading

*   Load ML models from a specified path in the application settings.
*   Handle `FileNotFoundError` if the model is not found.