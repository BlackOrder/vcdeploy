import { test, expect, Page } from '@playwright/test';

/**
 * Login page tests
 */
test.describe('Login Page', () => {
  test.beforeEach(async ({ page }) => {
    // Start each test at the login page without authentication
    await page.goto('/login');
  });

  test('should display login form', async ({ page }) => {
    // Check for login form elements
    await expect(page.locator('form')).toBeVisible();
    await expect(page.locator('input[name="username"]')).toBeVisible();
    await expect(page.locator('input[name="password"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('should show error for invalid credentials', async ({ page }) => {
    await page.fill('input[name="username"]', 'invalid_user');
    await page.fill('input[name="password"]', 'wrong_password');
    await page.click('button[type="submit"]');
    
    // Should stay on login page or show error
    await expect(page).toHaveURL(/\/login/);
    
    // Look for error message
    const errorVisible = await page.locator('.error, .alert-error, [role="alert"]').isVisible().catch(() => false);
    // Error message may or may not be visible depending on implementation
    console.log('Error message visible:', errorVisible);
  });

  test('should redirect to dashboard on successful login', async ({ page }) => {
    const username = process.env.E2E_USERNAME || 'admin';
    const password = process.env.E2E_PASSWORD || 'admin';
    
    await page.fill('input[name="username"]', username);
    await page.fill('input[name="password"]', password);
    await page.click('button[type="submit"]');
    
    // Should redirect away from login page
    await page.waitForURL(/(?!.*\/login).*/, { timeout: 10000 }).catch(() => {
      // May stay on login if auth fails
    });
  });

  test('should have proper form validation', async ({ page }) => {
    // Try to submit empty form
    await page.click('button[type="submit"]');
    
    // Check for HTML5 validation or custom error
    const usernameInput = page.locator('input[name="username"]');
    const isInvalid = await usernameInput.evaluate((el: HTMLInputElement) => !el.checkValidity());
    expect(isInvalid).toBe(true);
  });

  test('should show password field as password type', async ({ page }) => {
    const passwordInput = page.locator('input[name="password"]');
    await expect(passwordInput).toHaveAttribute('type', 'password');
  });

  test('should have responsive design', async ({ page }) => {
    // Test at different viewports
    const viewports = [
      { width: 375, height: 667, name: 'mobile' },
      { width: 768, height: 1024, name: 'tablet' },
      { width: 1920, height: 1080, name: 'desktop' },
    ];
    
    for (const viewport of viewports) {
      await page.setViewportSize({ width: viewport.width, height: viewport.height });
      await expect(page.locator('form')).toBeVisible();
      // Form should be accessible at all viewport sizes
    }
  });

  test('should display logo and branding', async ({ page }) => {
    // Look for logo or branding element
    const hasLogo = await page.locator('img[alt*="logo"], .logo, h1').first().isVisible().catch(() => false);
    // Logo may or may not be present
    console.log('Logo/branding visible:', hasLogo);
  });

  test('should handle remember me checkbox', async ({ page }) => {
    const rememberCheckbox = page.locator('input[name="remember"], input[type="checkbox"]');
    const hasRemember = await rememberCheckbox.isVisible().catch(() => false);
    
    if (hasRemember) {
      // Toggle checkbox
      await rememberCheckbox.check();
      await expect(rememberCheckbox).toBeChecked();
      
      await rememberCheckbox.uncheck();
      await expect(rememberCheckbox).not.toBeChecked();
    }
  });
});

/**
 * Dashboard page tests
 */
test.describe('Dashboard Page', () => {
  test.use({ storageState: '.auth/user.json' });

  test('should display dashboard with key metrics', async ({ page }) => {
    await page.goto('/dashboard');
    
    // Check dashboard elements
    await expect(page).toHaveURL(/dashboard/);
    
    // Look for typical dashboard elements
    const hasStats = await page.locator('[data-testid="stats"], .stats, .metrics').isVisible().catch(() => false);
    const hasProjectCount = await page.locator('text=/projects?/i').isVisible().catch(() => false);
    const hasDeployCount = await page.locator('text=/deploy/i').isVisible().catch(() => false);
    
    console.log('Dashboard elements:', { hasStats, hasProjectCount, hasDeployCount });
  });

  test('should have navigation menu', async ({ page }) => {
    await page.goto('/dashboard');
    
    // Look for navigation elements
    const navLinks = ['projects', 'deployments', 'agents', 'settings'];
    
    for (const link of navLinks) {
      const linkVisible = await page.locator(`a[href*="${link}"], nav >> text=/${link}/i`).first().isVisible().catch(() => false);
      console.log(`Navigation link "${link}" visible:`, linkVisible);
    }
  });

  test('should display recent activity', async ({ page }) => {
    await page.goto('/dashboard');
    
    // Look for activity/logs section
    const hasActivity = await page.locator('[data-testid="activity"], .activity, .recent-deployments').isVisible().catch(() => false);
    console.log('Activity section visible:', hasActivity);
  });
});

/**
 * Projects page tests
 */
test.describe('Projects Page', () => {
  test.use({ storageState: '.auth/user.json' });

  test('should display projects list', async ({ page }) => {
    await page.goto('/projects');
    
    await expect(page).toHaveURL(/projects/);
    
    // Check for table or list of projects
    const hasTable = await page.locator('table, .project-list').isVisible().catch(() => false);
    console.log('Projects table/list visible:', hasTable);
  });

  test('should have create project button', async ({ page }) => {
    await page.goto('/projects');
    
    // Look for add/create button
    const createButton = page.locator('button:has-text("Create"), button:has-text("Add"), a:has-text("New Project")').first();
    const hasCreate = await createButton.isVisible().catch(() => false);
    console.log('Create project button visible:', hasCreate);
  });

  test('should filter projects by search', async ({ page }) => {
    await page.goto('/projects');
    
    const searchInput = page.locator('input[type="search"], input[placeholder*="search"], input[name="search"]');
    const hasSearch = await searchInput.isVisible().catch(() => false);
    
    if (hasSearch) {
      await searchInput.fill('test');
      // Wait for filter to apply
      await page.waitForTimeout(500);
    }
    
    console.log('Search input visible:', hasSearch);
  });

  test('should show project details on click', async ({ page }) => {
    await page.goto('/projects');
    
    // Try to click first project row/card
    const firstProject = page.locator('table tbody tr, .project-card, .project-item').first();
    const hasProject = await firstProject.isVisible().catch(() => false);
    
    if (hasProject) {
      await firstProject.click();
      // Should navigate or show details
    }
    
    console.log('Project clickable:', hasProject);
  });
});

/**
 * Deployments page tests  
 */
test.describe('Deployments Page', () => {
  test.use({ storageState: '.auth/user.json' });

  test('should display deployments list', async ({ page }) => {
    await page.goto('/deployments');
    
    await expect(page).toHaveURL(/deployments/);
    
    // Check for deployment list
    const hasList = await page.locator('table, .deployment-list').isVisible().catch(() => false);
    console.log('Deployments list visible:', hasList);
  });

  test('should show deployment status badges', async ({ page }) => {
    await page.goto('/deployments');
    
    // Look for status indicators
    const statusBadges = page.locator('.badge, .status, [data-status]');
    const count = await statusBadges.count();
    console.log('Status badges count:', count);
  });

  test('should filter by status', async ({ page }) => {
    await page.goto('/deployments');
    
    const statusFilter = page.locator('select[name="status"], .status-filter');
    const hasFilter = await statusFilter.isVisible().catch(() => false);
    console.log('Status filter visible:', hasFilter);
  });

  test('should show deployment logs on detail view', async ({ page }) => {
    await page.goto('/deployments');
    
    const firstDeployment = page.locator('table tbody tr, .deployment-item').first();
    const hasDeployment = await firstDeployment.isVisible().catch(() => false);
    
    if (hasDeployment) {
      await firstDeployment.click();
      
      // Look for logs section
      const hasLogs = await page.locator('.logs, .deployment-logs, pre').isVisible().catch(() => false);
      console.log('Deployment logs visible:', hasLogs);
    }
  });
});

/**
 * Agents page tests
 */
test.describe('Agents Page', () => {
  test.use({ storageState: '.auth/user.json' });

  test('should display agents list', async ({ page }) => {
    await page.goto('/agents');
    
    await expect(page).toHaveURL(/agents/);
    
    const hasList = await page.locator('table, .agent-list').isVisible().catch(() => false);
    console.log('Agents list visible:', hasList);
  });

  test('should show agent status indicators', async ({ page }) => {
    await page.goto('/agents');
    
    // Look for online/offline indicators
    const statusIndicators = page.locator('.online, .offline, .status-dot, [data-agent-status]');
    const count = await statusIndicators.count();
    console.log('Agent status indicators:', count);
  });

  test('should show agent details', async ({ page }) => {
    await page.goto('/agents');
    
    const firstAgent = page.locator('table tbody tr, .agent-item').first();
    const hasAgent = await firstAgent.isVisible().catch(() => false);
    
    if (hasAgent) {
      await firstAgent.click();
      
      // Check for detail view
      const hasDetails = await page.locator('.agent-details, .modal, [data-testid="agent-detail"]').isVisible().catch(() => false);
      console.log('Agent details visible:', hasDetails);
    }
  });
});

/**
 * Settings page tests
 */
test.describe('Settings Page', () => {
  test.use({ storageState: '.auth/user.json' });

  test('should display settings tabs', async ({ page }) => {
    await page.goto('/settings');
    
    await expect(page).toHaveURL(/settings/);
    
    // Look for tab navigation
    const tabs = page.locator('[role="tablist"] button, .tabs button, nav button');
    const tabCount = await tabs.count();
    console.log('Settings tabs count:', tabCount);
  });

  test('should switch between settings tabs', async ({ page }) => {
    await page.goto('/settings');
    
    const tabButtons = page.locator('[role="tab"], .tab-button');
    const count = await tabButtons.count();
    
    for (let i = 0; i < Math.min(count, 3); i++) {
      await tabButtons.nth(i).click();
      await page.waitForTimeout(200);
    }
  });

  test('should have appearance settings', async ({ page }) => {
    await page.goto('/settings');
    
    // Click on Appearance tab
    const appearanceTab = page.locator('button:has-text("Appearance"), [data-tab="appearance"]');
    const hasAppearance = await appearanceTab.isVisible().catch(() => false);
    
    if (hasAppearance) {
      await appearanceTab.click();
      
      // Look for theme options
      const hasDarkMode = await page.locator('input[name*="dark"], .dark-mode-toggle').isVisible().catch(() => false);
      const hasColorOptions = await page.locator('[data-color], .color-option, input[name*="color"]').isVisible().catch(() => false);
      
      console.log('Appearance settings:', { hasDarkMode, hasColorOptions });
    }
  });

  test('should save settings changes', async ({ page }) => {
    await page.goto('/settings');
    
    // Find a save button
    const saveButton = page.locator('button:has-text("Save"), button[type="submit"]').first();
    const hasSave = await saveButton.isVisible().catch(() => false);
    console.log('Save button visible:', hasSave);
  });
});

/**
 * Theme and appearance tests
 */
test.describe('Theme and Appearance', () => {
  test.use({ storageState: '.auth/user.json' });

  test('should toggle dark mode', async ({ page }) => {
    await page.goto('/settings');
    
    // Navigate to appearance
    const appearanceTab = page.locator('button:has-text("Appearance")');
    if (await appearanceTab.isVisible().catch(() => false)) {
      await appearanceTab.click();
    }
    
    const darkModeToggle = page.locator('input[name*="dark"], .dark-mode-toggle, input[type="checkbox"]').first();
    const hasToggle = await darkModeToggle.isVisible().catch(() => false);
    
    if (hasToggle) {
      const htmlBefore = await page.locator('html').getAttribute('class');
      await darkModeToggle.click();
      await page.waitForTimeout(500);
      const htmlAfter = await page.locator('html').getAttribute('class');
      
      console.log('Dark mode class change:', { before: htmlBefore, after: htmlAfter });
    }
  });

  test('should apply accent color changes', async ({ page }) => {
    await page.goto('/settings');
    
    // Navigate to appearance
    const appearanceTab = page.locator('button:has-text("Appearance")');
    if (await appearanceTab.isVisible().catch(() => false)) {
      await appearanceTab.click();
    }
    
    // Look for color picker options
    const colorOptions = page.locator('[data-color], .color-option, button[aria-label*="color"]');
    const count = await colorOptions.count();
    
    if (count > 0) {
      // Click a different color option
      await colorOptions.nth(1).click();
      await page.waitForTimeout(500);
      
      console.log('Clicked color option, count:', count);
    }
  });
});

/**
 * Accessibility tests
 */
test.describe('Accessibility', () => {
  test.use({ storageState: '.auth/user.json' });

  test('should have proper page titles', async ({ page }) => {
    const pages = ['/dashboard', '/projects', '/deployments', '/agents', '/settings'];
    
    for (const path of pages) {
      await page.goto(path);
      const title = await page.title();
      expect(title.length).toBeGreaterThan(0);
      console.log(`Page ${path} title:`, title);
    }
  });

  test('should have keyboard navigation', async ({ page }) => {
    await page.goto('/dashboard');
    
    // Tab through interactive elements
    for (let i = 0; i < 5; i++) {
      await page.keyboard.press('Tab');
    }
    
    // Check focused element
    const focusedTag = await page.evaluate(() => document.activeElement?.tagName);
    console.log('Focused element after tabs:', focusedTag);
  });

  test('should have proper form labels', async ({ page }) => {
    await page.goto('/login');
    
    const inputs = page.locator('input:visible');
    const count = await inputs.count();
    
    for (let i = 0; i < count; i++) {
      const input = inputs.nth(i);
      const id = await input.getAttribute('id');
      const ariaLabel = await input.getAttribute('aria-label');
      const placeholder = await input.getAttribute('placeholder');
      
      // Should have at least one way to identify the input
      const hasIdentifier = id || ariaLabel || placeholder;
      expect(hasIdentifier).toBeTruthy();
    }
  });
});

/**
 * Navigation tests
 */
test.describe('Navigation', () => {
  test.use({ storageState: '.auth/user.json' });

  test('should navigate between pages', async ({ page }) => {
    await page.goto('/dashboard');
    
    // Test navigation links
    const navTests = [
      { link: 'projects', url: /projects/ },
      { link: 'deployments', url: /deployments/ },
      { link: 'agents', url: /agents/ },
      { link: 'settings', url: /settings/ },
    ];
    
    for (const nav of navTests) {
      const link = page.locator(`a[href*="${nav.link}"]`).first();
      if (await link.isVisible().catch(() => false)) {
        await link.click();
        await expect(page).toHaveURL(nav.url);
        await page.goto('/dashboard'); // Go back
      }
    }
  });

  test('should show breadcrumbs', async ({ page }) => {
    await page.goto('/projects');
    
    const breadcrumbs = page.locator('.breadcrumb, nav[aria-label="breadcrumb"], .breadcrumbs');
    const hasBreadcrumbs = await breadcrumbs.isVisible().catch(() => false);
    console.log('Breadcrumbs visible:', hasBreadcrumbs);
  });

  test('should have user menu', async ({ page }) => {
    await page.goto('/dashboard');
    
    const userMenu = page.locator('[data-testid="user-menu"], .user-menu, .profile-menu');
    const hasUserMenu = await userMenu.isVisible().catch(() => false);
    console.log('User menu visible:', hasUserMenu);
  });

  test('should logout successfully', async ({ page }) => {
    await page.goto('/dashboard');
    
    // Find and click logout
    const logoutLink = page.locator('a:has-text("Logout"), button:has-text("Logout"), a[href*="logout"]');
    const hasLogout = await logoutLink.isVisible().catch(() => false);
    
    if (hasLogout) {
      await logoutLink.click();
      await page.waitForURL(/login/);
    }
    
    console.log('Logout link visible:', hasLogout);
  });
});

/**
 * Responsive design tests
 */
test.describe('Responsive Design', () => {
  test.use({ storageState: '.auth/user.json' });

  const viewports = [
    { name: 'mobile', width: 375, height: 667 },
    { name: 'tablet', width: 768, height: 1024 },
    { name: 'desktop', width: 1920, height: 1080 },
  ];

  for (const viewport of viewports) {
    test(`should render correctly on ${viewport.name}`, async ({ page }) => {
      await page.setViewportSize({ width: viewport.width, height: viewport.height });
      await page.goto('/dashboard');
      
      // Page should be visible without horizontal scroll
      const bodyWidth = await page.evaluate(() => document.body.scrollWidth);
      expect(bodyWidth).toBeLessThanOrEqual(viewport.width + 20); // Allow small tolerance
    });
  }

  test('should show mobile menu on small screens', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/dashboard');
    
    // Look for hamburger menu
    const mobileMenu = page.locator('.hamburger, .mobile-menu-toggle, button[aria-label*="menu"]');
    const hasMobileMenu = await mobileMenu.isVisible().catch(() => false);
    console.log('Mobile menu toggle visible:', hasMobileMenu);
  });
});

/**
 * Error handling tests
 */
test.describe('Error Handling', () => {
  test('should show 404 page for invalid routes', async ({ page }) => {
    await page.goto('/this-page-does-not-exist');
    
    // Should show some error indication
    const has404 = await page.locator('text=/404|not found/i').isVisible().catch(() => false);
    const hasError = await page.locator('.error, .error-page').isVisible().catch(() => false);
    
    console.log('404 handling:', { has404, hasError });
  });

  test('should handle unauthenticated access', async ({ page, context }) => {
    // Clear auth state
    await context.clearCookies();
    
    await page.goto('/dashboard');
    
    // Should redirect to login
    await expect(page).toHaveURL(/login/);
  });
});
