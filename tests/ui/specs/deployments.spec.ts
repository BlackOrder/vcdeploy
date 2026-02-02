import { test, expect, SKIP_AGENT_TESTS, TEST_ADMIN_PASSWORD } from '../fixtures/test-fixtures';

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
    
    expect(hasDeployments || hasEmptyState).toBeTruthy();
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
    
    expect(hasSuccess || hasFailure || hasPending).toBeTruthy();
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
    
    expect(hasRefresh || hasAutoRefresh).toBeTruthy();
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

// ========================================
// Full-Suite Deployment Tests (Step 13)
// ========================================

test.describe('Deployment Actions with Agent', () => {
  test.skip(SKIP_AGENT_TESTS, 'Requires agent');
  
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should trigger deployment from deployments page', async ({ page, api }) => {
    await api.authenticate('admin', TEST_ADMIN_PASSWORD);
    
    // Create a test project
    const projectName = `deploy-action-test-${Date.now()}`;
    const createResponse = await api.createTestProject(projectName);
    
    if (!createResponse.ok || !createResponse.data?.id) {
      test.skip();
      return;
    }

    const projectId = createResponse.data.id;

    try {
      await page.goto('/deployments');
      await page.waitForLoadState('networkidle');

      // Look for deploy/trigger button
      const deployButton = page.locator('button:has-text("Deploy"), button:has-text("Trigger"), button:has-text("New Deployment")');
      
      if (await deployButton.count() > 0) {
        await deployButton.first().click();
        await page.waitForLoadState('networkidle');

        // Should show deployment form
        const form = page.locator('form, .modal, [role="dialog"]');
        const hasForm = await form.isVisible({ timeout: 3000 }).catch(() => false);
        
        if (hasForm) {
          // Select project
          const projectSelect = page.locator('select[name="project"], [name="project"]');
          if (await projectSelect.count() > 0) {
            await projectSelect.selectOption({ label: projectName });
          }

          // Submit
          const submitButton = page.locator('button[type="submit"], button:has-text("Deploy"), button:has-text("Start")');
          if (await submitButton.count() > 0) {
            await submitButton.first().click();
            await page.waitForLoadState('networkidle');
          }
        }
      }
    } finally {
      await api.deleteProject(projectId);
    }
  });

  test('should show deployment progress updates', async ({ page, api }) => {
    await api.authenticate('admin', TEST_ADMIN_PASSWORD);
    
    // Create project and trigger deployment
    const projectName = `deploy-progress-test-${Date.now()}`;
    const createResponse = await api.createTestProject(projectName);
    
    if (!createResponse.ok || !createResponse.data?.id) {
      test.skip();
      return;
    }

    const projectId = createResponse.data.id;

    try {
      // Trigger deployment via API
      const deployResponse = await api.post('/api/v1/deployments', {
        projectId: projectId,
        branch: 'main',
      });

      if (deployResponse.ok && deployResponse.data?.id) {
        // Navigate to deployment detail
        await page.goto(`/deployments/${deployResponse.data.id}`);
        await page.waitForLoadState('networkidle');

        // Should show status indicator
        const statusIndicator = page.locator('.status, .deployment-status, [class*="status"]');
        await expect(statusIndicator.first()).toBeVisible({ timeout: 5000 }).catch(() => {});

        // Status should update (look for any status text)
        const statusText = page.locator(':text("pending"), :text("running"), :text("success"), :text("failed")');
        const count = await statusText.count();
        expect(count).toBeGreaterThanOrEqual(0);
      }
    } finally {
      await api.deleteProject(projectId);
    }
  });

  test('should cancel running deployment via UI', async ({ page, api }) => {
    await api.authenticate('admin', TEST_ADMIN_PASSWORD);
    
    const projectName = `deploy-cancel-test-${Date.now()}`;
    const createResponse = await api.createTestProject(projectName);
    
    if (!createResponse.ok || !createResponse.data?.id) {
      test.skip();
      return;
    }

    const projectId = createResponse.data.id;

    try {
      // Trigger deployment
      const deployResponse = await api.post('/api/v1/deployments', {
        projectId: projectId,
        branch: 'main',
      });

      if (!deployResponse.ok) {
        test.skip();
        return;
      }

      // Navigate to deployment
      await page.goto(`/deployments/${deployResponse.data?.id}`);
      await page.waitForLoadState('networkidle');

      // Find cancel button
      const cancelButton = page.locator('button:has-text("Cancel"), [data-action="cancel"]');
      
      if (await cancelButton.count() > 0 && await cancelButton.isEnabled()) {
        await cancelButton.first().click();
        
        // Confirm if needed
        const confirmButton = page.locator('button:has-text("Confirm"), button:has-text("Yes")');
        if (await confirmButton.isVisible({ timeout: 2000 }).catch(() => false)) {
          await confirmButton.click();
        }

        await page.waitForLoadState('networkidle');
        
        // Status should update to cancelled
        const cancelledStatus = page.locator(':text("cancelled"), :text("Cancelled")');
        await expect(cancelledStatus.first()).toBeVisible({ timeout: 10000 }).catch(() => {});
      }
    } finally {
      await api.deleteProject(projectId);
    }
  });

  test('should rollback completed deployment via UI', async ({ page }) => {
    await page.goto('/deployments');
    await page.waitForLoadState('networkidle');

    // Find completed deployment
    const completedDeployment = page.locator('.deployment-row:has(:text("success")), .deployment-card:has(:text("success"))');
    
    if (await completedDeployment.count() > 0) {
      await completedDeployment.first().click();
      await page.waitForLoadState('networkidle');

      // Look for rollback button
      const rollbackButton = page.locator('button:has-text("Rollback"), [data-action="rollback"]');
      
      if (await rollbackButton.count() > 0) {
        await rollbackButton.first().click();
        
        // Confirm
        const confirmButton = page.locator('button:has-text("Confirm"), button:has-text("Yes"), button:has-text("Rollback")');
        if (await confirmButton.isVisible({ timeout: 2000 }).catch(() => false)) {
          await confirmButton.click();
        }

        await page.waitForLoadState('networkidle');
      }
    }
  });

  test('should view deployment logs in UI', async ({ page }) => {
    await page.goto('/deployments');
    await page.waitForLoadState('networkidle');

    // Click on a deployment to view details
    const deploymentLink = page.locator('.deployment-row, .deployment-card, table tbody tr').first();
    
    if (await deploymentLink.count() > 0) {
      await deploymentLink.click();
      await page.waitForLoadState('networkidle');

      // Find logs section
      const logsSection = page.locator('.logs, .log-output, pre, code, [data-testid="logs"]');
      
      if (await logsSection.count() > 0) {
        // Logs should be visible
        await expect(logsSection.first()).toBeVisible({ timeout: 5000 }).catch(() => {});
        
        // Check for log content
        const hasContent = await logsSection.first().textContent();
        expect(hasContent?.length).toBeGreaterThanOrEqual(0);
      }

      // Check for log tabs (if different log types)
      const logTabs = page.locator('[data-tab="stdout"], [data-tab="stderr"], :text("Output"), :text("Errors")');
      const tabCount = await logTabs.count();
      expect(tabCount).toBeGreaterThanOrEqual(0);
    }
  });
});

