import { expect, test } from "../fixtures/pomFixtures";
import { LoginPage } from "../pages/auth/LoginPage";

// Auth tests need to run without pre-authenticated storage
test.use({ storageState: { cookies: [], origins: [] } });

test.describe("Login Page", () => {
	let loginPage: LoginPage;

	test.beforeEach(async ({ page }) => {
		loginPage = new LoginPage(page);
	});

	test("renders login form with title and description", async () => {
		await loginPage.goto();
		await loginPage.waitForFormReady();

		await expect(loginPage.cardTitle).toBeVisible();
		await expect(loginPage.cardTitle).toContainText("Login");
		await expect(loginPage.cardDescription).toBeVisible();
	});

	test("has email and password input fields", async () => {
		await loginPage.goto();
		await loginPage.waitForFormReady();

		await expect(loginPage.emailInput).toBeVisible();
		await expect(loginPage.passwordInput).toBeVisible();
	});

	test("has submit button", async () => {
		await loginPage.goto();
		await loginPage.waitForFormReady();

		await expect(loginPage.submitButton).toBeVisible();
		await expect(loginPage.submitButton).toHaveText("Login");
	});

	test("has link to register page", async () => {
		await loginPage.goto();
		await loginPage.waitForFormReady();

		await expect(loginPage.registerLink).toBeVisible();
		await expect(loginPage.registerLink).toContainText("Register");
	});

	test("navigates to register page when clicking register link", async ({
		page,
	}) => {
		await loginPage.goto();
		await loginPage.waitForFormReady();

		await loginPage.goToRegister();

		await expect(page).toHaveURL(/\/register/);
	});

	test("can fill login form", async () => {
		await loginPage.goto();
		await loginPage.waitForFormReady();

		const testEmail = "test@example.com";
		const testPassword = "password123";

		await loginPage.fillLoginForm(testEmail, testPassword);

		await expect(loginPage.emailInput).toHaveValue(testEmail);
	});
});

test.describe("Login Page - Error Handling", () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test("displays error message on invalid credentials", async ({ page }) => {
		const loginPage = new LoginPage(page);

		await page.goto("./auth/login?flow=error-flow-id");
		await loginPage.waitForFormReady();

		await expect(loginPage.errorMessage.first()).toBeVisible();
		await expect(loginPage.errorMessage.first()).toContainText(
			"Invalid credentials",
		);
	});
});

test.describe("Login Page - Accessibility", () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test("form inputs have proper labels", async ({ page }) => {
		const loginPage = new LoginPage(page);

		await loginPage.goto();
		await loginPage.waitForFormReady();

		const emailLabel = page.getByText("Email", { exact: true });
		await expect(emailLabel).toBeVisible();

		const passwordLabel = page.getByText("Password", { exact: true });
		await expect(passwordLabel).toBeVisible();
	});
});
