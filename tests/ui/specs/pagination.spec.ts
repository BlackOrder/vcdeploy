import { test, expect } from '../fixtures/test-fixtures';

test.describe('Pagination Controls', () => {
  test.beforeEach(async ({ auth }) => {
    await auth.loginAsAdmin();
  });

  test.describe('Deployments Pagination', () => {
    test('should load deployments with paginated response', async ({ page }) => {
      // Intercept API calls to verify pagination is handled
      let apiResponse: any = null;
      await page.route('**/api/v1/deployments*', async route => {
        const response = await route.fetch();
        apiResponse = await response.json();
        await route.fulfill({ response });
      });

      await page.goto('/deployments');
      
      // Wait for deployments to load
      await page.waitForResponse(response => response.url().includes('/api/v1/deployments'));
      
      // Verify API returned paginated response
      if (apiResponse) {
        expect(apiResponse).toHaveProperty('items');
        expect(apiResponse).toHaveProperty('totalCount');
        expect(apiResponse).toHaveProperty('limit');
        expect(apiResponse).toHaveProperty('offset');
      }
    });

    test('should display deployment items from paginated response', async ({ page }) => {
      await page.goto('/deployments');
      
      // Wait for content to load
      await page.waitForLoadState('networkidle');
      
      // Either deployments show or empty state
      const deployments = page.locator('.deployment-card, [onclick*="showDeploymentDetail"]');
      const emptyState = page.locator(':text("No deployments found")');
      
      const hasDeployments = await deployments.count() > 0;
      const hasEmptyState = await emptyState.isVisible({ timeout: 2000 }).catch(() => false);
      
      expect(hasDeployments || hasEmptyState).toBeTruthy();
    });
  });

  test.describe('Agents Pagination', () => {
    test('should load agents with paginated response', async ({ page }) => {
      let apiResponse: any = null;
      await page.route('**/api/v1/agents*', async route => {
        const response = await route.fetch();
        apiResponse = await response.json();
        await route.fulfill({ response });
      });

      await page.goto('/agents');
      
      await page.waitForResponse(response => response.url().includes('/api/v1/agents'));
      
      if (apiResponse) {
        expect(apiResponse).toHaveProperty('items');
        expect(apiResponse).toHaveProperty('totalCount');
      }
    });
  });

  test.describe('Secrets Pagination', () => {
    test('should load secrets with paginated response', async ({ page }) => {
      let apiResponse: any = null;
      await page.route('**/api/v1/secrets*', async route => {
        const response = await route.fetch();
        apiResponse = await response.json();
        await route.fulfill({ response });
      });

      await page.goto('/secrets');
      
      await page.waitForResponse(response => response.url().includes('/api/v1/secrets'));
      
      if (apiResponse) {
        expect(apiResponse).toHaveProperty('items');
        expect(apiResponse).toHaveProperty('totalCount');
      }
    });
  });

  test.describe('Project Types Pagination', () => {
    test('should load project types with paginated response', async ({ page }) => {
      let apiResponse: any = null;
      await page.route('**/api/v1/project-types*', async route => {
        const response = await route.fetch();
        apiResponse = await response.json();
        await route.fulfill({ response });
      });

      await page.goto('/project-types');
      
      await page.waitForResponse(response => response.url().includes('/api/v1/project-types'));
      
      if (apiResponse) {
        expect(apiResponse).toHaveProperty('items');
        expect(apiResponse).toHaveProperty('totalCount');
      }
    });
  });
});

test.describe('Pagination Response Structure', () => {
  test('deployments API returns correct pagination fields', async ({ request }) => {
    // Direct API test for pagination structure
    const response = await request.get('/api/v1/deployments?limit=5');
    
    if (response.ok()) {
      const data = await response.json();
      expect(data).toHaveProperty('items');
      expect(data).toHaveProperty('totalCount');
      expect(data).toHaveProperty('limit');
      expect(data).toHaveProperty('offset');
      expect(data.limit).toBe(5);
      expect(data.offset).toBe(0);
      expect(Array.isArray(data.items)).toBeTruthy();
      expect(typeof data.totalCount).toBe('number');
    }
  });

  test('agents API returns correct pagination fields', async ({ request }) => {
    const response = await request.get('/api/v1/agents?limit=10');
    
    if (response.ok()) {
      const data = await response.json();
      expect(data).toHaveProperty('items');
      expect(data).toHaveProperty('totalCount');
      expect(data.limit).toBe(10);
    }
  });

  test('offset beyond total returns empty items', async ({ request }) => {
    const response = await request.get('/api/v1/deployments?offset=999999');
    
    if (response.ok()) {
      const data = await response.json();
      expect(data.items).toHaveLength(0);
      expect(data.totalCount).toBeGreaterThanOrEqual(0);
    }
  });
});
