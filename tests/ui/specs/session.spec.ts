import { test, expect } from '../fixtures/auth.fixture';
import { LoginPage } from '../pages';

test.describe('Session Expiry', () => {
  test('should redirect to login when session expires during navigation', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    
    // Clear cookies to simulate session expiry
    await page.context().clearCookies();
    
    // Try to access protected page
    await page.goto('/projects');
    
    // Should redirect to login
    await page.waitForURL('**/login', { timeout: 10000 });
    expect(page.url()).toContain('/login');
  });

  test('should redirect to login when session cookie is invalid', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    
    // Set an invalid session cookie
    await page.context().addCookies([
      {
        name: 'session_id',
        value: 'invalid-session-token',
        domain: 'localhost',
        path: '/',
      },
    ]);
    
    // Try to access protected page
    await page.goto('/settings');
    
    // Should redirect to login
    await page.waitForURL('**/login', { timeout: 10000 });
    expect(page.url()).toContain('/login');
  });

  test('should show login page after session timeout', async ({ page, auth }) => {
    // This test simulates what happens when a session times out
    // In production, sessions have a configurable timeout
    await auth.loginAsAdmin();
    
    // Remove all session storage
    await page.evaluate(() => {
      localStorage.clear();
      sessionStorage.clear();
    });
    
    // Clear cookies
    await page.context().clearCookies();
    
    // Reload the page
    await page.reload();
    
    // Should be redirected to login
    await page.waitForURL('**/login', { timeout: 10000 });
    expect(page.url()).toContain('/login');
  });
});

test.describe('Session Persistence', () => {
  test('should maintain session across page navigation', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    
    // Navigate through multiple pages
    const pages = ['/projects', '/nodes', '/settings'];
    
    for (const pagePath of pages) {
      await page.goto(pagePath);
      // Should not be redirected to login
      expect(page.url()).not.toContain('/login');
    }
  });

  test('should maintain session after page refresh', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    
    // Navigate to a protected page
    await page.goto('/projects');
    expect(page.url()).not.toContain('/login');
    
    // Refresh the page
    await page.reload();
    
    // Should still be authenticated
    expect(page.url()).not.toContain('/login');
  });
});

test.describe('Session Security', () => {
  test('should not expose session token in URL', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    
    // Check that URL doesn't contain session identifiers
    const url = page.url();
    expect(url).not.toContain('session');
    expect(url).not.toContain('token');
    expect(url).not.toContain('sid');
  });

  test('should use httpOnly cookies for session', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    
    // JavaScript shouldn't be able to access httpOnly cookies
    const cookies = await page.evaluate(() => document.cookie);
    
    // Session cookie should not be accessible via JavaScript if httpOnly
    // This test verifies the security practice
    // Note: An empty or minimal cookie string indicates httpOnly is working
    expect(cookies).not.toContain('session_id=');
  });
});

test.describe('Logout Behavior', () => {
  test('should clear session on logout', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    
    // Verify logged in
    await page.goto('/projects');
    expect(page.url()).not.toContain('/login');
    
    // Logout
    await auth.logout();
    
    // Try to access protected page
    await page.goto('/projects');
    
    // Should redirect to login
    await page.waitForURL('**/login', { timeout: 10000 });
    expect(page.url()).toContain('/login');
  });

  test('should redirect to login after logout', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await auth.logout();
    
    // Should be on login page
    await page.waitForURL('**/login', { timeout: 10000 });
    
    const loginPage = new LoginPage(page);
    expect(await loginPage.usernameInput.isVisible()).toBeTruthy();
  });

  test('should not allow back button to access protected pages after logout', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    
    // Navigate to protected page
    await page.goto('/projects');
    
    // Logout
    await auth.logout();
    
    // Try to go back
    await page.goBack();
    
    // Should not show protected content (either redirects or shows login)
    await page.waitForTimeout(1000);
    
    // Either we're on login page or the page requires re-authentication
    const url = page.url();
    // The server should prevent access to cached protected content
    expect(url).toContain('/login');
  });
});
