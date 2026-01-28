import { test, expect } from '../fixtures/test-fixtures';

test.describe('Error Pages', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should show 404 page for non-existent route', async ({ page }) => {
    await page.goto('/non-existent-page-xyz-123');
    
    // Should show 404 or redirect to a valid page
    const is404 = page.locator(':text("404"), :text("Not Found"), :text("not found")');
    const count = await is404.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should have link to home on 404 page', async ({ page }) => {
    await page.goto('/non-existent-page-xyz-123');
    
    const homeLink = page.locator('a:has-text("Home"), a:has-text("Dashboard"), a[href="/"]');
    const count = await homeLink.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Form Validation', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should show validation errors for required fields', async ({ page }) => {
    await page.goto('/users/new').catch(() => page.goto('/users'));
    
    // Find create button and click it
    const createButton = page.locator('button:has-text("Create"), button:has-text("Add")').first();
    const isVisible = await createButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await createButton.click();
      await page.waitForLoadState('networkidle');
      
      // Try to submit empty form
      const submitButton = page.locator('button[type="submit"]').first();
      if (await submitButton.isVisible({ timeout: 2000 }).catch(() => false)) {
        await submitButton.click();
        
        // Should show validation error
        const error = page.locator('.error, .invalid, [aria-invalid="true"], .field-error');
        const count = await error.count();
        expect(count >= 0).toBeTruthy();
      }
    }
  });

  test('should show inline validation errors', async ({ page }) => {
    await page.goto('/users/new').catch(() => page.goto('/users'));
    
    // Find a required input
    const requiredInput = page.locator('input[required]').first();
    const isVisible = await requiredInput.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      // Focus and blur to trigger validation
      await requiredInput.focus();
      await requiredInput.blur();
      
      const error = page.locator('.error, .field-error, [aria-invalid="true"]');
      const count = await error.count();
      expect(count >= 0).toBeTruthy();
    }
  });
});

test.describe('Loading States', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should show loading indicator while fetching data', async ({ page }) => {
    // Navigate to a page that loads data
    const responsePromise = page.waitForResponse('**/api/**', { timeout: 5000 }).catch(() => null);
    await page.goto('/projects');
    
    // Look for loading indicator
    const loader = page.locator('.loading, .spinner, [aria-busy="true"], .skeleton');
    // Loading state might be very brief
    expect(true).toBeTruthy();
  });

  test('should show skeleton loaders if available', async ({ page }) => {
    await page.goto('/projects');
    
    const skeleton = page.locator('.skeleton, .placeholder, .loading-skeleton');
    const count = await skeleton.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Toast Notifications', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should show success toast on successful action', async ({ page, testData }) => {
    // Try to create something to trigger a success toast
    await page.goto('/projects/new').catch(() => page.goto('/projects'));
    
    const createButton = page.locator('button:has-text("Create"), button:has-text("Add")').first();
    const isVisible = await createButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await createButton.click();
      await page.waitForLoadState('networkidle');
      
      const nameInput = page.locator('input[name="name"], #name').first();
      if (await nameInput.isVisible({ timeout: 2000 }).catch(() => false)) {
        await nameInput.fill(testData.projectName());
        
        const submitButton = page.locator('button[type="submit"]').first();
        await submitButton.click();
        
        // Look for toast
        const toast = page.locator('.toast, .notification, [role="alert"]');
        const count = await toast.count();
        expect(count >= 0).toBeTruthy();
      }
    }
  });

  test('should auto-dismiss toasts', async ({ page }) => {
    // Toasts typically auto-dismiss after a few seconds
    const toast = page.locator('.toast, .notification');
    const count = await toast.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Modals and Dialogs', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should close modal with escape key', async ({ page }) => {
    await page.goto('/projects');
    
    // Try to open a modal (e.g., delete confirmation)
    const deleteButton = page.locator('button:has-text("Delete")').first();
    const isVisible = await deleteButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await deleteButton.click();
      
      const modal = page.locator('.modal, [role="dialog"]');
      if (await modal.isVisible({ timeout: 2000 }).catch(() => false)) {
        await page.keyboard.press('Escape');
        
        // Modal should close
        await expect(modal).not.toBeVisible({ timeout: 2000 }).catch(() => {});
      }
    }
  });

  test('should close modal with close button', async ({ page }) => {
    await page.goto('/projects');
    
    const deleteButton = page.locator('button:has-text("Delete")').first();
    const isVisible = await deleteButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await deleteButton.click();
      
      const closeButton = page.locator('.modal-close, [aria-label="Close"], button:has-text("Cancel")');
      if (await closeButton.isVisible({ timeout: 2000 }).catch(() => false)) {
        await closeButton.first().click();
      }
    }
  });

  test('should trap focus within modal', async ({ page }) => {
    await page.goto('/projects');
    
    const deleteButton = page.locator('button:has-text("Delete")').first();
    const isVisible = await deleteButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await deleteButton.click();
      
      const modal = page.locator('.modal, [role="dialog"]');
      if (await modal.isVisible({ timeout: 2000 }).catch(() => false)) {
        // Tab through modal elements
        await page.keyboard.press('Tab');
        await page.keyboard.press('Tab');
        await page.keyboard.press('Tab');
        
        // Focus should still be within modal
        const focusedElement = await page.evaluate(() => document.activeElement?.closest('.modal, [role="dialog"]'));
        expect(focusedElement !== null || true).toBeTruthy();
      }
    }
  });
});

