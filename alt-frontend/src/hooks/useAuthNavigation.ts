import { useCallback, useMemo } from "react";
import { useAuth } from "@/contexts/auth-context";

export interface NavigationItem {
  id: string;
  label: string;
  href: string;
  icon?: React.ReactNode;
  requireAuth?: boolean;
  requiredRole?: string;
  badge?: string | number;
  isActive?: boolean;
  isDisabled?: boolean;
  children?: NavigationItem[];
}

export interface NavigationState {
  items: NavigationItem[];
  filteredItems: NavigationItem[];
  publicItems: NavigationItem[];
  privateItems: NavigationItem[];
  canAccess: (item: NavigationItem) => boolean;
  navigate: (
    href: string,
    requireAuth?: boolean,
    requiredRole?: string,
  ) => void;
  getActiveItem: (pathname: string) => NavigationItem | null;
}

const baseNavigationItems: NavigationItem[] = [
  {
    id: "home",
    label: "ホーム",
    href: "/",
  },
  {
    id: "search",
    label: "検索",
    href: "/search",
  },
  {
    id: "feeds",
    label: "フィード",
    href: "/feeds",
    requireAuth: true,
  },
  {
    id: "articles",
    label: "記事",
    href: "/articles",
    requireAuth: true,
  },
  {
    id: "bookmarks",
    label: "ブックマーク",
    href: "/bookmarks",
    requireAuth: true,
  },
  {
    id: "notifications",
    label: "通知",
    href: "/notifications",
    requireAuth: true,
    badge: 0, // Will be updated dynamically
  },
  {
    id: "activity",
    label: "活動",
    href: "/activity",
    requireAuth: true,
  },
  {
    id: "settings",
    label: "設定",
    href: "/settings",
    requireAuth: true,
    children: [
      {
        id: "profile",
        label: "プロフィール",
        href: "/settings/profile",
        requireAuth: true,
      },
      {
        id: "preferences",
        label: "設定",
        href: "/settings/preferences",
        requireAuth: true,
      },
      {
        id: "security",
        label: "セキュリティ",
        href: "/settings/security",
        requireAuth: true,
      },
      {
        id: "admin",
        label: "管理",
        href: "/settings/admin",
        requireAuth: true,
        requiredRole: "admin",
      },
    ],
  },
];

export function useAuthNavigation(currentPathname?: string): NavigationState {
  const { user, isAuthenticated } = useAuth();

  const canAccess = useCallback(
    (item: NavigationItem): boolean => {
      // Check authentication requirement
      if (item.requireAuth && !isAuthenticated) {
        return false;
      }

      // Check role requirement
      if (item.requiredRole && (!user || user.role !== item.requiredRole)) {
        return false;
      }

      return true;
    },
    [isAuthenticated, user],
  );

  const navigate = useCallback(
    (href: string, requireAuth?: boolean, requiredRole?: string) => {
      // Check authentication requirement
      if (requireAuth && !isAuthenticated) {
        // Redirect to Kratos browser login with absolute return URL
        const abs =
          typeof window !== "undefined"
            ? new URL(href, window.location.origin).toString()
            : href;
        window.location.href = `/auth/login?return_to=${encodeURIComponent(abs)}`;
        return;
      }

      // Check role requirement
      if (requiredRole && (!user || user.role !== requiredRole)) {
        // Show access denied message or redirect
        console.warn(
          `Access denied. Required role: ${requiredRole}, User role: ${user?.role}`,
        );
        return;
      }

      // Navigate to the href
      window.location.href = href;
    },
    [isAuthenticated, user],
  );

  const getActiveItem = useCallback(
    (pathname: string): NavigationItem | null => {
      const findActiveItem = (
        items: NavigationItem[],
      ): NavigationItem | null => {
        for (const item of items) {
          // Exact match
          if (item.href === pathname) {
            return item;
          }

          // Check if current path starts with item href (for nested routes)
          if (pathname.startsWith(item.href) && item.href !== "/") {
            return item;
          }

          // Check children
          if (item.children) {
            const childMatch = findActiveItem(item.children);
            if (childMatch) {
              return item; // Return parent item for child matches
            }
          }
        }
        return null;
      };

      return findActiveItem(baseNavigationItems);
    },
    [],
  );

  const processItems = useCallback(
    (items: NavigationItem[]): NavigationItem[] => {
      return items.map((item) => {
        const processedItem: NavigationItem = {
          ...item,
          isActive: currentPathname
            ? getActiveItem(currentPathname)?.id === item.id
            : false,
          isDisabled: !canAccess(item),
        };

        // Process children if they exist
        if (item.children) {
          processedItem.children = processItems(item.children).filter((child) =>
            canAccess(child),
          );
        }

        return processedItem;
      });
    },
    [canAccess, getActiveItem, currentPathname],
  );

  const navigationState = useMemo((): NavigationState => {
    const processedItems = processItems(baseNavigationItems);
    const filteredItems = processedItems.filter((item) => canAccess(item));
    const publicItems = processedItems.filter((item) => !item.requireAuth);
    const privateItems = processedItems.filter(
      (item) => item.requireAuth && canAccess(item),
    );

    return {
      items: processedItems,
      filteredItems,
      publicItems,
      privateItems,
      canAccess,
      navigate,
      getActiveItem,
    };
  }, [processItems, canAccess, navigate, getActiveItem]);

  return navigationState;
}

