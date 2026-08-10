/**
 * Static file server for E2E tests
 *
 * Serves the SDK bundle and test fixtures only.
 * The SDK sends tracking data to the real API (port 4000).
 */

import express from 'express';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();

// Serve SDK dist
app.use('/dist', express.static(path.join(__dirname, '../../../dist')));

// Serve test fixtures (HTML pages)
app.use('/', express.static(path.join(__dirname, '../fixtures')));

// Health check for Playwright webServer
app.get('/health', (_req, res) => {
  res.json({ status: 'ok' });
});

const PORT = 3333;
app.listen(PORT, () => {
  console.log(`Static server running on http://localhost:${PORT}`);
});
