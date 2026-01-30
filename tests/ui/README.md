# VCDeploy UI Tests

Playwright-based end-to-end UI tests for VCDeploy.

## Setup

1. Install dependencies:
   ```bash
   cd tests/ui
   npm install
   ```

2. Install browsers:
   ```bash
   npx playwright install
   ```

3. Copy environment file and adjust values:
   ```bash
   cp .env.test .env.test.local
   ```

## Running Tests

### Run all tests
```bash
npm test
```

### Run tests with browser UI visible
```bash
npm run test:headed
```

### Run tests with debugging
```bash
npm run test:debug
```

### Run specific browser only
```bash
npm run test:chromium
npm run test:firefox
npm run test:webkit
```

### Run mobile tests
```bash
npm run test:mobile
```

### Interactive UI mode
```bash
npm run test:ui
```

### View test report
```bash
npm run report
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `VCDEPLOY_WEB_URL` | Base URL of the web application | `http://localhost:8080` |
| `VCDEPLOY_API_URL` | Base URL of the API | `http://localhost:8080/api` |
| `TEST_ADMIN_USERNAME` | Admin username for tests | `admin` |
| `TEST_ADMIN_PASSWORD` | Admin password for tests | `Admin@Password123!` |
| `TEST_NO_PARALLEL` | Disable parallel tests | `false` |
| `CI` | CI environment flag | (auto-detected) |

## Project Structure

```
tests/ui/
├── playwright.config.ts    # Playwright configuration
├── package.json            # Dependencies
├── tsconfig.json           # TypeScript configuration
├── .env.test               # Environment variables template
├── global-setup.ts         # Global setup (runs before all tests)
├── global-teardown.ts      # Global teardown (runs after all tests)
├── fixtures/
│   └── test-fixtures.ts    # Custom test fixtures and helpers
├── pages/
│   ├── index.ts            # Page object exports
│   ├── base-page.ts        # Base page object
│   ├── login-page.ts       # Login page object
│   ├── dashboard-page.ts   # Dashboard page object
│   ├── projects-page.ts    # Projects page objects
│   └── users-page.ts       # Users page objects
└── specs/
    ├── login.spec.ts       # Login tests
    ├── dashboard.spec.ts   # Dashboard tests
    ├── projects.spec.ts    # Project CRUD tests
    ├── users.spec.ts       # User management tests
    ├── agents.spec.ts      # Agent tests
    ├── deployments.spec.ts # Deployment tests
    ├── secrets.spec.ts     # Secret management tests
    ├── api-keys.spec.ts    # API key tests
    ├── settings.spec.ts    # Settings tests
    ├── audit.spec.ts       # Audit log tests
    ├── navigation.spec.ts  # Navigation tests
    └── common.spec.ts      # Common UI element tests
```

## Writing Tests

### Using Page Objects
```typescript
import { test, expect } from '../fixtures/test-fixtures';
import { LoginPage } from '../pages';

test('should login successfully', async ({ page }) => {
  const loginPage = new LoginPage(page);
  await loginPage.goto();
  await loginPage.login('admin', 'admin');
  await expect(page).not.toHaveURL(/.*login/);
});
```

### Using Fixtures
```typescript
import { test, expect } from '../fixtures/test-fixtures';

test('should be authenticated', async ({ page, auth }) => {
  await auth.loginAsAdmin();
  await page.goto('/projects');
  await expect(page).not.toHaveURL(/.*login/);
});
```

### Using API Helper
```typescript
import { test, expect } from '../fixtures/test-fixtures';

test('should create and delete project', async ({ page, auth, api }) => {
  // Login
  await auth.loginAsAdmin();
  
  // Create project via API
  await api.authenticate('admin', 'admin');
  const project = await api.createTestProject('test-project');
  
  // Test UI
  await page.goto('/projects');
  
  // Cleanup
  if (project.data?.id) {
    await api.deleteProject(project.data.id);
  }
});
```

## CI Integration

Tests are configured to run in CI environments with:
- Single worker for stability
- Retry on failure (2 retries)
- JUnit XML output for test results
- HTML report for debugging

## Debugging Failed Tests

1. View HTML report:
   ```bash
   npm run report
   ```

2. Check `test-results/` directory for:
   - Screenshots on failure
   - Videos of failed tests
   - Traces for debugging

3. Use debug mode:
   ```bash
   npm run test:debug
   ```

4. Use Playwright Inspector:
   ```bash
   PWDEBUG=1 npm test
   ```

## Code Generation

Generate test code by recording browser actions:
```bash
npm run codegen
```
