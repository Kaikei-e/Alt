import React from 'react';
import { render, screen } from '@testing-library/react';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import { TrendingTopics } from '@/components/desktop/analytics/TrendingTopics';
import { mockTrendingTopics } from '@/data/mockAnalyticsData';
import { describe, it, expect } from 'vitest';

const renderWithChakra = (ui: React.ReactElement) => {
  return render(
    <ChakraProvider value={defaultSystem}>
      {ui}
    </ChakraProvider>
  );
};

describe('TrendingTopics', () => {
  it('should display trending topics correctly', () => {
    renderWithChakra(
      <TrendingTopics topics={mockTrendingTopics} isLoading={false} />
    );

    expect(screen.getByText('#AI')).toBeInTheDocument();
    expect(screen.getByText('#React')).toBeInTheDocument();
    expect(screen.getByText('45 articles')).toBeInTheDocument();
  });

  it('should show glass effect styling', () => {
    renderWithChakra(
      <TrendingTopics topics={mockTrendingTopics} isLoading={false} />
    );

    const glassElements = document.querySelectorAll('.glass');
    expect(glassElements.length).toBeGreaterThan(0);
  });

  it('should display trend indicators correctly', () => {
    renderWithChakra(
      <TrendingTopics topics={mockTrendingTopics} isLoading={false} />
    );

    expect(screen.getByText('+23%')).toBeInTheDocument(); // AI trend
    expect(screen.getByText('+12%')).toBeInTheDocument(); // React trend
  });

  it('should show loading state', () => {
    renderWithChakra(
      <TrendingTopics topics={[]} isLoading={true} />
    );

    // Chakra Spinner doesn't have progressbar role by default
    const spinner = document.querySelector('.chakra-spinner');
    expect(spinner).toBeInTheDocument();
  });

  it('should limit displayed topics to 6', () => {
    const manyTopics = Array.from({ length: 10 }, (_, i) => ({
      ...mockTrendingTopics[0],
      id: `topic-${i}`,
      tag: `Topic${i}`
    }));

    renderWithChakra(
      <TrendingTopics topics={manyTopics} isLoading={false} />
    );

    // Should only show first 6 topics
    expect(screen.getByText('#Topic0')).toBeInTheDocument();
    expect(screen.getByText('#Topic5')).toBeInTheDocument();
    expect(screen.queryByText('#Topic6')).not.toBeInTheDocument();
  });
});