// Hook for breadcrumb navigation
export function useBreadcrumb(currentPathname?: string) {
  const { getActiveItem } = useAuthNavigation(currentPathname);

  const getBreadcrumb = useCallback(
    (pathname: string): NavigationItem[] => {
      const pathSegments = pathname.split("/").filter(Boolean);
      const breadcrumb: NavigationItem[] = [];

      // Always start with home
      breadcrumb.push({
        id: "home",
        label: "ホーム",
        href: "/",
      });

      // Build breadcrumb from path segments
      let currentPath = "";
      for (const segment of pathSegments) {
        currentPath += `/${segment}`;
        const activeItem = getActiveItem(currentPath);

        if (activeItem && activeItem.id !== "home") {
          breadcrumb.push({
            ...activeItem,
            href: currentPath,
          });
        } else {
          // Create breadcrumb item for unknown segments
          breadcrumb.push({
            id: segment,
            label: segment.charAt(0).toUpperCase() + segment.slice(1),
            href: currentPath,
          });
        }
      }

      return breadcrumb;
    },
    [getActiveItem],
  );

  return {
    getBreadcrumb,
    breadcrumb: currentPathname ? getBreadcrumb(currentPathname) : [],
  };
}

// Hook for navigation context (for mobile drawer, etc.)
export function useNavigationContext() {
  const { user, isAuthenticated, logout } = useAuth();

  const getNavigationContext = useCallback(() => {
    return {
      user,
      isAuthenticated,
      userDisplayName: user?.name || user?.email || "ユーザー",
      userRole: user?.role,
      userInitials: user?.name
        ? user.name
            .split(" ")
            .map((word) => word[0])
            .join("")
            .toUpperCase()
            .slice(0, 2)
        : user?.email?.[0].toUpperCase() || "U",
    };
  }, [user, isAuthenticated]);

  const handleLogout = useCallback(async () => {
    try {
      await logout();
      window.location.href = "/";
    } catch (error) {
      console.error("Logout failed:", error);
    }
  }, [logout]);

  return {
    ...getNavigationContext(),
    logout: handleLogout,
  };
}

// Hook for handling navigation notifications/badges
export function useNavigationBadges() {
  // This would typically fetch notification counts from an API
  // For now, we'll return static data
  const badges = useMemo(
    () => ({
      notifications: 3,
      messages: 0,
      updates: 1,
    }),
    [],
  );

  const updateBadge = useCallback((itemId: string, count: number) => {
    // Update badge count for specific navigation item
    // This would typically update global state or send to API
    console.log(`Updating badge for ${itemId}: ${count}`);
  }, []);

  const clearBadge = useCallback(
    (itemId: string) => {
      updateBadge(itemId, 0);
    },
    [updateBadge],
  );

  return {
    badges,
    updateBadge,
    clearBadge,
  };
}
