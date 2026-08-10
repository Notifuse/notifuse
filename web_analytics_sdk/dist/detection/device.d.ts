/**
 * Device detection using ua-parser-js with Client Hints support
 */
import type { DeviceInfo } from '../types';
export declare class DeviceDetector {
    private parser;
    constructor();
    /**
     * Detect device info with Client Hints (Chrome 90+)
     * Client Hints provide accurate OS versions (Win10 vs 11, macOS versions)
     * This is a SILENT API - no user prompts or permissions required
     */
    detectWithClientHints(): Promise<DeviceInfo>;
    /**
     * Synchronous detection (fallback for non-Client Hints browsers)
     */
    detect(): DeviceInfo;
    /**
     * Map ua-parser-js result to DeviceInfo
     */
    private mapResult;
    /**
     * Normalize device type
     */
    private normalizeDeviceType;
    /**
     * Normalize OS name
     */
    private normalizeOS;
    /**
     * Detect special browser types
     */
    private getBrowserType;
    /**
     * Get connection type via Network Information API
     * Only Chromium-based browsers support this (Chrome 61+, Edge 79+, Opera 48+)
     * Firefox/Safari return empty string (graceful degradation)
     */
    private getConnectionType;
    /**
     * Get timezone
     */
    private getTimezone;
}
