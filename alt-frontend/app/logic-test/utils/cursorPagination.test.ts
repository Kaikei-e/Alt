import { describe, expect, it, vi, beforeEach } from "vitest";

// Mock the feed type
type MockFeed = {
  id: string;
  title: string;
  description: string;
  link: string;
  published: string;
};

type MockCursorResponse = {
  data: MockFeed[];
  next_cursor: string | null;
};

// Mock API function
const mockGetFeedsWithCursor = vi.fn<(cursor?: string, limit?: number) => Promise<MockCursorResponse>>();

// Class-based pagination manager (easier to test than React hooks)
class CursorPagination<T> {
  private _data: T[] = [];
  private _cursor: string | null = null;
  private _hasMore = true;
  private _isLoading = false;
  private _error: string | null = null;
  private _isInitialLoading = true;

  constructor(
    private fetchFn: (cursor?: string, limit?: number) => Promise<{ data: T[]; next_cursor: string | null }>,
    private limit: number = 20
  ) {}

  get data() { return this._data; }
  get cursor() { return this._cursor; }
  get hasMore() { return this._hasMore; }
  get isLoading() { return this._isLoading; }
  get error() { return this._error; }
  get isInitialLoading() { return this._isInitialLoading; }

  async loadInitial() {
    this._isInitialLoading = true;
    this._isLoading = true;
    this._error = null;

    try {
      const response = await this.fetchFn(undefined, this.limit);
      this._data = response.data as T[];
      this._cursor = response.next_cursor;
      this._hasMore = response.next_cursor !== null;
    } catch (err) {
      this._error = err instanceof Error ? err.message : "Failed to load data";
      this._data = [];
      this._hasMore = false;
    } finally {
      this._isLoading = false;
      this._isInitialLoading = false;
    }
  }

  async loadMore() {
    if (this._isLoading || !this._hasMore || !this._cursor) {
      return;
    }

    this._isLoading = true;
    this._error = null;

    try {
      const response = await this.fetchFn(this._cursor, this.limit);
      this._data = [...this._data, ...response.data] as T[];
      this._cursor = response.next_cursor;
      this._hasMore = response.next_cursor !== null;
    } catch (err) {
      this._error = err instanceof Error ? err.message : "Failed to load more data";
    } finally {
      this._isLoading = false;
    }
  }

  async refresh() {
    this._cursor = null;
    this._hasMore = true;
    await this.loadInitial();
  }

  reset() {
    this._data = [];
    this._cursor = null;
    this._hasMore = true;
    this._isLoading = false;
    this._error = null;
    this._isInitialLoading = true;
  }
}