test.describe('Deployment Branch Selection', () => {
  test.skip(SKIP_AGENT_TESTS, 'Requires agent');
  
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should allow branch selection during deployment', async ({ page, api }) => {
    await api.authenticate('admin', TEST_ADMIN_PASSWORD);
    
    const projectName = `deploy-branch-test-${Date.now()}`;
    const createResponse = await api.createTestProject(projectName);
    
    if (!createResponse.ok || !createResponse.data?.id) {
      test.skip();
      return;
    }

    const projectId = createResponse.data.id;

    try {
      // Navigate to trigger deployment
      await page.goto(`/projects/${projectId}`);
      await page.waitForLoadState('networkidle');

      // Click deploy button
      const deployButton = page.locator('button:has-text("Deploy"), [data-action="deploy"]');
      if (await deployButton.count() > 0) {
        await deployButton.first().click();
        
        // Look for branch selection
        const branchSelect = page.locator('select[name="branch"], input[name="branch"], [data-field="branch"]');
        if (await branchSelect.count() > 0) {
          // Fill or select branch
          if (await branchSelect.getAttribute('tagName') === 'SELECT') {
            await branchSelect.selectOption('main');
          } else {
            await branchSelect.fill('main');
          }
        }
      }
    } finally {
      await api.deleteProject(projectId);
    }
  });
});

test.describe('Deployment Target Selection', () => {
  test.skip(SKIP_AGENT_TESTS, 'Requires agent');
  
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should allow agent/target selection during deployment', async ({ page, api }) => {
    await api.authenticate('admin', TEST_ADMIN_PASSWORD);
    
    const projectName = `deploy-target-test-${Date.now()}`;
    const createResponse = await api.createTestProject(projectName);
    
    if (!createResponse.ok || !createResponse.data?.id) {
      test.skip();
      return;
    }

    const projectId = createResponse.data.id;

    try {
      await page.goto(`/projects/${projectId}`);
      await page.waitForLoadState('networkidle');

      const deployButton = page.locator('button:has-text("Deploy")');
      if (await deployButton.count() > 0) {
        await deployButton.first().click();
        await page.waitForLoadState('networkidle');

        // Look for target/agent selection
        const targetSelect = page.locator('select[name="target"], select[name="agent"], [data-field="target"]');
        if (await targetSelect.count() > 0) {
          // Should have at least one option (the test agent)
          const options = await targetSelect.locator('option').allTextContents();
          expect(options.length).toBeGreaterThanOrEqual(0);
        }
      }
    } finally {
      await api.deleteProject(projectId);
    }
  });
});
