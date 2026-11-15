# Newsletters (maquettes 61, 62, 63) : alignement sur « Nuit claire »

Branche `feat/newsletters-mockup`, depuis `origin/dev` (2431381bf). Front seulement (`console/`) : aucun changement backend, SQL ni variable d’environnement. Icônes : lucide, comme le reste de la console, jamais d’emoji.

## Sources

- Diff écran par écran : `~/Desktop/suite-maquettes/2-diff/diff-console-vs-maquettes.md`, section Newsletters, et les captures `2-diff/v2/cote-a-cote/61.png`, `62.png`, `63.png`.
- Source de la maquette : `3-maquettes/maquette-console-v2.patch`, fichiers `console/src/identity/NewslettersRedesign.tsx` (61, 62) et `NewsletterCreate.tsx` (63).
- État réel relevé le 2026-11-15 sur `dev` à 2431381bf, avec les données de démo, en français, en 1440 et en 1280 (Playwright, viewport exact).
- Relecture du plan v1 par un agent (Opus, effort medium) : 23 points, tous intégrés ci-dessous.

## Ce que montre l’état réel

| Écran | Écart du diff | État sur dev | Mesure |
|---|---|---|---|
| 61 | Dernière colonne coupée en 1440 | Tient en 1440 (la sidebar se replie sur cette page), défile en 1280 | 1440 : tableau 1 342 px pour 1 342 ; 1280 : 1 326 pour 1 182 |
| 61 | Colonne « Envoi en cours » | Toujours là | msgid `Sending` traduit « Envoi en cours » |
| 62 | Graphique en premières ouvertures | Toujours là, avec en plus : infobulle en date ISO brute (« 2026-11-11T09:00:00Z »), axe en « 11-10 10:00 », repères verticaux qui se chevauchent | — |
| 62 | KPI Livré vide | Voulu : la démo envoie en SMTP ; le KPI affiche « — » avec une infobulle qui l’explique (`BroadcastFunnel.tsx`) | — |
| 62 | (non relevé par le diff) | Le tableau Test A/B déborde : Envoyé à moitié visible, Ouvertures et Clics masqués | 1440 : 1 426 px pour 1 066 ; 1280 : 1 426 pour 906 |
| 63 | Section « Envoi en cours » | Toujours là | même msgid |
| 63 | Aides en longs paragraphes | Toujours là (Audience, Suivi web) | — |

Les données du graphique ne sont pas fausses : l’historique ne garde que la première ouverture et le premier clic d’un message, donc « ouvertures par heure » et « premières ouvertures par heure » sont le même nombre. Seuls le titre, la légende et la mise en forme changent.

## Décisions de Pierre (planche http://127.0.0.1:19181/, 2026-11-15)

- **A1** : resserrer les colonnes de la liste pour qu’elle tienne en 1280.
- **B1** : tableau A/B sur deux lignes par cellule (variante + modèle, sujet + expéditeur), puis Envoyés, Ouvertures, Clics.
- **C1** : graphique comme la maquette : « Ouvertures et clics par heure » (« par jour » sur 30 j), légende « Ouvertures / Clics » à droite, phrase d’explication dans un « ? ».
- **D1** : une ligne d’aide sous chaque champ, le paragraphe complet dans un « ? ».
- **E1** : « Sending » séparé par contexte ; le nom devient « Envoi » ; l’état d’une newsletter devient « En cours » ; ses statuts passent au féminin.
- **F1** : dans la liste, une newsletter en cours d’envoi affiche une barre et un pourcentage ; le texte actuel passe dans l’infobulle.
- **G1** : dans la colonne Statut (120 px), une pastille trop longue se tronque ; son infobulle donne le texte entier.
- **H1** : date programmée sur deux lignes : la date, puis l’heure et le fuseau en petit.
- **I1** : tout au féminin sur les écrans Newsletters (Gagnante, Choisir comme gagnante, Gagnante envoyée, Modifiée, Gagnante choisie…), avec un contexte propre aux newsletters.
- Écarts voulus validés (voir plus bas).

## Changements

### 1. Libellés : contextes Lingui (E1, I1)

Le msgid `Sending` a 10 usages ; les 7 langues le traduisent comme un état (« Envoi en cours », « Wird gesendet », « Enviando »…).

