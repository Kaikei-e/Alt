/**
 * Generic cursor-based pagination utility
 * Provides efficient pagination without OFFSET-based performance issues
 */
export class CursorPagination<T> {
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

  get data() { 
    return this._data; 
  }
  
  get cursor() { 
    return this._cursor; 
  }
  
  get hasMore() { 
    return this._hasMore; 
  }
  
  get isLoading() { 
    return this._isLoading; 
  }
  
  get error() { 
    return this._error; 
  }
  
  get isInitialLoading() { 
    return this._isInitialLoading; 
  }

  async loadInitial() {
    this._isInitialLoading = true;
    this._isLoading = true;
    this._error = null;

    try {
      const response = await this.fetchFn(undefined, this.limit);
      this._data = response.data;
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
      this._data = [...this._data, ...response.data];
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