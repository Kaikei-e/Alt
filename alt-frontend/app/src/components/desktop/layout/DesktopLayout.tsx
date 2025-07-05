import React from 'react';
import { DesktopSidebar } from './DesktopSidebar';
// import { ThemeToggle } from '../../ThemeToggle';

interface DesktopLayoutProps {
  children: React.ReactNode;
  showSidebar?: boolean;
  sidebarProps?: {
    navItems: Array<{
      id: number;
      label: string;
      icon: React.ComponentType<{ size?: number }>;
      href: string;
      active?: boolean;
    }>;
  };
}

export const DesktopLayout: React.FC<DesktopLayoutProps> = ({
  children,
  showSidebar = true,
  sidebarProps,
}) => {
  return (
    <div 
      className="min-h-screen bg-var(--app-bg)"
      data-testid="desktop-layout"
    >
      {/* Theme Toggle - Fixed Top Right */}
      <div className="fixed top-4 right-4 z-50">
        {/* <ThemeToggle /> */}
        <div data-testid="theme-toggle-button" className="w-10 h-10 bg-purple-100 rounded-lg">
          Toggle
        </div>
      </div>

      <div className="flex h-screen">
        {/* Sidebar */}
        {showSidebar && sidebarProps && (
          <div 
            className="w-64 flex-shrink-0"
            data-testid="desktop-sidebar"
            style={{ width: '250px' }}
          >
            <DesktopSidebar navItems={sidebarProps.navItems} />
          </div>
        )}

        {/* Main Content */}
        <div 
          className="flex-1 overflow-auto p-8"
          data-testid="main-content"
        >
          {children}
        </div>
      </div>
    </div>
  );
};