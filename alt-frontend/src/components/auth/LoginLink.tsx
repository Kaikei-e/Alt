'use client'

interface LoginLinkProps {
  children: React.ReactNode
  returnTo?: string
  className?: string
}

export function LoginLink({ children, returnTo, className }: LoginLinkProps) {
  const href = `${process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL}/self-service/login/browser?return_to=${encodeURIComponent(returnTo || window.location.href)}`
  
  return (
    <a
      href={href}
      className={className}
      style={{
        color: 'var(--alt-primary)',
        textDecoration: 'underline'
      }}
    >
      {children}
    </a>
  )
}