/**
 * Tests for sync-version.cjs script
 * Verifies that the script correctly reads VERSION from config/config.go
 * and syncs it to sdk/package.json
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';
import { createRequire } from 'module';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const require = createRequire(import.meta.url);
const { toPackageVersion } = require('../../scripts/sync-version.cjs');

// npm rejects anything semver cannot parse, so `npm publish` fails outright on
// a two-part version like the 38.0 the Notifuse release uses.
const SEMVER = /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/;

describe('sync-version script', () => {
  const apiVersionPath = path.join(__dirname, '../../../config/config.go');
  const sdkPackageJsonPath = path.join(__dirname, '../../package.json');

  let originalVersionContent: string;
  let originalPackageJson: string;

  beforeEach(() => {
    // Save original files
    originalVersionContent = fs.readFileSync(apiVersionPath, 'utf-8');
    originalPackageJson = fs.readFileSync(sdkPackageJsonPath, 'utf-8');
  });

  afterEach(() => {
    // Restore original files
    fs.writeFileSync(apiVersionPath, originalVersionContent);
    fs.writeFileSync(sdkPackageJsonPath, originalPackageJson);
  });

  it('should read VERSION from config/config.go', () => {
    const versionContent = fs.readFileSync(apiVersionPath, 'utf-8');
    const match = versionContent.match(/VERSION\s*=\s*['"]([^'"]+)['"]/);

    expect(match).not.toBeNull();
    expect(match![1]).toBeDefined();
    expect(match![1]).toMatch(/^\d+\.\d+(\.\d+)?$/);
  });

  it('should publish a semver version derived from the release', () => {
    const versionContent = fs.readFileSync(apiVersionPath, 'utf-8');
    const versionMatch = versionContent.match(/VERSION\s*=\s*['"]([^'"]+)['"]/);
    const apiVersion = versionMatch![1];

    const packageJson = JSON.parse(fs.readFileSync(sdkPackageJsonPath, 'utf-8'));

    // sync-version runs on prebuild, so the committed manifest must already be
    // publishable and traceable back to the release.
    expect(packageJson.version).toMatch(SEMVER);
    expect(packageJson.version).toBe(toPackageVersion(apiVersion));
  });

  it('should pad a vMAJOR.minor release to semver and leave other shapes alone', () => {
    expect(toPackageVersion('38.0')).toBe('38.0.0');
    expect(toPackageVersion('38.12')).toBe('38.12.0');
    // Already publishable: never rewrite what semver accepts.
    expect(toPackageVersion('39.0.1')).toBe('39.0.1');
    expect(toPackageVersion('1.0.0-beta')).toBe('1.0.0-beta');
  });

  it('should parse various version formats correctly', () => {
    const testCases = [
      { input: "export const VERSION = '3.0.0';", expected: '3.0.0' },
      { input: 'export const VERSION = "4.1.2";', expected: '4.1.2' },
      { input: "export const VERSION='10.20.30';", expected: '10.20.30' },
      { input: 'const VERSION = "1.0.0-beta";', expected: '1.0.0-beta' },
    ];

    for (const { input, expected } of testCases) {
      const match = input.match(/VERSION\s*=\s*['"]([^'"]+)['"]/);
      expect(match).not.toBeNull();
      expect(match![1]).toBe(expected);
    }
  });
});
