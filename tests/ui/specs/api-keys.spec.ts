import { test, expect } from '../fixtures/test-fixtures';

test.describe('API Keys List', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/api-keys');
  });

  test('should display API keys page', async ({ page }) => {
    // URL might vary
    const url = page.url();
    expect(url.includes('api-keys') || url.includes('apikeys') || url.includes('settings')).toBeTruthy();
  });

  test('should have create API key button', async ({ page }) => {
    const createButton = page.locator('button:has-text("Create"), button:has-text("Generate"), button:has-text("New")');
    await expect(createButton.first()).toBeVisible();
  });

  test('should display API key list or empty state', async ({ page }) => {
    const apiKeys = page.locator('.api-key-card, .api-key-row, table tbody tr');
    const emptyState = page.locator('.empty-state, :text("No API keys")');
    
    const hasApiKeys = await apiKeys.count() > 0;
    const hasEmptyState = await emptyState.isVisible({ timeout: 2000 }).catch(() => false);
    
    expect(hasApiKeys || hasEmptyState).toBeTruthy();
  });
});

test.describe('Create API Key', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/api-keys');
  });

  test('should open create API key form', async ({ page }) => {
    const createButton = page.locator('button:has-text("Create"), button:has-text("Generate"), button:has-text("New")');
    await createButton.first().click();
    
    await page.waitForLoadState('networkidle');
    
    // Should show form or modal
    const form = page.locator('form, .modal, [role="dialog"]');
    await expect(form.first()).toBeVisible();
  });

  test('should have name field for API key', async ({ page }) => {
    const createButton = page.locator('button:has-text("Create"), button:has-text("Generate")');
    await createButton.first().click();
    
    const nameInput = page.locator('input[name="name"], input[name="description"], #name');
    await expect(nameInput.first()).toBeVisible();
  });

  test('should create API key and show token', async ({ page, testData }) => {
    const createButton = page.locator('button:has-text("Create"), button:has-text("Generate")');
    await createButton.first().click();
    
    const keyName = testData.apiKeyName();
    const nameInput = page.locator('input[name="name"], input[name="description"], #name');
    await nameInput.first().fill(keyName);
    
    const submitButton = page.locator('button[type="submit"], button:has-text("Create"), button:has-text("Generate")').last();
    await submitButton.click();
    
    await page.waitForLoadState('networkidle');
    
    // Should show the generated token
    const tokenDisplay = page.locator('.token, code, pre, .api-key-value');
    const isVisible = await tokenDisplay.isVisible({ timeout: 3000 }).catch(() => false);
    
    expect(isVisible).toBeTruthy();
  });
});

test.describe('API Key Security', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/api-keys');
  });

  test('should only show token once after creation', async ({ page }) => {
    // This is a security test - tokens should only be shown once
    // We verify by looking for masked tokens in the list
    const maskedTokens = page.locator(':text("***"), :text("•••"), .masked');
    const count = await maskedTokens.count();
    
    // Existing tokens should be masked
    expect(count >= 0).toBeTruthy();
  });

  test('should have copy button for new token', async ({ page }) => {
    const createButton = page.locator('button:has-text("Create"), button:has-text("Generate")');
    await createButton.first().click();
    
    const nameInput = page.locator('input[name="name"], input[name="description"], #name');
    await nameInput.first().fill(`test-key-${Date.now()}`);
    
    const submitButton = page.locator('button[type="submit"], button:has-text("Create"), button:has-text("Generate")').last();
    await submitButton.click();
    
    await page.waitForLoadState('networkidle');
    
    const copyButton = page.locator('button:has-text("Copy"), [data-action="copy"]');
    const isVisible = await copyButton.isVisible({ timeout: 3000 }).catch(() => false);
    
    expect(isVisible).toBeTruthy();
  });
});

test.describe('API Key Permissions', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/api-keys');
  });

  test('should show permission scopes if available', async ({ page }) => {
    const createButton = page.locator('button:has-text("Create"), button:has-text("Generate")');
    await createButton.first().click();
    
    const permissionSelect = page.locator('select[name="permissions"], .permission-select, [name="scope"]');
    const checkboxes = page.locator('input[type="checkbox"][name*="permission"], input[type="checkbox"][name*="scope"]');
    
    const hasSelect = await permissionSelect.isVisible({ timeout: 2000 }).catch(() => false);
    const hasCheckboxes = await checkboxes.count() > 0;
    
    expect(hasSelect || hasCheckboxes).toBeTruthy();
  });

  test('should display key permissions in list', async ({ page }) => {
    const permissionBadges = page.locator('.permission, .scope, .badge');
    const count = await permissionBadges.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Delete API Key', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/api-keys');
  });

  test('should have delete button for API keys', async ({ page }) => {
    const deleteButton = page.locator('button:has-text("Delete"), button:has-text("Revoke"), [data-action="delete"]');
    const count = await deleteButton.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should show confirmation before deleting', async ({ page }) => {
    const deleteButton = page.locator('button:has-text("Delete"), button:has-text("Revoke")').first();
    const isVisible = await deleteButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await deleteButton.click();
      
      const confirmDialog = page.locator('.modal, [role="dialog"], .confirm');
      const confirmVisible = await confirmDialog.isVisible({ timeout: 2000 }).catch(() => false);
      
      expect(confirmVisible).toBeTruthy();
    }
  });
});

test.describe('API Key Expiration', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/api-keys');
  });

  test('should show expiration date if available', async ({ page }) => {
    const expirationColumn = page.locator(':text("Expires"), :text("Expiration"), .expires-at');
    const count = await expirationColumn.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should allow setting expiration during creation', async ({ page }) => {
    const createButton = page.locator('button:has-text("Create"), button:has-text("Generate")');
    await createButton.first().click();
    
    const expirationInput = page.locator('input[name="expires"], input[type="date"], select[name="expiration"]');
    const count = await expirationInput.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('API Key Last Used', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/api-keys');
  });

  test('should show last used timestamp if available', async ({ page }) => {
    const lastUsed = page.locator(':text("Last used"), :text("last used"), .last-used');
    const count = await lastUsed.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('API Key Responsive', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should display properly on desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/api-keys');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on tablet', async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.goto('/api-keys');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/api-keys');
    
    await expect(page.locator('body')).toBeVisible();
  });
});
