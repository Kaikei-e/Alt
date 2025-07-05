import { DesktopLayout } from '../../components/desktop/layout/DesktopLayout';
import { PageHeader } from '../../components/desktop/home/PageHeader';
import { CallToActionBar } from '../../components/desktop/home/CallToActionBar';
import { ThemeProvider } from '../../providers/ThemeProvider';
import { Home, Rss, BarChart3, Settings, ArrowRight, Download } from 'lucide-react';

export default function DesktopHomePageSimpleTest() {
  const sidebarProps = {
    navItems: [
      {
        id: 1,
        label: 'Dashboard',
        icon: Home,
        href: '/desktop',
        active: true
      },
      {
        id: 2,
        label: 'Feeds',
        icon: Rss,
        href: '/desktop/feeds',
        active: false
      }
    ]
  };

  const ctaActions = [
    {
      label: "Browse Feeds",
      href: "/desktop/feeds",
      icon: ArrowRight
    },
    {
      label: "Add New Feed",
      href: "/desktop/feeds/register",
      icon: Download
    }
  ];

  return (
    <ThemeProvider>
      <DesktopLayout sidebarProps={sidebarProps}>
        <div 
          className="flex flex-col gap-8"
          data-testid="desktop-home-container"
        >
          <PageHeader
            title="Dashboard Overview"
            description="Monitor your RSS feeds and AI-powered content insights"
          />
          
          {/* Mock StatsGrid */}
          <div data-testid="stats-grid" className="grid grid-cols-3 gap-6">
            <div className="glass p-4 rounded-lg">
              <div>Total Feeds: 24</div>
            </div>
            <div className="glass p-4 rounded-lg">
              <div>Unread: 156</div>
            </div>
            <div className="glass p-4 rounded-lg">
              <div>Reading Time: 45m</div>
            </div>
          </div>
          
          <div className="grid grid-cols-2 gap-8">
            <div>
              {/* Mock ActivityFeed */}
              <div data-testid="activity-feed" className="glass p-4 rounded-lg">
                <h3>Recent Activity</h3>
                <div>Added TechCrunch RSS feed</div>
              </div>
            </div>
            <div>
              {/* Mock QuickActionsPanel */}
              <div data-testid="quick-actions-panel" className="glass p-4 rounded-lg">
                <h3>Quick Actions</h3>
                <div>Add Feed, Search, Bookmarks</div>
              </div>
            </div>
          </div>
          
          <CallToActionBar
            title="Ready to explore?"
            description="Discover new content and manage your feeds"
            actions={ctaActions}
          />
        </div>
      </DesktopLayout>
    </ThemeProvider>
  );
}