import { FullConfig } from '@playwright/test';

/**
 * Global teardown for Playwright tests.
 * This runs once after all tests complete.
 */
async function globalTeardown(config: FullConfig) {
  console.log('🏁 Global Teardown: VCDeploy UI tests completed');
  
  // Cleanup any test data if needed
  // This could include:
  // - Deleting test users
  // - Removing test projects
  // - Cleaning up test secrets
  
  // For now, we rely on the test database being reset between runs
}

export default globalTeardown;
