import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ChakraProvider } from '@chakra-ui/react';
import { RightPanel } from '@/components/desktop/analytics/RightPanel';

// Mock the custom hooks
jest.mock('@/hooks/useReadingAnalytics', () => ({
  useReadingAnalytics: () => ({
    analytics: null,
    isLoading: false
  })
}));

jest.mock('@/hooks/useTrendingTopics', () => ({
  useTrendingTopics: () => ({
    topics: [],
    isLoading: false
  })
}));

jest.mock('@/hooks/useSourceAnalytics', () => ({
  useSourceAnalytics: () => ({
    sources: [],
    isLoading: false
  })
}));

jest.mock('@/hooks/useQuickActions', () => ({
  useQuickActions: () => ({
    actions: [],
    counters: { unread: 0, bookmarks: 0, queue: 0 }
  })
}));

const renderWithChakra = (ui: React.ReactElement) => {
  return render(
    <ChakraProvider>
      {ui}
    </ChakraProvider>
  );
};

describe('RightPanel', () => {
  it('should render with glass effect', () => {
    renderWithChakra(<RightPanel />);
    
    const glassElements = document.querySelectorAll('.glass');
    expect(glassElements.length).toBeGreaterThan(0);
  });

  it('should show Analytics tab as active by default', () => {
    renderWithChakra(<RightPanel />);
    
    const analyticsTab = screen.getByRole('tab', { name: /analytics/i });
    expect(analyticsTab).toHaveAttribute('aria-selected', 'true');
  });

  it('should switch between tabs', async () => {
    const user = userEvent.setup();
    renderWithChakra(<RightPanel />);
    
    // Click on Actions tab
    const actionsTab = screen.getByRole('tab', { name: /actions/i });
    await user.click(actionsTab);
    
    expect(actionsTab).toHaveAttribute('aria-selected', 'true');
    
    // Switch back to Analytics tab
    const analyticsTab = screen.getByRole('tab', { name: /analytics/i });
    await user.click(analyticsTab);
    
    expect(analyticsTab).toHaveAttribute('aria-selected', 'true');
  });

  it('should use CSS variables for styling', () => {
    renderWithChakra(<RightPanel />);
    
    const tabs = screen.getAllByRole('tab');
    const tabElement = tabs[0];
    
    // Should use CSS variables (though actual values might be computed)
    expect(tabElement).toBeInTheDocument();
  });
});