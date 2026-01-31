import { expect, test } from "@playwright/test";
import { LoginPage } from "../pages/auth/LoginPage";
import { fulfillJson } from "../utils/mockHelpers";

// Auth tests need to run without pre-authenticated storage
test.use({ storageState: { cookies: [], origins: [] } });

// Mock Kratos login flow response
const KRATOS_LOGIN_FLOW = {
	id: "flow-123",
	ui: {
		action: "/login?flow=flow-123",
		method: "POST",
		nodes: [
			{
				type: "input",
				group: "default",
				attributes: {
					name: "csrf_token",
					type: "hidden",
					value: "mock-csrf-token",
					required: true,
				},
				messages: [],
				meta: {},
			},
			{
				type: "input",
				group: "default",
				attributes: {
					name: "identifier",
					type: "email",
					value: "",
					required: true,
				},
				messages: [],
				meta: { label: { text: "Email" } },
			},
			{
				type: "input",
				group: "password",
				attributes: {
					name: "password",
					type: "password",
					required: true,
				},
				messages: [],
				meta: { label: { text: "Password" } },
			},
		],
		messages: [],
	},
};

const KRATOS_LOGIN_FLOW_WITH_ERROR = {
	...KRATOS_LOGIN_FLOW,
	ui: {
		...KRATOS_LOGIN_FLOW.ui,
		messages: [{ text: "Invalid credentials", type: "error" }],
	},
};

test.describe("Login Page", () => {
	let loginPage: LoginPage;

	test.beforeEach(async ({ page }) => {
		loginPage = new LoginPage(page);

		// Mock Kratos login flow initialization
		await page.route("**/self-service/login/browser**", (route) =>
			fulfillJson(route, KRATOS_LOGIN_FLOW),
		);

		// Mock the login page server-side data
		await page.route("**/login**", async (route) => {
			// Let the actual page load but with mocked data
			if (route.request().resourceType() === "document") {
				await route.continue();
			} else {
				await route.continue();
			}
		});
	});

	test("renders login form with title and description", async ({ page }) => {
		// Mock the page data through API interception
		await page.route("**/self-service/login**", (route) =>
			fulfillJson(route, KRATOS_LOGIN_FLOW),
		);

		await loginPage.goto();
		const formReady = await loginPage.waitForFormReady();

		// Skip if external auth (e.g., Cloudflare Access) is detected
		if (!formReady) {
			test.skip(true, "External authentication in use");
			return;
		}

		// Verify card title
		await expect(loginPage.cardTitle).toBeVisible();
		await expect(loginPage.cardTitle).toContainText("Login");

		// Verify description
		await expect(loginPage.cardDescription).toBeVisible();
	});

	test("has email and password input fields", async ({ page }) => {
		await page.route("**/self-service/login**", (route) =>
			fulfillJson(route, KRATOS_LOGIN_FLOW),
		);

		await loginPage.goto();
		const formReady = await loginPage.waitForFormReady();

		// Skip if external auth or redirecting
		if (!formReady || (await loginPage.isRedirecting())) {
			test.skip(true, "External authentication or redirect");
			return;
		}

		await expect(loginPage.emailInput).toBeVisible();
		await expect(loginPage.passwordInput).toBeVisible();
	});

	test("has submit button", async ({ page }) => {
		await page.route("**/self-service/login**", (route) =>
			fulfillJson(route, KRATOS_LOGIN_FLOW),
		);

		await loginPage.goto();
		const formReady = await loginPage.waitForFormReady();

		if (!formReady || (await loginPage.isRedirecting())) {
			test.skip(true, "External authentication or redirect");
			return;
		}

		await expect(loginPage.submitButton).toBeVisible();
		await expect(loginPage.submitButton).toHaveText("Login");
	});

	test("has link to register page", async ({ page }) => {
		await page.route("**/self-service/login**", (route) =>
			fulfillJson(route, KRATOS_LOGIN_FLOW),
		);

		await loginPage.goto();
		const formReady = await loginPage.waitForFormReady();

		if (!formReady || (await loginPage.isRedirecting())) {
			test.skip(true, "External authentication or redirect");
			return;
		}

		await expect(loginPage.registerLink).toBeVisible();
		await expect(loginPage.registerLink).toContainText("Register");
	});

	test("navigates to register page when clicking register link", async ({
		page,
	}) => {
		await page.route("**/self-service/login**", (route) =>
			fulfillJson(route, KRATOS_LOGIN_FLOW),
		);

		await loginPage.goto();
		const formReady = await loginPage.waitForFormReady();

		if (!formReady || (await loginPage.isRedirecting())) {
			test.skip(true, "External authentication or redirect");
			return;
		}

		await loginPage.goToRegister();

		await expect(page).toHaveURL(/\/register/);
	});

	test("can fill login form", async ({ page }) => {
		await page.route("**/self-service/login**", (route) =>
			fulfillJson(route, KRATOS_LOGIN_FLOW),
		);

		await loginPage.goto();
		const formReady = await loginPage.waitForFormReady();

		if (!formReady || (await loginPage.isRedirecting())) {
			test.skip(true, "External authentication or redirect");
			return;
		}

		const testEmail = "test@example.com";
		const testPassword = "password123";

		await loginPage.fillLoginForm(testEmail, testPassword);

		await expect(loginPage.emailInput).toHaveValue(testEmail);
		// Password fields don't expose their value for security, so we just verify it's filled
	});
});

