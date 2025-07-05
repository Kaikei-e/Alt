import { CallToActionBar } from '../../components/desktop/home/CallToActionBar';
import { ArrowRight, Download } from 'lucide-react';

export default function CallToActionBarTest() {
  const mockActions = [
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
    <div className="p-8">
      <CallToActionBar
        title="Ready to explore?"
        description="Discover new content and manage your feeds"
        actions={mockActions}
      />
    </div>
  );
}