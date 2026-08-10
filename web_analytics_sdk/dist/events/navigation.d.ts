/**
 * SPA Navigation tracking
 * Detects pushState, replaceState, popstate, and hashchange
 */
export declare class NavigationTracker {
    private currentUrl;
    private onNavigate;
    private originalPushState;
    private originalReplaceState;
    constructor();
    /**
     * Start tracking navigation
     */
    start(): void;
    /**
     * Stop tracking navigation
     */
    stop(): void;
    /**
     * Set navigation callback
     */
    setNavigationCallback(callback: (newUrl: string) => void): void;
    /**
     * Patch History API
     */
    private patchHistory;
    /**
     * Restore original History API
     */
    private restoreHistory;
    /**
     * Handle navigation event
     */
    private handleNavigation;
    /**
     * Get current URL
     */
    getCurrentUrl(): string;
}
