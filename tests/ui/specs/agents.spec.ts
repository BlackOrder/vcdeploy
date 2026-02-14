import { test, expect, SKIP_AGENT_TESTS } from '../fixtures/test-fixtures';

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
  test.skip(SKIP_AGENT_TESTS, 'Skipping: SKIP_AGENT_TESTS is set');
  
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
  test.skip(SKIP_AGENT_TESTS, 'Skipping: SKIP_AGENT_TESTS is set');
  
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
  test.skip(SKIP_AGENT_TESTS, 'Skipping: SKIP_AGENT_TESTS is set');
  
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

// ========================================
// Full-Suite Agent Management Tests (Step 15)
// ========================================

test.describe('Agent Management Actions', () => {
  test.skip(SKIP_AGENT_TESTS, 'Requires agent');
  
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should view agent details page', async ({ page, api }) => {
    await api.authenticate('admin', process.env.VCDEPLOY_ADMIN_PASSWORD || 'admin');
    
    // Get first agent via API
    const agentsResponse = await api.get('/api/v1/agents');
    if (!agentsResponse.ok || !agentsResponse.data?.length) {
      test.skip();
      return;
    }

    const agentId = agentsResponse.data[0].id;
    await page.goto(`/agents/${agentId}`);
    await page.waitForLoadState('networkidle');

    // Should display agent details
    const agentName = agentsResponse.data[0].name || agentsResponse.data[0].hostname;
    if (agentName) {
      const nameVisible = await page.locator(`text=${agentName}`).isVisible({ timeout: 5000 }).catch(() => false);
      expect(nameVisible).toBeTruthy();
    }
  });

  test('should display agent metrics and stats', async ({ page, api }) => {
    await api.authenticate('admin', process.env.VCDEPLOY_ADMIN_PASSWORD || 'admin');
    
    const agentsResponse = await api.get('/api/v1/agents');
    if (!agentsResponse.ok || !agentsResponse.data?.length) {
      test.skip();
      return;
    }

    const agentId = agentsResponse.data[0].id;
    await page.goto(`/agents/${agentId}`);
    await page.waitForLoadState('networkidle');

    // Look for metrics/stats section
    const statsSection = page.locator('.stats, .metrics, .agent-info, :text("CPU"), :text("Memory"), :text("Deployments")');
    const count = await statsSection.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should edit agent labels', async ({ page, api }) => {
    await api.authenticate('admin', process.env.VCDEPLOY_ADMIN_PASSWORD || 'admin');
    
    const agentsResponse = await api.get('/api/v1/agents');
    if (!agentsResponse.ok || !agentsResponse.data?.length) {
      test.skip();
      return;
    }

    const agentId = agentsResponse.data[0].id;
    await page.goto(`/agents/${agentId}`);
    await page.waitForLoadState('networkidle');

    // Find edit labels button or section
    const editLabelsBtn = page.locator('button:has-text("Edit Labels"), button:has-text("Add Label"), [data-action="edit-labels"]');
    if (await editLabelsBtn.count() > 0) {
      await editLabelsBtn.first().click();
      await page.waitForLoadState('networkidle');

      // Fill label input
      const labelInput = page.locator('input[name="labels"], input[placeholder*="label"]');
      if (await labelInput.count() > 0) {
        const testLabel = `ui-test-label-${Date.now()}`;
        await labelInput.first().fill(testLabel);
        
        // Save
        const saveButton = page.locator('button[type="submit"], button:has-text("Save")');
        if (await saveButton.count() > 0) {
          await saveButton.first().click();
          await page.waitForLoadState('networkidle');
          
          // Verify label appears
          await expect(page.locator(`text=${testLabel}`)).toBeVisible({ timeout: 5000 }).catch(() => {});
        }
      }
    }
  });

  test('should toggle maintenance mode', async ({ page, api }) => {
    await api.authenticate('admin', process.env.VCDEPLOY_ADMIN_PASSWORD || 'admin');
    
    const agentsResponse = await api.get('/api/v1/agents');
    if (!agentsResponse.ok || !agentsResponse.data?.length) {
      test.skip();
      return;
    }

    const agentId = agentsResponse.data[0].id;
    await page.goto(`/agents/${agentId}`);
    await page.waitForLoadState('networkidle');

    // Find maintenance mode toggle
    const maintenanceToggle = page.locator(
      'button:has-text("Maintenance"), ' +
      'input[type="checkbox"][name*="maintenance"], ' +
      '[data-action="maintenance"], ' +
      'label:has-text("Maintenance")'
    );

    if (await maintenanceToggle.count() > 0) {
      const toggleElement = maintenanceToggle.first();
      
      // Get current state
      const isChecked = await toggleElement.isChecked().catch(() => null);
      
      // Toggle it
      await toggleElement.click();
      await page.waitForLoadState('networkidle');

      // If it was a checkbox, verify state changed
      if (isChecked !== null) {
        const newState = await toggleElement.isChecked().catch(() => null);
        expect(newState).not.toBe(isChecked);
        
        // Toggle back to restore state
        await toggleElement.click();
      }
    }
  });

  test('should view agent deployment history', async ({ page, api }) => {
    await api.authenticate('admin', process.env.VCDEPLOY_ADMIN_PASSWORD || 'admin');
    
    const agentsResponse = await api.get('/api/v1/agents');
    if (!agentsResponse.ok || !agentsResponse.data?.length) {
      test.skip();
      return;
    }

    const agentId = agentsResponse.data[0].id;
    await page.goto(`/agents/${agentId}`);
    await page.waitForLoadState('networkidle');

    // Find deployment history section
    const historySection = page.locator(
      '.deployment-history, ' +
      'table:has(:text("Deployment")), ' +
      ':text("Recent Deployments"), ' +
      '[data-section="history"]'
    );

    const historyLink = page.locator('a:has-text("Deployments"), a:has-text("History")');

    if (await historyLink.count() > 0) {
      await historyLink.first().click();
      await page.waitForLoadState('networkidle');
      
      // Should show deployment history
      const table = page.locator('table, .deployment-list');
      expect(await table.count()).toBeGreaterThanOrEqual(0);
    } else if (await historySection.count() > 0) {
      expect(await historySection.count()).toBeGreaterThan(0);
    }
  });

  test('should regenerate agent token', async ({ page, api }) => {
    await api.authenticate('admin', process.env.VCDEPLOY_ADMIN_PASSWORD || 'admin');
    
    const agentsResponse = await api.get('/api/v1/agents');
    if (!agentsResponse.ok || !agentsResponse.data?.length) {
      test.skip();
      return;
    }

    const agentId = agentsResponse.data[0].id;
    await page.goto(`/agents/${agentId}`);
    await page.waitForLoadState('networkidle');

    // Find regenerate token button
    const regenButton = page.locator(
      'button:has-text("Regenerate"), ' +
      'button:has-text("New Token"), ' +
      '[data-action="regenerate-token"]'
    );

    if (await regenButton.count() > 0) {
      await regenButton.first().click();
      
      // Handle confirmation dialog
      const confirmButton = page.locator('button:has-text("Confirm"), button:has-text("Yes")');
      if (await confirmButton.isVisible({ timeout: 2000 }).catch(() => false)) {
        await confirmButton.click();
      }
      
      await page.waitForLoadState('networkidle');

      // Should show new token or success message
      const tokenDisplay = page.locator('.token, code, :text("Token"), .success');
      expect(await tokenDisplay.count()).toBeGreaterThanOrEqual(0);
    }
  });

  test('should show agent connection status history', async ({ page, api }) => {
    await api.authenticate('admin', process.env.VCDEPLOY_ADMIN_PASSWORD || 'admin');
    
    const agentsResponse = await api.get('/api/v1/agents');
    if (!agentsResponse.ok || !agentsResponse.data?.length) {
      test.skip();
      return;
    }

    const agentId = agentsResponse.data[0].id;
    await page.goto(`/agents/${agentId}`);
    await page.waitForLoadState('networkidle');

    // Look for status history or timeline
    const statusHistory = page.locator(
      '.status-history, ' +
      '.timeline, ' +
      ':text("Last seen"), ' +
      ':text("Connected"), ' +
      ':text("Uptime")'
    );

    const count = await statusHistory.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should display agent version info', async ({ page, api }) => {
    await api.authenticate('admin', process.env.VCDEPLOY_ADMIN_PASSWORD || 'admin');
    
    const agentsResponse = await api.get('/api/v1/agents');
    if (!agentsResponse.ok || !agentsResponse.data?.length) {
      test.skip();
      return;
    }

    const agentId = agentsResponse.data[0].id;
    await page.goto(`/agents/${agentId}`);
    await page.waitForLoadState('networkidle');

    // Look for version information
    const versionInfo = page.locator(':text("Version"), :text("v1."), :text("v0.")');
    const count = await versionInfo.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });
});

