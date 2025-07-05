import { PageHeader } from '@/components/desktop/home/PageHeader';

export default function PageHeaderTest() {
  return (
    <div className="p-8">
      <PageHeader
        title="Dashboard Overview"
        description="Monitor your RSS feeds and AI-powered content insights"
      />
    </div>
  );
}