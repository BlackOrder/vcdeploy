import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from './base-page';

/**
 * Setup wizard page object for first-run configuration
 */
export class SetupPage extends BasePage {
  readonly usernameInput: Locator;
  readonly emailInput: Locator;
  readonly passwordInput: Locator;
  readonly confirmPasswordInput: Locator;
  readonly submitButton: Locator;
  readonly errorMessage: Locator;
  readonly pageTitle: Locator;

  constructor(page: Page) {
    super(page);
    this.usernameInput = page.locator('input[name="username"], input[id="username"], #username');
    this.emailInput = page.locator('input[name="email"], input[id="email"], #email');
    this.passwordInput = page.locator('input[name="password"], input[id="password"], #password');
    this.confirmPasswordInput = page.locator('input[name="confirm_password"], input[id="confirm_password"], #confirm_password');
    this.submitButton = page.locator('button[type="submit"]');
    this.errorMessage = page.locator('.error, .alert-error, .alert-danger, [role="alert"]');
    this.pageTitle = page.locator('h1, h2, .page-title');
  }

  /**
   * Navigate to setup page
   */
  async goto() {
    await super.goto('/setup');
  }

  /**
   * Fill setup form with admin credentials
   */
  async fillSetupForm(username: string, email: string, password: string, confirmPassword?: string) {
    await this.usernameInput.fill(username);
    await this.emailInput.fill(email);
    await this.passwordInput.fill(password);
    await this.confirmPasswordInput.fill(confirmPassword ?? password);
  }

  /**
   * Submit setup form
   */
  async submit() {
    await this.submitButton.click();
  }

  /**
   * Complete setup with credentials
   */
  async completeSetup(username: string, email: string, password: string) {
    await this.fillSetupForm(username, email, password);
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
   * Verify setup page is displayed
   */
  async verifyPage() {
    await expect(this.usernameInput).toBeVisible();
    await expect(this.emailInput).toBeVisible();
    await expect(this.passwordInput).toBeVisible();
    await expect(this.confirmPasswordInput).toBeVisible();
    await expect(this.submitButton).toBeVisible();
  }

  /**
   * Check if currently on setup page
   */
  async isOnSetupPage(): Promise<boolean> {
    return this.page.url().includes('/setup');
  }
}
