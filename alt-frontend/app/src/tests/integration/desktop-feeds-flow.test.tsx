import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ChakraProvider } from '@chakra-ui/react';
import { OptimizedDesktopFeeds } from '@/components/desktop/optimized/OptimizedDesktopFeeds';
import { mockDesktopFeeds, mockFilters } from '@/data/test-data';
import { desktopFeedsApi } from '@/lib/api/desktop-feeds';

// Mock API
jest.mock('@/lib/api/desktop-feeds');
const mockApi = desktopFeedsApi as jest.Mocked<typeof desktopFeedsApi>;

const renderWithChakra = (ui: React.ReactElement) => {
  return render(
    <ChakraProvider>
      {ui}
    </ChakraProvider>
  );
};

describe('Desktop Feeds Integration', () => {
  beforeEach(() => {
    mockApi.markAsRead.mockResolvedValue();
    mockApi.toggleFavorite.mockResolvedValue();
    mockApi.toggleBookmark.mockResolvedValue();
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('should handle complete user workflow', async () => {
    const user = userEvent.setup();
    const mockOnFilterChange = jest.fn();

    renderWithChakra(
      <OptimizedDesktopFeeds
        feeds={mockDesktopFeeds}
        filters={mockFilters}
        onFilterChange={mockOnFilterChange}
      />
    );

    // 1. 初期レンダリングの確認
    expect(screen.getByText('📰 Alt Feeds')).toBeInTheDocument();

    // 2. フィードカードが表示される
    const feedCards = screen.getAllByTestId(/desktop-feed-card-/);
    expect(feedCards).toHaveLength(3);

    // 3. フィルタリングテスト
    await user.click(screen.getByText('Unread'));
    expect(mockOnFilterChange).toHaveBeenCalledWith({
      ...mockFilters,
      readStatus: 'unread'
    });

    // 4. フィード操作テスト
    const markReadButton = screen.getAllByText('Mark as Read')[0];
    await user.click(markReadButton);

    await waitFor(() => {
      expect(mockApi.markAsRead).toHaveBeenCalledWith('1');
    });

    // 5. お気に入り切り替え
    const favoriteButton = screen.getAllByLabelText(/Toggle favorite/)[0];
    await user.click(favoriteButton);

    await waitFor(() => {
      expect(mockApi.toggleFavorite).toHaveBeenCalledWith('1', true);
    });

    // 6. アナリティクスパネルのテスト
    const analyticsTab = screen.getByText('Analytics');
    await user.click(analyticsTab);

    expect(screen.getByText(/Today's Reading/)).toBeInTheDocument();

    // 7. CSS変数の使用確認
    const feedCard = feedCards[0];
    const styles = window.getComputedStyle(feedCard);
    expect(styles.getPropertyValue('background')).toContain('var(');
  });

  it('should handle error states gracefully', async () => {
    const user = userEvent.setup();
    mockApi.markAsRead.mockRejectedValue(new Error('Network error'));

    renderWithChakra(
      <OptimizedDesktopFeeds
        feeds={mockDesktopFeeds}
        filters={mockFilters}
        onFilterChange={jest.fn()}
      />
    );

    const markReadButton = screen.getAllByText('Mark as Read')[0];
    await user.click(markReadButton);

    // エラーハンドリングの確認
    await waitFor(() => {
      expect(screen.getByText(/エラーが発生しました/)).toBeInTheDocument();
    });
  });

  it('should maintain performance with large datasets', async () => {
    const largeDataset = Array.from({ length: 1000 }, (_, i) => ({
      ...mockDesktopFeeds[0],
      id: `feed-${i}`,
      title: `Feed ${i}`
    }));

    const startTime = performance.now();

    renderWithChakra(
      <OptimizedDesktopFeeds
        feeds={largeDataset}
        filters={mockFilters}
        onFilterChange={jest.fn()}
      />
    );

    const endTime = performance.now();
    const renderTime = endTime - startTime;

    // レンダリング時間の確認 (< 100ms)
    expect(renderTime).toBeLessThan(100);

    // メモ化が機能しているか確認
    const visibleItems = screen.getAllByTestId(/desktop-feed-card-/);
    expect(visibleItems.length).toBeLessThan(largeDataset.length);
  });

  it('should work with Chakra UI theme', async () => {
    renderWithChakra(
      <OptimizedDesktopFeeds
        feeds={mockDesktopFeeds}
        filters={mockFilters}
        onFilterChange={jest.fn()}
      />
    );

    // Chakra UIコンポーネントが正しく使用されているか確認
    const chakraBox = screen.getByTestId('desktop-feed-card-1');
    expect(chakraBox).toHaveClass('chakra-box');

    // CSS変数がChakra UIで正しく適用されているか確認
    const styles = window.getComputedStyle(chakraBox);
    expect(styles.borderRadius).toBe('var(--radius-xl)');
  });
});