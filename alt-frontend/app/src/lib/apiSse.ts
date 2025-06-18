import { FeedStatsSummary } from "@/schema/feedStats";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost/api";

function apiSseForStats(
  endpoint: string,
  onMessage: (data: FeedStatsSummary) => void,
  onError: (event: Event) => void,
) {
  let eventSource: EventSource | null = null;
  let reconnectAttempts = 0;
  const maxReconnectAttempts = 5;
  const reconnectDelay = 2000; // 2 seconds

  function connect() {
    const fullUrl = `${API_BASE_URL}${endpoint}`;
    console.log(
      `Connecting to SSE endpoint: ${fullUrl} (attempt ${reconnectAttempts + 1})`,
    );

    eventSource = new EventSource(fullUrl);

    eventSource.onopen = (event) => {
      console.log("SSE connection opened:", event);
      reconnectAttempts = 0; // Reset reconnect attempts on successful connection
    };

    eventSource.onmessage = (event) => {
      console.log("SSE message received:", event.data);
      try {
        const parsedData = JSON.parse(event.data) as FeedStatsSummary;
        onMessage(parsedData);
      } catch (error) {
        console.error("Error parsing SSE data:", error);
      }
    };

    eventSource.onerror = (event) => {
      console.error("SSE error:", event);
      console.log("EventSource readyState:", eventSource?.readyState);

      if (eventSource?.readyState === EventSource.CLOSED) {
        console.log("SSE connection closed, attempting to reconnect...");

        if (reconnectAttempts < maxReconnectAttempts) {
          reconnectAttempts++;
          setTimeout(() => {
            connect();
          }, reconnectDelay * reconnectAttempts); // Exponential backoff
        } else {
          console.error("Max reconnection attempts reached");
          onError(event);
        }
      } else {
        onError(event);
      }
    };
  }

  connect();

  return {
    close: () => {
      if (eventSource) {
        eventSource.close();
      }
    },
    getReadyState: () => eventSource?.readyState ?? EventSource.CLOSED,
  };
}

export const feedsApiSse = {
  getFeedsStats(
    onMessage: (data: FeedStatsSummary) => void,
    onError?: (event: Event) => void,
  ) {
    return apiSseForStats(
      "/v1/sse/feeds/stats",
      onMessage,
      onError ||
        ((event) => {
          console.error("SSE error:", event);
        }),
    );
  },
};
