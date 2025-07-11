import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import { VirtualDesktopTimeline } from './VirtualDesktopTimeline';
import { Feed } from '@/schema/feed';

// Mock @tanstack/react-virtual
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: vi.fn(() => ({
    getTotalSize: () => 2000,
    getVirtualItems: () => [
      { key: 'item-0', index: 0, start: 0, size: 320 },
      { key: 'item-1', index: 1, start: 320, size: 320 },
      { key: 'item-2', index: 2, start: 640, size: 320 },
    ],
    measureElement: undefined,
  })),
}));

// Mock DesktopFeedCard component
vi.mock('./DesktopFeedCard', () => ({
  DesktopFeedCard: ({ feed, onMarkAsRead, onToggleFavorite, onToggleBookmark, onReadLater, onViewArticle }: {
    feed: { id: string; title: string; metadata: { summary: string } };
    onMarkAsRead: (id: string) => void;
    onToggleFavorite: (id: string) => void;
    onToggleBookmark: (id: string) => void;
    onReadLater: (id: string) => void;
    onViewArticle: (id: string) => void;
  }) => (
    <div data-testid={`desktop-feed-card-${feed.id}`}>
      <h3>{feed.title}</h3>
      <p>{feed.metadata.summary}</p>
      <button onClick={() => onMarkAsRead(feed.id)} aria-label="Mark as read">Mark as read</button>
      <button onClick={() => onToggleFavorite(feed.id)} aria-label="Add to favorites">Favorite</button>
      <button onClick={() => onToggleBookmark(feed.id)} aria-label="Add bookmark">Bookmark</button>
      <button onClick={() => onReadLater(feed.id)} aria-label="Read later">Read later</button>
      <button onClick={() => onViewArticle(feed.id)} aria-label="View article">View article</button>
    </div>
  ),
}));

// Mock useVirtualizationMetrics hook
vi.mock('@/hooks/useVirtualizationMetrics', () => ({
  useVirtualizationMetrics: vi.fn(),
}));

