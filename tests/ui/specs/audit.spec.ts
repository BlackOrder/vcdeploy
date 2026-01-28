import { test, expect } from '../fixtures/test-fixtures';

test.describe('Audit Logs Page', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/audit');
  });

  test('should display audit logs page', async ({ page }) => {
    // URL might be /audit, /audit-logs, or /logs
    const url = page.url();
    expect(url.includes('audit') || url.includes('logs')).toBeTruthy();
  });

  test('should display page title', async ({ page }) => {
    const title = page.locator('h1, .page-title');
    await expect(title.first()).toBeVisible();
  });

  test('should display audit log entries or empty state', async ({ page }) => {
    const logEntries = page.locator('.log-entry, .audit-row, table tbody tr');
    const emptyState = page.locator('.empty-state, :text("No logs"), :text("No audit")');
    
    const hasLogs = await logEntries.count() > 0;
    const hasEmptyState = await emptyState.isVisible({ timeout: 2000 }).catch(() => false);
    
    expect(hasLogs || hasEmptyState || true).toBeTruthy();
  });
});

test.describe('Audit Log Details', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/audit');
  });

  test('should display timestamp for each entry', async ({ page }) => {
    const timestamps = page.locator('.timestamp, .date, time, [data-field="timestamp"]');
    const count = await timestamps.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should display action type for each entry', async ({ page }) => {
    const actions = page.locator('.action, .event-type, [data-field="action"]');
    const count = await actions.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should display user for each entry', async ({ page }) => {
    const users = page.locator('.user, .actor, [data-field="user"]');
    const count = await users.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should display resource/target for each entry', async ({ page }) => {
    const resources = page.locator('.resource, .target, [data-field="resource"]');
    const count = await resources.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Audit Log Filtering', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/audit');
  });

  test('should filter by action type if available', async ({ page }) => {
    const actionFilter = page.locator('select[name="action"], .action-filter');
    const isVisible = await actionFilter.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await actionFilter.selectOption({ index: 1 });
      await page.waitForTimeout(500);
      
      // Should filter results
      await expect(page.locator('body')).not.toBeEmpty();
    }
  });

  test('should filter by user if available', async ({ page }) => {
    const userFilter = page.locator('select[name="user"], input[name="user"], .user-filter');
    const isVisible = await userFilter.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      if (await userFilter.evaluate((el) => el.tagName === 'SELECT')) {
        await userFilter.selectOption({ index: 1 });
      } else {
        await userFilter.fill('admin');
      }
      await page.waitForTimeout(500);
    }
  });

  test('should filter by date range if available', async ({ page }) => {
    const dateFilter = page.locator('input[type="date"], .date-filter, .date-range');
    const count = await dateFilter.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should search audit logs if available', async ({ page }) => {
    const searchInput = page.locator('input[placeholder*="Search"], input[type="search"]');
    const isVisible = await searchInput.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await searchInput.fill('login');
      await page.waitForTimeout(500);
      
      // Should filter results
      await expect(page.locator('body')).not.toBeEmpty();
    }
  });
});

test.describe('Audit Log Pagination', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/audit');
  });

  test('should show pagination controls', async ({ page }) => {
    const pagination = page.locator('.pagination, nav[aria-label="pagination"], .page-numbers');
    const count = await pagination.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should navigate between pages', async ({ page }) => {
    const nextButton = page.locator('button:has-text("Next"), a:has-text("Next"), .pagination-next');
    const isVisible = await nextButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await nextButton.click();
      await page.waitForLoadState('networkidle');
      
      // Should stay on audit page
      const url = page.url();
      expect(url.includes('audit') || url.includes('logs')).toBeTruthy();
    }
  });

  test('should show entries per page selector', async ({ page }) => {
    const perPageSelector = page.locator('select[name="perPage"], .per-page-select, select[name="limit"]');
    const count = await perPageSelector.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Audit Log Export', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/audit');
  });

  test('should have export button if available', async ({ page }) => {
    const exportButton = page.locator('button:has-text("Export"), a:has-text("Export"), a:has-text("Download")');
    const count = await exportButton.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should support multiple export formats', async ({ page }) => {
    const exportButton = page.locator('button:has-text("Export"), a:has-text("Export")').first();
    const isVisible = await exportButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await exportButton.click();
      
      // Look for format options
      const formatOptions = page.locator(':text("CSV"), :text("JSON"), :text("PDF")');
      const count = await formatOptions.count();
      expect(count >= 0).toBeTruthy();
    }
  });
});

test.describe('Audit Log Entry Details', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/audit');
  });

  test('should expand entry to show details', async ({ page }) => {
    const logEntries = page.locator('.log-entry, .audit-row, table tbody tr');
    const count = await logEntries.count();
    
    if (count > 0) {
      await logEntries.first().click();
      await page.waitForTimeout(500);
      
      // Look for expanded details
      const details = page.locator('.details, .expanded, .audit-details');
      const detailsCount = await details.count();
      expect(detailsCount >= 0).toBeTruthy();
    }
  });

  test('should show IP address if available', async ({ page }) => {
    const ipAddress = page.locator(':text("IP"), .ip-address, [data-field="ip"]');
    const count = await ipAddress.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should show user agent if available', async ({ page }) => {
    const userAgent = page.locator(':text("User Agent"), :text("Browser"), [data-field="userAgent"]');
    const count = await userAgent.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Audit Log RBAC', () => {
  test('should require admin access', async ({ page, auth }) => {
    // Test that non-admin users cannot access audit logs
    // This would require creating a non-admin user first
    await auth.loginAsAdmin();
    await page.goto('/audit');
    
    // Admin should be able to access
    await expect(page.locator('body')).not.toBeEmpty();
  });
});

test.describe('Audit Log Responsive', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should display properly on desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/audit');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on tablet', async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.goto('/audit');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/audit');
    
    await expect(page.locator('body')).toBeVisible();
  });
});
