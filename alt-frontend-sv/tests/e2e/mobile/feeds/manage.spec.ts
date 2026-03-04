import { expect, test } from "../../fixtures/pomFixtures";

test.describe("mobile feeds routes - manage", () => {
	test("manage page can open the add feed form", async ({
		mobileManagePage,
	}) => {
		await mobileManagePage.goto();

		await expect(mobileManagePage.pageTitle).toBeVisible();

		await mobileManagePage.addFeedButton.click();
		await expect(mobileManagePage.feedUrlInput).toBeVisible();

		await mobileManagePage.submitButton.click();
		await expect(mobileManagePage.validationError).toBeVisible();
	});
});