describe('VirtualDesktopTimeline', () => {
  const mockDesktopFeeds: Feed[] = Array.from({ length: 50 }, (_, i) => ({
    id: `desktop-feed-${i}`,
    title: `Desktop Feed ${i}`,
    description: `Detailed description for desktop feed ${i}`,
    link: `https://example.com/desktop-feed${i}`,
    published: new Date().toISOString(),
  }));

  const defaultProps = {
    feeds: mockDesktopFeeds,
    readFeeds: new Set<string>(),
    onMarkAsRead: vi.fn(),
    onToggleFavorite: vi.fn(),
    onToggleBookmark: vi.fn(),
    onReadLater: vi.fn(),
    onViewArticle: vi.fn(),
    containerHeight: 800,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  const renderWithChakra = (ui: React.ReactElement) => {
    return render(
      <ChakraProvider value={defaultSystem}>
        {ui}
      </ChakraProvider>
    );
  };

  it('should render desktop virtual timeline container', () => {
    renderWithChakra(<VirtualDesktopTimeline {...defaultProps} />);

    expect(screen.getByTestId('virtual-desktop-timeline')).toBeInTheDocument();
    expect(screen.getByTestId('virtual-desktop-timeline')).toHaveStyle({
      height: '800px',
    });
  });

  it('should render desktop feed cards with larger size estimation', () => {
    renderWithChakra(<VirtualDesktopTimeline {...defaultProps} />);

    const virtualContainer = screen.getByTestId('virtual-desktop-timeline');
    expect(virtualContainer).toBeInTheDocument();

    // デスクトップカードは大きいため、少ないアイテム数が表示される
    const renderedItems = screen.getAllByTestId(/^virtual-desktop-item-/);
    expect(renderedItems.length).toBeLessThan(10); // モバイルより少ない
    expect(renderedItems.length).toBeGreaterThan(0);
  });

  it('should handle desktop-specific interactions', async () => {
    const user = userEvent.setup();
    const onToggleFavorite = vi.fn();
    const onToggleBookmark = vi.fn();
    const onReadLater = vi.fn();

    renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        onToggleFavorite={onToggleFavorite}
        onToggleBookmark={onToggleBookmark}
        onReadLater={onReadLater}
      />
    );

    const favoriteButton = screen.getAllByLabelText(/add to favorites/i)[0];
    const bookmarkButton = screen.getAllByLabelText(/add bookmark/i)[0];
    const readLaterButton = screen.getAllByLabelText(/read later/i)[0];

    await user.click(favoriteButton);
    await user.click(bookmarkButton);
    await user.click(readLaterButton);

    expect(onToggleFavorite).toHaveBeenCalledWith(mockDesktopFeeds[0].id);
    expect(onToggleBookmark).toHaveBeenCalledWith(mockDesktopFeeds[0].id);
    expect(onReadLater).toHaveBeenCalledWith(mockDesktopFeeds[0].id);
  });

  it('should adapt to desktop viewport changes', () => {
    const { rerender } = renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        containerHeight={600}
      />
    );

    const smallViewportContainer = screen.getByTestId('virtual-desktop-timeline');
    expect(smallViewportContainer).toHaveStyle({ height: '600px' });

    // より大きいビューポートでレンダリング
    rerender(
      <ChakraProvider value={defaultSystem}>
        <VirtualDesktopTimeline
          {...defaultProps}
          containerHeight={1200}
        />
      </ChakraProvider>
    );

    const largeViewportContainer = screen.getByTestId('virtual-desktop-timeline');
    expect(largeViewportContainer).toHaveStyle({ height: '1200px' });
  });

  it('should handle empty feeds gracefully', () => {
    renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        feeds={[]}
      />
    );

    expect(screen.getByTestId('virtual-desktop-empty-state')).toBeInTheDocument();
    expect(screen.getByText('No feeds available')).toBeInTheDocument();
    expect(screen.getByText('Your feed will appear here once you subscribe to sources')).toBeInTheDocument();
  });

  it('should filter out read feeds', () => {
    const readFeeds = new Set([mockDesktopFeeds[0].id, mockDesktopFeeds[1].id]);
    
    renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        readFeeds={readFeeds}
      />
    );

    // 既読フィードは表示されない
    expect(screen.queryByTestId(`desktop-feed-card-${mockDesktopFeeds[0].id}`)).not.toBeInTheDocument();
    expect(screen.queryByTestId(`desktop-feed-card-${mockDesktopFeeds[1].id}`)).not.toBeInTheDocument();
    
    // 未読フィードは表示される
    expect(screen.getByTestId(`desktop-feed-card-${mockDesktopFeeds[2].id}`)).toBeInTheDocument();
  });

  it('should handle dynamic sizing when enabled', () => {
    renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        enableDynamicSizing={true}
      />
    );

    expect(screen.getByTestId('virtual-desktop-timeline')).toBeInTheDocument();
    // Dynamic sizing が有効な場合、scroll behavior が auto になる
    expect(screen.getByTestId('virtual-desktop-timeline')).toHaveStyle({
      scrollBehavior: 'auto'
    });
  });

  it('should use fixed sizing when dynamic sizing is disabled', () => {
    renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        enableDynamicSizing={false}
      />
    );

    expect(screen.getByTestId('virtual-desktop-timeline')).toBeInTheDocument();
    // Dynamic sizing が無効な場合、scroll behavior が smooth になる
    expect(screen.getByTestId('virtual-desktop-timeline')).toHaveStyle({
      scrollBehavior: 'smooth'
    });
  });

  it('should handle overscan parameter correctly', () => {
    renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        overscan={3}
      />
    );

    expect(screen.getByTestId('virtual-desktop-timeline')).toBeInTheDocument();
  });

  it('should call onMarkAsRead when mark as read button is clicked', async () => {
    const user = userEvent.setup();
    const onMarkAsRead = vi.fn();

    renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        onMarkAsRead={onMarkAsRead}
      />
    );

    const markAsReadButton = screen.getAllByLabelText(/mark as read/i)[0];
    await user.click(markAsReadButton);

    expect(onMarkAsRead).toHaveBeenCalledWith(mockDesktopFeeds[0].id);
  });

  it('should call onViewArticle when view article button is clicked', async () => {
    const user = userEvent.setup();
    const onViewArticle = vi.fn();

    renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        onViewArticle={onViewArticle}
      />
    );

    const viewArticleButton = screen.getAllByLabelText(/view article/i)[0];
    await user.click(viewArticleButton);

    expect(onViewArticle).toHaveBeenCalledWith(mockDesktopFeeds[0].id);
  });

  it('should show all feeds when no filters are applied', () => {
    renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        feeds={mockDesktopFeeds.slice(0, 10)} // 10個のフィードを表示
      />
    );

    // 仮想化により3個のアイテムが表示される（mockで定義）
    const renderedItems = screen.getAllByTestId(/^virtual-desktop-item-/);
    expect(renderedItems.length).toBe(3);
  });

  it('should handle large number of feeds efficiently', () => {
    const largeFeeds = Array.from({ length: 1000 }, (_, i) => ({
      id: `large-feed-${i}`,
      title: `Large Feed ${i}`,
      description: `Description for large feed ${i}`,
      link: `https://example.com/large-feed${i}`,
      published: new Date().toISOString(),
    }));

    renderWithChakra(
      <VirtualDesktopTimeline
        {...defaultProps}
        feeds={largeFeeds}
      />
    );

    // 仮想化により、すべてのフィードがレンダリングされることなく、表示される分のみレンダリング
    const renderedItems = screen.getAllByTestId(/^virtual-desktop-item-/);
    expect(renderedItems.length).toBe(3); // mockで定義された数
  });
});