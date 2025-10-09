# DEPRECATED: /e2e/page-objects/

## Status: Deprecated (2025-10-09)

This directory structure is **deprecated** and being phased out in favor of the unified `/tests/pages/` structure.

## Migration Path

All new E2E tests should use the Page Object Models from `/tests/pages/` instead of `/e2e/page-objects/`.

### Why the change?

1. **Single Source of Truth**: Having two POM structures caused confusion and maintenance issues
2. **Simpler Design**: `/tests/pages/` uses a more flexible design with `goto(url: string)` instead of abstract methods
3. **Already in Use**: Multiple tests already use `/tests/pages/` successfully
4. **Existing Fixes**: Timeout adjustments and URL-based validation are already applied to `/tests/pages/`

### Migration Guide

**Before** (using `/e2e/page-objects/`):
```typescript
import { LoginPage } from '../../page-objects/auth/login.page';
import { DesktopFeedsPage } from '../../page-objects/desktop/feeds.page';

const loginPage = new LoginPage(page);
await loginPage.goto();
```

**After** (using `/tests/pages/`):
```typescript
import { LoginPage, DesktopFeedsPage } from '../../../tests/pages';

const loginPage = new LoginPage(page);
await loginPage.goto('/auth/login');
```

### Available Page Objects in `/tests/pages/`

- `BasePage` - Base class with common functionality
- `LoginPage` - Login page interactions
- `DesktopPage` - Desktop navigation and common elements
- `HomePage` - Desktop home page
- `DesktopFeedsPage` - Feeds management
- `ArticlesPage` - Articles display and interaction

### Good Features to Preserve

The following features from `/e2e/page-objects/` should be gradually migrated to `/tests/pages/`:

1. **Accessibility checking**: `checkA11y()` method from `base.page.ts`
2. **Detailed locator patterns**: Well-defined locators with fallbacks
3. **Error handling patterns**: Graceful handling of missing elements

### Timeline

- **Now**: New tests must use `/tests/pages/`
- **Phase 1**: Existing tests gradually migrated
- **Phase 2**: `/e2e/page-objects/` marked as deprecated (current)
- **Future**: `/e2e/page-objects/` removed once all tests migrated

### Questions?

Refer to the unified POM strategy in `/memo.md` or consult the E2E test plan documentation.
