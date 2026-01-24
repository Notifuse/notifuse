import { defineConfig } from "@lingui/cli"

export default defineConfig({
  sourceLocale: "en",
  locales: ["en", "fr", "es", "de", "ca", "pt-BR", "ja", "it"],
  catalogs: [
    {
      path: "src/i18n/locales/{locale}",
      include: ["src"],
    },
  ],
})
