import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from './base-page';

/**
 * Projects list page object
 */
export class ProjectsPage extends BasePage {
  readonly pageTitle: Locator;
  readonly createButton: Locator;
  readonly searchInput: Locator;
  readonly projectList: Locator;
  readonly projectCards: Locator;
  readonly emptyState: Locator;
  readonly pagination: Locator;

  constructor(page: Page) {
    super(page);
    this.pageTitle = page.locator('h1:has-text("Projects"), .page-title');
    this.createButton = page.locator('button:has-text("Create"), button:has-text("New"), a:has-text("Create Project")');
    this.searchInput = page.locator('input[placeholder*="Search"], input[type="search"], .search-input');
    this.projectList = page.locator('.project-list, .projects-grid, table tbody');
    this.projectCards = page.locator('.project-card, .project-item, table tbody tr');
    this.emptyState = page.locator('.empty-state, .no-projects, :text("No projects")');
    this.pagination = page.locator('.pagination, nav[aria-label="pagination"]');
  }

  /**
   * Navigate to projects page
   */
  async goto() {
    await super.goto('/projects');
  }

  /**
   * Click create project button
   */
  async clickCreate() {
    await this.createButton.click();
  }

  /**
   * Search for projects
   */
  async search(query: string) {
    await this.searchInput.fill(query);
    // M11 FIX: Wait for network idle instead of fixed timeout (debounce)
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Get project count
   */
  async getProjectCount(): Promise<number> {
    return this.projectCards.count();
  }

  /**
   * Click on a project by name
   */
  async clickProject(name: string) {
    await this.page.click(`.project-card:has-text("${name}"), .project-item:has-text("${name}"), tr:has-text("${name}")`);
  }

  /**
   * Check if project exists by name
   */
  async hasProject(name: string): Promise<boolean> {
    const project = this.page.locator(`:text("${name}")`);
    return project.isVisible({ timeout: 2000 });
  }

  /**
   * Delete a project by name
   */
  async deleteProject(name: string) {
    const projectRow = this.page.locator(`.project-card:has-text("${name}"), tr:has-text("${name}")`);
    const deleteButton = projectRow.locator('button:has-text("Delete"), [data-action="delete"]');
    await deleteButton.click();
    
    // Confirm deletion
    const confirmButton = this.page.locator('button:has-text("Confirm"), button:has-text("Yes"), .confirm-delete');
    if (await confirmButton.isVisible({ timeout: 2000 })) {
      await confirmButton.click();
    }
  }

  /**
   * Verify projects page is displayed
   */
  async verifyPage() {
    await expect(this.page).toHaveURL(/.*projects/);
  }
}

/**
 * Project detail page object
 */
export class ProjectDetailPage extends BasePage {
  readonly projectName: Locator;
  readonly editButton: Locator;
  readonly deleteButton: Locator;
  readonly deployButton: Locator;
  readonly tabNavigation: Locator;
  readonly deploymentsList: Locator;
  readonly settingsTab: Locator;
  readonly secretsTab: Locator;

  constructor(page: Page) {
    super(page);
    this.projectName = page.locator('h1, .project-name, .page-title');
    this.editButton = page.locator('button:has-text("Edit"), a:has-text("Edit")');
    this.deleteButton = page.locator('button:has-text("Delete")');
    this.deployButton = page.locator('button:has-text("Deploy"), a:has-text("Deploy")');
    this.tabNavigation = page.locator('.tabs, [role="tablist"]');
    this.deploymentsList = page.locator('.deployments-list, .deployment-history');
    this.settingsTab = page.locator('[role="tab"]:has-text("Settings"), a:has-text("Settings")');
    this.secretsTab = page.locator('[role="tab"]:has-text("Secrets"), a:has-text("Secrets")');
  }

  /**
   * Navigate to a project by ID
   */
  async goto(projectId: string) {
    await super.goto(`/projects/${projectId}`);
  }

  /**
   * Get project name
   */
  async getProjectName(): Promise<string | null> {
    return this.projectName.textContent();
  }

  /**
   * Click deploy button
   */
  async clickDeploy() {
    await this.deployButton.click();
  }

  /**
   * Go to settings tab
   */
  async goToSettings() {
    await this.settingsTab.click();
  }

  /**
   * Go to secrets tab
   */
  async goToSecrets() {
    await this.secretsTab.click();
  }

  /**
   * Delete the project
   */
  async deleteProject() {
    await this.deleteButton.click();
    
    // Confirm deletion
    const confirmButton = this.page.locator('button:has-text("Confirm"), button:has-text("Yes")');
    if (await confirmButton.isVisible({ timeout: 2000 })) {
      await confirmButton.click();
    }
  }
}

/**
 * Create/Edit project form page object
 */
export class ProjectFormPage extends BasePage {
  readonly nameInput: Locator;
  readonly descriptionInput: Locator;
  readonly gitRepoInput: Locator;
  readonly branchInput: Locator;
  readonly deployScriptInput: Locator;
  readonly saveButton: Locator;
  readonly cancelButton: Locator;
  readonly errorMessage: Locator;

  constructor(page: Page) {
    super(page);
    this.nameInput = page.locator('input[name="name"], #name, input[placeholder*="name" i]');
    this.descriptionInput = page.locator('textarea[name="description"], #description, textarea[placeholder*="description" i]');
    this.gitRepoInput = page.locator('input[name="git_repo_url"], input[name="gitRepoUrl"], #git_repo_url, input[placeholder*="git" i]');
    this.branchInput = page.locator('input[name="branch"], #branch, input[placeholder*="branch" i]');
    this.deployScriptInput = page.locator('textarea[name="deploy_script"], #deploy_script, textarea[placeholder*="script" i]');
    this.saveButton = page.locator('button[type="submit"], button:has-text("Save"), button:has-text("Create")');
    this.cancelButton = page.locator('button:has-text("Cancel"), a:has-text("Cancel")');
    this.errorMessage = page.locator('.error, .alert-error, [role="alert"]');
  }

  /**
   * Navigate to create project page
   */
  async gotoCreate() {
    await super.goto('/projects/new');
  }

  /**
   * Navigate to edit project page
   */
  async gotoEdit(projectId: string) {
    await super.goto(`/projects/${projectId}/edit`);
  }

  /**
   * Fill the project form
   */
  async fillForm(data: {
    name?: string;
    description?: string;
    gitRepoUrl?: string;
    branch?: string;
    deployScript?: string;
  }) {
    if (data.name) {
      await this.nameInput.fill(data.name);
    }
    if (data.description) {
      await this.descriptionInput.fill(data.description);
    }
    if (data.gitRepoUrl) {
      await this.gitRepoInput.fill(data.gitRepoUrl);
    }
    if (data.branch) {
      await this.branchInput.fill(data.branch);
    }
    if (data.deployScript) {
      await this.deployScriptInput.fill(data.deployScript);
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

  /**
   * Get error text
   */
  async getErrorText(): Promise<string | null> {
    if (await this.hasError()) {
      return this.errorMessage.textContent();
    }
    return null;
  }
}
