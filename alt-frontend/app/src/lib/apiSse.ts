import { FeedStatsSummary } from "@/schema/feedStats";
import { SseConfig, defaultSseConfig } from "@/lib/config";

export function setupSSE(
  endpoint: string,
  onData: (data: FeedStatsSummary) => void,
  onError?: () => void
): EventSource | null {
  try {
    const eventSource = new EventSource(endpoint);

    eventSource.onopen = () => {
      // Connection established
    };

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as FeedStatsSummary;
        onData(data);
      } catch {
        // Error parsing SSE data - handled silently
      }
    };

    eventSource.onerror = () => {
      // SSE error - handled silently
      if (onError) {
        onError();
      }
    };

    return eventSource;
  } catch {
    // Error creating SSE connection - handled silently
    if (onError) {
      onError();
    }
    return null;
  }
}

export function setupSSEWithReconnect(
  endpoint: string,
  onData: (data: FeedStatsSummary) => void,
  onError?: () => void,
  maxReconnectAttempts: number = 3
): { eventSource: EventSource | null; cleanup: () => void } {
  let eventSource: EventSource | null = null;
  let reconnectAttempts = 0;
  let reconnectTimeout: NodeJS.Timeout | null = null;

  const connect = () => {
    try {
      eventSource = new EventSource(endpoint);

      eventSource.onopen = () => {
        reconnectAttempts = 0; // Reset on successful connection
      };

      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as FeedStatsSummary;
          onData(data);
        } catch {
          // Error parsing SSE data - handled silently
        }
      };

      eventSource.onerror = () => {
        // SSE error - handled silently
        eventSource?.close();

        if (reconnectAttempts < maxReconnectAttempts) {
          reconnectAttempts++;
          const delay = Math.pow(2, reconnectAttempts - 1) * 1000; // Exponential backoff
          reconnectTimeout = setTimeout(connect, delay);
        } else {
          // Max reconnection attempts reached - handled silently
          if (onError) {
            onError();
          }
        }
      };

    } catch {
      // Error creating SSE connection - handled silently
      if (onError) {
        onError();
      }
    }
  };

  const cleanup = () => {
    if (reconnectTimeout) {
      clearTimeout(reconnectTimeout);
      reconnectTimeout = null;
    }
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
  };

  connect();

  return { eventSource, cleanup };
}

export class SseClient {
  private config: SseConfig;

  constructor(config: SseConfig = defaultSseConfig) {
    this.config = config;
  }

  getFeedsStats(
    onMessage: (data: FeedStatsSummary) => void,
    onError?: () => void,
  ) {
    return setupSSE(
      `${this.config.baseUrl}/v1/sse/feeds/stats`,
      onMessage,
      onError ||
        (() => {
          // SSE error - handled silently
        })
    );
  }
}

export const feedsApiSse = new SseClient();
