import { test as base, expect, Page } from '@playwright/test';

// Test credentials - these MUST match the VCDEPLOY_ADMIN_PASSWORD set on the server
// The fallback is only for local development; CI must set these explicitly
export const TEST_ADMIN_USERNAME = process.env.TEST_ADMIN_USERNAME || 'admin';
export const TEST_ADMIN_PASSWORD = process.env.TEST_ADMIN_PASSWORD || 'Admin@Password123!';

// Test user for non-admin tests
export const TEST_USER_USERNAME = 'testuser';
export const TEST_USER_PASSWORD = 'TestUser12345!';
export const TEST_USER_EMAIL = 'testuser@example.com';

// Skip flags for conditional test execution
export const SKIP_AGENT_TESTS = process.env.SKIP_AGENT_TESTS === '1';
export const SKIP_TARGET_TESTS = process.env.SKIP_TARGET_TESTS === '1';

/**
 * Skip the test if agent tests are disabled
 */
export function skipIfNoAgent(test: typeof base) {
  test.skip(SKIP_AGENT_TESTS, 'Skipping: SKIP_AGENT_TESTS is set');
}

/**
 * Skip the test if target/SSH tests are disabled
 */
export function skipIfNoTarget(test: typeof base) {
  test.skip(SKIP_TARGET_TESTS, 'Skipping: SKIP_TARGET_TESTS is set');
}

/**
 * Authentication helper functions
 */
export class AuthHelper {
  constructor(private page: Page) {}

  /**
   * Login with the given credentials
   */
  async login(username: string, password: string) {
    await this.page.goto('/login');
    
    // Check if we were redirected to setup (server has no users configured)
    if (this.page.url().includes('/setup')) {
      throw new Error(
        'Server redirected to /setup - no admin user configured. ' +
        'Ensure VCDEPLOY_ADMIN_PASSWORD is set when starting the server.'
      );
    }
    
    await this.page.fill('input[name="username"], input[id="username"], #username', username);
    await this.page.fill('input[name="password"], input[id="password"], #password', password);
    await this.page.click('button[type="submit"]');
    
    // Wait for successful login (redirect away from login page)
    await this.page.waitForURL(url => !url.pathname.includes('/login'), { timeout: 10000 });
  }

  /**
   * Login as admin
   */
  async loginAsAdmin() {
    await this.login(TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD);
  }

  /**
   * Login as test user
   */
  async loginAsUser() {
    await this.login(TEST_USER_USERNAME, TEST_USER_PASSWORD);
  }

  /**
   * Logout the current user
   */
  async logout() {
    // Try common logout methods
    try {
      // Look for logout button/link
      const logoutButton = this.page.locator('button:has-text("Logout"), a:has-text("Logout"), [data-testid="logout"]');
      if (await logoutButton.isVisible({ timeout: 2000 })) {
        await logoutButton.click();
        await this.page.waitForURL('**/login');
        return;
      }

      // Try user menu dropdown
      const userMenu = this.page.locator('[data-testid="user-menu"], .user-menu, .avatar');
      if (await userMenu.isVisible({ timeout: 2000 })) {
        await userMenu.click();
        await this.page.click('text=Logout');
        await this.page.waitForURL('**/login');
        return;
      }
    } catch {
      // Fallback: just navigate to login
      await this.page.goto('/login');
    }
  }

  /**
   * Check if currently logged in
   */
  async isLoggedIn(): Promise<boolean> {
    const url = this.page.url();
    return !url.includes('/login');
  }
}

/**
 * API helper for direct API calls during tests
 */
export class APIHelper {
  private baseURL: string;
  private authToken: string | null = null;

  constructor(baseURL: string) {
    this.baseURL = baseURL;
  }

  /**
   * Authenticate and get API token
   */
  async authenticate(username: string, password: string) {
    const response = await fetch(`${this.baseURL}/api/v1/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });

    if (!response.ok) {
      throw new Error(`Authentication failed: ${response.status}`);
    }

    const data = await response.json();
    this.authToken = data.token;
    return data;
  }

  /**
   * Make an authenticated API request
   */
  async request(method: string, path: string, body?: unknown) {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };

    if (this.authToken) {
      headers['Authorization'] = `Bearer ${this.authToken}`;
    }

    const response = await fetch(`${this.baseURL}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });

    return {
      status: response.status,
      ok: response.ok,
      data: response.ok ? await response.json().catch(() => null) : null,
    };
  }

  /**
   * Create a test user via API
   */
  async createTestUser(username: string, email: string, password: string, role: string = 'user') {
    return this.request('POST', '/api/v1/users', {
      username,
      email,
      password,
      role,
    });
  }

  /**
   * Alias for createTestUser - used by some tests
   */
  async createUser(username: string, email: string, password: string, role: string = 'user') {
    return this.createTestUser(username, email, password, role);
  }

  /**
   * Generic POST request - alias for request('POST', ...)
   */
  async post(path: string, body?: unknown) {
    return this.request('POST', path, body);
  }

  /**
   * Generic GET request
   */
  async get(path: string) {
    return this.request('GET', path);
  }

  /**
   * Delete a user via API
   */
  async deleteUser(userId: string) {
    return this.request('DELETE', `/api/v1/users/${userId}`);
  }

  /**
   * Create a test project via API
   */
  async createTestProject(name: string, gitRepoUrl?: string) {
    return this.request('POST', '/api/v1/projects', {
      name,
      git_repo_url: gitRepoUrl || 'https://github.com/example/test.git',
    });
  }

  /**
   * Delete a project via API
   */
  async deleteProject(projectId: string) {
    return this.request('DELETE', `/api/v1/projects/${projectId}`);
  }
}

/**
 * Test data generator
 */
export class TestDataGenerator {
  private static counter = 0;

  /**
   * Generate a unique test ID
   */
  static uniqueId(): string {
    return `${Date.now()}-${++this.counter}`;
  }

  /**
   * Generate a unique username
   */
  static username(): string {
    return `testuser-${this.uniqueId()}`;
  }

  /**
   * Generate a unique email
   */
  static email(): string {
    return `test-${this.uniqueId()}@example.com`;
  }

  /**
   * Generate a unique project name
   */
  static projectName(): string {
    return `test-project-${this.uniqueId()}`;
  }

  /**
   * Generate a unique secret name
   */
  static secretName(): string {
    return `test-secret-${this.uniqueId()}`;
  }

  /**
   * Generate a unique API key name
   */
  static apiKeyName(): string {
    return `test-apikey-${this.uniqueId()}`;
  }
}

/**
 * Extended test fixture with helpers
 */
export interface TestFixtures {
  auth: AuthHelper;
  api: APIHelper;
  apiClient: APIHelper;  // Alias for api
  testData: typeof TestDataGenerator;
}

/**
 * Extended test with custom fixtures
 */
export const test = base.extend<TestFixtures>({
  auth: async ({ page }, use) => {
    const auth = new AuthHelper(page);
    await use(auth);
  },

  api: async ({ baseURL }, use) => {
    const api = new APIHelper(baseURL || 'http://localhost:9000');
    await use(api);
  },

  // Alias for api - some tests use apiClient
  apiClient: async ({ api }, use) => {
    await use(api);
  },

  testData: async ({}, use) => {
    await use(TestDataGenerator);
  },
});

export { expect };
