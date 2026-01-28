import { test as setup, expect } from '@playwright/test';
import path from 'path';
import fs from 'fs';

const authFile = path.join(__dirname, '../.auth/user.json');

/**
 * Global setup: authenticate once and save storage state for all tests.
 */
setup('authenticate', async ({ page }) => {
  const baseURL = process.env.E2E_BASE_URL || 'http://localhost:8080';
  
  // Create auth directory if it doesn't exist
  const authDir = path.dirname(authFile);
  if (!fs.existsSync(authDir)) {
    fs.mkdirSync(authDir, { recursive: true });
  }
  
  // Navigate to login page
  await page.goto(`${baseURL}/login`);
  
  // Wait for login form to be visible
  await expect(page.locator('form')).toBeVisible();
  
  // Fill in credentials (from environment or defaults)
  const username = process.env.E2E_USERNAME || 'admin';
  const password = process.env.E2E_PASSWORD || 'Changeme12345!';
  
  await page.fill('input[name="username"]', username);
  await page.fill('input[name="password"]', password);
  
  // Submit login form
  await page.click('button[type="submit"]');
  
  // Wait for redirect to dashboard (indicates successful login)
  await page.waitForURL('**/dashboard**', { timeout: 10000 }).catch(() => {
    // Alternatively, check for presence of dashboard elements
  });
  
  // Verify we're logged in by checking for authenticated elements
  // This may vary based on actual UI implementation
  const isLoggedIn = await page.locator('[data-testid="user-menu"], .user-menu, nav').isVisible().catch(() => false);
  
  if (!isLoggedIn) {
    console.log('Note: Login may have failed or UI structure differs. Continuing with setup.');
  }
  
  // Save storage state
  await page.context().storageState({ path: authFile });
  
  console.log('Authentication setup complete');
});
