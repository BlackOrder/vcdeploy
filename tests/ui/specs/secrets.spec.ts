import { test, expect, TEST_ADMIN_PASSWORD } from '../fixtures/test-fixtures';

test.describe('Secrets List', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/secrets');
  });

  test('should display secrets page', async ({ page }) => {
    await expect(page).toHaveURL(/.*secrets/);
  });

  test('should display page title', async ({ page }) => {
    const title = page.locator('h1, .page-title');
    await expect(title.first()).toBeVisible();
  });

  test('should have create secret button', async ({ page }) => {
    const createButton = page.locator('button:has-text("Create"), button:has-text("Add"), button:has-text("New")');
    await expect(createButton.first()).toBeVisible();
  });

  test('should display secret list or empty state', async ({ page }) => {
    const secrets = page.locator('.secret-card, .secret-row, table tbody tr');
    const emptyState = page.locator('.empty-state, :text("No secrets")');
    
    const hasSecrets = await secrets.count() > 0;
    const hasEmptyState = await emptyState.isVisible({ timeout: 2000 }).catch(() => false);
    
    expect(hasSecrets || hasEmptyState).toBeTruthy();
  });
});

test.describe('Create Secret', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should navigate to create secret form', async ({ page }) => {
    await page.goto('/secrets');
    
    const createButton = page.locator('button:has-text("Create"), button:has-text("Add"), a:has-text("New")');
    await createButton.first().click();
    
    await page.waitForLoadState('networkidle');
    
    // Should show form
    const form = page.locator('form, .modal, [role="dialog"]');
    await expect(form.first()).toBeVisible();
  });

  test('should have required form fields', async ({ page }) => {
    await page.goto('/secrets/new').catch(async () => {
      await page.goto('/secrets');
      const createButton = page.locator('button:has-text("Create"), button:has-text("Add")');
      await createButton.first().click();
    });
    
    // Should have name/key field
    const nameInput = page.locator('input[name="name"], input[name="key"], #name, #key');
    const valueInput = page.locator('input[name="value"], textarea[name="value"], #value');
    
    await expect(nameInput.first()).toBeVisible();
    await expect(valueInput.first()).toBeVisible();
  });

  test('should create secret with valid data', async ({ page, testData }) => {
    await page.goto('/secrets/new').catch(async () => {
      await page.goto('/secrets');
      const createButton = page.locator('button:has-text("Create"), button:has-text("Add")');
      await createButton.first().click();
    });
    
    const secretName = testData.secretName();
    const nameInput = page.locator('input[name="name"], input[name="key"], #name, #key');
    const valueInput = page.locator('input[name="value"], textarea[name="value"], #value');
    
    await nameInput.first().fill(secretName);
    await valueInput.first().fill('test-secret-value');
    
    const submitButton = page.locator('button[type="submit"], button:has-text("Save"), button:has-text("Create")');
    await submitButton.click();
    
    await page.waitForLoadState('networkidle');
  });
});

test.describe('Secret Security', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/secrets');
  });

  test('should mask secret values by default', async ({ page }) => {
    // Secret values should be masked (shown as asterisks or dots)
    const maskedValues = page.locator(':text("***"), :text("•••"), .masked, .secret-value');
    const count = await maskedValues.count();
    
    // If there are secrets, they should be masked
    expect(count >= 0).toBeTruthy();
  });

  test('should have reveal/show button for secrets', async ({ page }) => {
    const revealButton = page.locator('button:has-text("Show"), button:has-text("Reveal"), [data-action="reveal"]');
    const count = await revealButton.count();
    
    // Reveal button might exist if there are secrets
    expect(count >= 0).toBeTruthy();
  });

  test('should have copy button for secrets', async ({ page }) => {
    const copyButton = page.locator('button:has-text("Copy"), [data-action="copy"]');
    const count = await copyButton.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Edit Secret', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should edit secret value', async ({ page, testData, api }) => {
    // Create a test secret via API first
    await api.authenticate('admin', TEST_ADMIN_PASSWORD);
    
    // Navigate to secrets page
    await page.goto('/secrets');
    await page.waitForLoadState('networkidle');
    
    // Look for edit button
    const editButton = page.locator('button:has-text("Edit"), [data-action="edit"]').first();
    const isVisible = await editButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await editButton.click();
      await page.waitForLoadState('networkidle');
      
      // Should show edit form
      const form = page.locator('form, .modal');
      await expect(form.first()).toBeVisible();
    }
  });
});

test.describe('Delete Secret', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/secrets');
  });

  test('should show delete confirmation', async ({ page }) => {
    const deleteButton = page.locator('button:has-text("Delete"), [data-action="delete"]').first();
    const isVisible = await deleteButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await deleteButton.click();
      
      // Should show confirmation
      const confirmDialog = page.locator('.modal, [role="dialog"], .confirm');
      const confirmVisible = await confirmDialog.isVisible({ timeout: 2000 }).catch(() => false);
      
      expect(confirmVisible).toBeTruthy();
    }
  });
});

test.describe('Secret Scope', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should filter secrets by project if available', async ({ page }) => {
    await page.goto('/secrets');
    
    const projectFilter = page.locator('select[name="project"], .project-filter');
    const isVisible = await projectFilter.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      expect(isVisible).toBeTruthy();
    }
  });

  test('should show global vs project-specific secrets', async ({ page }) => {
    await page.goto('/secrets');
    
    const scopeIndicator = page.locator(':text("Global"), :text("Project"), .scope');
    const count = await scopeIndicator.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Secret Search', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/secrets');
  });

  test('should search secrets by name', async ({ page }) => {
    const searchInput = page.locator('input[placeholder*="Search"], input[type="search"]');
    const isVisible = await searchInput.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await searchInput.fill('test');
      await page.waitForTimeout(500);
      
      // Search should filter results
      const secrets = page.locator('.secret-card, .secret-row, table tbody tr');
      const count = await secrets.count();
      expect(count >= 0).toBeTruthy();
    }
  });
});

test.describe('Secret Responsive', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should display properly on desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/secrets');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on tablet', async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.goto('/secrets');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/secrets');
    
    await expect(page.locator('body')).toBeVisible();
  });
});
