/**
 * Device detection using ua-parser-js with Client Hints support
 */

import * as UAParser from 'ua-parser-js';
import type { DeviceInfo } from '../types';

// Result interface from UAParser
interface UAParserResult {
  browser: { name?: string; version?: string; major?: string; type?: string };
  device: { type?: string; vendor?: string; model?: string };
  os: { name?: string; version?: string };
  engine: { name?: string; version?: string };
  cpu: { architecture?: string };
  ua: string;
}

export class DeviceDetector {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private parser: any;

  constructor() {
    this.parser = new UAParser.UAParser();
  }

  /**
   * Detect device info with Client Hints (Chrome 90+)
   * Client Hints provide accurate OS versions (Win10 vs 11, macOS versions)
   * This is a SILENT API - no user prompts or permissions required
   */
  async detectWithClientHints(): Promise<DeviceInfo> {
    try {
      // withClientHints() uses navigator.userAgentData.getHighEntropyValues()
      // This is completely silent - no browser prompts
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const result = await (this.parser as any).withClientHints();
      return this.mapResult(result);
    } catch {
      // Fallback if Client Hints unavailable or blocked
      return this.detect();
    }
  }

  /**
   * Synchronous detection (fallback for non-Client Hints browsers)
   */
  detect(): DeviceInfo {
    const result = this.parser.getResult();
    return this.mapResult(result);
  }

  /**
   * Map ua-parser-js result to DeviceInfo
   */
  private mapResult(result: UAParserResult): DeviceInfo {
    return {
      screen_width: window.screen.width,
      screen_height: window.screen.height,
      viewport_width: window.innerWidth,
      viewport_height: window.innerHeight,
      device: this.normalizeDeviceType(result.device.type),
      browser: result.browser.name || 'Unknown',
      browser_type: this.getBrowserType(result),
      os: this.normalizeOS(result.os.name, result.device.type),
      user_agent: navigator.userAgent,
      connection_type: this.getConnectionType(),
      timezone: this.getTimezone(),
      language: navigator.language || 'en',
    };
  }

  /**
   * Normalize device type
   */
  private normalizeDeviceType(type?: string): 'desktop' | 'mobile' | 'tablet' {
    switch (type) {
      case 'mobile':
        return 'mobile';
      case 'tablet':
        return 'tablet';
      default:
        // ua-parser-js returns undefined for desktop
        return 'desktop';
    }
  }

  /**
   * Normalize OS name
   */
  private normalizeOS(osName?: string, deviceType?: string): string {
    if (!osName) return 'Unknown';

    // Handle iPad specifically (iPadOS vs iOS)
    if (osName === 'iOS' && deviceType === 'tablet') {
      return 'iPadOS';
    }

    // Normalize common OS names
    const osMap: Record<string, string> = {
      'Mac OS': 'macOS',
      Windows: 'Windows',
      iOS: 'iOS',
      Android: 'Android',
      Linux: 'Linux',
      'Chrome OS': 'Chrome OS',
      Ubuntu: 'Linux',
      Fedora: 'Linux',
      Debian: 'Linux',
    };

    return osMap[osName] || osName;
  }

  /**
   * Detect special browser types
   */
  private getBrowserType(_result: UAParserResult): string | null {
    const ua = navigator.userAgent.toLowerCase();

    // Crawler/bot detection
    if (/bot|crawler|spider|scraper/i.test(ua)) {
      return 'crawler';
    }

    // In-app browsers
    if (/fbav|fban|instagram|twitter|linkedin|pinterest/i.test(ua)) {
      return 'inapp';
    }

    // Email clients
    if (/thunderbird|outlook/i.test(ua)) {
      return 'email';
    }

    // Headless/fetchers
    if (/headless|phantom|puppeteer|selenium/i.test(ua)) {
      return 'fetcher';
    }

    // CLI tools
    if (/curl|wget|httpie/i.test(ua)) {
      return 'cli';
    }

    return null;
  }

  /**
   * Get connection type via Network Information API
   * Only Chromium-based browsers support this (Chrome 61+, Edge 79+, Opera 48+)
   * Firefox/Safari return empty string (graceful degradation)
   */
  private getConnectionType(): string {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const connection = (navigator as any).connection;
    return connection?.effectiveType || '';
  }

  /**
   * Get timezone
   */
  private getTimezone(): string {
    try {
      return Intl.DateTimeFormat().resolvedOptions().timeZone;
    } catch {
      return '';
    }
  }
}
