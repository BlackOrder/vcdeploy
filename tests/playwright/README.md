# vcdeploy Playwright UI Tests

This directory contains end-to-end UI tests for vcdeploy using [Playwright](https://playwright.dev/).

## Prerequisites

- Node.js 18+ installed
- vcdeploy master server running (see main project README)

## Installation

```bash
# Install dependencies
npm install

# Install Playwright browsers
npx playwright install
```

## Configuration

Tests can be configured via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `E2E_BASE_URL` | `http://localhost:18080` | Base URL for vcdeploy master UI |
| `E2E_USERNAME` | `admin` | Login username for tests |
| `E2E_PASSWORD` | `admin` | Login password for tests |

## Running Tests

```bash
# Run all tests
npm test

# Run tests with headed browser (visible)
npm run test:headed

# Run tests with UI mode (interactive)
npm run test:ui

# Run tests in debug mode
npm run test:debug

# Run specific test file
npx playwright test tests/ui.spec.ts

# Run specific test by name
npx playwright test -g "should display login form"
```

## Test Structure

```
tests/
├── global.setup.ts     # Authentication setup (runs once before tests)
└── ui.spec.ts          # Main UI test suite
```

### Test Categories

1. **Login Page** - Authentication flow, form validation, error handling
2. **Dashboard Page** - Metrics display, navigation, recent activity
3. **Projects Page** - Project listing, creation, search, details
4. **Deployments Page** - Deployment list, status badges, logs
5. **Agents Page** - Agent listing, status indicators, details
6. **Settings Page** - Tab navigation, appearance settings, save functionality
7. **Theme and Appearance** - Dark mode toggle, accent colors
8. **Accessibility** - Page titles, keyboard navigation, form labels
9. **Navigation** - Page navigation, breadcrumbs, user menu, logout
10. **Responsive Design** - Mobile, tablet, and desktop viewports
11. **Error Handling** - 404 pages, unauthenticated access

## Viewing Reports

After running tests, view the HTML report:

```bash
npm run report
```

Reports are generated in `playwright-report/` directory.

## CI/CD Integration

Tests are designed to work with CI/CD pipelines:

```yaml
- name: Install Playwright
  run: |
    cd tests/playwright
    npm ci
    npx playwright install --with-deps

- name: Run Playwright tests
  run: |
    cd tests/playwright
    npm test
  env:
    E2E_BASE_URL: http://localhost:18080
    E2E_USERNAME: admin
    E2E_PASSWORD: admin
```

## Debugging Failed Tests

- Screenshots are captured on failure in `test-results/`
- Videos are recorded on retry
- Traces are collected for detailed debugging

To view a trace:
```bash
npx playwright show-trace test-results/path-to-trace.zip
```

## Adding New Tests

1. Add tests to `tests/ui.spec.ts` or create new spec files
2. Use `test.use({ storageState: '.auth/user.json' })` for authenticated tests
3. Follow the existing patterns for assertions and error handling

## Browser Support

Tests run on:
- Chromium (Desktop Chrome)
- Firefox (Desktop Firefox)
- WebKit (Desktop Safari)
- Mobile Chrome (Pixel 5 viewport)

To run on a specific browser:
```bash
npx playwright test --project=chromium
npx playwright test --project=firefox
npx playwright test --project=webkit
npx playwright test --project=mobile-chrome
```