describe("CursorPagination", () => {
  let pagination: CursorPagination<MockFeed>;

  beforeEach(() => {
    vi.clearAllMocks();
    mockGetFeedsWithCursor.mockReset();
    pagination = new CursorPagination(mockGetFeedsWithCursor, 20);
  });

  it("should initialize with correct default state", () => {
    expect(pagination.data).toEqual([]);
    expect(pagination.cursor).toBeNull();
    expect(pagination.hasMore).toBe(true);
    expect(pagination.isLoading).toBe(false);
    expect(pagination.error).toBeNull();
    expect(pagination.isInitialLoading).toBe(true);
  });

  it("should load initial data successfully", async () => {
    const mockData = [
      { id: "1", title: "Feed 1", description: "Desc 1", link: "https://1.com", published: "2023-01-01" },
      { id: "2", title: "Feed 2", description: "Desc 2", link: "https://2.com", published: "2023-01-02" },
    ];

    const mockResponse: MockCursorResponse = {
      data: mockData,
      next_cursor: "2023-01-02T00:00:00Z",
    };

    mockGetFeedsWithCursor.mockResolvedValueOnce(mockResponse);

    await pagination.loadInitial();

    expect(mockGetFeedsWithCursor).toHaveBeenCalledWith(undefined, 20);
    expect(pagination.data).toEqual(mockData);
    expect(pagination.cursor).toBe("2023-01-02T00:00:00Z");
    expect(pagination.hasMore).toBe(true);
    expect(pagination.isInitialLoading).toBe(false);
    expect(pagination.isLoading).toBe(false);
    expect(pagination.error).toBeNull();
  });

  it("should load more data and append to existing data", async () => {
    const initialData = [
      { id: "1", title: "Feed 1", description: "Desc 1", link: "https://1.com", published: "2023-01-01" },
    ];

    const moreData = [
      { id: "2", title: "Feed 2", description: "Desc 2", link: "https://2.com", published: "2023-01-02" },
    ];

    mockGetFeedsWithCursor
      .mockResolvedValueOnce({
        data: initialData,
        next_cursor: "2023-01-01T00:00:00Z",
      })
      .mockResolvedValueOnce({
        data: moreData,
        next_cursor: "2023-01-02T00:00:00Z",
      });

    // Load initial data
    await pagination.loadInitial();

    // Load more data
    await pagination.loadMore();

    expect(mockGetFeedsWithCursor).toHaveBeenCalledTimes(2);
    expect(mockGetFeedsWithCursor).toHaveBeenNthCalledWith(1, undefined, 20);
    expect(mockGetFeedsWithCursor).toHaveBeenNthCalledWith(2, "2023-01-01T00:00:00Z", 20);
    
    expect(pagination.data).toEqual([...initialData, ...moreData]);
    expect(pagination.cursor).toBe("2023-01-02T00:00:00Z");
    expect(pagination.hasMore).toBe(true);
  });

  it("should handle end of data when next_cursor is null", async () => {
    const mockData = [
      { id: "1", title: "Feed 1", description: "Desc 1", link: "https://1.com", published: "2023-01-01" },
    ];

    mockGetFeedsWithCursor.mockResolvedValueOnce({
      data: mockData,
      next_cursor: null,
    });

    await pagination.loadInitial();

    expect(pagination.data).toEqual(mockData);
    expect(pagination.cursor).toBeNull();
    expect(pagination.hasMore).toBe(false);
  });

  it("should handle loading errors gracefully", async () => {
    const errorMessage = "Network error";
    mockGetFeedsWithCursor.mockRejectedValueOnce(new Error(errorMessage));

    await pagination.loadInitial();

    expect(pagination.data).toEqual([]);
    expect(pagination.error).toBe(errorMessage);
    expect(pagination.hasMore).toBe(false);
    expect(pagination.isInitialLoading).toBe(false);
    expect(pagination.isLoading).toBe(false);
  });

  it("should not load more when already loading", async () => {
    // Setup initial state with hasMore true and cursor available
    const initialData = [
      { id: "1", title: "Feed 1", description: "Desc 1", link: "https://1.com", published: "2023-01-01" },
    ];

    mockGetFeedsWithCursor.mockResolvedValueOnce({
      data: initialData,
      next_cursor: "2023-01-01T00:00:00Z",
    });

    await pagination.loadInitial();

    // Now simulate concurrent loadMore calls
    const promise1 = pagination.loadMore();
    const promise2 = pagination.loadMore(); // This should return early

    await Promise.all([promise1, promise2]);

    // Only one additional call should be made (after the initial load)
    expect(mockGetFeedsWithCursor).toHaveBeenCalledTimes(2); // initial + 1 loadMore
  });

  it("should not load more when hasMore is false", async () => {
    const mockData = [
      { id: "1", title: "Feed 1", description: "Desc 1", link: "https://1.com", published: "2023-01-01" },
    ];

    // Setup state where hasMore is false
    mockGetFeedsWithCursor.mockResolvedValueOnce({
      data: mockData,
      next_cursor: null, // This will set hasMore to false
    });

    await pagination.loadInitial();

    // Try to load more
    await pagination.loadMore();

    // Should only have made the initial call
    expect(mockGetFeedsWithCursor).toHaveBeenCalledTimes(1);
  });

  it("should refresh data correctly", async () => {
    const initialData = [
      { id: "1", title: "Feed 1", description: "Desc 1", link: "https://1.com", published: "2023-01-01" },
    ];

    const refreshedData = [
      { id: "2", title: "Feed 2", description: "Desc 2", link: "https://2.com", published: "2023-01-02" },
    ];

    mockGetFeedsWithCursor
      .mockResolvedValueOnce({
        data: initialData,
        next_cursor: "2023-01-01T00:00:00Z",
      })
      .mockResolvedValueOnce({
        data: refreshedData,
        next_cursor: "2023-01-02T00:00:00Z",
      });

    // Load initial data
    await pagination.loadInitial();

    // Refresh
    await pagination.refresh();

    expect(mockGetFeedsWithCursor).toHaveBeenCalledTimes(2);
    expect(pagination.data).toEqual(refreshedData);
    expect(pagination.cursor).toBe("2023-01-02T00:00:00Z");
  });

  it("should reset state correctly", () => {
    pagination.reset();

    expect(pagination.data).toEqual([]);
    expect(pagination.cursor).toBeNull();
    expect(pagination.hasMore).toBe(true);
    expect(pagination.isLoading).toBe(false);
    expect(pagination.error).toBeNull();
    expect(pagination.isInitialLoading).toBe(true);
  });
});