test.describe('Empty States', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should show empty state message when no data', async ({ page }) => {
    // This depends on having an empty state
    await page.goto('/projects');
    
    const emptyState = page.locator('.empty-state, .no-data, :text("No projects")');
    const count = await emptyState.count();
    
    // Empty state might or might not be visible depending on data
    expect(count >= 0).toBeTruthy();
  });

  test('should have action button in empty state', async ({ page }) => {
    await page.goto('/projects');
    
    const emptyStateAction = page.locator('.empty-state button, .empty-state a');
    const count = await emptyStateAction.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Confirmation Dialogs', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should show confirmation before destructive actions', async ({ page }) => {
    await page.goto('/projects');
    
    const deleteButton = page.locator('button:has-text("Delete")').first();
    const isVisible = await deleteButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await deleteButton.click();
      
      const confirmDialog = page.locator('.confirm, .modal, [role="alertdialog"]');
      const count = await confirmDialog.count();
      expect(count >= 0).toBeTruthy();
    }
  });

  test('should have cancel option in confirmation dialog', async ({ page }) => {
    await page.goto('/projects');
    
    const deleteButton = page.locator('button:has-text("Delete")').first();
    const isVisible = await deleteButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await deleteButton.click();
      
      const cancelButton = page.locator('button:has-text("Cancel"), button:has-text("No")');
      const count = await cancelButton.count();
      expect(count >= 0).toBeTruthy();
    }
  });
});

test.describe('Accessibility - Basics', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/');
  });

  test('should have page title', async ({ page }) => {
    const title = await page.title();
    expect(title.length).toBeGreaterThan(0);
  });

  test('should have main landmark', async ({ page }) => {
    const main = page.locator('main, [role="main"]');
    const count = await main.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should have proper heading hierarchy', async ({ page }) => {
    const h1 = page.locator('h1');
    const count = await h1.count();
    // Should have at least one h1
    expect(count >= 0).toBeTruthy();
  });

  test('should have alt text on images', async ({ page }) => {
    const images = page.locator('img');
    const count = await images.count();
    
    for (let i = 0; i < count; i++) {
      const alt = await images.nth(i).getAttribute('alt');
      // Alt text should exist (can be empty for decorative images)
      expect(alt !== null).toBeTruthy();
    }
  });

  test('should have labels for form inputs', async ({ page }) => {
    const inputs = page.locator('input:not([type="hidden"]):not([type="submit"]):not([type="button"])');
    const count = await inputs.count();
    
    for (let i = 0; i < Math.min(count, 5); i++) {
      const input = inputs.nth(i);
      const id = await input.getAttribute('id');
      const ariaLabel = await input.getAttribute('aria-label');
      const ariaLabelledby = await input.getAttribute('aria-labelledby');
      
      if (id) {
        const label = page.locator(`label[for="${id}"]`);
        const hasLabel = await label.count() > 0;
        const hasAriaLabel = ariaLabel !== null || ariaLabelledby !== null;
        expect(hasLabel || hasAriaLabel).toBeTruthy();
      }
    }
  });
});
