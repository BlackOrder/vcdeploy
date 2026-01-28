import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from './base-page';

/**
 * Users list page object
 */
export class UsersPage extends BasePage {
  readonly pageTitle: Locator;
  readonly createButton: Locator;
  readonly searchInput: Locator;
  readonly userList: Locator;
  readonly userRows: Locator;
  readonly emptyState: Locator;
  readonly roleFilter: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1:has-text("Users"), .page-title');
    this.createButton = page.locator('button:has-text("Create"), button:has-text("Add User"), a:has-text("Create User")');
    this.searchInput = page.locator('input[placeholder*="Search"], input[type="search"]');
    this.userList = page.locator('.user-list, table tbody');
    this.userRows = page.locator('.user-row, .user-item, table tbody tr');
    this.emptyState = page.locator('.empty-state, :text("No users")');
    this.roleFilter = page.locator('select[name="role"], .role-filter');
  }

  /**
   * Navigate to users page
   */
  async goto() {
    await super.goto('/users');
  }

  /**
   * Click create user button
   */
  async clickCreate() {
    await this.createButton.click();
  }

  /**
   * Search for users
   */
  async search(query: string) {
    await this.searchInput.fill(query);
    await this.page.waitForTimeout(500);
  }

  /**
   * Get user count
   */
  async getUserCount(): Promise<number> {
    return this.userRows.count();
  }

  /**
   * Click on a user by username
   */
  async clickUser(username: string) {
    await this.page.click(`tr:has-text("${username}"), .user-row:has-text("${username}")`);
  }

  /**
   * Check if user exists
   */
  async hasUser(username: string): Promise<boolean> {
    const user = this.page.locator(`:text("${username}")`);
    return user.isVisible({ timeout: 2000 });
  }

  /**
   * Delete a user by username
   */
  async deleteUser(username: string) {
    const userRow = this.page.locator(`tr:has-text("${username}"), .user-row:has-text("${username}")`);
    const deleteButton = userRow.locator('button:has-text("Delete"), [data-action="delete"]');
    await deleteButton.click();
    
    const confirmButton = this.page.locator('button:has-text("Confirm"), button:has-text("Yes")');
    if (await confirmButton.isVisible({ timeout: 2000 })) {
      await confirmButton.click();
    }
  }

  /**
   * Filter users by role
   */
  async filterByRole(role: string) {
    await this.roleFilter.selectOption(role);
  }

  /**
   * Verify users page is displayed
   */
  async verifyPage() {
    await expect(this.page).toHaveURL(/.*users/);
  }
}

/**
 * User detail page object
 */
export class UserDetailPage extends BasePage {
  readonly username: Locator;
  readonly email: Locator;
  readonly role: Locator;
  readonly editButton: Locator;
  readonly deleteButton: Locator;
  readonly apiKeysSection: Locator;
  readonly activitySection: Locator;

  constructor(page: Page) {
    super(page);
    this.username = page.locator('.username, h1, [data-field="username"]');
    this.email = page.locator('.email, [data-field="email"]');
    this.role = page.locator('.role, [data-field="role"]');
    this.editButton = page.locator('button:has-text("Edit"), a:has-text("Edit")');
    this.deleteButton = page.locator('button:has-text("Delete")');
    this.apiKeysSection = page.locator('.api-keys, [data-section="api-keys"]');
    this.activitySection = page.locator('.activity, [data-section="activity"]');
  }

  /**
   * Navigate to user detail page
   */
  async goto(userId: string) {
    await super.goto(`/users/${userId}`);
  }

  /**
   * Click edit button
   */
  async clickEdit() {
    await this.editButton.click();
  }

  /**
   * Delete the user
   */
  async deleteUser() {
    await this.deleteButton.click();
    
    const confirmButton = this.page.locator('button:has-text("Confirm"), button:has-text("Yes")');
    if (await confirmButton.isVisible({ timeout: 2000 })) {
      await confirmButton.click();
    }
  }
}

/**
 * Create/Edit user form page object
 */
export class UserFormPage extends BasePage {
  readonly usernameInput: Locator;
  readonly emailInput: Locator;
  readonly passwordInput: Locator;
  readonly confirmPasswordInput: Locator;
  readonly roleSelect: Locator;
  readonly saveButton: Locator;
  readonly cancelButton: Locator;
  readonly errorMessage: Locator;

  constructor(page: Page) {
    super(page);
    this.usernameInput = page.locator('input[name="username"], #username');
    this.emailInput = page.locator('input[name="email"], #email, input[type="email"]');
    this.passwordInput = page.locator('input[name="password"], #password, input[type="password"]').first();
    this.confirmPasswordInput = page.locator('input[name="confirmPassword"], input[name="password_confirm"], #confirmPassword');
    this.roleSelect = page.locator('select[name="role"], #role');
    this.saveButton = page.locator('button[type="submit"], button:has-text("Save"), button:has-text("Create")');
    this.cancelButton = page.locator('button:has-text("Cancel"), a:has-text("Cancel")');
    this.errorMessage = page.locator('.error, .alert-error, [role="alert"]');
  }

  /**
   * Navigate to create user page
   */
  async gotoCreate() {
    await super.goto('/users/new');
  }

  /**
   * Navigate to edit user page
   */
  async gotoEdit(userId: string) {
    await super.goto(`/users/${userId}/edit`);
  }

  /**
   * Fill the user form
   */
  async fillForm(data: {
    username?: string;
    email?: string;
    password?: string;
    confirmPassword?: string;
    role?: string;
  }) {
    if (data.username) {
      await this.usernameInput.fill(data.username);
    }
    if (data.email) {
      await this.emailInput.fill(data.email);
    }
    if (data.password) {
      await this.passwordInput.fill(data.password);
    }
    if (data.confirmPassword) {
      await this.confirmPasswordInput.fill(data.confirmPassword);
    }
    if (data.role) {
      await this.roleSelect.selectOption(data.role);
    }
  }

  /**
   * Submit the form
   */
  async submit() {
    await this.saveButton.click();
  }

  /**
   * Cancel form
   */
  async cancel() {
    await this.cancelButton.click();
  }

  /**
   * Check if there's an error
   */
  async hasError(): Promise<boolean> {
    return this.errorMessage.isVisible({ timeout: 2000 });
  }
}
