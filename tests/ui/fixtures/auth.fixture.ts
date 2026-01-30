/**
 * Auth fixture - Re-exports from test-fixtures for backwards compatibility
 */
export { test, expect, AuthHelper, APIHelper, TestDataGenerator } from './test-fixtures';
export type { TestFixtures } from './test-fixtures';
export { TEST_ADMIN_USERNAME, TEST_ADMIN_PASSWORD, TEST_USER_USERNAME, TEST_USER_PASSWORD, TEST_USER_EMAIL } from './test-fixtures';

// Re-export the apiClient fixture type for tests that use it
export type { APIHelper as ApiClient } from './test-fixtures';
