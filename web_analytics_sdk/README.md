# NotifuseAnalytics Web SDK v5.0

Ultra-reliable web analytics SDK for tracking **TimeScore** metrics with millisecond precision.

## Mission Critical

- **Zero Data Loss**: Every session MUST be captured and transmitted
- **Exact Duration**: Focus time measured with millisecond precision, counting only truly active engagement

## Features

- **Focus State Machine**: FOCUSED → BLURRED → HIDDEN states with precise transitions
- **Multi-Channel Transmission**: Beacon → Fetch → Offline Queue (never lose data)
- **localStorage + Memory Fallback**: Simple, reliable storage (Safari Private Mode safe)
- **SPA Support**: Auto-detects pushState, replaceState, popstate, hashchange
- **Client Hints**: Accurate OS detection (Win10 vs 11, macOS versions) via ua-parser-js
- **Bot Detection**: User-agent patterns + webdriver + fingerprinting
- **Custom Dimensions**: custom_1...custom_10 for custom tracking
- **Ad Click ID Tracking**: gclid, fbclid, msclkid, and more

## Installation

### Via script tag

Your Notifuse server serves the SDK at `/na.js`, built from the same release
that receives the beats — so the SDK and the `/track` endpoint can never drift
apart. Copy the snippet from the workspace's Web Analytics settings, or write it
by hand:

```html
<script>
window.NotifuseAnalyticsConfig = {
  workspace_id: 'ws_your_workspace_id',
  endpoint: 'https://your-notifuse-instance.com',
  debug: false // optional
};
</script>
<script async src="https://your-notifuse-instance.com/na.js"></script>
```

The SDK auto-initializes from `window.NotifuseAnalyticsConfig`. No explicit
`init()` call required. Keep the `async` attribute: it is what lets the script
load without blocking the page.

This package is **not published to npm** on purpose. Bundling the SDK into an
application would let a pinned copy drift away from the server that receives its
payloads, and a payload contract mismatch is silent — sessions simply stop
updating. If you need to serve the bundle from your own CDN, fetch `/na.js` from
your instance and host that file — request it with `Accept-Encoding: identity`,
or you will save the gzipped bytes without the header that describes them.

## API

All methods (except `getConfig()` and `debug()`) are async and return Promises.

```typescript
// Session info (async)
await NotifuseAnalytics.getSessionId();       // Current session UUID
await NotifuseAnalytics.getFocusDuration();   // Active time in milliseconds
await NotifuseAnalytics.getTotalDuration();   // Wall clock time in milliseconds

// Synchronous methods
NotifuseAnalytics.getConfig();                // Returns config or null
NotifuseAnalytics.debug();                    // Get debug info

// Manual tracking (async)
await NotifuseAnalytics.trackPageView(url?);  // Track SPA navigation
await NotifuseAnalytics.trackGoal({ action, type, value?, properties? });
// type is REQUIRED: 'purchase' | 'subscription' | 'lead' | 'signup' | 'booking' | 'trial' | 'other'
// a currency or order id goes in properties: { currency: 'EUR', order_id: 'A-1234' }

// Custom Dimensions (async)
await NotifuseAnalytics.setDimension(1, 'premium');    // Set custom_1 = 'premium'
await NotifuseAnalytics.setDimensions({ 1: 'a', 2: 'b' }); // Set multiple
await NotifuseAnalytics.getDimension(1);               // Get dimension value
await NotifuseAnalytics.clearDimensions();             // Clear all

// Control (async)
await NotifuseAnalytics.pause();              // Pause tracking
await NotifuseAnalytics.resume();             // Resume tracking
await NotifuseAnalytics.reset();              // Clear session, start fresh
```

## Configuration

Set `window.NotifuseAnalyticsConfig` before loading the SDK script:

```typescript
interface NotifuseAnalyticsConfig {
  // Required
  workspace_id: string // Workspace identifier
  endpoint: string // API endpoint (required - no default)

  // Optional
  debug?: boolean // Default: false
  sessionTimeout?: number // Default: 30 * 60 * 1000 (30 min)
  adClickIds?: string[] // REPLACES the recognised click ids. [] disables capture.
  extraAdClickIds?: string[] // ADDS to the 13 built-in click ids. Use this to add one.
  trackSPA?: boolean // Default: true
  trackScroll?: boolean // Default: true
}

// TypeScript global declaration
declare global {
  interface Window {
    NotifuseAnalyticsConfig?: NotifuseAnalyticsConfig;
  }
}
```

## Events Tracked

| Event         | Trigger                                 | Data                      |
| ------------- | --------------------------------------- | ------------------------- |
| `screen_view` | Page load, SPA navigation               | path, referrer, UTM       |
| `ping`        | Heartbeat (10s desktop, 7s mobile)      | duration, max_scroll      |
| `scroll`      | Scroll milestones (25%, 50%, 75%, 100%) | max_scroll                |
| `goal`        | trackGoal() call                        | action, goal_type, value, properties |

## Ad Click ID Tracking

The SDK automatically captures advertising click IDs from URLs:

| Parameter   | Platform                  |
| ----------- | ------------------------- |
| `gclid`     | Google Ads                |
| `fbclid`    | Facebook/Meta Ads         |
| `msclkid`   | Microsoft Ads             |
| `dclid`     | DoubleClick               |
| `twclid`    | Twitter/X Ads             |
| `ttclid`    | TikTok Ads                |
| `li_fat_id` | LinkedIn Ads              |
| `wbraid`    | Google Ads (iOS)          |
| `gbraid`    | Google Ads (cross-device) |
| `epik`      | Pinterest Ads             |
| `ScCid`     | Snapchat Ads              |
| `rdt_cid`   | Reddit Ads                |
| `qclid`     | Quora Ads                 |

Matching is case-insensitive — `?sccid=`, `?ScCid=` and `?SCCID=` are all
recognised — but the canonical spelling above is what gets reported, so the
built-in attribution rules match it.

When found, the SDK sends:

- `utm_id_from`: The parameter name, in its canonical spelling (e.g. "gclid")
- `utm_id`: The parameter value

To recognise your own click id as well, use `extraAdClickIds: ['myclid']`. Use
`adClickIds` only to replace the list outright, or `adClickIds: []` to capture
none.

## Browser Support

| Browser        | Version |
| -------------- | ------- |
| Chrome         | 60+     |
| Firefox        | 55+     |
| Safari         | 11+     |
| Edge           | 79+     |
| iOS Safari     | 11+     |
| Android Chrome | 60+     |

## Bundle Size

| Bundle         | Size      |
| -------------- | --------- |
| UMD (minified) | ~48KB     |
| UMD (gzipped)  | **~18KB** |

Includes ua-parser-js for accurate device/OS detection with Client Hints support.

## Documentation

See [SPECS.md](./SPECS.md) for detailed technical specifications.
