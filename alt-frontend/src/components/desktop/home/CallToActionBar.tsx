import React from "react";
import Link from "next/link.js";

interface CallToActionBarProps {
  title: string;
  description: string;
  actions: Array<{
    label: string;
    href: string;
    icon?: React.ComponentType<{ size?: number }>;
    variant?: "primary" | "secondary";
  }>;
}

export const CallToActionBar: React.FC<CallToActionBarProps> = ({
  title,
  description,
  actions,
}) => {
  return (
    <div className="glass p-6 rounded-xl" data-testid="cta-container">
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        {/* Text content */}
        <div className="flex-1">
          <h2 className="text-xl font-bold text-gray-900 mb-2">{title}</h2>
          <p className="text-gray-600">{description}</p>
        </div>

        {/* Action buttons */}
        <div className="flex gap-3">
          {actions.map((action, index) => (
            <Link
              key={index}
              href={action.href}
              className={`inline-flex items-center gap-2 px-4 py-2 rounded-lg transition-colors ${
                action.variant === "primary" || !action.variant
                  ? "btn-primary"
                  : "btn-secondary"
              }`}
            >
              {action.icon && <action.icon size={16} />}
              {action.label}
            </Link>
          ))}
        </div>
      </div>
    </div>
  );
};