**Contexte `section`** (le nom) : `{ message: 'Sending', context: 'section' }`, avec fr « Envoi », ca « Enviament », de « Versand », es « Envío », it « Invio », ja « 送信 », pt-BR « Envio ».
- `components/analytics/SendingCard.tsx:39` (panneau du tableau de bord) ;
- `components/broadcasts/BroadcastsTable.tsx:268` (colonne) ;
- `components/broadcasts/compose/SendingCard.tsx:44` et `:82` (section du formulaire) ;
- `components/settings/SettingsSidebar.tsx:27` (groupe des paramètres, `msg`).

**Contexte `newsletter status`** (états et libellés d’une newsletter, accordés au féminin en fr). Les autres langues gardent leur traduction actuelle, recopiée à la main dans chaque `.po` : `lingui:extract` crée ces entrées vides.
- `BroadcastStatusPill.tsx`, toutes les étiquettes (Draft, Scheduled, Testing, Pick a winner, Sending ×3 aux lignes 39, 43 et 51, Paused, Sent, `# failed`, Cancelled, Failed). En fr : Brouillon, Programmée, Test en cours, Choisir une gagnante, En cours, En pause, Envoyée, `{failed, plural, one {# échec} other {# échecs}}`, Annulée, Échouée. La variable devient nommée : `const failed = spec.count ?? 0`.
- `templateUsage.ts:80-89` : `scheduled`, `processing`, `testing`, `test_completed`, `winner_selected` (fr : Programmée, En cours, Test en cours, Test terminé, Gagnante choisie). `draft`, `paused` et `live` sont partagés avec les automatisations et ne bougent pas.
- `detail/AbTestPanel.tsx` : pastille `Winner` (fr « Gagnante », ligne 138), bouton `Pick as winner` (« Choisir comme gagnante », ligne 203), ainsi que `AbTestSummary` (« gagnante choisie manuellement », « gagnante envoyée à {time} », « gagnante sur le {metric} après {hours} h »).
- `detail/EngagementChart.tsx` : repère `Winner sent` (« Gagnante envoyée »). `BroadcastAside` (Historique) : même libellé.
- `BroadcastsTable.tsx:119` : `Edited ${ago}` (« Modifiée {ago} »).
- L’implémentation commence par un relevé : `grep -n "gagnant\|Modifié\|Programmé\|Envoyé" fr.po` sur les fichiers de `components/broadcasts/` et `pages/BroadcastsPage.tsx`. Chaque libellé de newsletter au masculin passe en contexte. La liste figure dans le compte rendu.

**Contexte `newsletter filter`** (onglets de `BroadcastsPage.tsx:181-188`, au pluriel) : All, Scheduled, Sending, Sent, et `Failed` (ligne 188, ajouté quand il y a des échecs). En fr : Toutes, Programmées, En cours, Envoyées, Échouées. Les autres langues reprennent leur traduction actuelle.

**Contexte `table column`** : en-tête court de la colonne Visites (§ 2), `Site visits` (msgid qui existe déjà sans contexte dans `ContactKpis.tsx:68`, où il ne bouge pas). Traductions courtes dans les 7 langues : fr « Visites site », ca « Visites », de « Besuche », es « Visitas », it « Visite », ja « 訪問 », pt-BR « Visitas ».

Le msgid `Sending` sans contexte n’a plus d’usage. Lancé sans `--clean`, `lingui:extract` le garde en `#~`, et `catalogues.test.ts` ignore ces lignes. Les mots anglais Sent, Scheduled, Cancelled, Failed et All restent sans contexte partout ailleurs, au masculin, pour des emails ou des messages.

**Variables nommées** : tout message touché passe en variables nommées (`SendingProgress.tsx:71` `${int(processed)} of ${int(enqueuedCount)}` → `const done = int(processed); const total = int(enqueuedCount)`).

### 2. Liste (61) : `BroadcastsTable.tsx`, `SendingProgress.tsx`, `layout/Pill.tsx`

