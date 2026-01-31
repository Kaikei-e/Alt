import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Page Object for Login page (/login)
 */
export class LoginPage extends BasePage {
	// Card elements
	readonly cardTitle: Locator;
	readonly cardDescription: Locator;

	// Form inputs
	readonly emailInput: Locator;
	readonly passwordInput: Locator;
	readonly submitButton: Locator;

	// Error messages
	readonly errorMessage: Locator;
	readonly emailError: Locator;
	readonly passwordError: Locator;

	// Navigation
	readonly registerLink: Locator;

	// Loading state
	readonly redirectingText: Locator;

	constructor(page: Page) {
		super(page);

		// Card elements
		this.cardTitle = page.getByRole("heading", { name: /login/i });
		this.cardDescription = page.getByText(/enter your credentials/i);

		// Form inputs - using labels
		this.emailInput = page.getByLabel(/email/i);
		this.passwordInput = page.getByLabel(/password/i);
		this.submitButton = page.getByRole("button", { name: /login/i });

		// Error messages
		this.errorMessage = page.locator('[style*="color: #dc2626"]');
		this.emailError = page.locator("#identifier + p, #identifier ~ p").first();
		this.passwordError = page.locator("#password + p, #password ~ p").first();

		// Navigation
		this.registerLink = page.getByRole("link", { name: /register/i });

		// Loading state
		this.redirectingText = page.getByText("Redirecting...");
	}

	get url(): string {
		return "./login";
	}

	/**
	 * Fill login form with credentials
	 */
	async fillLoginForm(email: string, password: string): Promise<void> {
		await this.emailInput.fill(email);
		await this.passwordInput.fill(password);
	}

	/**
	 * Submit the login form
	 */
	async submitLogin(): Promise<void> {
		await this.submitButton.click();
	}

	/**
	 * Complete login flow (fill form and submit)
	 */
	async login(email: string, password: string): Promise<void> {
		await this.fillLoginForm(email, password);
		await this.submitLogin();
	}

	/**
	 * Navigate to register page
	 */
	async goToRegister(): Promise<void> {
		await this.registerLink.click();
	}

	/**
	 * Check if any error message is visible
	 */
	async hasError(): Promise<boolean> {
		return this.errorMessage.first().isVisible();
	}

	/**
	 * Get all error messages
	 */
	async getErrorMessages(): Promise<string[]> {
		const errors = await this.errorMessage.allTextContents();
		return errors.filter((e) => e.trim().length > 0);
	}

	/**
	 * Wait for the form to be ready (not redirecting)
	 * Returns false if external auth (Cloudflare Access) is detected
	 */
	async waitForFormReady(): Promise<boolean> {
		// Wait for either form to appear, redirecting text, or external auth (Cloudflare)
		const externalAuth = this.page.getByText(/send me a code|cloudflare/i);
		try {
			await expect(
				this.emailInput.or(this.redirectingText).or(externalAuth).first(),
			).toBeVisible({ timeout: 10000 });

			// Check if external auth is detected
			if (await externalAuth.isVisible()) {
				return false;
			}
			return true;
		} catch {
			return false;
		}
	}

	/**
	 * Check if form is being redirected
	 */
	async isRedirecting(): Promise<boolean> {
		return this.redirectingText.isVisible();
	}

	/**
	 * Check if external auth (e.g., Cloudflare Access) is being used
	 */
	async isExternalAuth(): Promise<boolean> {
		const externalAuth = this.page.getByText(/send me a code|cloudflare/i);
		return externalAuth.isVisible();
	}
}
