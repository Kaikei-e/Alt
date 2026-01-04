package rest

import (
	"alt/di"
	"alt/port/rag_integration_port"
	"alt/usecase/answer_chat_usecase"
	"alt/usecase/retrieve_context_usecase"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
)

type AugurHandler struct {
	retrieveContextUsecase retrieve_context_usecase.RetrieveContextUsecase
	answerChatUsecase      answer_chat_usecase.AnswerChatUsecase
}

func NewAugurHandler(
	retrieveContextUsecase retrieve_context_usecase.RetrieveContextUsecase,
	answerChatUsecase answer_chat_usecase.AnswerChatUsecase,
) *AugurHandler {
	return &AugurHandler{
		retrieveContextUsecase: retrieveContextUsecase,
		answerChatUsecase:      answerChatUsecase,
	}
}

func (h *AugurHandler) RetrieveContext(c echo.Context) error {
	query := c.QueryParam("q")
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "query parameter 'q' is required"})
	}

	contexts, err := h.retrieveContextUsecase.Execute(c.Request().Context(), query)
	if err != nil {
		return HandleError(c, err, "RetrieveContext")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"contexts": contexts,
	})
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnswerRequest struct {
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

func (h *AugurHandler) Answer(c echo.Context) error {
	var req AnswerRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Extract last user message as query
	var query string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			query = req.Messages[i].Content
			break
		}
	}

	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no user message found"})
	}

	input := rag_integration_port.AnswerInput{
		Query:  query,
		Stream: req.Stream,
	}

	answerChan, err := h.answerChatUsecase.Execute(c.Request().Context(), input)
	if err != nil {
		return HandleError(c, err, "Answer")
	}

	if req.Stream {
		c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
		c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
		c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
		c.Response().WriteHeader(http.StatusOK)

		// SEE Stream Filtering
		// We need to filter out 'event: meta' to remove sensitive 'Contexts' and 'Debug' info
		// before sending to the client.

		buffer := ""

		for chunk := range answerChan {
			buffer += chunk

			// Process complete events separated by double newline
			for {
				splitIdx := -1
				// Simple search for \n\n
				for i := 0; i < len(buffer)-1; i++ {
					if buffer[i] == '\n' && buffer[i+1] == '\n' {
						splitIdx = i
						break
					}
				}

				if splitIdx == -1 {
					break // No complete event yet
				}

				// Extract event string including the double newline
				eventStr := buffer[:splitIdx+2]
				buffer = buffer[splitIdx+2:]

				// Check if it's a meta event
				// eventStr format: "event:meta\ndata:{\"Contexts\":...}\n\n"
				if len(eventStr) > 10 && (eventStr[:10] == "event:meta" || eventStr[:11] == "event: meta") {
					// We need to sanitize this
					processedEvent := sanitizeMetaEvent(eventStr)
					if _, err := c.Response().Write([]byte(processedEvent)); err != nil {
						// Client disconnected, stop streaming
						return nil
					}
				} else {
					// Pass through other events (delta, etc)
					if _, err := c.Response().Write([]byte(eventStr)); err != nil {
						// Client disconnected, stop streaming
						return nil
					}
				}
				c.Response().Flush()
			}
		}
		return nil
	}

	// Non-streaming response
	// We consume the channel (should be one item)
	var fullAnswer string
	for chunk := range answerChan {
		fullAnswer += chunk
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"answer": fullAnswer,
	})
}

// sanitizeMetaEvent parses the meta event, removes sensitive fields, and reconstructs it.
func sanitizeMetaEvent(eventStr string) string {
	// 1. Extract data payload
	// Find "data:"
	dataIdx := -1
	lines := splitLines(eventStr)
	for i, line := range lines {
		if len(line) >= 5 && line[:5] == "data:" {
			dataIdx = i
			break
		}
	}

	if dataIdx == -1 {
		return eventStr // malformed, pass thorough or drop? pass for now
	}

	dataLine := lines[dataIdx]
	jsonPayload := dataLine[5:] // skip "data:"

	// 2. Parse JSON
	// We define a struct that matches the expected sensitive fields to drop them
	// and map to a cleaner structure.
	type ContextItem struct {
		ChunkText       string  `json:"ChunkText"` // Sensitive/Large
		URL             string  `json:"URL"`
		Title           string  `json:"Title"`
		PublishedAt     string  `json:"PublishedAt"`
		Score           float64 `json:"Score"`
		DocumentVersion int     `json:"DocumentVersion"`
		ChunkID         string  `json:"ChunkID"` // Internal ID
	}

	type IncomingMeta struct {
		Contexts []ContextItem `json:"Contexts"`
		Debug    interface{}   `json:"Debug"` // Sensitive
	}

	// Clean Output Structure
	type SafeCitation struct {
		URL         string `json:"URL"`
		Title       string `json:"Title"`
		PublishedAt string `json:"PublishedAt"`
	}

	type OutgoingMeta struct {
		Citations []SafeCitation `json:"Citations"`
	}

	var in IncomingMeta
	if err := json.Unmarshal([]byte(jsonPayload), &in); err != nil {
		// If parse fails, just return original (or empty to be safe? return original for backward compat if schema changes)
		return eventStr
	}

	// 3. Map to safe structure
	out := OutgoingMeta{
		Citations: make([]SafeCitation, 0, len(in.Contexts)),
	}

	for _, ctx := range in.Contexts {
		out.Citations = append(out.Citations, SafeCitation{
			URL:         ctx.URL,
			Title:       ctx.Title,
			PublishedAt: ctx.PublishedAt,
		})
	}

	// 4. Marshaling
	safeJSON, err := json.Marshal(out)
	if err != nil {
		return eventStr
	}

	// 5. Reconstruct Event
	return "event:meta\ndata:" + string(safeJSON) + "\n\n"
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func RegisterAugurRoutes(e *echo.Echo, g *echo.Group, container *di.ApplicationComponents) {
	handler := NewAugurHandler(container.RetrieveContextUsecase, container.AnswerChatUsecase)
	g.GET("/rag/context", handler.RetrieveContext)
	e.POST("/sse/v1/rag/answer", handler.Answer)
}