- Largeurs (A1, corrigées par la relecture) : Statut **120**, Audience 128, Envoi 146, Destinataires 100, Ouvertures 100, Clics 100, Désab. 64, Rebonds 72, Visites 80, actions **64**. Largeurs fixes : 974 px, plus 200 px au moins pour le nom (`BROADCAST_NAME_MIN_WIDTH`), soit **1 174 px** pour 1 182 disponibles en 1280. Sans la colonne Visites : 1 094 px.
- Statut (G1) : nouvelle prop `truncate` de `Pill` (`min-w-0 max-w-full`, texte en `truncate`, le point reste visible), utilisée par `BroadcastStatusPill` dans le tableau. Son infobulle existante donne le sens. Le texte entier est ajouté en tête de l’infobulle quand la pastille est tronquée (toujours, plus simple : `title` = libellé + tooltip actuel).
- Envoi (H1) : une newsletter programmée affiche la date sur la première ligne (icône `CalendarClock` lucide), puis l’heure et le fuseau en dessous en `text-xs text-ink-3`. Une date d’envoi passée reste sur une ligne (« 10 nov. 2026, 10:43 », 120 px, elle tient). Les phrases (« Modifiée il y a… », « Le test se termine dans… ») passent à la ligne naturellement.
- En-tête Visites : `Site visits` en contexte `table column` (§ 1). L’infobulle de l’en-tête ne change pas.
- Envoi en cours (F1) : nouvelle prop `compact` de `SendingProgress`. Elle réutilise `RateBar` (`rate = processed / enqueued`) et affiche le pourcentage via `useNumberFormat().percent`. L’infobulle reprend les textes actuels, « Sending emails... » et « {done} of {total} » (~ETA). Le wrapper `w-[180px]` (classe entre crochets) disparaît. Quand `remaining === 0`, la cellule affiche la date de début, comme une newsletter envoyée, au lieu d’une cellule vide. Le détail (`BroadcastDetail.tsx:375`) garde la variante complète, inchangée.

### 3. Détail (62)

`detail/AbTestPanel.tsx` (B1) et `detail/VariationSender.tsx` :
- Colonnes : Variante (200 px : la lettre et le nom de la variante, puis le nom du modèle dessous en `text-xs text-ink-3`) ; Sujet (le reste : le sujet et la pastille Gagnante ou Recommandée, puis `subject_preview` s’il existe, puis l’expéditeur, chacun sur sa ligne en `text-xs`) ; Envoyé 90 ; Ouvertures 110 ; Clics 110 ; actions **116** (aperçu, envoi de test, deux liens d’analyse web : 4 boutons de 22 px et 3 écarts de 4 px, plus le padding), ou **290** quand « Choisir comme gagnante » est proposé.
- `VariationSender` prend une prop `size` (`text-sm` par défaut, `text-xs` dans la cellule). `AbTestPanel` est son seul usage.
- `tableLayout="fixed"`. `scroll.x` vaut la somme des largeurs fixes, plus 240 pour le sujet : 866 px (≤ 906 en 1280), ou 1 040 px avec le choix de la gagnante (≤ 1 066 en 1440 ; le tableau défile alors en 1280). La valeur dépend de `winnerSelectable`.
- Les colonnes Modèle et De disparaissent comme colonnes. Leurs valeurs restent visibles dans les cellules : aucune donnée n’est retirée.

`detail/EngagementChart.tsx` et `detail/engagementChartOption.ts` (C1) :
- Titre : `Opens and clicks per hour` (48 h, 7 j) ou `Opens and clicks per day` (30 j), à la place de `Opens and clicks over time`.
- Légende : `Opens` et `Clicks` (msgids existants), à droite.
- La phrase d’explication quitte le corps du panneau pour l’infobulle d’un « ? » à côté du titre, en deux msgids, un par granularité : `Each message counts once, in the hour of its first open or click; clicks count each message clicked once.` et la même phrase avec `day`. Les deux sont traduits dans les 7 langues. L’ancien msgid n’a plus d’usage.
- Axe des x : `buildChartOption` reçoit `locale`. En granularité heure, l’étiquette est l’heure seule (`Intl.DateTimeFormat(locale, { hour: 'numeric', timeZone: 'UTC' })` : « 15 h » en fr, « 3 PM » en en), et minuit est remplacé par le jour (`{ day: 'numeric', month: 'short' }` : « 11 nov. »). En granularité jour : le jour. `timeZone: 'UTC'`, parce que la clé est l’heure locale écrite avec un Z (commentaire de `fillBuckets.ts`).
- Pas des étiquettes : `axisLabel.interval` est une fonction qui garde les index dont l’heure est un multiple du pas. Le pas divise 24 : 6 h en 48 h ; 24 h en 7 j, où l’axe n’affiche que les jours. En 30 j, toutes les catégories sont gardées, avec `hideOverlap`. `hideOverlap` reste comme filet de sécurité.
- Infobulle : un `tooltip.formatter` écrit l’en-tête en clair (« mar. 11 nov., 15 h », ou le jour seul), puis chaque série avec `param.marker` et son nombre formaté (`useNumberFormat().int`, passé en paramètre). `confine: true`. Pas de couleur en dur.
- Repères Test envoyé et Gagnante envoyée : étiquettes horizontales en haut de la ligne (`position: 'end'`). Quand les deux repères existent, la deuxième est toujours décalée d’une ligne. Une étiquette dans le premier quart est alignée à gauche (`align: 'left'`), une dans le dernier quart à droite. `grid.top` réserve la place de la légende et de deux lignes d’étiquettes.
- Ligne des clics lissée : `smooth: true` avec `smoothMonotone: 'x'`, pour qu’elle ne passe pas sous l’axe entre deux zéros. Les couleurs restent celles des jetons (`colors.chart`).
- `useMemo` : `i18n.locale` entre dans les dépendances, sinon l’axe et l’infobulle restent dans l’ancienne langue.

