/**
 * Common types shared across different schema files
 */

// Generic API Response wrapper
export interface ApiResponse<T> {
  data: T;
  status?: number;
  message?: string;
}

// Generic error response
export interface ApiError {
  message: string;
  status?: number;
  code?: string;
}

// Cursor-based pagination types
export interface CursorResponse<T> {
  data: T[];
  next_cursor: string | null;
  has_more?: boolean;
}

export interface CursorRequest {
  cursor?: string;
  limit?: number;
}

// Async state management types
export interface AsyncState<T> {
  data: T | null;
  isLoading: boolean;
  error: Error | null;
  isInitialLoading?: boolean;
}

// Pagination state types
export interface PaginationState<T> extends AsyncState<T[]> {
  hasMore: boolean;
  cursor: string | null;
}

// Hook result types for consistency
export interface UseAsyncResult<T> extends AsyncState<T> {
  refetch: () => Promise<void>;
  reset: () => void;
}

export interface UsePaginationResult<T> extends PaginationState<T> {
  loadMore: () => Promise<void>;
  refresh: () => Promise<void>;
  reset: () => void;
  loadInitial: () => Promise<void>;
}

// Generic message response
export interface MessageResponse {
  message: string;
}

// Validation error type
export interface ValidationError {
  field: string;
  message: string;
  code?: string;
}

// Status types
export type LoadingStatus = 'idle' | 'loading' | 'success' | 'error';

// Common utility types
export type Optional<T, K extends keyof T> = Omit<T, K> & Partial<Pick<T, K>>;
export type RequiredFields<T, K extends keyof T> = T & Required<Pick<T, K>>;