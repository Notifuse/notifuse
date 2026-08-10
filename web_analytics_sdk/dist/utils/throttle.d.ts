/**
 * Throttle utility for performance
 */
export declare function throttle<T extends (...args: Parameters<T>) => void>(fn: T, delay: number): T;
/**
 * Debounce utility
 */
export declare function debounce<T extends (...args: Parameters<T>) => void>(fn: T, delay: number): T;
