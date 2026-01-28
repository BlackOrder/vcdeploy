import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from './base-page';

/**
 * Login page object
 */
export class LoginPage extends BasePage {
  readonly usernameInput: Locator;
  readonly passwordInput: Locator;
  readonly submitButton: Locator;
  readonly errorMessage: Locator;
  readonly forgotPasswordLink: Locator;

  constructor(page: Page) {
    super(page);
    this.usernameInput = page.locator('input[name="username"], input[id="username"], #username');
    this.passwordInput = page.locator('input[name="password"], input[id="password"], #password');
    this.submitButton = page.locator('button[type="submit"]');
    this.errorMessage = page.locator('.error, .alert-error, .alert-danger, [role="alert"]');
    this.forgotPasswordLink = page.locator('a:has-text("Forgot"), a:has-text("forgot")');
  }

  /**
   * Navigate to login page
   */
  async goto() {
    await super.goto('/login');
  }

  /**
   * Fill login form
   */
  async fillLoginForm(username: string, password: string) {
    await this.usernameInput.fill(username);
    await this.passwordInput.fill(password);
  }

  /**
   * Submit login form
   */
  async submit() {
    await this.submitButton.click();
  }

  /**
   * Login with credentials
   */
  async login(username: string, password: string) {
    await this.fillLoginForm(username, password);
    await this.submit();
  }

  /**
   * Check if error message is displayed
   */
  async hasError(): Promise<boolean> {
    return this.errorMessage.isVisible({ timeout: 2000 });
  }

  /**
   * Get error message text
   */
  async getErrorText(): Promise<string | null> {
    if (await this.hasError()) {
      return this.errorMessage.textContent();
    }
    return null;
  }

  /**
   * Verify login page is displayed
   */
  async verifyPage() {
    await expect(this.usernameInput).toBeVisible();
    await expect(this.passwordInput).toBeVisible();
    await expect(this.submitButton).toBeVisible();
  }
}
