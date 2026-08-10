import { describe, it, expect, beforeAll } from 'vitest';
import fs from 'fs';
import path from 'path';

/**
 * Guards on the artifact that actually ships — the minified bundle embedded in
 * the Go binary and served as /na.js. Importing ./index cannot replace these:
 * the module graph is not what a customer's browser executes.
 */
const BUNDLE = path.resolve(__dirname, '../dist/notifuse-analytics.min.js');

describe('shipped bundle', () => {
  let code: string;

  beforeAll(() => {
    if (!fs.existsSync(BUNDLE)) {
      throw new Error(`${BUNDLE} is missing — run \`npm run build\` before the tests`);
    }
    code = fs.readFileSync(BUNDLE, 'utf-8');
  });

  it('exposes the API on window, not the module namespace', () => {
    // window.eval, not new Function(code): the bug this guards against is a
    // top-level `var NotifuseAnalytics` at real script scope overwriting the
    // global with rollup's module-namespace object. Function scope hides it,
    // because `var` never reaches window there.
    window.eval(code);

    const global = (window as unknown as Record<string, unknown>).NotifuseAnalytics as
      | Record<string, unknown>
      | undefined;

    expect(global, 'the bundle must define window.NotifuseAnalytics').toBeDefined();
    expect(typeof global!.trackGoal).toBe('function');
    expect(typeof global!.init).toBe('function');
    expect(typeof global!.getSessionId).toBe('function');
    expect(typeof global!.setDimension).toBe('function');
  });

  it('carries no Staminads branding', () => {
    // The SDK is a Staminads port; none of that naming may reach a customer's
    // browser, devtools or URLs.
    expect(code).not.toMatch(/staminads/i);
    expect(code).not.toContain('stm_');
    expect(code).not.toContain('_stm');
  });

  it('namespaces browser storage and the cross-domain param under nf', () => {
    expect(code).toContain('nf_');
    expect(code).toContain('_nf');
  });
});
