"use client";
import { KRATOS_PUBLIC_URL } from "@/lib/env.public";

interface LoginLinkProps {
  children: React.ReactNode;
  returnTo?: string;
  className?: string;
}

export function LoginLink({ children, returnTo, className }: LoginLinkProps) {
  const href = `${KRATOS_PUBLIC_URL}/self-service/login/browser?return_to=${encodeURIComponent(returnTo || window.location.href)}`;

  return (
    <a
      href={href}
      className={className}
      style={{
        color: "var(--alt-primary)",
        textDecoration: "underline",
      }}
    >
      {children}
    </a>
  );
}
