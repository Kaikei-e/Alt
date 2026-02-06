/**
 * HttpClient port - interface for HTTP operations with timeout and proxy support
 */
export interface HttpClient {
  fetch(url: string, options?: RequestInit): Promise<Response>;
}
