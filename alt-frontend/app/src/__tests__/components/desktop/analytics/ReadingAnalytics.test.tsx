import { render, screen } from '@testing-library/react';
import { ChakraProvider } from '@chakra-ui/react';
import { ReadingAnalytics } from '@/components/desktop/analytics/ReadingAnalytics';
import { mockAnalytics } from '@/data/mockAnalyticsData';

const renderWithChakra = (ui: React.ReactElement) => {
  return render(
    <ChakraProvider>
      {ui}
    </ChakraProvider>
  );
};

describe('ReadingAnalytics', () => {
  it('should display today stats correctly', () => {
    renderWithChakra(
      <ReadingAnalytics analytics={mockAnalytics} isLoading={false} />
    );
    
    expect(screen.getByText('12')).toBeInTheDocument(); // articles read
    expect(screen.getByText('45m')).toBeInTheDocument(); // time spent
    expect(screen.getByText('3')).toBeInTheDocument(); // favorites
  });

  it('should show glass effect styling', () => {
    renderWithChakra(
      <ReadingAnalytics analytics={mockAnalytics} isLoading={false} />
    );
    
    const glassElements = document.querySelectorAll('.glass');
    expect(glassElements.length).toBeGreaterThan(0);
  });

  it('should use CSS variables for colors', () => {
    renderWithChakra(
      <ReadingAnalytics analytics={mockAnalytics} isLoading={false} />
    );
    
    const primaryElement = screen.getByText('12');
    const styles = window.getComputedStyle(primaryElement);
    expect(styles.color).toContain('var(');
  });

  it('should show loading state', () => {
    renderWithChakra(
      <ReadingAnalytics analytics={null} isLoading={true} />
    );
    
    expect(screen.getByRole('progressbar')).toBeInTheDocument(); // Chakra Spinner
  });

  it('should show no data message when analytics is null', () => {
    renderWithChakra(
      <ReadingAnalytics analytics={null} isLoading={false} />
    );
    
    expect(screen.getByText('データがありません')).toBeInTheDocument();
  });
});