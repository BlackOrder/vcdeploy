import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from './base-page';

/**
 * Dashboard page object
 */
export class DashboardPage extends BasePage {
  readonly welcomeMessage: Locator;
  readonly statsCards: Locator;
  readonly recentActivity: Locator;
  readonly quickActions: Locator;
  readonly projectsLink: Locator;
  readonly deploymentsLink: Locator;
  readonly agentsLink: Locator;

  constructor(page: Page) {
    super(page);
    this.welcomeMessage = page.locator('h1, .welcome, .greeting');
    this.statsCards = page.locator('.stats-card, .stat-card, .dashboard-stat');
    this.recentActivity = page.locator('.recent-activity, .activity-list');
    this.quickActions = page.locator('.quick-actions, .action-buttons');
    this.projectsLink = page.locator('a:has-text("Projects"), [href*="projects"]');
    this.deploymentsLink = page.locator('a:has-text("Deployments"), [href*="deployments"]');
    this.agentsLink = page.locator('a:has-text("Agents"), [href*="agents"]');
  }

  /**
   * Navigate to dashboard
   */
  async goto() {
    await super.goto('/');
  }

  /**
   * Navigate to dashboard via explicit path
   */
  async gotoDashboard() {
    await super.goto('/dashboard');
  }

  /**
   * Get total number of stats cards
   */
  async getStatsCount(): Promise<number> {
    return this.statsCards.count();
  }

  /**
   * Get stat value by label
   */
  async getStatValue(label: string): Promise<string | null> {
    const stat = this.page.locator(`.stat-card:has-text("${label}"), .stats-card:has-text("${label}")`);
    const value = stat.locator('.value, .stat-value, .number');
    if (await value.isVisible({ timeout: 2000 })) {
      return value.textContent();
    }
    return null;
  }

  /**
   * Navigate to projects
   */
  async goToProjects() {
    await this.projectsLink.first().click();
    await this.page.waitForURL('**/projects');
  }

  /**
   * Navigate to deployments
   */
  async goToDeployments() {
    await this.deploymentsLink.first().click();
    await this.page.waitForURL('**/deployments');
  }

  /**
   * Navigate to agents
   */
  async goToAgents() {
    await this.agentsLink.first().click();
    await this.page.waitForURL('**/agents');
  }

  /**
   * Verify dashboard is displayed
   */
  async verifyPage() {
    // Dashboard should have some content
    await expect(this.page.locator('body')).not.toBeEmpty();
    // URL should be dashboard or root
    const url = this.page.url();
    expect(url.includes('/dashboard') || url.endsWith('/')).toBeTruthy();
  }
}
