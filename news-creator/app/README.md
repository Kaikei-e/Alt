# News Creator Service

LLM-based content generation service for the Alt RSS Reader project. Generates Japanese summaries and derivative content from English articles using local LLM (Ollama/Gemma).

## 🏗️ Architecture

This service follows **Clean Architecture** principles with a 5-layer design:

```
Handler → Usecase → Port → Gateway → Driver
```

- **Handler**: REST API endpoints (FastAPI)
- **Usecase**: Business logic orchestration
- **Port**: Abstract interfaces for external dependencies
- **Gateway**: Anti-Corruption Layer for external services
- **Driver**: HTTP clients for external APIs

For detailed architecture documentation, see [CLAUDE.md](CLAUDE.md).

## 🚀 Quick Start

### Prerequisites

- Python 3.11+
- Ollama running locally or accessible via network
- Service secret key

### Installation

```bash
# Install dependencies
pip install -r requirements.txt

# Set required environment variables
export SERVICE_SECRET=your-secret-key
export LLM_SERVICE_URL=http://localhost:11434
export LLM_MODEL=gemma3:4b

# Run the service
python main.py
```

The service will start on `http://localhost:8001`.

### Docker

```bash
# Build image
docker build -t news-creator:latest .

# Run container
docker run -p 8001:8001 \
  -e SERVICE_SECRET=your-secret-key \
  -e LLM_SERVICE_URL=http://host.docker.internal:11434 \
  news-creator:latest
```

## 📝 API Usage

### Generate Japanese Summary

```bash
curl -X POST http://localhost:8001/api/v1/summarize \
  -H "Content-Type: application/json" \
  -d '{
    "article_id": "article-123",
    "content": "Full article text in English..."
  }'
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

### Generic LLM Generation (Ollama-compatible)

```bash
curl -X POST http://localhost:8001/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Your prompt here",
    "model": "gemma3:4b",
    "options": {
      "temperature": 0.7
    }
  }'
```

### Health Check

```bash
curl http://localhost:8001/health
```

## ⚙️ Configuration

All configuration is done via environment variables:

### Required

| Variable | Description | Example |
|----------|-------------|---------|
| `SERVICE_SECRET` | Service authentication secret | `your-secret-key` |

### LLM Service

| Variable | Description | Default |
|----------|-------------|---------|
| `LLM_SERVICE_URL` | Ollama service URL | `http://localhost:11434` |
| `LLM_MODEL` | Model name | `gemma3:4b` |
| `LLM_TIMEOUT_SECONDS` | Request timeout | `300` (5 minutes) |
| `LLM_KEEP_ALIVE_SECONDS` | Model keep-alive (fallback for unknown models) | `-1` (forever) |
| `LLM_KEEP_ALIVE_8K` | Keep-alive for 8K model (always loaded) | `24h` |
| `LLM_KEEP_ALIVE_16K` | Keep-alive for 16K model (on-demand) | `30m` |
| `LLM_KEEP_ALIVE_80K` | Keep-alive for 80K model (on-demand) | `15m` |

### LLM Parameters

| Variable | Description | Default |
|----------|-------------|---------|
| `LLM_TEMPERATURE` | Generation temperature | `0.0` |
| `LLM_TOP_P` | Nucleus sampling parameter | `0.9` |
| `LLM_NUM_PREDICT` | Max tokens to generate | `500` |
| `LLM_REPEAT_PENALTY` | Repetition penalty | `1.0` |
| `LLM_NUM_CTX` | Context window size | `8192` |
| `LLM_STOP_TOKENS` | Stop tokens (comma-separated) | `<\|user\|>,<\|system\|>` |
| `SUMMARY_NUM_PREDICT` | Max tokens for summaries | `500` |

### Authentication (Optional)

| Variable | Description | Default |
|----------|-------------|---------|
| `AUTH_SERVICE_URL` | Auth service URL | `http://auth-service:8080` |

## 🧪 Testing

### Run All Tests

```bash
SERVICE_SECRET=test-secret pytest
```

### Run Tests by Layer

```bash
# Config layer
SERVICE_SECRET=test-secret pytest tests/config/

# Domain layer
SERVICE_SECRET=test-secret pytest tests/domain/

# Usecase layer
SERVICE_SECRET=test-secret pytest tests/usecase/

# Handler layer
SERVICE_SECRET=test-secret pytest tests/handler/
```

### Coverage Report

```bash
SERVICE_SECRET=test-secret pytest --cov=news_creator --cov-report=html
```

## 📁 Project Structure

