import { render, screen } from '@testing-library/react';
import { ChakraProvider } from '@chakra-ui/react';
import { TrendingTopics } from '@/components/desktop/analytics/TrendingTopics';
import { mockTrendingTopics } from '@/data/mockAnalyticsData';

const renderWithChakra = (ui: React.ReactElement) => {
  return render(
    <ChakraProvider>
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
    
    expect(screen.getByRole('progressbar')).toBeInTheDocument(); // Chakra Spinner
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