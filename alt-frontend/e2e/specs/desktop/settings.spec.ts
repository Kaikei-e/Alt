import { test, expect } from "@playwright/test";
import { SettingsPage } from "../../pages/desktop/SettingsPage";
import { setupAllMocks } from "../../utils/api-mock";

test.describe("Desktop Settings", () => {
  let settingsPage: SettingsPage;

  test.beforeEach(async ({ page }) => {
    settingsPage = new SettingsPage(page);
    await setupAllMocks(page);

    // Mock profile API
    await page.route("**/api/user/profile", async (route) => {
      if (route.request().method() === "PUT") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ success: true }),
        });
      } else {
        await route.continue();
      }
    });
  });

  test("should display settings page", async () => {
    await settingsPage.goto();
    await settingsPage.waitForReady();

    await expect(settingsPage.settingsHeading).toBeVisible();
    await expect(settingsPage.settingsHeading).toHaveText("Settings");
  });

  test("should display settings form with name input", async () => {
    await settingsPage.goto();
    await settingsPage.waitForReady();

    await expect(settingsPage.settingsForm).toBeVisible();
    await expect(settingsPage.nameInput).toBeVisible();
    await expect(settingsPage.saveButton).toBeVisible();
  });

  test("should update name and show success message", async () => {
    await settingsPage.goto();
    await settingsPage.waitForReady();

    await settingsPage.updateName("New Name");
    await settingsPage.saveChanges();

    const hasSuccess = await settingsPage.hasSuccessMessage();
    expect(hasSuccess).toBe(true);
  });

  test("should have editable name field", async () => {
    await settingsPage.goto();
    await settingsPage.waitForReady();

    const initialValue = await settingsPage.getNameValue();
    expect(initialValue.length).toBeGreaterThan(0);

    await settingsPage.updateName("Test Name");
    const newValue = await settingsPage.getNameValue();
    expect(newValue).toBe("Test Name");
  });
});
