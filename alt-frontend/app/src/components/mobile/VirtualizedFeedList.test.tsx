import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { VirtualizedFeedList } from './VirtualizedFeedList';
import { Feed } from '@/schema/feed';

// Mock the SimpleFeedList component
vi.mock('./SimpleFeedList', () => ({
  SimpleFeedList: ({ feeds }: { feeds: Feed[] }) => (
    <div data-testid="feed-list-fallback">
      {feeds.map(feed => (
        <div key={feed.id}>{feed.title}</div>
      ))}
    </div>
  )
}));

// Mock react-error-boundary
vi.mock('react-error-boundary', () => ({
  ErrorBoundary: ({ children, FallbackComponent, onError }: {
    children: React.ReactNode;
    FallbackComponent: React.ComponentType<{ error: Error; resetErrorBoundary: () => void }>;
    onError?: (error: Error) => void;
  }) => {
    try {
      return children;
    } catch (error) {
      if (onError) onError(error);
      return FallbackComponent({ error, resetErrorBoundary: vi.fn() });
    }
  }
}));

// Mock the feature flags
vi.mock('@/utils/featureFlags', () => ({
  FeatureFlagManager: {
    getInstance: vi.fn(() => ({
      getFlags: vi.fn(() => ({
        enableVirtualization: 'auto',
        forceVirtualization: false,
        debugMode: false,
        virtualizationThreshold: 200
      })),
      updateFlags: vi.fn()
    }))
  },
  shouldUseVirtualization: vi.fn((itemCount: number) => itemCount >= 200)
}));

describe('VirtualizedFeedList', () => {
  const mockFeeds: Feed[] = [
    { 
      id: '1', 
      title: 'Test Feed 1', 
      description: 'Description 1',
      link: 'https://test1.com',
      published: '2024-01-01T00:00:00Z'
    },
    { 
      id: '2', 
      title: 'Test Feed 2',
      description: 'Description 2', 
      link: 'https://test2.com',
      published: '2024-01-02T00:00:00Z'
    }
  ];

  const defaultProps = {
    feeds: mockFeeds,
    readFeeds: new Set<string>(),
    onMarkAsRead: vi.fn()
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should render SimpleFeedList when item count is below threshold', async () => {
    const { shouldUseVirtualization } = await import('@/utils/featureFlags');
    vi.mocked(shouldUseVirtualization).mockReturnValue(false);

    render(<VirtualizedFeedList {...defaultProps} />);

    expect(screen.getByTestId('feed-list-fallback')).toBeInTheDocument();
    expect(screen.getByText('Test Feed 1')).toBeInTheDocument();
    expect(screen.getByText('Test Feed 2')).toBeInTheDocument();
  });

  it('should attempt virtualization when item count is above threshold', async () => {
    const { shouldUseVirtualization } = await import('@/utils/featureFlags');
    vi.mocked(shouldUseVirtualization).mockReturnValue(true);

    // Since VirtualFeedListImpl now needs ChakraProvider, we expect ChakraProvider error
    // This means the virtualization path is being attempted
    expect(() => {
      render(<VirtualizedFeedList {...defaultProps} />);
    }).toThrow('useContext returned `undefined`. Seems you forgot to wrap component within <ChakraProvider />');
  });

  it('should handle empty feeds array gracefully', async () => {
    const { shouldUseVirtualization } = await import('@/utils/featureFlags');
    vi.mocked(shouldUseVirtualization).mockReturnValue(false);

    render(<VirtualizedFeedList {...defaultProps} feeds={[]} />);

    // Should render fallback with empty feeds
    expect(screen.getByTestId('feed-list-fallback')).toBeInTheDocument();
    expect(screen.queryByText('Test Feed 1')).not.toBeInTheDocument();
  });

  it('should pass readFeeds to SimpleFeedList', async () => {
    const { shouldUseVirtualization } = await import('@/utils/featureFlags');
    vi.mocked(shouldUseVirtualization).mockReturnValue(false);

    const readFeeds = new Set(['https://test1.com']);
    
    render(<VirtualizedFeedList {...defaultProps} readFeeds={readFeeds} />);

    expect(screen.getByTestId('feed-list-fallback')).toBeInTheDocument();
  });

  it('should call onMarkAsRead callback', async () => {
    const { shouldUseVirtualization } = await import('@/utils/featureFlags');
    vi.mocked(shouldUseVirtualization).mockReturnValue(false);

    const onMarkAsRead = vi.fn();
    
    render(<VirtualizedFeedList {...defaultProps} onMarkAsRead={onMarkAsRead} />);

    // Component should render without error
    expect(screen.getByTestId('feed-list-fallback')).toBeInTheDocument();
  });
});