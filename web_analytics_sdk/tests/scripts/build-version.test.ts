/**
 * Tests for SDK build version injection
 * Verifies that __SDK_VERSION__ is correctly replaced in the built output
 */

import { describe, it, expect } from 'vitest';
import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

describe('SDK build version injection', () => {
  const apiVersionPath = path.join(__dirname, '../../../config/config.go');
  const sdkDistPath = path.join(__dirname, '../../dist');

  // Read the expected version from config/config.go
  function getExpectedVersion(): string {
    const versionContent = fs.readFileSync(apiVersionPath, 'utf-8');
    const match = versionContent.match(/VERSION\s*=\s*['"]([^'"]+)['"]/);
    if (!match) throw new Error('Could not parse VERSION');
    return match[1];
  }

  it('should inject version into UMD bundle (notifuse-analytics.min.js)', () => {
    const bundlePath = path.join(sdkDistPath, 'notifuse-analytics.min.js');

    // Skip if bundle doesn't exist (e.g., clean build state)
    if (!fs.existsSync(bundlePath)) {
      console.warn('UMD bundle not found - run npm run build first');
      return;
    }

    const expectedVersion = getExpectedVersion();
    const bundleContent = fs.readFileSync(bundlePath, 'utf-8');

    // Should NOT contain placeholder
    expect(bundleContent).not.toContain('__SDK_VERSION__');

    // Should contain the actual version string somewhere
    expect(bundleContent).toContain(expectedVersion);
  });

  it('should inject version into ESM bundle (notifuse-analytics.esm.js)', () => {
    const bundlePath = path.join(sdkDistPath, 'notifuse-analytics.esm.js');

    if (!fs.existsSync(bundlePath)) {
      console.warn('ESM bundle not found - run npm run build first');
      return;
    }

    const expectedVersion = getExpectedVersion();
    const bundleContent = fs.readFileSync(bundlePath, 'utf-8');

    // Should NOT contain placeholder
    expect(bundleContent).not.toContain('__SDK_VERSION__');

    // Should contain SDK_VERSION constant with correct version
    expect(bundleContent).toContain(`SDK_VERSION = "${expectedVersion}"`);
  });

  it('should inject version into CJS bundle (notifuse-analytics.cjs.js)', () => {
    const bundlePath = path.join(sdkDistPath, 'notifuse-analytics.cjs.js');

    if (!fs.existsSync(bundlePath)) {
      console.warn('CJS bundle not found - run npm run build first');
      return;
    }

    const expectedVersion = getExpectedVersion();
    const bundleContent = fs.readFileSync(bundlePath, 'utf-8');

    // Should NOT contain placeholder
    expect(bundleContent).not.toContain('__SDK_VERSION__');

    // Should contain the actual version string
    expect(bundleContent).toContain(expectedVersion);
  });

  it('should have consistent version across all bundles', () => {
    const expectedVersion = getExpectedVersion();
    const bundles = ['notifuse-analytics.min.js', 'notifuse-analytics.esm.js', 'notifuse-analytics.cjs.js'];
    const versions: string[] = [];

    for (const bundle of bundles) {
      const bundlePath = path.join(sdkDistPath, bundle);
      if (!fs.existsSync(bundlePath)) continue;

      const content = fs.readFileSync(bundlePath, 'utf-8');

      // Extract version from SDK_VERSION constant
      const match = content.match(/SDK_VERSION[^=]*=\s*["']([^"']+)["']/);
      if (match) {
        versions.push(match[1]);
      }
    }

    // All extracted versions should match
    expect(versions.length).toBeGreaterThan(0);
    expect(versions.every(v => v === expectedVersion)).toBe(true);
  });
});