### 4. Création (63)

« ? » accessibles (relecture, point 11) : un petit composant `HelpTip` dans `components/layout/` (un `<button type="button">` avec `aria-label`, qui contient `CircleHelp` et ouvre un `Tooltip` au survol et au focus). Le texte complet est aussi rendu en `sr-only`, et le contrôle voisin le référence par `aria-describedby`. Le graphique (§ 3) utilise ce même composant.

`compose/AudienceCard.tsx` (D1) :
- Liste et segments côte à côte (`grid grid-cols-2 gap-3`).
- Segments : l’aide courte `Optional. Empty means the whole list.` (fr « Facultatif : vide, toute la liste. ») et un `HelpTip` qui porte le paragraphe actuel, mot pour mot.
- « Exclure les destinataires désinscrits » devient une ligne : à gauche le titre, un `HelpTip` (paragraphe actuel, mot pour mot) et dessous l’aide courte `Turn off only for a notice everyone must receive (security, legal).` (fr « À désactiver seulement pour un avis que tous doivent recevoir (sécurité, mentions légales). ») ; à droite le `Switch`, avec un filet au-dessus. `aria-describedby` du switch → l’aide courte et le paragraphe. Son `aria-label` ne change pas.

`compose/TrackingCard.tsx` (D1) :
- Même ligne pour le suivi. Le titre reprend le msgid existant `Don't track opens and clicks for this broadcast`, avec dessous l’aide actuelle et « En savoir plus », et le `Switch` à droite, avec `aria-describedby`. Le label `Open and click tracking` disparaît : il doublait le titre.
- En fr seulement :
  - le titre devient « Ne pas suivre les ouvertures et les clics de cette newsletter » (au lieu de « tracker… ce broadcast ») ;
  - l’aide est resserrée : « Aucun pixel ni lien suivi ; les paramètres UTM restent. Sinon, le consentement de chaque contact s’applique. » ;
  - `Generated from the name until you edit it` devient « Généré depuis le nom, modifiable ».

`compose/SendingCard.tsx` : titre « Envoi » (contexte `section`, § 1).

`compose/AdvancedRow.tsx` : une ligne d’aide sous chaque titre, comme la maquette. C’est du texte seulement, aucun champ n’est ajouté : `Data shared by every recipient, in the global_feed variable` et `One HTTPS call per contact before sending, in the recipient_feed variable`. Les noms de variables sont vérifiés dans `internal/domain/template.go:1315` et `internal/service/broadcast/message_sender.go:712`, et le HTTPS dans `data_feed.go:159-162`. L’aide est hors du nom accessible du bouton : `aria-describedby`.

Le pied de la carte Contenu (deux lignes courtes) reste tel quel.

## Écarts voulus

