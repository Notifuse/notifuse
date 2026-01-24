import { defineConfig } from "@lingui/cli"

export default defineConfig({
  sourceLocale: "en",
  locales: ["en", "fr", "es", "de", "ca"],
  catalogs: [
    {
      path: "src/i18n/locales/{locale}",
      include: ["src"],
    },
  ],
})
