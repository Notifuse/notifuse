/**
 * Stealth script to avoid bot detection in E2E tests.
 * Removes automation indicators that trigger the SDK's bot detection.
 */
export const stealthScript = `
  // Delete webdriver property
  Object.defineProperty(navigator, 'webdriver', {
    get: () => undefined,
  });

  // Ensure plugins exist (Chrome-like)
  if (!navigator.plugins || navigator.plugins.length === 0) {
    Object.defineProperty(navigator, 'plugins', {
      get: () => [1, 2, 3, 4, 5],
    });
  }

  // Ensure languages exist
  if (!navigator.languages || navigator.languages.length === 0) {
    Object.defineProperty(navigator, 'languages', {
      get: () => ['en-US', 'en'],
    });
  }

  // Add chrome object if missing
  if (!window.chrome) {
    window.chrome = {
      runtime: {},
    };
  }
`;