- **Q16** : pas de « Envoyer un test à moi-même » dans le récapitulatif (63).
- Filtre « Toutes les listes » et tri « Date d’envoi » (61) : `broadcasts.list` n’a ni filtre par liste ni tri. Ce serait du backend.
- Recherche « Nom ou sujet » (61) : l’API ne cherche que dans le nom ; le placeholder reste « Rechercher par nom ».
- « Fuseau du destinataire » (63) : `use_recipient_timezone` est enregistré (`broadcast_service.go:488`), mais aucun code d’envoi ne le lit.
- « En-têtes personnalisés » (63) : n’existe pas pour une newsletter (les en-têtes de l’API sont ceux des flux de données).
- KPI « Livré — » (62) : la démo envoie en SMTP, sans webhook de livraison ; l’infobulle l’explique.
- « Charger plus » (61) : la démo a 4 newsletters, ce n’est pas vérifiable ici ; la pagination existante n’est pas touchée.
- Boutons « Analyse web » et « Dupliquer » et menu ⋯ du détail (62) : hors du diff, non traités ici.
- Le widget complet de `SendingProgress` garde ses couleurs en dur (`text-amber-500`, `#f59e0b`) : il n’est pas redessiné ici.
- Sidebar et barre du haut : écarts normaux (consigne de Pierre).

## Fichiers et tests

| Fichier | Test | Cas |
|---|---|---|
| `src/__tests__/setup.tsx` | (tous) | Le mock `msg` traite `context` comme le mock `t` : `id: context ? generateMessageId(message, context) : message`. Sans cela, aucun test ne voit un contexte oublié |
| `components/broadcasts/BroadcastsTable.tsx` | `BroadcastsTable.test.tsx` | Mise à jour des valeurs épinglées `'1216px'` (l. 69) et `'1326px'` (l. 78) : `'1094px'` et `'1174px'`. Commentaire l. 60-61 réécrit sans nombres. Une ligne programmée : la date et « heure + fuseau » sont dans deux éléments. Une ligne en cours d’envoi : le pourcentage, sans « Sending emails... » dans la cellule. Avec `remaining === 0` : la date de début. Lignes `pickWinner` et `scheduled` : la pastille est tronquable (`truncate`). En-tête Visites `Site visits` |
| `components/layout/Pill.tsx` | `Pill.test.tsx` | `truncate` : classes de troncature sur le texte, point conservé ; sans `truncate` : inchangé |
| `components/layout/HelpTip.tsx` (nouveau) | `HelpTip.test.tsx` | Bouton focusable avec son `aria-label` ; l’infobulle s’ouvre au focus ; le texte est présent en `sr-only` avec l’id passé |
| `components/broadcasts/SendingProgress.tsx` | `SendingProgress.test.tsx` | `compact` : `RateBar` au bon taux, pourcentage formaté, infobulle avec le texte complet ; sans `compact` : rendu inchangé ; `remaining === 0` : rien dans les deux variantes |
| `components/broadcasts/BroadcastStatusPill.tsx` | `BroadcastStatusPill.test.tsx` | Libellés anglais inchangés (existants) ; catalogue fr de test indexé par `generateMessageId(…, 'newsletter status')` : les trois cas `Sending` donnent « En cours », `Sent` « Envoyée » ; `# failed` au pluriel en fr |
| `components/templates/templateUsage.ts` | `templateUsage.test.ts` | Même catalogue de test : `processing` → « En cours », `winner_selected` → « Gagnante choisie » ; `draft` et `paused` inchangés |
| `pages/BroadcastsPage.tsx` | `BroadcastsPage.test.tsx` | Onglets anglais inchangés ; catalogue de test `newsletter filter` : « Toutes », « En cours », « Échouées » |
| `components/settings/SettingsSidebar.tsx`, `components/analytics/SendingCard.tsx`, `compose/SendingCard.tsx` | `SettingsSidebar.test.tsx`, `analytics/SendingCard.test.tsx`, `compose/cards.test.tsx` | Anglais inchangé ; catalogue de test `section` : « Envoi » |
| `components/broadcasts/detail/AbTestPanel.tsx`, `detail/VariationSender.tsx` | `AbTestPanel.test.tsx` | En-têtes `Variant, Subject, Sent, Opens, Clicks` ; le nom du modèle dans la cellule Variante ; l’expéditeur (en `text-xs`) et `subject_preview` dans la cellule Sujet ; pastille Gagnante ; `scroll.x` = 866 sans le choix de la gagnante, 1 040 avec |
| `components/broadcasts/detail/engagementChartOption.ts` | nouveau `engagementChartOption.test.ts` (les cas de `buildChartOption` de `EngagementChart.test.tsx:50-83` y sont déplacés et mis à jour) | Formatter : « 15 h » (fr), « 3 PM » (en), minuit → « 11 nov. », jour → « 11 nov. » ; `interval` : index gardés pour 48 h (toutes les 6 h) et 7 j (minuits seulement) ; infobulle : jamais la clé ISO, nombres formatés ; deux repères : la deuxième étiquette décalée ; un repère au début aligné à gauche, à la fin à droite ; un repère hors plage : absent ; légende à droite ; `smooth` et `smoothMonotone` |
| `components/broadcasts/detail/EngagementChart.tsx` | `EngagementChart.test.tsx` | Titre « per hour » en 48 h et 7 j, « per day » en 30 j ; la phrase d’explication n’est plus dans le corps, elle est dans le `HelpTip` (texte selon la période) ; légendes `Opens` et `Clicks` |
| `components/broadcasts/compose/AudienceCard.tsx`, `TrackingCard.tsx`, `AdvancedRow.tsx` | `compose/cards.test.tsx` | Liste et segments dans la même grille ; aide courte des segments ; `getByRole('switch', { description: … })` pour Exclure et pour le suivi ; les switchs basculent toujours `excludeUnsubscribed` et `noTracking` (qui coupe `autoSendWinner`, cas existant) ; le lien En savoir plus reste ; les deux aides de `AdvancedRow` (`cards.test.tsx:295` et suivants), dans la description et pas dans le nom du bouton |
| `i18n/locales/*.po` | nouveau `src/i18n/sendingContexts.test.ts` (avec un lecteur `.po` qui garde `msgctxt` : celui de `catalogues.test.ts` l’ignore) et `catalogues.test.ts` | fr : `Sending` en `section` = « Envoi », en `newsletter status` = « En cours », en `newsletter filter` = « En cours » ; `Sent` en `newsletter status` = « Envoyée » ; aucune entrée vide pour ces contextes dans les 7 langues |
| e2e | `e2e/features/broadcasts.spec.ts`, `e2e/protected-pages.spec.ts` | À relancer : les textes anglais sélectionnés ne changent pas (lignes 36-37, 48, 67, 80, 98 ; `protected-pages.spec.ts:67`) |