```
news-creator/app/
├── main.py                          # FastAPI app + DI Container
├── requirements.txt                 # Python dependencies
├── CLAUDE.md                        # Detailed architecture docs
├── README.md                        # This file
├── news_creator/                    # Main package
│   ├── config/                      # Configuration
│   │   └── config.py
│   ├── domain/                      # Domain models
│   │   ├── models.py
│   │   └── prompts.py
│   ├── port/                        # Port interfaces
│   │   ├── llm_provider_port.py
│   │   ├── auth_port.py
│   │   └── user_preferences_port.py
│   ├── driver/                      # External API clients
│   │   └── ollama_driver.py
│   ├── gateway/                     # Anti-Corruption Layer
│   │   └── ollama_gateway.py
│   ├── usecase/                     # Business logic
│   │   └── summarize_usecase.py
│   └── handler/                     # REST endpoints
│       ├── summarize_handler.py
│       ├── generate_handler.py
│       └── health_handler.py
└── tests/                           # Test suite
    ├── config/
    ├── domain/
    ├── driver/
    ├── gateway/
    ├── usecase/
    └── handler/
```

## 🛠️ Development

### Adding a New Feature

1. **Write tests first** (TDD approach)
2. Start from the **Handler** layer
3. Work inward through **Usecase**, **Gateway**, **Driver**
4. Update **DependencyContainer** in `main.py`

### Adding a New External Service

1. Define a **Port** interface in `port/`
2. Implement the **Driver** in `driver/`
3. Implement the **Gateway** in `gateway/`
4. Use the Port in **Usecase**
5. Wire in **DependencyContainer**

See [CLAUDE.md](CLAUDE.md) for detailed development guidelines.

## 🔒 Security

### LLM Security Testing

This service includes security testing for LLM-specific vulnerabilities:

- **Prompt Injection**: Testing against adversarial prompts
- **Output Sanitization**: Validating LLM outputs before use
- **Information Disclosure**: Preventing leakage of sensitive data

### OWASP Top 10 for LLM Applications

We follow the [OWASP Top 10 for LLM Applications](https://owasp.org/www-project-top-10-for-large-language-model-applications/) security guidelines.

## 📚 Related Documentation

- [CLAUDE.md](CLAUDE.md) - Detailed architecture and development guide
- [Alt Project README](../../README.md) - Main project documentation
- [OWASP LLM Top 10](https://owasp.org/www-project-top-10-for-large-language-model-applications/)
- [FastAPI Documentation](https://fastapi.tiangolo.com/)
- [Ollama Documentation](https://github.com/ollama/ollama)

## 🏃 Running in Production

### Environment Setup

```bash
# Create .env file
cat > .env << EOF
SERVICE_SECRET=$(openssl rand -hex 32)
LLM_SERVICE_URL=http://ollama-service:11434
LLM_MODEL=gemma3:4b
LLM_TIMEOUT_SECONDS=300
AUTH_SERVICE_URL=http://auth-service:8080
EOF

# Load environment
source .env

# Run with gunicorn
gunicorn main:app \
  --workers 4 \
  --worker-class uvicorn.workers.UvicornWorker \
  --bind 0.0.0.0:8001
```

### Health Monitoring

```bash
# Check service health
curl http://localhost:8001/health

# Expected response
{"status":"healthy","service":"news-creator"}
```

## 🤝 Contributing

1. Follow TDD principles
2. Write tests for each layer
3. Maintain Clean Architecture boundaries
4. Update documentation
5. Run all tests before committing

## 📄 License

Part of the Alt RSS Reader project.

## 🐛 Troubleshooting

### LLM Connection Issues

```bash
# Check Ollama is running
curl http://localhost:11434/api/tags

# Test LLM directly
curl http://localhost:11434/api/generate -d '{
  "model": "gemma3:4b",
  "prompt": "Hello"
}'
```

### Import Errors

```bash
# Ensure news_creator package is in PYTHONPATH
export PYTHONPATH="${PYTHONPATH}:$(pwd)"

# Or run from the app directory
cd /path/to/news-creator/app
python main.py
```

### Test Failures

```bash
# Ensure SERVICE_SECRET is set
export SERVICE_SECRET=test-secret

# Run with verbose output
pytest -v -s

# Check specific test
pytest tests/usecase/test_summarize_usecase.py -v
```

## 📊 Performance

- **Average summary generation**: ~1-3 seconds
- **Max tokens**: 1500 characters (enforced)
- **Concurrent requests**: Supports async operations
- **Memory usage**: ~200MB base + model memory

## 🔄 Version History

### v2.0.0 (Current)
- **Major refactoring** to Clean Architecture
- 5-layer design with strict separation of concerns
- Improved testability with dependency injection
- Better error handling and logging

### v1.0.0
- Initial monolithic implementation
- Basic summarization functionality
