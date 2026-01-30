import { FullConfig } from '@playwright/test';
import * as dotenv from 'dotenv';

// Load test environment variables
dotenv.config({ path: '.env.test' });

/**
 * Global setup for Playwright tests.
 * This runs once before all tests.
 */
async function globalSetup(config: FullConfig) {
  const baseURL = config.projects[0].use.baseURL || process.env.VCDEPLOY_WEB_URL || 'http://localhost:8080';
  
  console.log('🚀 Global Setup: Starting VCDeploy UI tests');
  console.log(`   Base URL: ${baseURL}`);
  console.log(`   Parallel: ${process.env.TEST_NO_PARALLEL !== 'true'}`);
  
  // Wait for the application to be ready
  const maxRetries = 30;
  const retryDelay = 2000;
  
  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await fetch(`${baseURL}/api/v1/health`);
      if (response.ok) {
        console.log('   ✅ Application is ready');
        return;
      }
    } catch (error) {
      // Application not ready yet
    }
    
    if (i < maxRetries - 1) {
      console.log(`   ⏳ Waiting for application... (${i + 1}/${maxRetries})`);
      await new Promise(resolve => setTimeout(resolve, retryDelay));
    }
  }
  
  console.warn('   ⚠️ Application health check failed, proceeding anyway');
}

export default globalSetup;