## Vérification à l’écran

- Script Playwright de mesure (`shoot.mjs`, dans le scratchpad), en 1440 et en 1280, en français. Il vérifie :
  - le tableau de la liste (61) : largeur ≤ conteneur aux deux largeurs ;
  - le tableau A/B : largeur ≤ conteneur aux deux largeurs ;
  - le graphique : axe, infobulle (survol), repères ;
  - le formulaire (63) : captures de l’Audience et du Suivi web, avec les « ? » ouverts.
- Pastilles tronquées, date programmée sur deux lignes, envoi en cours : la démo n’en a pas. Le script intercepte la réponse de `broadcasts.list` et y injecte une newsletter programmée, une en cours d’envoi et une `test_completed`. Aucune écriture en base.
- Comparaison côte à côte avec les PNG des maquettes, puis liste des écarts restants.

## Étape finale : tests

Depuis `console/` du worktree :

1. `npm run lingui:extract` (sans `--clean`), puis les traductions des 7 langues, y compris la recopie des traductions actuelles dans les nouvelles entrées à contexte.
2. `npx vitest run src/components/broadcasts src/components/layout src/pages/BroadcastsPage.test.tsx src/components/analytics/SendingCard.test.tsx src/components/settings/SettingsSidebar.test.tsx src/components/templates/templateUsage.test.ts src/i18n`.
3. `npx vitest run` complet, une fois (le mock `msg` change pour tous les tests).
4. `npx tsc -b` (il vérifie aussi les tests), puis `npx eslint` sur les fichiers touchés. Prettier (`--single-quote --no-semi --print-width 120 --trailing-comma none`) en entier sur les fichiers qui étaient déjà formatés, sinon seulement sur les lignes modifiées.
5. e2e : `npx playwright test e2e/features/broadcasts.spec.ts e2e/protected-pages.spec.ts --config <scratchpad>/playwright.19173.config.ts`. La config vise la console du worktree sur 19173 ; celle du dépôt vise 5173, la console de la session notifuse-42. Les tests simulent l’API. En cas d’échec sur `waitForLoadState` juste après une recompilation, les relancer une fois.
6. `/code-review medium` sur le diff final, puis une nouvelle revue après toute correction. Commit seulement avec l’accord de Pierre.
