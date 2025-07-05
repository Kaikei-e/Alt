import React from 'react';
import Link from 'next/link';

interface DesktopSidebarProps {
  navItems: Array<{
    id: number;
    label: string;
    icon: React.ComponentType<{ size?: number }>;
    href: string;
    active?: boolean;
  }>;
  logoText?: string;
  logoSubtext?: string;
}

export const DesktopSidebar: React.FC<DesktopSidebarProps> = ({
  navItems,
  logoText = 'Alt RSS',
  logoSubtext = 'Feed Reader',
}) => {
  return (
    <div className="glass h-full flex flex-col p-6">
      {/* Logo Section */}
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-gray-900 mb-1">
          {logoText}
        </h1>
        <p className="text-sm text-gray-600">
          {logoSubtext}
        </p>
      </div>

      {/* Navigation */}
      <nav className="flex-1" aria-label="Main navigation">
        <ul className="space-y-2">
          {navItems.map((item) => (
            <li key={item.id}>
              <Link
                href={item.href}
                className={`flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${
                  item.active
                    ? 'bg-purple-100 text-purple-700 active'
                    : 'text-gray-700 hover:bg-gray-100'
                }`}
              >
                <item.icon size={20} />
                <span className="font-medium">{item.label}</span>
              </Link>
            </li>
          ))}
        </ul>
      </nav>
    </div>
  );
};