import { test, expect } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";

test.describe("mobile feeds routes - manage", () => {
  test("manage page can open the add feed form", async ({ page }) => {
    await gotoMobileRoute(page, "feeds/manage");

    await expect(page.getByText("Feed Management")).toBeVisible();

    await page.getByRole("button", { name: "Add a new feed" }).click();
    await expect(
      page.getByPlaceholder("https://example.com/feed.xml"),
    ).toBeVisible();

    await page.getByRole("button", { name: "Add feed" }).click();
    await expect(page.getByText("Please enter the RSS URL.")).toBeVisible();
  });
});
