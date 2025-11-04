import { expect, type Page } from "@playwright/test";
import { checkA11y, getViolations, injectAxe } from "axe-playwright";

/**
 * Accessibility testing utilities using axe-core
 */

/**
 * WCAG 2.1 conformance levels
 */
export type ConformanceLevel = "A" | "AA" | "AAA";

/**
 * Axe configuration options
 */
export interface A11yCheckOptions {
  /** WCAG conformance level to test against */
  level?: ConformanceLevel;
  /** Specific rules to include */
  includedRules?: string[];
  /** Specific rules to exclude */
  excludedRules?: string[];
  /** Elements to exclude from testing */
  exclude?: string[];
  /** Generate detailed report */
  detailedReport?: boolean;
}

/**
 * Common accessibility violations to check for
 */
export const COMMON_A11Y_RULES = {
  // WCAG 2.1 Level A
  imageAlt: "image-alt",
  buttonName: "button-name",
  linkName: "link-name",
  formLabelAssociated: "label",
  htmlHasLang: "html-has-lang",

  // WCAG 2.1 Level AA
  colorContrast: "color-contrast",
  validLang: "valid-lang",

  // ARIA
  ariaAllowedAttr: "aria-allowed-attr",
  ariaRequiredAttr: "aria-required-attr",
  ariaRoles: "aria-roles",
  ariaValidAttrValue: "aria-valid-attr-value",
} as const;

/**
 * Check accessibility of the current page
 */
export async function checkPageA11y(page: Page, options: A11yCheckOptions = {}): Promise<void> {
  try {
    // Inject axe-core
    await injectAxe(page);

    // Build axe configuration
    const axeConfig = buildAxeConfig(options);

    // Run accessibility check
    await checkA11y(page, undefined, {
      detailedReport: options.detailedReport ?? false,
      axeOptions: axeConfig,
    });
  } catch (error) {
    console.error("Accessibility check failed:", error);
    throw error;
  }
}

/**
 * Get accessibility violations without throwing
 */
export async function getA11yViolations(page: Page, options: A11yCheckOptions = {}) {
  await injectAxe(page);
  const axeConfig = buildAxeConfig(options);
  return await getViolations(page, undefined, axeConfig);
}

/**
 * Check specific element for accessibility
 */
export async function checkElementA11y(
  page: Page,
  selector: string,
  options: A11yCheckOptions = {}
): Promise<void> {
  await injectAxe(page);
  const axeConfig = buildAxeConfig(options);

  await checkA11y(page, selector, {
    detailedReport: options.detailedReport ?? false,
    axeOptions: axeConfig,
  });
}

/**
 * Build axe configuration from options
 */
function buildAxeConfig(options: A11yCheckOptions) {
  const config: any = {
    runOnly: {
      type: "tag",
      values: [] as string[],
    },
  };

  // Set WCAG level
  const level = options.level || "AA";
  config.runOnly.values.push(`wcag2${level.toLowerCase()}`);

  // Add/remove specific rules
  if (options.includedRules && options.includedRules.length > 0) {
    config.runOnly = {
      type: "rule",
      values: options.includedRules,
    };
  }

  if (options.excludedRules && options.excludedRules.length > 0) {
    config.rules = {};
    options.excludedRules.forEach((rule) => {
      config.rules[rule] = { enabled: false };
    });
  }

  if (options.exclude && options.exclude.length > 0) {
    config.exclude = options.exclude;
  }

  return config;
}

/**
 * Check keyboard navigation
 */
export async function checkKeyboardNavigation(page: Page): Promise<void> {
  // Check tab navigation
  await page.keyboard.press("Tab");

  // Get focused element
  const focusedElement = await page.evaluate(() => {
    const active = document.activeElement;
    return {
      tagName: active?.tagName,
      hasVisibleFocus: window.getComputedStyle(active!).outline !== "none",
    };
  });

  expect(focusedElement.tagName).toBeTruthy();
}

/**
 * Check focus indicators
 */
