import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import { DesktopFeedCard } from './DesktopFeedCard';
import { mockDesktopFeeds } from '@/data/mockDesktopFeeds';
import { DesktopFeedCardProps } from '@/types/desktop-feed';

const renderWithChakra = (ui: React.ReactElement) => {
  return render(
    <ChakraProvider value={defaultSystem}>
      {ui}
    </ChakraProvider>
  );
};

describe('DesktopFeedCard', () => {
  const mockProps: DesktopFeedCardProps = {
    feed: mockDesktopFeeds[0],
    onMarkAsRead: vi.fn(),
    onToggleFavorite: vi.fn(),
    onToggleBookmark: vi.fn(),
    onReadLater: vi.fn(),
    onViewArticle: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should render feed with glass effect styling', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    const card = screen.getByTestId(`desktop-feed-card-${mockDesktopFeeds[0].id}`);
    expect(card).toBeInTheDocument();
    expect(card).toHaveClass('glass');
  });

  it('should display feed metadata correctly', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    expect(screen.getByText(mockDesktopFeeds[0].title)).toBeInTheDocument();
    expect(screen.getByText(mockDesktopFeeds[0].metadata.source.name)).toBeInTheDocument();
    expect(screen.getByText(/5 min read/)).toBeInTheDocument();
    // No longer expecting views/comments to be displayed
  });

  it('should handle mark as read action', async () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    const markAsReadButton = screen.getByText('Mark as Read');
    fireEvent.click(markAsReadButton);

    expect(mockProps.onMarkAsRead).toHaveBeenCalledWith(mockDesktopFeeds[0].id);
  });

  it('should handle favorite toggle action', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    const favoriteButton = screen.getByLabelText('Toggle favorite');
    fireEvent.click(favoriteButton);

    expect(mockProps.onToggleFavorite).toHaveBeenCalledWith(mockDesktopFeeds[0].id);
  });

  it('should handle bookmark toggle action', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    const bookmarkButton = screen.getByLabelText('Toggle bookmark');
    fireEvent.click(bookmarkButton);

    expect(mockProps.onToggleBookmark).toHaveBeenCalledWith(mockDesktopFeeds[0].id);
  });

  it('should apply priority styling correctly', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    const card = screen.getByTestId(`desktop-feed-card-${mockDesktopFeeds[0].id}`);
    expect(card).toHaveStyle({
      borderLeftColor: 'var(--accent-primary)' // high priority
    });
  });

  it('should handle read later action', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    const readLaterButton = screen.getByText('Read Later');
    fireEvent.click(readLaterButton);

    expect(mockProps.onReadLater).toHaveBeenCalledWith(mockDesktopFeeds[0].id);
  });

  it('should handle view article action', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    const viewArticleButton = screen.getByText('View Article');
    fireEvent.click(viewArticleButton);

    expect(mockProps.onViewArticle).toHaveBeenCalledWith(mockDesktopFeeds[0].id);
  });

  it('should show reading progress when feed is read', () => {
    const readFeed = {
      ...mockDesktopFeeds[2], // This one is read with progress
      isRead: true,
      readingProgress: 78
    };

    renderWithChakra(
      <DesktopFeedCard {...mockProps} feed={readFeed} />
    );

    expect(screen.getByText('Reading progress: 78%')).toBeInTheDocument();
  });

  it('should display tags correctly', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    mockDesktopFeeds[0].metadata.tags.slice(0, 4).forEach(tag => {
      expect(screen.getByText(`#${tag}`)).toBeInTheDocument();
    });
  });

  it('should show difficulty badge', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    expect(screen.getByText('intermediate')).toBeInTheDocument();
  });

  // TDD Test: SNS engagement stats should be removed
  it('should not display SNS engagement stats (views and comments)', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    // These SNS elements should no longer be present
    expect(screen.queryByText(/views/)).not.toBeInTheDocument();
    expect(screen.queryByText(/comments/)).not.toBeInTheDocument();
  });

  // TDD Test: RSS-specific elements should remain after SNS removal
  it('should preserve RSS-specific elements after SNS removal', () => {
    renderWithChakra(<DesktopFeedCard {...mockProps} />);

    // These should remain after SNS removal
    expect(screen.getByText('3 related')).toBeInTheDocument();
    expect(screen.getByText('intermediate')).toBeInTheDocument();
    expect(screen.getByText(/5 min read/)).toBeInTheDocument();
  });
});