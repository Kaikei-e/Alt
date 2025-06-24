import { UnsummarizedFeedStatsSummary } from "@/schema/feedStats";
import { SseConfig, defaultSseConfig } from "@/lib/config";

export function setupSSE(
  endpoint: string,
  onData: (data: UnsummarizedFeedStatsSummary) => void,
  onError?: () => void
): EventSource | null {
  try {
    const eventSource = new EventSource(endpoint);

    eventSource.onopen = () => {
      // Connection established
    };

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as UnsummarizedFeedStatsSummary;
        // Validate basic structure before passing to callback
        if (data && typeof data === 'object') {
          onData(data);
        } else {
          console.warn('Invalid SSE data structure:', data);
        }
      } catch (error) {
        console.error('Error parsing SSE data:', error, 'Raw data:', event.data);
      }
    };

    eventSource.onerror = (error) => {
      console.error('SSE connection error:', {
        readyState: eventSource.readyState,
        url: endpoint,
        error
      });
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
  onData: (data: UnsummarizedFeedStatsSummary) => void,
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
          const data = JSON.parse(event.data) as UnsummarizedFeedStatsSummary;
          // Validate basic structure before passing to callback
          if (data && typeof data === 'object') {
            onData(data);
          } else {
            console.warn('Invalid SSE data structure:', data);
          }
        } catch (error) {
          console.error('Error parsing SSE data:', error, 'Raw data:', event.data);
        }
      };

      eventSource.onerror = (error) => {
        console.error('SSE connection error:', {
          readyState: eventSource?.readyState,
          url: endpoint,
          attempts: reconnectAttempts,
          maxAttempts: maxReconnectAttempts,
          error
        });
        
        eventSource?.close();

        if (reconnectAttempts < maxReconnectAttempts) {
          reconnectAttempts++;
          const delay = Math.pow(2, reconnectAttempts - 1) * 1000; // Exponential backoff
          console.log(`Retrying SSE connection (${reconnectAttempts}/${maxReconnectAttempts}) in ${delay}ms...`);
          reconnectTimeout = setTimeout(connect, delay);
        } else {
          console.error('Max SSE reconnection attempts reached');
          if (onError) {
            onError();
          }
        }
      };

    } catch (error) {
      console.error('Failed to create SSE connection:', error);
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
    onMessage: (data: UnsummarizedFeedStatsSummary) => void,
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