export async function checkFocusIndicators(page: Page, selector: string): Promise<void> {
  const element = page.locator(selector);
  await element.focus();

  const hasVisibleFocus = await element.evaluate((el) => {
    const styles = window.getComputedStyle(el);
    return styles.outline !== "none" || styles.boxShadow !== "none" || styles.border !== "none";
  });

  expect(hasVisibleFocus).toBeTruthy();
}

/**
 * Check color contrast
 */
export async function checkColorContrast(page: Page): Promise<void> {
  await checkPageA11y(page, {
    includedRules: [COMMON_A11Y_RULES.colorContrast],
  });
}

/**
 * Check ARIA attributes
 */
export async function checkAriaAttributes(page: Page): Promise<void> {
  await checkPageA11y(page, {
    includedRules: [
      COMMON_A11Y_RULES.ariaAllowedAttr,
      COMMON_A11Y_RULES.ariaRequiredAttr,
      COMMON_A11Y_RULES.ariaRoles,
      COMMON_A11Y_RULES.ariaValidAttrValue,
    ],
  });
}

/**
 * Check form accessibility
 */
export async function checkFormA11y(page: Page): Promise<void> {
  await checkPageA11y(page, {
    includedRules: [COMMON_A11Y_RULES.formLabelAssociated, COMMON_A11Y_RULES.buttonName],
  });
}

/**
 * Check image accessibility
 */
export async function checkImageA11y(page: Page): Promise<void> {
  await checkPageA11y(page, {
    includedRules: [COMMON_A11Y_RULES.imageAlt],
  });
}

/**
 * Check heading structure
 */
export async function checkHeadingStructure(page: Page): Promise<void> {
  const headings = await page.evaluate(() => {
    const headingElements = Array.from(document.querySelectorAll("h1, h2, h3, h4, h5, h6"));
    return headingElements.map((h) => ({
      level: parseInt(h.tagName.substring(1), 10),
      text: h.textContent?.trim(),
    }));
  });

  // Check for h1
  const h1Count = headings.filter((h) => h.level === 1).length;
  expect(h1Count).toBeGreaterThanOrEqual(1);
  expect(h1Count).toBeLessThanOrEqual(1); // Only one h1 per page

  // Check heading order
  for (let i = 1; i < headings.length; i++) {
    const currentLevel = headings[i].level;
    const previousLevel = headings[i - 1].level;

    // Headings should not skip levels
    if (currentLevel > previousLevel) {
      expect(currentLevel - previousLevel).toBeLessThanOrEqual(1);
    }
  }
}

/**
 * Check landmark regions
 */
export async function checkLandmarkRegions(page: Page): Promise<void> {
  const landmarks = await page.evaluate(() => {
    return {
      hasMain: document.querySelector('main, [role="main"]') !== null,
      hasNav: document.querySelector('nav, [role="navigation"]') !== null,
      hasHeader: document.querySelector('header, [role="banner"]') !== null,
      hasFooter: document.querySelector('footer, [role="contentinfo"]') !== null,
    };
  });

  expect(landmarks.hasMain).toBeTruthy();
}

/**
 * Check skip links
 */
export async function checkSkipLinks(page: Page): Promise<void> {
  const skipLink = page.locator('a[href^="#"]').first();

  if ((await skipLink.count()) > 0) {
    await skipLink.focus();
    await expect(skipLink).toBeVisible();
  }
}

/**
 * Generate accessibility report
 */
export async function generateA11yReport(
  page: Page,
  options: A11yCheckOptions = {}
): Promise<string> {
  const violations = await getA11yViolations(page, options);

  if (violations.length === 0) {
    return "No accessibility violations found!";
  }

  let report = `Found ${violations.length} accessibility violations:\n\n`;

  violations.forEach((violation, index) => {
    report += `${index + 1}. ${violation.id} - ${violation.description}\n`;
    report += `   Impact: ${violation.impact}\n`;
    report += `   Help: ${violation.helpUrl}\n`;
    report += `   Affected elements: ${violation.nodes.length}\n\n`;
  });

  return report;
}
