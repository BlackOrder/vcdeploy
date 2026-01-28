import { Page, Locator } from '@playwright/test';

/**
 * Base page object with common functionality
 */
export class BasePage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /**
   * Navigate to a path
   */
  async goto(path: string) {
    await this.page.goto(path);
  }

  /**
   * Wait for page to load
   */
  async waitForLoad() {
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Get toast/notification message
   */
  async getToastMessage(): Promise<string | null> {
    const toast = this.page.locator('.toast, .notification, [role="alert"], .alert');
    if (await toast.isVisible({ timeout: 2000 })) {
      return toast.textContent();
    }
    return null;
  }

  /**
   * Wait for toast with specific text
   */
  async waitForToast(text: string) {
    await this.page.waitForSelector(`:text("${text}")`, { timeout: 5000 });
  }

  /**
   * Check if element exists
   */
  async elementExists(selector: string): Promise<boolean> {
    return this.page.locator(selector).count().then(count => count > 0);
  }

  /**
   * Get breadcrumb text
   */
  async getBreadcrumbs(): Promise<string[]> {
    const breadcrumbs = this.page.locator('.breadcrumb, nav[aria-label="breadcrumb"]').locator('a, span');
    return breadcrumbs.allTextContents();
  }

  /**
   * Click a navigation link
   */
  async clickNavLink(text: string) {
    await this.page.click(`nav a:has-text("${text}"), aside a:has-text("${text}")`);
  }
}
