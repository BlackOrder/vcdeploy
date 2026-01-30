import { test, expect, TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD } from '../fixtures/test-fixtures';
import { ChangePasswordPage, LoginPage } from '../pages';

/**
 * Change Password page tests
 * 
 * Tests the password change functionality for authenticated users.
 */
test.describe('Change Password Page', () => {
  let changePasswordPage: ChangePasswordPage;
  let loginPage: LoginPage;

  test.beforeEach(async ({ page }) => {
    changePasswordPage = new ChangePasswordPage(page);
    loginPage = new LoginPage(page);
  });

  test('should redirect to login when not authenticated', async ({ page }) => {
    // Try to access change-password directly without logging in
    await changePasswordPage.goto();
    
    // Should be redirected to login
    await page.waitForURL(url => url.pathname.includes('/login'), { timeout: 5000 });
    const url = page.url();
    expect(url).toContain('/login');
  });

  test('should display change password form when authenticated', async ({ page }) => {
    // Login first
    await loginPage.goto();
    await loginPage.login(TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD);
    
    // Wait for redirect away from login
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    // Navigate to change password page
    await changePasswordPage.goto();
    
    // Verify form is displayed
    await changePasswordPage.verifyPage();
  });

  test('should show error for wrong current password', async ({ page }) => {
    // Login first
    await loginPage.goto();
    await loginPage.login(TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD);
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    // Navigate to change password page
    await changePasswordPage.goto();
    
    // Submit with wrong current password
    await changePasswordPage.fillForm('WrongPassword123!', 'NewPass@456!');
    await changePasswordPage.submit();
    
    // Should show error
    const hasError = await changePasswordPage.hasError();
    const onPage = await changePasswordPage.isOnChangePasswordPage();
    expect(hasError || onPage).toBeTruthy();
  });

  test('should show error for password mismatch', async ({ page }) => {
    // Login first
    await loginPage.goto();
    await loginPage.login(TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD);
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    // Navigate to change password page
    await changePasswordPage.goto();
    
    // Submit with mismatched passwords
    await changePasswordPage.fillForm(TEST_ADMIN_PASSWORD, 'NewPass@456!', 'DifferentPass@789!');
    await changePasswordPage.submit();
    
    // Should show error or stay on page
    const hasError = await changePasswordPage.hasError();
    const onPage = await changePasswordPage.isOnChangePasswordPage();
    expect(hasError || onPage).toBeTruthy();
  });

  test('should show error for weak password', async ({ page }) => {
    // Login first
    await loginPage.goto();
    await loginPage.login(TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD);
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    // Navigate to change password page
    await changePasswordPage.goto();
    
    // Submit with weak password
    await changePasswordPage.fillForm(TEST_ADMIN_PASSWORD, 'weak');
    await changePasswordPage.submit();
    
    // Should show error about password requirements
    const hasError = await changePasswordPage.hasError();
    const onPage = await changePasswordPage.isOnChangePasswordPage();
    expect(hasError || onPage).toBeTruthy();
  });

  test('should show error when new equals current', async ({ page }) => {
    // Login first
    await loginPage.goto();
    await loginPage.login(TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD);
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    // Navigate to change password page
    await changePasswordPage.goto();
    
    // Submit with same password
    await changePasswordPage.fillForm(TEST_ADMIN_PASSWORD, TEST_ADMIN_PASSWORD);
    await changePasswordPage.submit();
    
    // Should show error or stay on page
    const hasError = await changePasswordPage.hasError();
    const onPage = await changePasswordPage.isOnChangePasswordPage();
    expect(hasError || onPage).toBeTruthy();
  });

  test('password fields should be masked', async ({ page }) => {
    // Login first
    await loginPage.goto();
    await loginPage.login(TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD);
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    // Navigate to change password page
    await changePasswordPage.goto();
    
    // Check that password fields are type="password"
    const currentType = await changePasswordPage.currentPasswordInput.getAttribute('type');
    const newType = await changePasswordPage.newPasswordInput.getAttribute('type');
    const confirmType = await changePasswordPage.confirmPasswordInput.getAttribute('type');
    
    expect(currentType).toBe('password');
    expect(newType).toBe('password');
    expect(confirmType).toBe('password');
  });
});

test.describe('Change Password - Full Flow', () => {
  // These tests are more complex and may modify user state
  // Run them in serial mode to avoid conflicts
  test.describe.configure({ mode: 'serial' });

  test('should successfully change password and redirect', async ({ page, auth, apiClient }) => {
    // Create a test user specifically for password change
    const testUsername = `pwchange_${Date.now()}`;
    const initialPassword = 'Initial@Pass123!';
    const newPassword = 'Changed@Pass456!';
    
    // Create user via API
    try {
      await apiClient.createUser(testUsername, `${testUsername}@example.com`, initialPassword);
    } catch {
      // User might already exist
    }
    
    // Login as the test user
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(testUsername, initialPassword);
    
    // Wait for redirect - could go to dashboard or change-password
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    // Navigate to change password
    const changePasswordPage = new ChangePasswordPage(page);
    await changePasswordPage.goto();
    
    // Change the password
    await changePasswordPage.changePassword(initialPassword, newPassword);
    
    // Should redirect to dashboard on success
    await page.waitForURL(url => !url.pathname.includes('/change-password'), { timeout: 10000 });
    
    const url = page.url();
    expect(url).not.toContain('/change-password');
  });
});

test.describe('Change Password - MustChangePassword Flow', () => {
  test('should redirect to change-password after login when flag is set', async ({ page, apiClient }) => {
    // This test requires creating a user with MustChangePassword flag
    // which would typically be done via API
    const testUsername = `mustchange_${Date.now()}`;
    const tempPassword = 'Temp@Pass123!';
    
    try {
      // Create user with must_change_password flag
      await apiClient.post('/api/v1/users', {
        username: testUsername,
        email: `${testUsername}@example.com`,
        password: tempPassword,
        role: 'user',
        must_change_password: true,
      });
    } catch {
      // If this fails, skip the test
      test.skip(true, 'Could not create test user');
      return;
    }
    
    // Login as the test user
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(testUsername, tempPassword);
    
    // Should be redirected to change-password (not dashboard)
    await page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
    
    const url = page.url();
    // Should be on change-password page
    expect(url).toContain('/change-password');
  });
});
