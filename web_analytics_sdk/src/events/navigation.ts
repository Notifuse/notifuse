/**
 * SPA Navigation tracking
 * Detects pushState, replaceState, popstate, and hashchange
 */

export class NavigationTracker {
  private currentUrl: string;
  private onNavigate: ((newUrl: string) => void) | null = null;
  private originalPushState: typeof history.pushState | null = null;
  private originalReplaceState: typeof history.replaceState | null = null;

  constructor() {
    this.currentUrl = window.location.href;
  }

  /**
   * Start tracking navigation
   */
  start(): void {
    this.patchHistory();
    window.addEventListener('popstate', this.handleNavigation);
    window.addEventListener('hashchange', this.handleNavigation);
  }

  /**
   * Stop tracking navigation
   */
  stop(): void {
    this.restoreHistory();
    window.removeEventListener('popstate', this.handleNavigation);
    window.removeEventListener('hashchange', this.handleNavigation);
  }

  /**
   * Set navigation callback
   */
  setNavigationCallback(callback: (newUrl: string) => void): void {
    this.onNavigate = callback;
  }

  /**
   * Patch History API
   */
  private patchHistory(): void {
    this.originalPushState = history.pushState;
    this.originalReplaceState = history.replaceState;

    history.pushState = (...args: Parameters<typeof history.pushState>) => {
      this.originalPushState?.apply(history, args);
      this.handleNavigation();
    };

    history.replaceState = (...args: Parameters<typeof history.replaceState>) => {
      this.originalReplaceState?.apply(history, args);
      this.handleNavigation();
    };
  }

  /**
   * Restore original History API
   */
  private restoreHistory(): void {
    if (this.originalPushState) {
      history.pushState = this.originalPushState;
    }
    if (this.originalReplaceState) {
      history.replaceState = this.originalReplaceState;
    }
  }

  /**
   * Handle navigation event
   */
  private handleNavigation = (): void => {
    const newUrl = window.location.href;

    if (newUrl !== this.currentUrl) {
      this.currentUrl = newUrl;

      if (this.onNavigate) {
        this.onNavigate(newUrl);
      }
    }
  };

  /**
   * Get current URL
   */
  getCurrentUrl(): string {
    return this.currentUrl;
  }
}
