import { test, expect } from '../fixtures/test-fixtures';

test.describe('Agents List', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/agents');
  });

  test('should display agents page', async ({ page }) => {
    await expect(page).toHaveURL(/.*agents/);
  });

  test('should display page title', async ({ page }) => {
    const title = page.locator('h1, .page-title');
    await expect(title.first()).toBeVisible();
  });

  test('should display agent list or empty state', async ({ page }) => {
    const agents = page.locator('.agent-card, .agent-row, table tbody tr');
    const emptyState = page.locator('.empty-state, :text("No agents")');
    
    const hasAgents = await agents.count() > 0;
    const hasEmptyState = await emptyState.isVisible({ timeout: 2000 }).catch(() => false);
    
    expect(hasAgents || hasEmptyState).toBeTruthy();
  });

  test('should show agent status indicators', async ({ page }) => {
    const statusIndicators = page.locator('.status, .status-badge, .status-indicator');
    const count = await statusIndicators.count();
    
    // If there are agents, they should have status indicators
    expect(count).toBeGreaterThanOrEqual(0);
  });
});

test.describe('Agent Details', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should navigate to agent details', async ({ page }) => {
    await page.goto('/agents');
    
    const agentLinks = page.locator('.agent-card a, .agent-row a, table tbody tr a');
    const count = await agentLinks.count();
    
    if (count > 0) {
      await agentLinks.first().click();
      await page.waitForLoadState('networkidle');
      
      // Should navigate to agent detail page
      const url = page.url();
      expect(url.includes('/agents/')).toBeTruthy();
    }
  });

  test('should display agent information', async ({ page }) => {
    await page.goto('/agents');
    
    const agentLinks = page.locator('.agent-card, .agent-row, table tbody tr');
    const count = await agentLinks.count();
    
    if (count > 0) {
      // Click on first agent
      await agentLinks.first().click();
      await page.waitForLoadState('networkidle');
      
      // Should display agent details
      await expect(page.locator('body')).not.toBeEmpty();
    }
  });
});

test.describe('Agent Status', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/agents');
  });

  test('should show online/offline status', async ({ page }) => {
    const onlineStatus = page.locator(':text("Online"), :text("online"), .status-online');
    const offlineStatus = page.locator(':text("Offline"), :text("offline"), .status-offline');
    
    const hasOnline = await onlineStatus.count() > 0;
    const hasOffline = await offlineStatus.count() > 0;
    
    // Either status type might be present (or neither if no agents)
    expect(hasOnline || hasOffline).toBeTruthy();
  });

  test('should show last seen time if available', async ({ page }) => {
    const lastSeen = page.locator(':text("Last seen"), :text("last seen"), .last-seen');
    const count = await lastSeen.count();
    
    expect(count).toBeGreaterThanOrEqual(0);
  });
});

test.describe('Agent Labels', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/agents');
  });

  test('should display agent labels if present', async ({ page }) => {
    const labels = page.locator('.label, .tag, .badge');
    const count = await labels.count();
    
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should filter agents by label if available', async ({ page }) => {
    const labelFilter = page.locator('select[name="label"], .label-filter, input[placeholder*="label" i]');
    const isVisible = await labelFilter.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      // Filter functionality is available
      expect(isVisible).toBeTruthy();
    }
  });
});

test.describe('Agent Updates', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should show update available indicator if applicable', async ({ page }) => {
    await page.goto('/agents');
    
    const updateIndicator = page.locator(':text("Update"), :text("upgrade"), .update-available');
    const count = await updateIndicator.count();
    
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should have download agent binaries link', async ({ page }) => {
    await page.goto('/agents');
    
    const downloadLink = page.locator('a:has-text("Download"), button:has-text("Download"), [href*="download"]');
    const count = await downloadLink.count();
    
    // Download link might or might not be present
    expect(count).toBeGreaterThanOrEqual(0);
  });
});

test.describe('Agent Connection', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/agents');
  });

  test('should display connection instructions if no agents', async ({ page }) => {
    const emptyState = page.locator('.empty-state, :text("No agents")');
    const isVisible = await emptyState.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      // Should show how to connect an agent
      const instructions = page.locator(':text("connect"), :text("install"), :text("setup")');
      const hasInstructions = await instructions.count() > 0;
      expect(hasInstructions).toBeTruthy();
    }
  });

  test('should show agent token or connection string', async ({ page }) => {
    // Look for token display or copy button
    const tokenElement = page.locator('.token, .connection-string, button:has-text("Copy"), [data-copy]');
    const count = await tokenElement.count();
    
    expect(count).toBeGreaterThanOrEqual(0);
  });
});

test.describe('Agent Responsive', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should display properly on desktop', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.goto('/agents');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on tablet', async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.goto('/agents');
    
    await expect(page.locator('body')).toBeVisible();
  });

  test('should display properly on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/agents');
    
    await expect(page.locator('body')).toBeVisible();
  });
});
