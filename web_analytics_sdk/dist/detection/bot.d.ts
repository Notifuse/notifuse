/**
 * Bot and crawler detection
 */
declare global {
    interface Window {
        chrome?: unknown;
    }
}
/**
 * Check if the current user is a bot/crawler
 */
export declare function isBot(): boolean;
/**
 * Get bot confidence score (0-100)
 * Higher = more likely to be a bot
 */
export declare function getBotScore(): number;
