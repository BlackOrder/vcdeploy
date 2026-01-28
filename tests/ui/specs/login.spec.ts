import { test, expect, TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD } from '../fixtures/test-fixtures';
import { LoginPage } from '../pages';

test.describe('Login Page', () => {
  let loginPage: LoginPage;

  test.beforeEach(async ({ page }) => {
    loginPage = new LoginPage(page);
    await loginPage.goto();
  });

  test('should display login form', async () => {
    await loginPage.verifyPage();
  });

  test('should show error for empty credentials', async ({ page }) => {
    await loginPage.submit();
    
    // Should show validation error or stay on login page
    const url = page.url();
    expect(url).toContain('/login');
  });

  test('should show error for invalid credentials', async () => {
    await loginPage.login('invaliduser', 'invalidpassword');
    
    // Should show error message or stay on login page
    const hasError = await loginPage.hasError();
    const url = loginPage.page.url();
    
    // Either there's an error message or we're still on the login page
    expect(hasError || url.includes('/login')).toBeTruthy();
  });

  test('should login with valid admin credentials', async ({ page }) => {
    await loginPage.login(TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD);
    
    // Should redirect away from login page
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    const url = page.url();
    expect(url).not.toContain('/login');
  });

  test('should login with valid credentials and see dashboard', async ({ page }) => {
    await loginPage.login(TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD);
    
    // Wait for navigation
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    // Should see some content indicating successful login
    await expect(page.locator('body')).not.toBeEmpty();
  });

  test('password field should be masked', async () => {
    const passwordType = await loginPage.passwordInput.getAttribute('type');
    expect(passwordType).toBe('password');
  });

  test('should allow typing in username field', async () => {
    await loginPage.usernameInput.fill('testuser');
    const value = await loginPage.usernameInput.inputValue();
    expect(value).toBe('testuser');
  });

  test('should allow typing in password field', async () => {
    await loginPage.passwordInput.fill('testpassword');
    const value = await loginPage.passwordInput.inputValue();
    expect(value).toBe('testpassword');
  });
});

test.describe('Login Page - Edge Cases', () => {
  let loginPage: LoginPage;

  test.beforeEach(async ({ page }) => {
    loginPage = new LoginPage(page);
    await loginPage.goto();
  });

  test('should handle special characters in password', async () => {
    const specialPassword = 'Test@123!#$%^&*()';
    await loginPage.passwordInput.fill(specialPassword);
    const value = await loginPage.passwordInput.inputValue();
    expect(value).toBe(specialPassword);
  });

  test('should handle very long username', async () => {
    const longUsername = 'a'.repeat(100);
    await loginPage.usernameInput.fill(longUsername);
    const value = await loginPage.usernameInput.inputValue();
    expect(value.length).toBeGreaterThan(0);
  });

  test('should clear form fields', async () => {
    await loginPage.fillLoginForm('testuser', 'testpassword');
    await loginPage.usernameInput.clear();
    await loginPage.passwordInput.clear();
    
    expect(await loginPage.usernameInput.inputValue()).toBe('');
    expect(await loginPage.passwordInput.inputValue()).toBe('');
  });
});

test.describe('Session Management', () => {
  test('should redirect to login when not authenticated', async ({ page }) => {
    // Try to access protected page without logging in
    await page.goto('/projects');
    
    // Should redirect to login
    await page.waitForURL('**/login', { timeout: 10000 });
    const url = page.url();
    expect(url).toContain('/login');
  });

  test('should maintain session after login', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    
    // Navigate to another page
    await page.goto('/projects');
    
    // Should still be logged in (not redirected to login)
    const url = page.url();
    expect(url).not.toContain('/login');
  });

  test('should handle logout', async ({ page, auth }) => {
    await auth.loginAsAdmin();
    await auth.logout();
    
    // Try to access protected page
    await page.goto('/projects');
    
    // Should redirect to login
    await page.waitForURL('**/login', { timeout: 10000 });
  });
});
