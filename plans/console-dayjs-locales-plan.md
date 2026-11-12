# Console dates in the console's language — plan

## Problem

No dayjs locale is registered in the console, so every date dayjs prints (relative times such as
"3 days ago", `ll` / `MMM` month names, weekday names) stays English whatever language the user
picked. antd's DatePicker panels pass ConfigProvider's locale to dayjs per call, so they too fall
back to English while that dayjs locale is not registered.

## Change

1. **`console/src/i18n/dayjsLocales.ts`** (new)
   - Statically import the seven dayjs locales the console supports (`fr`, `es`, `de`, `ca`, `pt-br`,
     `ja`, `it`). They are small, and a static import adds no failure path beside the catalog chunk.
   - `dayjsLocaleCodes: Record<Locale, string>` maps each console locale to its dayjs code
     (`pt-BR` becomes `pt-br`). `Record<Locale, …>` makes a newly added console locale a type error
     until it is mapped.
   - `activateDayjsLocale(locale: Locale): void` calls `dayjs.locale(code)`.
2. **`console/src/i18n/index.ts`**
   - Every `i18n.activate(locale)` in `loadLocale` goes through one private `activate(locale)` that
     sets dayjs first, then Lingui. dayjs then always follows the language that actually activated:
     the newest load, the superseded load when the winner's catalog failed, or English after a
     fallback.
   - The remount from `key={locale}` in `App.tsx` re-renders every formatted date.
3. **`console/src/components/analytics/dashboardFormat.ts`**: correct the header comment that says
   no dayjs locale is loaded.
4. **`console/src/components/timeline/ContactTimeline.tsx`**: the tooltips' "… in Europe/Paris" glue
   goes through Lingui (`{when} in {zone}`), so a translated date is not followed by English. The
   date is now formatted on that zone's clock (`.tz(zone)`, the contact's timezone, else the
   workspace's): it used the browser's, under the zone's name. Test in `ContactTimeline.test.tsx`:
   hovering an entry's time with an `Asia/Tokyo` timezone shows the Tokyo wall clock.

Out of scope (other screens, owned by other work in progress): fixed English-order patterns such as
`'MMM D'`, `'ha'`, `"MMM 'YY"`, `'D MMM'` and `'MMM D, YYYY HH:mm'` in web analytics, the blog and
annotations, and the "in {timezone}" glue in the message history and `utils/formatters.tsx`. They
print translated words in English order ("mars 5") until moved to localized tokens or Intl.

Behaviour to note in the PR: in fr, es, de, ca and it, dayjs weeks start on Monday. Web analytics'
"This week" and "Previous week" ranges and the DatePicker panels then start on Monday for those
users. That matches their calendar convention. The picker follows ConfigProvider's locale, and the
ranges follow the activated one; loadLocale's generation guard keeps the two equal.

## Tests (TDD: written first, seen failing)

- **`console/src/i18n/dayjsLocales.test.ts`** (new)
  - For each console locale, `activateDayjsLocale` makes `dayjs.locale()` its mapped code, and March
    formats with that locale's own month name (`mars`, `marzo`, `März`, `Març`, `março`, `3月`,
    `marzo`, `March`). This pins that each locale module is actually registered.
  - French relative time: `dayjs().subtract(3, 'day').fromNow()` is `il y a 3 jours`.
- **`console/src/__tests__/locale-race.test.tsx`** (extend existing cases)
  - The newest request wins: `dayjs.locale()` is `en`, in both latency orders. English is also
    dayjs's default, so these two lines only catch an activation placed before the generation
    guard (they pass without the change); the es, fr and fallback cases pin that dayjs follows.
  - The winner's catalog fails: dayjs stays on the superseded `es`.
  - A superseded failure does not drag dayjs to English: it stays `fr`.
  - Fallback after a failed `ja`: dayjs is `en`.
  - Reset dayjs to `en` in `beforeEach`, because the dayjs global survives `vi.resetModules`.
  - The fallback case first activates `fr`, so the English it ends on is one it activated.
- Week presets, in `dayjsLocales.test.ts`: under fr, es, de, ca and it, "This week" and "Previous
  week" start on Monday; under en, pt-BR and ja, on Sunday.
- **`console/src/__tests__/App.locale.test.tsx`** (new): a memoised page that formats only with
  dayjs repaints in French after a language switch. It fails without `key={locale}` in `App.tsx`
  and without the dayjs activation.

## Verification

- `cd console && npx vitest run src/i18n src/__tests__/locale-race.test.tsx src/__tests__/i18n.test.tsx src/__tests__/App.locale.test.tsx src/components/timeline`
- Then the full suite once: `cd console && npm test`, plus `npx tsc -b` and `npm run lint`.
