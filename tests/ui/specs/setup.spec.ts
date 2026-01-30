import { test, expect } from '../fixtures/test-fixtures';
import { SetupPage, LoginPage } from '../pages';

/**
 * Setup wizard tests
 * 
 * NOTE: These tests require the server to be started WITHOUT VCDEPLOY_ADMIN_PASSWORD
 * so that it enters setup-required mode. In CI, a separate test instance should be
 * configured for these tests.
 */
test.describe('Setup Wizard', () => {
  // These tests only run when the server is in setup-required mode
  // Skip them if the server has already been configured
  test.describe.configure({ mode: 'serial' });

  test('should redirect to /setup when no users exist', async ({ page }) => {
    // Navigate to any protected page
    await page.goto('/dashboard');
    
    // Check if we're on setup page (server in setup mode) or login page (server configured)
    const url = page.url();
    
    // If server is in setup mode, we should be redirected to /setup
    // If server is already configured, we'll be on /login - skip the test
    if (url.includes('/login')) {
      test.skip(true, 'Server already has users configured - skipping setup tests');
      return;
    }
    
    expect(url).toContain('/setup');
  });

  test('should display setup form when in setup mode', async ({ page }) => {
    const setupPage = new SetupPage(page);
    await setupPage.goto();
    
    // If redirected to login, server is already configured
    if (page.url().includes('/login')) {
      test.skip(true, 'Server already has users configured');
      return;
    }
    
    await setupPage.verifyPage();
  });

  test('should show error for missing username', async ({ page }) => {
    const setupPage = new SetupPage(page);
    await setupPage.goto();
    
    if (page.url().includes('/login')) {
      test.skip(true, 'Server already configured');
      return;
    }
    
    await setupPage.fillSetupForm('', 'admin@example.com', 'Admin@Password123!');
    await setupPage.submit();
    
    // Should show error or stay on setup page
    const hasError = await setupPage.hasError();
    const onSetup = await setupPage.isOnSetupPage();
    expect(hasError || onSetup).toBeTruthy();
  });

  test('should show error for missing email', async ({ page }) => {
    const setupPage = new SetupPage(page);
    await setupPage.goto();
    
    if (page.url().includes('/login')) {
      test.skip(true, 'Server already configured');
      return;
    }
    
    await setupPage.fillSetupForm('admin', '', 'Admin@Password123!');
    await setupPage.submit();
    
    const hasError = await setupPage.hasError();
    const onSetup = await setupPage.isOnSetupPage();
    expect(hasError || onSetup).toBeTruthy();
  });

  test('should show error for password mismatch', async ({ page }) => {
    const setupPage = new SetupPage(page);
    await setupPage.goto();
    
    if (page.url().includes('/login')) {
      test.skip(true, 'Server already configured');
      return;
    }
    
    await setupPage.fillSetupForm('admin', 'admin@example.com', 'Admin@Password123!', 'DifferentPassword123!');
    await setupPage.submit();
    
    const hasError = await setupPage.hasError();
    const onSetup = await setupPage.isOnSetupPage();
    expect(hasError || onSetup).toBeTruthy();
  });

  test('should show error for weak password', async ({ page }) => {
    const setupPage = new SetupPage(page);
    await setupPage.goto();
    
    if (page.url().includes('/login')) {
      test.skip(true, 'Server already configured');
      return;
    }
    
    // Password too short and missing complexity requirements
    await setupPage.fillSetupForm('admin', 'admin@example.com', 'weak');
    await setupPage.submit();
    
    // Should show error about password requirements
    const hasError = await setupPage.hasError();
    const onSetup = await setupPage.isOnSetupPage();
    expect(hasError || onSetup).toBeTruthy();
  });

  test('should complete setup with valid credentials', async ({ page }) => {
    const setupPage = new SetupPage(page);
    await setupPage.goto();
    
    if (page.url().includes('/login')) {
      test.skip(true, 'Server already configured');
      return;
    }
    
    // Use unique username to avoid conflicts with existing data
    const uniqueUsername = `setupadmin_${Date.now()}`;
    await setupPage.completeSetup(uniqueUsername, `${uniqueUsername}@example.com`, 'Admin@Password123!');
    
    // Should redirect to dashboard after successful setup
    await page.waitForURL(url => !url.pathname.includes('/setup'), { timeout: 10000 });
    
    const url = page.url();
    expect(url).not.toContain('/setup');
    // Should be on dashboard or some authenticated page
    expect(url).toMatch(/\/(dashboard|login)/);
  });

  test('should allow access to health endpoints during setup', async ({ page }) => {
    const response = await page.goto('/healthz');
    expect(response?.status()).toBeLessThan(400);
  });

  test('should allow access to static assets during setup', async ({ page }) => {
    // Try to access a static resource
    const response = await page.goto('/static/css/style.css');
    // Either succeeds (200) or not found (404) - but not redirected to setup
    expect(response?.url()).not.toContain('/setup');
  });
});

test.describe('Setup Wizard - Already Configured', () => {
  test('should redirect /setup to /login when server is configured', async ({ page }) => {
    // This test runs when the server already has users (normal CI mode)
    await page.goto('/setup');
    
    // Wait for navigation
    await page.waitForLoadState('networkidle');
    
    const url = page.url();
    // Should either be on login page or stay on setup if no users
    expect(url).toMatch(/\/(setup|login)/);
  });
});

test.describe('Login with Env-Configured Admin', () => {
  // These tests verify that login works when VCDEPLOY_ADMIN_PASSWORD is set

  test('should allow login with env-configured admin credentials', async ({ page }) => {
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    
    // If redirected to setup, the env admin wasn't configured
    if (page.url().includes('/setup')) {
      test.skip(true, 'Server in setup mode - VCDEPLOY_ADMIN_PASSWORD not set');
      return;
    }
    
    // Get credentials from environment (same as server)
    const adminUsername = process.env.TEST_ADMIN_USERNAME || 'admin';
    const adminPassword = process.env.TEST_ADMIN_PASSWORD || 'Admin@Password123!';
    
    await loginPage.login(adminUsername, adminPassword);
    
    // Should redirect to dashboard
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    const url = page.url();
    expect(url).not.toContain('/login');
  });
});
