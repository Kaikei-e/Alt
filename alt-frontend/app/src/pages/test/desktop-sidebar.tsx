import { DesktopSidebar } from '../../components/desktop/layout/DesktopSidebar';
import { Home, Rss, BarChart3, Settings } from 'lucide-react';

export default function DesktopSidebarTest() {
  const mockNavItems = [
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
    },
    {
      id: 3,
      label: 'Statistics',
      icon: BarChart3,
      href: '/desktop/stats',
      active: false
    },
    {
      id: 4,
      label: 'Settings',
      icon: Settings,
      href: '/desktop/settings',
      active: false
    }
  ];

  return (
    <div className="h-screen w-64 bg-gray-50">
      <DesktopSidebar navItems={mockNavItems} />
    </div>
  );
}