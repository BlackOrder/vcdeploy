import { Page, Locator, expect } from '@playwright/test';
import { BasePage } from './base-page';

/**
 * Stats page object
 */
export class StatsPage extends BasePage {
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
    this.statsCards = page.locator('.stats-card, .stat-card, .stats-stat');
    this.recentActivity = page.locator('.recent-activity, .activity-list');
    this.quickActions = page.locator('.quick-actions, .action-buttons');
    this.projectsLink = page.locator('a:has-text("Projects"), [href*="projects"]');
    this.deploymentsLink = page.locator('a:has-text("Deployments"), [href*="deployments"]');
    this.agentsLink = page.locator('a:has-text("Agents"), [href*="agents"]');
  }

  /**
   * Navigate to stats
   */
  async goto() {
    await super.goto('/');
  }

  /**
   * Navigate to stats via explicit path
   */
  async gotoStats() {
    await super.goto('/stats');
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
   * Verify stats page is displayed
   */
  async verifyPage() {
    // Stats page should have some content
    await expect(this.page.locator('body')).not.toBeEmpty();
    // URL should be stats or root
    const url = this.page.url();
    expect(url.includes('/stats') || url.endsWith('/')).toBeTruthy();
  }
}
