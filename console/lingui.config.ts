import { defineConfig } from "@lingui/cli"
import { createSwcExtractor } from "@lingui/native-tools"

export default defineConfig({
  sourceLocale: "en",
  locales: ["en", "fr", "es", "de", "ca", "pt-BR", "ja", "it"],
  catalogs: [
    {
      path: "src/i18n/locales/{locale}",
      include: ["src"],
    },
  ],
  // The same native SWC transform the build compiles macros with (vite.config.ts,
  // macroTransform), so the message IDs extracted here are the ones the bundle looks up.
  extractors: [createSwcExtractor()],
})
