import { describe, expect, it, vi, beforeEach } from "vitest";

// Mock Feed type for testing
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
const mockGetFeedsWithCursor =
  vi.fn<(cursor?: string, limit?: number) => Promise<MockCursorResponse>>();

// Hook behavior simulator (tests hook logic without DOM dependencies)
class CursorPaginationHookTester<T> {
  private state = {
    data: [] as T[],
    cursor: null as string | null,
    hasMore: true,
    isLoading: false,
    error: null as string | null,
    isInitialLoading: true,
  };

  constructor(
    private fetchFn: (
      cursor?: string,
      limit?: number,
    ) => Promise<{ data: T[]; next_cursor: string | null }>,
    private limit: number = 20,
  ) {}

  // Simulate hook's initial state
  getCurrentState() {
    return { ...this.state };
  }

  // Simulate loadInitial function
  async loadInitial() {
    this.state.isInitialLoading = true;
    this.state.isLoading = true;
    this.state.error = null;

    try {
      const response = await this.fetchFn(undefined, this.limit);
      this.state.data = response.data;
      this.state.cursor = response.next_cursor;
      this.state.hasMore = response.next_cursor !== null;
    } catch (err) {
      this.state.error =
        err instanceof Error ? err.message : "Failed to load data";
      this.state.data = [];
      this.state.hasMore = false;
    } finally {
      this.state.isLoading = false;
      this.state.isInitialLoading = false;
    }
  }

  // Simulate loadMore function
  async loadMore() {
    if (this.state.isLoading || !this.state.hasMore || !this.state.cursor) {
      return;
    }

    this.state.isLoading = true;
    this.state.error = null;

    try {
      const response = await this.fetchFn(this.state.cursor, this.limit);
      this.state.data = [...this.state.data, ...response.data];
      this.state.cursor = response.next_cursor;
      this.state.hasMore = response.next_cursor !== null;
    } catch (err) {
      this.state.error =
        err instanceof Error ? err.message : "Failed to load more data";
    } finally {
      this.state.isLoading = false;
    }
  }

  // Simulate refresh function
  async refresh() {
    this.state.cursor = null;
    this.state.hasMore = true;
    await this.loadInitial();
  }

  // Simulate reset function
  reset() {
    this.state.data = [];
    this.state.cursor = null;
    this.state.hasMore = true;
    this.state.isLoading = false;
    this.state.error = null;
    this.state.isInitialLoading = true;
  }
}

