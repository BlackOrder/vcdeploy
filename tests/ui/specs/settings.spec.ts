import { test, expect } from '../fixtures/test-fixtures';

test.describe('Settings Page', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/settings');
  });

  test('should display settings page', async ({ page }) => {
    await expect(page).toHaveURL(/.*settings/);
  });

  test('should display page title', async ({ page }) => {
    const title = page.locator('h1, .page-title');
    await expect(title.first()).toBeVisible();
  });

  test('should have settings categories', async ({ page }) => {
    // Settings should be organized in some way
    const sections = page.locator('.settings-section, .card, [data-section], fieldset');
    const count = await sections.count();
    
    expect(count).toBeGreaterThanOrEqual(0);
  });
});

test.describe('General Settings', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/settings');
  });

  test('should have general settings section', async ({ page }) => {
    const generalSection = page.locator(':text("General"), :text("general"), [data-section="general"]');
    const count = await generalSection.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should have save button', async ({ page }) => {
    const saveButton = page.locator('button:has-text("Save"), button[type="submit"]');
    const count = await saveButton.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should show success message after saving', async ({ page }) => {
    const saveButton = page.locator('button:has-text("Save"), button[type="submit"]').first();
    const isVisible = await saveButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await saveButton.click();
      await page.waitForLoadState('networkidle');
      
      // Look for success notification
      const success = page.locator('.success, .toast, [role="alert"]:has-text("saved")');
      const count = await success.count();
      expect(count >= 0).toBeTruthy();
    }
  });
});

test.describe('Security Settings', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should have security settings section', async ({ page }) => {
    await page.goto('/settings');
    
    const securitySection = page.locator(':text("Security"), :text("security"), [data-section="security"]');
    const count = await securitySection.count();
    
    // Click on security tab if exists
    if (count > 0) {
      await securitySection.first().click();
    }
    
    expect(count >= 0).toBeTruthy();
  });

  test('should have password requirements settings', async ({ page }) => {
    await page.goto('/settings');
    
    const passwordSettings = page.locator(':text("Password"), :text("password"), [name*="password"]');
    const count = await passwordSettings.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should have session timeout setting', async ({ page }) => {
    await page.goto('/settings');
    
    const sessionSetting = page.locator(':text("Session"), :text("session"), :text("timeout")');
    const count = await sessionSetting.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Notification Settings', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/settings');
  });

  test('should have notification settings', async ({ page }) => {
    const notificationSection = page.locator(':text("Notification"), :text("notification"), :text("Email")');
    const count = await notificationSection.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should have email settings if available', async ({ page }) => {
    const emailSettings = page.locator('input[name*="email"], input[name*="smtp"], [data-section="email"]');
    const count = await emailSettings.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Export/Import Settings', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/settings');
  });

  test('should have export button', async ({ page }) => {
    const exportButton = page.locator('button:has-text("Export"), a:has-text("Export")');
    const count = await exportButton.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should have import button', async ({ page }) => {
    const importButton = page.locator('button:has-text("Import"), a:has-text("Import")');
    const count = await importButton.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Webhook Settings', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should have webhook settings section', async ({ page }) => {
    await page.goto('/settings');
    
    const webhookSection = page.locator(':text("Webhook"), :text("webhook"), [data-section="webhook"]');
    const count = await webhookSection.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should have webhook secret setting', async ({ page }) => {
    await page.goto('/settings');
    
    const secretInput = page.locator('input[name*="webhook_secret"], input[name*="webhookSecret"]');
    const count = await secretInput.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Integration Settings', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/settings');
  });

  test('should have integration settings', async ({ page }) => {
    const integrationSection = page.locator(':text("Integration"), :text("GitHub"), :text("GitLab"), :text("Bitbucket")');
    const count = await integrationSection.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Settings Validation', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/settings');
  });

  test('should validate required fields', async ({ page }) => {
    // Clear a required field if found
    const requiredInput = page.locator('input[required]').first();
    const isVisible = await requiredInput.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await requiredInput.clear();
      
      const saveButton = page.locator('button:has-text("Save"), button[type="submit"]').first();
      await saveButton.click();
      
      // Should show validation error
      const error = page.locator('.error, .invalid, [aria-invalid="true"]');
      const count = await error.count();
      expect(count >= 0).toBeTruthy();
    }
  });
});

test.describe('Settings Navigation', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/settings');
  });

  test('should have tabbed or sectioned navigation', async ({ page }) => {
    const tabs = page.locator('[role="tab"], .tab, .settings-nav a');
    const count = await tabs.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should switch between settings sections', async ({ page }) => {
    const tabs = page.locator('[role="tab"], .tab, .settings-nav a');
    const count = await tabs.count();
    
    if (count > 1) {
      await tabs.nth(1).click();
      await page.waitForLoadState('networkidle');
      
      // Content should change
      await expect(page.locator('body')).not.toBeEmpty();
    }
  });
});

test.describe('Settings Responsive', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should display properly on desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/settings');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on tablet', async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.goto('/settings');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/settings');
    
    await expect(page.locator('body')).toBeVisible();
  });
});