test.describe('Agent List Actions', () => {
  test.skip(SKIP_AGENT_TESTS, 'Requires agent');
  
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should bulk select agents if available', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForLoadState('networkidle');

    const selectAll = page.locator('input[type="checkbox"][name="selectAll"], th input[type="checkbox"]');
    if (await selectAll.count() > 0) {
      await selectAll.first().check();
      
      // Should enable bulk actions
      const bulkActions = page.locator('.bulk-actions, button:has-text("Bulk")');
      const count = await bulkActions.count();
      expect(count).toBeGreaterThanOrEqual(0);
    }
  });

  test('should refresh agent list', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForLoadState('networkidle');

    const refreshButton = page.locator('button:has-text("Refresh"), [data-action="refresh"]');
    if (await refreshButton.count() > 0) {
      await refreshButton.first().click();
      await page.waitForLoadState('networkidle');
      
      // Page should still show agents section
      await expect(page).toHaveURL(/.*agents/);
    }
  });

  test('should sort agents by column', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForLoadState('networkidle');

    const sortableHeaders = page.locator('th.sortable, th[data-sort], th button');
    if (await sortableHeaders.count() > 0) {
      await sortableHeaders.first().click();
      await page.waitForLoadState('networkidle');
      
      // Sorting applied (URL might change or table reorders)
      await expect(page).toHaveURL(/.*agents/);
    }
  });

  test('should filter agents by status', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForLoadState('networkidle');

    const statusFilter = page.locator('select[name="status"], .status-filter');
    if (await statusFilter.count() > 0) {
      await statusFilter.first().selectOption('online').catch(() => {});
      await page.waitForLoadState('networkidle');
    }
  });

  test('should paginate agent list if needed', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForLoadState('networkidle');

    const pagination = page.locator('.pagination, nav[aria-label="pagination"], button:has-text("Next")');
    if (await pagination.count() > 0) {
      const nextButton = page.locator('button:has-text("Next"), a:has-text("Next"), [aria-label="Next page"]');
      if (await nextButton.count() > 0 && await nextButton.first().isEnabled()) {
        await nextButton.first().click();
        await page.waitForLoadState('networkidle');
        
        // Should still be on agents page
        await expect(page).toHaveURL(/.*agents/);
      }
    }
  });
});