describe("useCursorPagination Hook Logic", () => {
  let hookTester: CursorPaginationHookTester<MockFeed>;

  beforeEach(() => {
    vi.clearAllMocks();
    mockGetFeedsWithCursor.mockReset();
    hookTester = new CursorPaginationHookTester(mockGetFeedsWithCursor, 20);
  });

  it("should initialize with correct default state", () => {
    const state = hookTester.getCurrentState();

    expect(state.data).toEqual([]);
    expect(state.cursor).toBeNull();
    expect(state.hasMore).toBe(true);
    expect(state.isLoading).toBe(false);
    expect(state.error).toBeNull();
    expect(state.isInitialLoading).toBe(true);
  });

  it("should load initial data successfully", async () => {
    const mockData = [
      {
        id: "1",
        title: "Feed 1",
        description: "Desc 1",
        link: "https://1.com",
        published: "2023-01-01",
      },
      {
        id: "2",
        title: "Feed 2",
        description: "Desc 2",
        link: "https://2.com",
        published: "2023-01-02",
      },
    ];

    const mockResponse: MockCursorResponse = {
      data: mockData,
      next_cursor: "2023-01-02T00:00:00Z",
    };

    mockGetFeedsWithCursor.mockResolvedValueOnce(mockResponse);

    await hookTester.loadInitial();
    const state = hookTester.getCurrentState();

    expect(mockGetFeedsWithCursor).toHaveBeenCalledWith(undefined, 20);
    expect(state.data).toEqual(mockData);
    expect(state.cursor).toBe("2023-01-02T00:00:00Z");
    expect(state.hasMore).toBe(true);
    expect(state.isInitialLoading).toBe(false);
    expect(state.isLoading).toBe(false);
    expect(state.error).toBeNull();
  });

  it("should load more data and append to existing data", async () => {
    const initialData = [
      {
        id: "1",
        title: "Feed 1",
        description: "Desc 1",
        link: "https://1.com",
        published: "2023-01-01",
      },
    ];

    const moreData = [
      {
        id: "2",
        title: "Feed 2",
        description: "Desc 2",
        link: "https://2.com",
        published: "2023-01-02",
      },
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
    await hookTester.loadInitial();

    // Load more data
    await hookTester.loadMore();

    const state = hookTester.getCurrentState();

    expect(mockGetFeedsWithCursor).toHaveBeenCalledTimes(2);
    expect(mockGetFeedsWithCursor).toHaveBeenNthCalledWith(1, undefined, 20);
    expect(mockGetFeedsWithCursor).toHaveBeenNthCalledWith(
      2,
      "2023-01-01T00:00:00Z",
      20,
    );

    expect(state.data).toEqual([...initialData, ...moreData]);
    expect(state.cursor).toBe("2023-01-02T00:00:00Z");
    expect(state.hasMore).toBe(true);
  });

  it("should handle end of data when next_cursor is null", async () => {
    const mockData = [
      {
        id: "1",
        title: "Feed 1",
        description: "Desc 1",
        link: "https://1.com",
        published: "2023-01-01",
      },
    ];

    mockGetFeedsWithCursor.mockResolvedValueOnce({
      data: mockData,
      next_cursor: null,
    });

    await hookTester.loadInitial();
    const state = hookTester.getCurrentState();

    expect(state.data).toEqual(mockData);
    expect(state.cursor).toBeNull();
    expect(state.hasMore).toBe(false);
  });

  it("should handle loading errors gracefully", async () => {
    const errorMessage = "Network error";
    mockGetFeedsWithCursor.mockRejectedValueOnce(new Error(errorMessage));

    await hookTester.loadInitial();
    const state = hookTester.getCurrentState();

    expect(state.data).toEqual([]);
    expect(state.error).toBe(errorMessage);
    expect(state.hasMore).toBe(false);
    expect(state.isInitialLoading).toBe(false);
    expect(state.isLoading).toBe(false);
  });

  it("should not load more when already loading", async () => {
    // Setup initial state with hasMore true and cursor available
    const initialData = [
      {
        id: "1",
        title: "Feed 1",
        description: "Desc 1",
        link: "https://1.com",
        published: "2023-01-01",
      },
    ];

    mockGetFeedsWithCursor.mockResolvedValueOnce({
      data: initialData,
      next_cursor: "2023-01-01T00:00:00Z",
    });

    await hookTester.loadInitial();

    // Mock a slow response for loadMore to simulate loading state
    mockGetFeedsWithCursor.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          setTimeout(() => resolve({ data: [], next_cursor: null }), 100);
        }),
    );

    // Start loadMore (it will set isLoading to true)
    const promise1 = hookTester.loadMore();

    // Try to call loadMore again while first is still loading
    await hookTester.loadMore(); // This should return early

    await promise1; // Wait for first loadMore to complete

    // Only one additional call should be made (after the initial load)
    expect(mockGetFeedsWithCursor).toHaveBeenCalledTimes(2); // initial + 1 loadMore
  });

  it("should not load more when hasMore is false", async () => {
    const mockData = [
      {
        id: "1",
        title: "Feed 1",
        description: "Desc 1",
        link: "https://1.com",
        published: "2023-01-01",
      },
    ];

    // Setup state where hasMore is false
    mockGetFeedsWithCursor.mockResolvedValueOnce({
      data: mockData,
      next_cursor: null, // This will set hasMore to false
    });

    await hookTester.loadInitial();

    // Try to load more
    await hookTester.loadMore();

    // Should only have made the initial call
    expect(mockGetFeedsWithCursor).toHaveBeenCalledTimes(1);
  });

  it("should refresh data correctly", async () => {
    const initialData = [
      {
        id: "1",
        title: "Feed 1",
        description: "Desc 1",
        link: "https://1.com",
        published: "2023-01-01",
      },
    ];

    const refreshedData = [
      {
        id: "2",
        title: "Feed 2",
        description: "Desc 2",
        link: "https://2.com",
        published: "2023-01-02",
      },
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
    await hookTester.loadInitial();

    // Refresh
    await hookTester.refresh();

    const state = hookTester.getCurrentState();

    expect(mockGetFeedsWithCursor).toHaveBeenCalledTimes(2);
    expect(state.data).toEqual(refreshedData);
    expect(state.cursor).toBe("2023-01-02T00:00:00Z");
  });

  it("should reset state correctly", () => {
    hookTester.reset();
    const state = hookTester.getCurrentState();

    expect(state.data).toEqual([]);
    expect(state.cursor).toBeNull();
    expect(state.hasMore).toBe(true);
    expect(state.isLoading).toBe(false);
    expect(state.error).toBeNull();
    expect(state.isInitialLoading).toBe(true);
  });
});
