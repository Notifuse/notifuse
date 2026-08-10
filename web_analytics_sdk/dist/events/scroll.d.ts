/**
 * Scroll tracking
 */
export declare class ScrollTracker {
    private maxScrollPercent;
    private onMilestone;
    private lastMilestone;
    private boundHandler;
    private domReadyHandler;
    constructor();
    /**
     * Start tracking scroll
     */
    start(): void;
    /**
     * Stop tracking scroll
     */
    stop(): void;
    /**
     * Set milestone callback (25%, 50%, 75%, 100%)
     */
    setMilestoneCallback(callback: (percent: number) => void): void;
    /**
     * Get max scroll percentage
     */
    getMaxScrollPercent(): number;
    /**
     * Handle scroll event
     */
    private handleScroll;
    /**
     * Check and trigger milestone callbacks
     */
    private checkMilestones;
    /**
     * Reset scroll tracking
     */
    reset(): void;
}
