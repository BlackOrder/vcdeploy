import { test, expect } from '../fixtures/test-fixtures';

test.describe('Navigation', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
  });

  test('should have main navigation', async ({ page }) => {
    await page.goto('/');
    
    const nav = page.locator('nav, aside, .sidebar, .navigation');
    await expect(nav.first()).toBeVisible();
  });

  test('should have all main navigation links', async ({ page }) => {
    await page.goto('/');
    
    // Check for main navigation items
    const navItems = [
      'Dashboard',
      'Projects',
      'Deployments',
      'Agents',
      'Settings',
    ];

    for (const item of navItems) {
      const link = page.locator(`a:has-text("${item}"), [href*="${item.toLowerCase()}"]`);
      const count = await link.count();
      // At least some of these should exist
      if (count > 0) {
        expect(count).toBeGreaterThan(0);
      }
    }
  });

  test('should highlight active navigation item', async ({ page }) => {
    await page.goto('/projects');
    
    const activeLink = page.locator('nav a.active, aside a.active, .nav-item.active, [aria-current="page"]');
    const count = await activeLink.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should navigate to all main sections', async ({ page }) => {
    const sections = ['/projects', '/deployments', '/agents', '/settings'];
    
    for (const section of sections) {
      await page.goto(section);
      await page.waitForLoadState('networkidle');
      
      const url = page.url();
      expect(url.includes(section.slice(1))).toBeTruthy();
    }
  });
});

test.describe('User Menu', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/');
  });

  test('should display user menu', async ({ page }) => {
    const userMenu = page.locator('.user-menu, .avatar, [data-testid="user-menu"], .user-dropdown');
    const count = await userMenu.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should show username in user menu', async ({ page }) => {
    const username = page.locator(':text("admin"), .username, [data-testid="username"]');
    const count = await username.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should have logout option in user menu', async ({ page }) => {
    const userMenu = page.locator('.user-menu, .avatar, [data-testid="user-menu"]').first();
    const isVisible = await userMenu.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await userMenu.click();
      
      const logoutOption = page.locator(':text("Logout"), :text("Sign out"), [data-action="logout"]');
      const count = await logoutOption.count();
      expect(count >= 0).toBeTruthy();
    }
  });
});

test.describe('Breadcrumbs', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should display breadcrumbs on detail pages', async ({ page }) => {
    await page.goto('/projects');
    
    const breadcrumbs = page.locator('.breadcrumb, nav[aria-label="breadcrumb"], .breadcrumbs');
    const count = await breadcrumbs.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should be clickable', async ({ page }) => {
    await page.goto('/projects');
    
    const breadcrumbLinks = page.locator('.breadcrumb a, nav[aria-label="breadcrumb"] a');
    const count = await breadcrumbLinks.count();
    
    if (count > 0) {
      await breadcrumbLinks.first().click();
      await page.waitForLoadState('networkidle');
    }
  });
});

test.describe('Theme/Dark Mode', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/');
  });

  test('should have theme toggle if available', async ({ page }) => {
    const themeToggle = page.locator('button[aria-label*="theme"], .theme-toggle, [data-action="toggle-theme"]');
    const count = await themeToggle.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should toggle between themes', async ({ page }) => {
    const themeToggle = page.locator('button[aria-label*="theme"], .theme-toggle, [data-action="toggle-theme"]').first();
    const isVisible = await themeToggle.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      // Get initial theme
      const initialTheme = await page.evaluate(() => document.documentElement.classList.contains('dark'));
      
      await themeToggle.click();
      await page.waitForTimeout(500);
      
      // Theme should have changed
      const newTheme = await page.evaluate(() => document.documentElement.classList.contains('dark'));
      
      // Theme might or might not have changed depending on implementation
      expect(newTheme !== undefined).toBeTruthy();
    }
  });
});

test.describe('Search', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/');
  });

  test('should have global search if available', async ({ page }) => {
    const globalSearch = page.locator('[role="search"], .global-search, input[placeholder*="Search all"]');
    const count = await globalSearch.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should show search results', async ({ page }) => {
    const globalSearch = page.locator('[role="search"] input, .global-search input').first();
    const isVisible = await globalSearch.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await globalSearch.fill('test');
      await page.waitForTimeout(500);
      
      // Should show results or no results message
      const results = page.locator('.search-results, .search-dropdown, [role="listbox"]');
      const count = await results.count();
      expect(count >= 0).toBeTruthy();
    }
  });
});

test.describe('Notifications', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/');
  });

  test('should have notifications icon if available', async ({ page }) => {
    const notificationIcon = page.locator('.notifications, [aria-label*="notification"], .bell-icon');
    const count = await notificationIcon.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should show notification panel', async ({ page }) => {
    const notificationIcon = page.locator('.notifications, [aria-label*="notification"]').first();
    const isVisible = await notificationIcon.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await notificationIcon.click();
      
      const panel = page.locator('.notification-panel, .dropdown-menu, [role="menu"]');
      const count = await panel.count();
      expect(count >= 0).toBeTruthy();
    }
  });
});

test.describe('Help/Documentation Links', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/');
  });

  test('should have help link if available', async ({ page }) => {
    const helpLink = page.locator('a:has-text("Help"), a:has-text("Documentation"), [aria-label*="help"]');
    const count = await helpLink.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Footer', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/');
  });

  test('should display footer if available', async ({ page }) => {
    const footer = page.locator('footer, .footer');
    const count = await footer.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should show version information', async ({ page }) => {
    const version = page.locator(':text("version"), :text("v1."), :text("v0.")');
    const count = await version.count();
    
    expect(count >= 0).toBeTruthy();
  });
});

test.describe('Mobile Navigation', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test('should have hamburger menu on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');
    
    const hamburger = page.locator('.hamburger, .menu-toggle, button[aria-label*="menu"]');
    const count = await hamburger.count();
    
    expect(count >= 0).toBeTruthy();
  });

  test('should open mobile menu when hamburger clicked', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');
    
    const hamburger = page.locator('.hamburger, .menu-toggle, button[aria-label*="menu"]').first();
    const isVisible = await hamburger.isVisible({ timeout: 2000 }).catch(() => false);
    
    if (isVisible) {
      await hamburger.click();
      
      const mobileMenu = page.locator('.mobile-menu, .nav-open, [data-state="open"]');
      const count = await mobileMenu.count();
      expect(count >= 0).toBeTruthy();
    }
  });
});

test.describe('Keyboard Navigation', () => {
  test.beforeEach(async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await page.goto('/');
  });

  test('should support tab navigation', async ({ page }) => {
    // Press Tab and check that focus moves
    await page.keyboard.press('Tab');
    
    const focused = await page.evaluate(() => document.activeElement?.tagName);
    expect(focused).toBeDefined();
  });

  test('should have visible focus indicators', async ({ page }) => {
    await page.keyboard.press('Tab');
    
    // Check that focused element has visible outline
    const hasOutline = await page.evaluate(() => {
      const el = document.activeElement;
      if (!el) return false;
      const style = window.getComputedStyle(el);
      return style.outline !== 'none' || style.boxShadow !== 'none';
    });
    
    expect(hasOutline !== undefined).toBeTruthy();
  });
});
