import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../base.page";

/**
 * Desktop Settings Page Object
 * Represents the /desktop/settings page
 */
export class DesktopSettingsPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly profileSection: Locator;
  readonly appearanceSection: Locator;
  readonly notificationsSection: Locator;
  readonly privacySection: Locator;

  // Profile settings
  readonly nameInput: Locator;
  readonly emailInput: Locator;
  readonly bioInput: Locator;
  readonly avatarUpload: Locator;
  readonly saveProfileButton: Locator;

  // Appearance settings
  readonly themeSelect: Locator;
  readonly fontSizeSelect: Locator;
  readonly darkModeToggle: Locator;

  // Notification settings
  readonly emailNotificationsToggle: Locator;
  readonly pushNotificationsToggle: Locator;

  // Privacy settings
  readonly publicProfileToggle: Locator;
  readonly analyticsToggle: Locator;

  readonly successMessage: Locator;
  readonly errorMessage: Locator;
  readonly sidebar: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.pageHeading = page.getByRole("heading", { name: /settings/i });
    this.profileSection = page.getByRole("region", { name: /profile/i });
    this.appearanceSection = page.getByRole("region", { name: /appearance/i });
    this.notificationsSection = page.getByRole("region", {
      name: /notifications/i,
    });
    this.privacySection = page.getByRole("region", { name: /privacy/i });

    // Profile
    this.nameInput = page.getByLabel(/name|full name/i);
    this.emailInput = page.getByLabel(/email/i);
    this.bioInput = page.getByLabel(/bio|about/i);
    this.avatarUpload = page.getByLabel(/avatar|photo|image/i);
    this.saveProfileButton = page.getByRole("button", {
      name: /save|update.*profile/i,
    });

    // Appearance
    this.themeSelect = page.getByLabel(/theme/i);
    this.fontSizeSelect = page.getByLabel(/font.*size/i);
    this.darkModeToggle = page.getByRole("switch", { name: /dark mode/i });

    // Notifications
    this.emailNotificationsToggle = page.getByRole("switch", {
      name: /email.*notification/i,
    });
    this.pushNotificationsToggle = page.getByRole("switch", {
      name: /push.*notification/i,
    });

    // Privacy
    this.publicProfileToggle = page.getByRole("switch", {
      name: /public.*profile/i,
    });
    this.analyticsToggle = page.getByRole("switch", { name: /analytics/i });

    this.successMessage = page.getByRole("status");
    this.errorMessage = page.getByRole("alert");
    this.sidebar = page.getByRole("navigation", { name: /sidebar/i });
  }

  /**
   * Navigate to settings page
   */
  async goto(): Promise<void> {
    await this.page.goto("/desktop/settings");
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
    await this.waitForNetworkIdle();
  }

  /**
   * Update profile name
   */
  async updateName(name: string): Promise<void> {
    await this.nameInput.fill(name);
    await this.saveProfileButton.click();
    await this.waitForLoadingToComplete();
  }

  /**
   * Update profile email
   */
  async updateEmail(email: string): Promise<void> {
    await this.emailInput.fill(email);
    await this.saveProfileButton.click();
    await this.waitForLoadingToComplete();
  }

  /**
   * Update bio
   */
  async updateBio(bio: string): Promise<void> {
    if ((await this.bioInput.count()) > 0) {
      await this.bioInput.fill(bio);
      await this.saveProfileButton.click();
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Upload avatar
   */
  async uploadAvatar(filePath: string): Promise<void> {
    if ((await this.avatarUpload.count()) > 0) {
      await this.avatarUpload.setInputFiles(filePath);
      await this.saveProfileButton.click();
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Change theme
   */
  async changeTheme(theme: string): Promise<void> {
    if ((await this.themeSelect.count()) > 0) {
      await this.themeSelect.selectOption(theme);
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Change font size
   */
  async changeFontSize(size: string): Promise<void> {
    if ((await this.fontSizeSelect.count()) > 0) {
      await this.fontSizeSelect.selectOption(size);
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Toggle dark mode
   */
  async toggleDarkMode(): Promise<void> {
    if ((await this.darkModeToggle.count()) > 0) {
      await this.darkModeToggle.click();
    }
  }

  /**
   * Toggle email notifications
   */
  async toggleEmailNotifications(): Promise<void> {
    if ((await this.emailNotificationsToggle.count()) > 0) {
      await this.emailNotificationsToggle.click();
    }
  }

  /**
   * Toggle push notifications
   */
  async togglePushNotifications(): Promise<void> {
    if ((await this.pushNotificationsToggle.count()) > 0) {
      await this.pushNotificationsToggle.click();
    }
  }

  /**
   * Toggle public profile
   */
  async togglePublicProfile(): Promise<void> {
    if ((await this.publicProfileToggle.count()) > 0) {
      await this.publicProfileToggle.click();
    }
  }

  /**
   * Toggle analytics
   */
  async toggleAnalytics(): Promise<void> {
    if ((await this.analyticsToggle.count()) > 0) {
      await this.analyticsToggle.click();
    }
  }

  /**
   * Get success message
   */
  async getSuccessMessage(): Promise<string | null> {
    try {
      await expect(this.successMessage).toBeVisible({ timeout: 5000 });
      return await this.successMessage.textContent();
    } catch {
      return null;
    }
  }

  /**
   * Get error message
   */
  async getError(): Promise<string | null> {
    try {
      await expect(this.errorMessage).toBeVisible({ timeout: 5000 });
      return await this.errorMessage.textContent();
    } catch {
      return null;
    }
  }

  /**
   * Navigate to section
   */
  async goToSection(
    section: "profile" | "appearance" | "notifications" | "privacy"
  ): Promise<void> {
    const sectionLink = this.page.getByRole("link", {
      name: new RegExp(section, "i"),
    });

    if ((await sectionLink.count()) > 0) {
      await sectionLink.click();
    }
  }

  /**
   * Check if section is visible
   */
  async isSectionVisible(
    section: "profile" | "appearance" | "notifications" | "privacy"
  ): Promise<boolean> {
    const sectionLocator = {
      profile: this.profileSection,
      appearance: this.appearanceSection,
      notifications: this.notificationsSection,
      privacy: this.privacySection,
    }[section];

    try {
      await expect(sectionLocator).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Save all settings
   */
  async saveSettings(): Promise<void> {
    const saveAllButton = this.page.getByRole("button", {
      name: /save|apply.*changes/i,
    });

    if ((await saveAllButton.count()) > 0) {
      await saveAllButton.click();
      await this.waitForLoadingToComplete();
    }
  }
}
