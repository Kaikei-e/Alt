import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Register page (/register)
 */
export class RegisterPage extends BasePage {
	// Card elements
	readonly cardTitle: Locator;
	readonly cardDescription: Locator;

	// Form inputs
	readonly emailInput: Locator;
	readonly passwordInput: Locator;
	readonly submitButton: Locator;

	// Error messages
	readonly errorMessage: Locator;

	// Navigation
	readonly loginLink: Locator;

	// External auth detection
	readonly externalAuth: Locator;
	readonly redirectingText: Locator;

	constructor(page: Page) {
		super(page);

		this.cardTitle = page.getByRole("heading", { name: /register/i });
		this.cardDescription = page.getByText(/create a new account/i);

		this.emailInput = page.getByLabel(/email/i);
		this.passwordInput = page.getByLabel(/password/i);
		this.submitButton = page.getByRole("button", { name: /register/i });

		this.errorMessage = page.locator('[style*="color: #dc2626"]');

		this.loginLink = page.getByRole("link", { name: /login/i });

		this.externalAuth = page.getByText(/send me a code|cloudflare/i);
		this.redirectingText = page.getByText("Redirecting...");
	}

	get url(): string {
		return "./register";
	}

	/**
	 * Fill registration form.
	 */
	async fillForm(email: string, password: string): Promise<void> {
		await this.emailInput.fill(email);
		await this.passwordInput.fill(password);
	}

	/**
	 * Submit the registration form.
	 */
	async submit(): Promise<void> {
		await this.submitButton.click();
	}

	/**
	 * Navigate to login page.
	 */
	async goToLogin(): Promise<void> {
		await this.loginLink.click();
	}

	/**
	 * Wait for form to be ready. Returns false if external auth is detected.
	 */
	async waitForFormReady(): Promise<boolean> {
		try {
			await expect(
				this.emailInput.or(this.redirectingText).or(this.externalAuth).first(),
			).toBeVisible({ timeout: 10000 });

			if (await this.externalAuth.isVisible()) {
				return false;
			}
			return true;
		} catch {
			return false;
		}
	}
}
