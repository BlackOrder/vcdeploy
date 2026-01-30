import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from './base-page';

/**
 * Change password page object for password change functionality
 */
export class ChangePasswordPage extends BasePage {
  readonly currentPasswordInput: Locator;
  readonly newPasswordInput: Locator;
  readonly confirmPasswordInput: Locator;
  readonly submitButton: Locator;
  readonly errorMessage: Locator;
  readonly successMessage: Locator;
  readonly requiredIndicator: Locator;
  readonly pageTitle: Locator;

  constructor(page: Page) {
    super(page);
    this.currentPasswordInput = page.locator('input[name="current_password"], input[id="current_password"], #current_password');
    this.newPasswordInput = page.locator('input[name="new_password"], input[id="new_password"], #new_password');
    this.confirmPasswordInput = page.locator('input[name="confirm_password"], input[id="confirm_password"], #confirm_password');
    this.submitButton = page.locator('button[type="submit"]');
    this.errorMessage = page.locator('.error, .alert-error, .alert-danger, [role="alert"]');
    this.successMessage = page.locator('.success, .alert-success, [role="status"]');
    this.requiredIndicator = page.locator('.required-indicator, .must-change, .password-required, [data-required="true"]');
    this.pageTitle = page.locator('h1, h2, .page-title');
  }

  /**
   * Navigate to change password page
   */
  async goto() {
    await super.goto('/change-password');
  }

  /**
   * Fill change password form
   */
  async fillForm(currentPassword: string, newPassword: string, confirmPassword?: string) {
    await this.currentPasswordInput.fill(currentPassword);
    await this.newPasswordInput.fill(newPassword);
    await this.confirmPasswordInput.fill(confirmPassword ?? newPassword);
  }

  /**
   * Submit change password form
   */
  async submit() {
    await this.submitButton.click();
  }

  /**
   * Complete password change
   */
  async changePassword(currentPassword: string, newPassword: string) {
    await this.fillForm(currentPassword, newPassword);
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
   * Check if required indicator is visible (for MustChangePassword flow)
   */
  async isRequiredIndicatorVisible(): Promise<boolean> {
    return this.requiredIndicator.isVisible({ timeout: 2000 }).catch(() => false);
  }

  /**
   * Verify change password page is displayed
   */
  async verifyPage() {
    await expect(this.currentPasswordInput).toBeVisible();
    await expect(this.newPasswordInput).toBeVisible();
    await expect(this.confirmPasswordInput).toBeVisible();
    await expect(this.submitButton).toBeVisible();
  }

  /**
   * Check if currently on change password page
   */
  async isOnChangePasswordPage(): Promise<boolean> {
    return this.page.url().includes('/change-password');
  }
}
