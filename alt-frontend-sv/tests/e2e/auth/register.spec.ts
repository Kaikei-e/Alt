import { expect, test } from "../fixtures/pomFixtures";

// Auth tests need to run without pre-authenticated storage
test.use({ storageState: { cookies: [], origins: [] } });

test.describe("Register Page", () => {
	test("renders registration form with title", async ({
		page,
		registerPage,
	}) => {
		await page.goto("./register");
		await registerPage.waitForFormReady();

		await expect(registerPage.cardTitle).toBeVisible();
		await expect(registerPage.cardTitle).toContainText("Register");
	});

	test("has email and password fields", async ({ page, registerPage }) => {
		await page.goto("./register");
		await registerPage.waitForFormReady();

		await expect(registerPage.emailInput).toBeVisible();
		await expect(registerPage.passwordInput).toBeVisible();
	});

	test("has submit button", async ({ page, registerPage }) => {
		await page.goto("./register");
		await registerPage.waitForFormReady();

		await expect(registerPage.submitButton).toBeVisible();
	});

	test("has link to login page", async ({ page, registerPage }) => {
		await page.goto("./register");
		await registerPage.waitForFormReady();

		await expect(registerPage.loginLink).toBeVisible();
		await expect(registerPage.loginLink).toContainText("Login");
	});

	test("navigates to login page", async ({ page, registerPage }) => {
		await page.goto("./register");
		await registerPage.waitForFormReady();

		await registerPage.loginLink.click();
		await expect(page).toHaveURL(/\/login/);
	});

	test("can fill registration form", async ({ page, registerPage }) => {
		await page.goto("./register");
		await registerPage.waitForFormReady();

		await registerPage.emailInput.fill("newuser@example.com");
		await registerPage.passwordInput.fill("SecurePassword123!");

		await expect(registerPage.emailInput).toHaveValue("newuser@example.com");
	});
});

test.describe("Register Page - Validation", () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test("shows error for existing email", async ({ page, registerPage }) => {
		await page.goto("./register?flow=error-flow-id");
		await registerPage.waitForFormReady();

		await expect(registerPage.errorMessage.first()).toBeVisible();
		await expect(registerPage.errorMessage.first()).toContainText(
			"Email already exists",
		);
	});
});

test.describe("Register Page - Accessibility", () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test("form inputs have labels", async ({ page, registerPage }) => {
		await page.goto("./register");
		await registerPage.waitForFormReady();

		const emailLabel = page.getByText("Email", { exact: true });
		const passwordLabel = page.getByText("Password", { exact: true });

		await expect(emailLabel).toBeVisible();
		await expect(passwordLabel).toBeVisible();
	});
});
