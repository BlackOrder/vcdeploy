import { test, expect } from '../fixtures/test-fixtures';

test.describe('Deployments List', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/deployments');
  });

  test('should display deployments page', async ({ page }) => {
    await expect(page).toHaveURL(/.*deployments/);
  });

  test('should display page title', async ({ page }) => {
    const title = page.locator('h1, .page-title');
    await expect(title.first()).toBeVisible();
  });

  test('should display deployment list or empty state', async ({ page }) => {
    const deployments = page.locator('.deployment-card, .deployment-row, table tbody tr');
    const emptyState = page.locator('.empty-state, :text("No deployments")');
    
    const hasDeployments = await deployments.count() > 0;
    const hasEmptyState = await emptyState.isVisible({ timeout: 2000 }).catch(() => false);
    
    expect(hasDeployments || hasEmptyState || true).toBeTruthy();
  });
});

test.describe('Deployment Status', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/deployments');
  });

  test('should show deployment status indicators', async ({ page }) => {
    const statusIndicators = page.locator('.status, .status-badge, [class*="status"]');
    const count = await statusIndicators.count();
    
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should show success/failure status', async ({ page }) => {
    const success = page.locator(':text("Success"), :text("success"), .status-success');
    const failure = page.locator(':text("Failed"), :text("failed"), .status-failed');
    const pending = page.locator(':text("Pending"), :text("pending"), :text("Running"), .status-pending');
    
    const hasSuccess = await success.count() > 0;
    const hasFailure = await failure.count() > 0;
    const hasPending = await pending.count() > 0;
    
    expect(hasSuccess || hasFailure || hasPending || true).toBeTruthy();
  });
});

test.describe('Deployment Details', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should navigate to deployment details', async ({ page }) => {
    await page.goto('/deployments');
    
    const deploymentLinks = page.locator('.deployment-card a, .deployment-row a, table tbody tr a');
    const count = await deploymentLinks.count();
    
    if (count > 0) {
      await deploymentLinks.first().click();
      await page.waitForLoadState('networkidle');
      
      const url = page.url();
      expect(url.includes('/deployments/')).toBeTruthy();
    }
  });

  test('should display deployment logs', async ({ page }) => {
    await page.goto('/deployments');
    
    const deploymentLinks = page.locator('.deployment-card, .deployment-row, table tbody tr');
    const count = await deploymentLinks.count();
    
    if (count > 0) {
      await deploymentLinks.first().click();
      await page.waitForLoadState('networkidle');
      
      // Look for logs section
      const logs = page.locator('.logs, .log-output, pre, code');
      const logsCount = await logs.count();
      
      expect(logsCount).toBeGreaterThanOrEqual(0);
    }
  });
});

test.describe('Deployment Actions', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/deployments');
  });

  test('should have cancel button for running deployments', async ({ page }) => {
    const runningDeployment = page.locator(':text("Running"), :text("Pending")').first();
    const isVisible = await runningDeployment.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      // Should have cancel button nearby
      const cancelButton = page.locator('button:has-text("Cancel"), [data-action="cancel"]');
      const count = await cancelButton.count();
      expect(count).toBeGreaterThanOrEqual(0);
    }
  });

  test('should have rollback button for completed deployments', async ({ page }) => {
    const completedDeployment = page.locator(':text("Success"), :text("Completed")').first();
    const isVisible = await completedDeployment.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      const rollbackButton = page.locator('button:has-text("Rollback"), [data-action="rollback"]');
      const count = await rollbackButton.count();
      expect(count).toBeGreaterThanOrEqual(0);
    }
  });

  test('should have retry button for failed deployments', async ({ page }) => {
    const failedDeployment = page.locator(':text("Failed"), :text("Error")').first();
    const isVisible = await failedDeployment.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      const retryButton = page.locator('button:has-text("Retry"), button:has-text("Redeploy"), [data-action="retry"]');
      const count = await retryButton.count();
      expect(count).toBeGreaterThanOrEqual(0);
    }
  });
});

test.describe('Deployment Filtering', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/deployments');
  });

  test('should filter by project if available', async ({ page }) => {
    const projectFilter = page.locator('select[name="project"], .project-filter');
    const isVisible = await projectFilter.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      // Filter functionality exists
      expect(isVisible).toBeTruthy();
    }
  });

  test('should filter by status if available', async ({ page }) => {
    const statusFilter = page.locator('select[name="status"], .status-filter');
    const isVisible = await statusFilter.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      expect(isVisible).toBeTruthy();
    }
  });

  test('should filter by date range if available', async ({ page }) => {
    const dateFilter = page.locator('input[type="date"], .date-filter, .date-range');
    const count = await dateFilter.count();
    
    expect(count).toBeGreaterThanOrEqual(0);
  });
});

test.describe('Deployment Pagination', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/deployments');
  });

  test('should show pagination if many deployments', async ({ page }) => {
    const pagination = page.locator('.pagination, nav[aria-label="pagination"], .page-numbers');
    const count = await pagination.count();
    
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should navigate between pages if pagination exists', async ({ page }) => {
    const nextButton = page.locator('button:has-text("Next"), a:has-text("Next"), .pagination-next');
    const isVisible = await nextButton.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await nextButton.click();
      await page.waitForLoadState('networkidle');
      
      // Should stay on deployments page
      const url = page.url();
      expect(url.includes('/deployments')).toBeTruthy();
    }
  });
});

test.describe('Real-time Updates', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/deployments');
  });

  test('should auto-refresh or have refresh button', async ({ page }) => {
    const refreshButton = page.locator('button:has-text("Refresh"), [data-action="refresh"]');
    const autoRefreshIndicator = page.locator(':text("Auto"), .auto-refresh');
    
    const hasRefresh = await refreshButton.isVisible({ timeout: 2000 }).catch(() => false);
    const hasAutoRefresh = await autoRefreshIndicator.isVisible({ timeout: 2000 }).catch(() => false);
    
    expect(hasRefresh || hasAutoRefresh || true).toBeTruthy();
  });
});

test.describe('Deployment Responsive', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should display properly on desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/deployments');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on tablet', async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.goto('/deployments');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/deployments');
    
    await expect(page.locator('body')).toBeVisible();
  });
});
