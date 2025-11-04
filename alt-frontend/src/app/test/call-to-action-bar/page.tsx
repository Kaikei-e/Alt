"use client";

import { ArrowRight, Download } from "lucide-react";
import { CallToActionBar } from "@/components/desktop/home/CallToActionBar";
import { ThemeProvider } from "@/providers/ThemeProvider";

export default function CallToActionBarTest() {
  const ctaActions = [
    {
      label: "Browse Feeds",
      href: "/desktop/feeds",
      icon: ArrowRight,
    },
    {
      label: "Add New Feed",
      href: "/desktop/feeds/register",
      icon: Download,
    },
  ];

  return (
    <ThemeProvider>
      <div className="p-8">
        <CallToActionBar
          title="Ready to explore?"
          description="Discover new content and manage your feeds"
          actions={ctaActions}
        />
      </div>
    </ThemeProvider>
  );
}