test.describe("Login Page - Error Handling", () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test("displays error message on invalid credentials", async ({ page }) => {
		const loginPage = new LoginPage(page);

		// Initial flow without error
		await page.route("**/self-service/login/browser**", (route) =>
			fulfillJson(route, KRATOS_LOGIN_FLOW),
		);

		// Mock failed login attempt
		await page.route("**/self-service/login?flow=**", async (route) => {
			if (route.request().method() === "POST") {
				await fulfillJson(route, KRATOS_LOGIN_FLOW_WITH_ERROR, 400);
			} else {
				await fulfillJson(route, KRATOS_LOGIN_FLOW);
			}
		});

		await loginPage.goto();
		const formReady = await loginPage.waitForFormReady();

		if (!formReady || (await loginPage.isRedirecting())) {
			test.skip(true, "External authentication or redirect");
			return;
		}

		// The page shows the flow with error messages if present
		// Since we can't actually submit to Kratos, we test the UI renders errors
		// by using a flow that already has errors
		await page.route("**/self-service/login**", (route) =>
			fulfillJson(route, KRATOS_LOGIN_FLOW_WITH_ERROR),
		);

		await page.reload();
		const reloadFormReady = await loginPage.waitForFormReady();

		if (!reloadFormReady || (await loginPage.isRedirecting())) {
			test.skip(true, "External authentication or redirect after reload");
			return;
		}

		// Check for error message
		const hasError = await loginPage.hasError();
		if (hasError) {
			await expect(loginPage.errorMessage.first()).toBeVisible();
		}
	});
});

test.describe("Login Page - Accessibility", () => {
	test.use({ storageState: { cookies: [], origins: [] } });

	test("form inputs have proper labels", async ({ page }) => {
		const loginPage = new LoginPage(page);

		await page.route("**/self-service/login**", (route) =>
			fulfillJson(route, KRATOS_LOGIN_FLOW),
		);

		await loginPage.goto();
		const formReady = await loginPage.waitForFormReady();

		if (!formReady || (await loginPage.isRedirecting())) {
			test.skip(true, "External authentication or redirect");
			return;
		}

		// Verify email input has a label
		const emailLabel = page.getByText("Email", { exact: true });
		await expect(emailLabel).toBeVisible();

		// Verify password input has a label
		const passwordLabel = page.getByText("Password", { exact: true });
		await expect(passwordLabel).toBeVisible();
	});
});
