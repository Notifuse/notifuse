# Transactionnel (maquettes 91 et 92) : alignement sur « Nuit claire »

Branche `feat/transactional-mockup`, partie de `origin/dev` (2431381b). Front seulement : aucun changement de backend, de SQL ni de variable d'environnement.

## État de départ (mesuré le 2026-11-15)

Console de cette branche sur le workspace de démo, captures Playwright en 1440 et 1280 (`deviceScaleFactor: 2`).

- **91 Liste**
  - Les actions de ligne (Send test, API command, ⋯) sont à `opacity: 0` hors survol et focus (`opacity-0 group-hover:opacity-100 …` dans `TransactionalTable.tsx`). Parmi les tableaux de liste principaux, c'est le seul à masquer ses actions ; seul le tableau des filtres de l'analyse web (`FilterTable.tsx`) le fait aussi. La maquette les montre sur toutes les lignes.
  - La table est en largeur automatique. En 1440 : Notification 212, Template 154, Sent 57, Delivered 95, Opens 125, Clicks 125, Failed 68, Bounces 89, Last sent 129, actions 116 px. Le « 733 » de Sent colle au « — » de Delivered. La maquette fixe : Template 170, Envoyés 84, Délivrés 110, Ouvertures 106, Clics 106, Échecs 62, Rebonds 70, Dernier envoi 120, actions 96.
  - « New notification » n'a pas l'icône « + » de la maquette (Newsletters et Automatisations l'ont).
- **92 Détail**
  - « Edit » n'a pas le crayon de la maquette.
  - Les valeurs des KPI sont en primaire (`rgb(91, 75, 235)`), parce que ce sont des liens vers les logs filtrés.
- **Déjà conforme ou déjà réglé** (rien à faire) :
  - Le KPI et la colonne Delivered montrent « — » sur la démo : son fournisseur est SMTP, qui ne remonte ni livraisons ni rebonds. La console le dit déjà dans une info-bulle (`SmtpDash`, et le `hint` des KPI).
  - Le reste de 92 suit la maquette.
- **Le diff se trompait sur un point** : la maquette 91 n'a pas de colonne Fournisseur. Celle-ci n'existe que dans « Derniers envois » de la maquette 92 (voir les écarts voulus).

## Décisions de Pierre (2026-11-15, planche d'options sur le port 19281)

| # | Question | Choix |
|---|---|---|
| Q1 | Actions de ligne de 91 | A · toujours visibles, comme la maquette |
| Q2 | Couleur des valeurs KPI de 92 | C · garder la couleur primaire (liens vers les logs) |
| Q3 | Colonne Fournisseur de « Derniers envois » | A · pas de colonne |
| Q4 | Petits alignements | D · icônes « + » et crayon ; E · largeurs de colonnes de la maquette |
| Q5 | Nom du fournisseur dans la barre d'outils de 91 | A · en gras, comme la maquette |
| Q6 | Partage de la place entre nom et modèle (après la revue de code) | B · table en largeur fixe, modèle à 120 px, le nom prend le reste |
| Q7 | En-tête « Failed » trop long en allemand dans 62 px (seconde revue) | A · libellé propre à la colonne (contexte Lingui `failures column`) : « Échecs », « Fehler »… |
| Q8 | Pastille « Deleted template » dans 120 px (seconde revue) | A · sous le nom du modèle |
| Q9 | La pastille déborde quand même : 122 à 157 px pour 104 de place (troisième revue) | A · pastille courte dans la liste (« Supprimé », contexte `deleted template pill`), la phrase complète en info-bulle et pour les lecteurs d'écran ; la fiche garde « Modèle supprimé » |
| Q10 | Actions hors de vue quand la table défile, sous ~1 100 px (cinquième revue) | A · colonne d'actions épinglée à droite (`fixed: 'right'`), comme la table des contacts |
| Q11 | En 1280, l'étiquette « Supabase » ne laisse que 43 px au nom (septième revue) | L'icône de l'intégration à la place de l'étiquette, dans la liste |

Pierre garde aussi la disposition actuelle de 92 : aperçu en haut à droite, KPI limités à la colonne de gauche.

Pas de barre du haut : elle est fusionnée dans la sidebar. Les écarts de sidebar et de barre du haut ne comptent pas.

## Changements

### 1. Actions de ligne toujours visibles (Q1)

`console/src/components/transactional/TransactionalTable.tsx`, colonne `actions` :
- Retirer `opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100` du conteneur. Garder `flex items-center justify-end gap-1` et le `onClick={stop}`.
- `rowClassName="group cursor-pointer"` devient `"cursor-pointer"` : `group` ne servait qu'au masquage, et le fond de survol vient d'antd.
- Mettre à jour le JSDoc ou le commentaire s'il parle du survol.

### 2. Largeurs de colonnes de la maquette (Q4 · E, Q6)

Même fichier :
- `width` sur Sent 84, Delivered 110, Opens 106, Clicks 106, Failed 62, Bounces 70, Last sent 120 et actions 96 (au lieu de 116). Trois boutons `small` de 22 px (`controlHeightSM: 22`, `antdTheme.ts`), deux espaces `gap-1` de 4 px et 2 × 8 px de padding de cellule font 90 px, ce qui tient dans 96.
- `tableLayout="fixed"` et Template à 120 px. Le nom prend le reste : 296 px en 1440, 136 px en 1280. Le `TemplateLabel` est enveloppé dans un `div` `flex min-w-0` pour que son nom se tronque.
- Pourquoi la largeur fixe : la revue de code a montré qu'en largeur automatique, un nom long ne se tronque pas. Avec un nom de 70 caractères, la colonne Notification montait à 473 px, Sent tombait à 42 px, et la table débordait de 158 px en 1280. Les variantes en largeur automatique ont été mesurées et écartées : `max-w-0` laisse le reste au modèle, `w-full max-w-0` écrase les colonnes chiffrées. Pierre a choisi B sur captures réelles (planche `largeurs.html`).
- En dessous de la largeur minimale, la table défile au lieu d'écraser le nom : `scroll={{ x: minTableWidth(columns, NOTIFICATION_NAME_MIN_WIDTH) }}`. Le helper vient de `components/broadcasts/tableWidth.ts`, comme dans BroadcastsTable ; il n'est pas déplacé, pour ne pas toucher aux fichiers de Newsletters, chantier de toto. Le nom ne descend pas sous 120 px : 874 + 120 = 994 px. Mesuré en 1024 : nom à 120 px, et la table défile de 240 px dans son panneau.
- La colonne Failed a son propre libellé, `t({ message: 'Failed', context: 'failures column' })`. Il reste « Failed » en anglais et devient « Échecs » en français (comme la maquette), « Fehler » en allemand, « Errors » en catalan, « Fallos » en espagnol, « Errori » en italien, « 失敗 » en japonais, « Falhas » en portugais. Mesuré avec les polices de la console (`scratchpad/shots/fit.mjs`) : dans les 8 langues, chaque en-tête et chaque valeur de « Last sent » (à l'instant, minutes, heures, hier, date) tient dans sa colonne.
- `TemplateLabel` empile la pastille « Deleted template » sous le nom (`flex-col`), comme l'ID sous le nom d'une notification. À côté du nom, elle ne lui laissait aucune place dans 120 px.
- `DeletedTemplatePill` reçoit `short` (liste seulement) : le mot court est `aria-hidden`, et le texte `sr-only` comme l'info-bulle disent « Deleted template. This notification still sends with it ». Mesuré dans les 8 langues (`scratchpad/shots/fit2.mjs`) : la pastille courte, « Not tracked » et l'étiquette d'une intégration (« Supabase ») tiennent dans leurs colonnes en 1440, 1280 et 1024.
- L'étiquette de secours « Managed by an integration », qui débordait de 159 à 193 px, n'est plus dans la liste : l'icône la remplace (Q11).
- Capacité mesurée des colonnes chiffrées (`scratchpad/shots/fit3.mjs`), dans les 8 langues, avec le groupement des milliers : Sent tient jusqu'à 9 999 999 envois sur la période, Failed jusqu'à 99 999.
- La phrase de l'info-bulle et du lecteur d'écran de la pastille courte est un seul message (« Deleted template. This notification still sends with it »), pour que chaque langue choisisse sa ponctuation : le japonais met « 。 ». Nouveau texte français avec l'apostrophe typographique ’ ; une apostrophe droite serait un échappement ICU pour Lingui.
- Points des revues réfutés par la mesure : les dates de « Last sent » et les en-têtes Bounces, Opens et Last sent tiennent dans toutes les langues. La revue estimait le texte à 14 px, alors que la cellule est en 12,5 px et l'en-tête en 11,5 px.
- La colonne d'actions est épinglée à droite (`fixed: 'right'`, comme `ContactsTable`). Mesuré en 1024 (`scratchpad/shots/pinned.mjs`) : les boutons de chaque ligne restent dans la zone visible de la table, au repos comme après un défilement de 120 px. En 1280 et 1440, rien ne change.
- Dans la liste, `IntegrationIcon` (dans `notificationParts.tsx`) remplace l'étiquette `IntegrationTag`. C'est l'éclair du logo Supabase : `supabase.png` est le logo complet, et une boîte carrée `size-3.5 overflow-hidden` masque le mot qui suit. Pour un autre type d'intégration, c'est l'icône générique de la page Modèles (`SquareTerminal`). Son nom accessible et son info-bulle reprennent les textes déjà traduits (« Managed by {name} ({type} integration) », « Managed by an integration »). Mesuré en 1280 : le nom garde 98 px, au lieu de 43 avec l'étiquette. La fiche 92 garde son étiquette, faute de manque de place.
- Critère mesuré, atteint : chaque colonne fixée à sa largeur exacte en 1440 et en 1280, nom long compris, et aucun débordement (`scrollWidth - clientWidth = 0`).

### 3. Icône « + » sur « New notification » (Q4 · D)

`console/src/pages/TransactionalNotificationsPage.tsx` : ajouter `icon: <Plus size={14} aria-hidden="true" />` aux `buttonProps` passés à `UpsertTransactionalNotificationDrawer`, le bouton qui ouvre la création. Il est utilisé dans l'en-tête et dans l'état vide, comme Automatisations. Le nom accessible ne change pas (icône `aria-hidden`).

### 4. Crayon sur « Edit » dans la fiche (Q4 · D)

`console/src/components/transactional/TransactionalDetail.tsx` : `icon={<Pencil size={14} aria-hidden="true" />}` sur le `RefusedButton` Edit. `RefusedButton` transmet déjà `icon` (utilisé par Send test).

### 5. Nom du fournisseur en gras (Q5)

`console/src/pages/TransactionalNotificationsPage.tsx`, `providerLine` : les deux `t` (« Sent via ${provider.name} · ${sender.email} » et « Sent via ${provider.name} ») deviennent des `<Trans>` à variables nommées (`providerName`, `senderEmail`). Le nom est dans un `<span className="font-medium text-ink">`, le reste garde `text-ink-3`. Nouveaux msgid : `Sent via <0>{providerName}</0> · {senderEmail}` et `Sent via <0>{providerName}</0>`. `npm run lingui:extract` (sans `--clean`), puis traduction des 7 langues (ca, de, es, fr, it, ja, pt-BR). En français : « Envoyés via <0>{providerName}</0> · {senderEmail} », comme la maquette.

## Écarts voulus (restent après ce chantier)

- **91 · largeur du modèle** : 120 px au lieu des 170 de la maquette, pour laisser la place au nom en 1280 (décision Q6).

- **92 · disposition** : l'aperçu du modèle reste en haut de la colonne de droite, et la barre des KPI ne couvre que la colonne de gauche. La maquette met les KPI sur toute la largeur et l'aperçu en dessous. Pierre préfère la disposition de la console (2026-11-15) : `TransactionalDetail.tsx` garde sa grille.
- **92 · couleur des KPI** : les valeurs restent en couleur primaire, parce que ce sont des liens vers les logs filtrés (décision Q2).
- **92 · colonne Fournisseur de « Derniers envois »** : absente. Un message n'enregistre pas le fournisseur qui l'a envoyé (`message_history` n'a pas ce champ). Afficher le fournisseur actuel serait faux pour les envois passés dès que celui du workspace a changé. L'enregistrer demanderait une migration et un chantier backend séparé (décision Q3).
- **91 et 92 · Livré et Rebonds « — »** avec un fournisseur SMTP : voulu, déjà expliqué par une info-bulle.
- **92 · Réglages** : la maquette montre « Variables attendues ». Ce champ n'existe pas dans l'API, il n'est donc pas ajouté (règle : aucun champ inventé).
- **92 · Aperçu** : la console garde « Open » (aperçu du modèle) au lieu de « Éditer le modèle » ; on ne touche pas à l'éditeur d'email.
- **91 · trombone « pièce jointe »** (ligne « Facture payée » de la maquette) : aucun champ de la notification ne le porte. Les pièces jointes ne sont qu'une option d'un envoi (`EmailOptions`). Pas ajouté.
- **92 · « Modifiée le 2 nov. par Pierre »** : la notification n'a pas d'auteur de modification (pas de `updated_by`). La console garde « Updated » avec la date et l'heure complètes.
- **92 · sous-titres des KPI** : la maquette compte des « contacts » et des « adresses » ; la console compte des emails, ce que mesurent les statistiques (des messages, pas des contacts distincts). Elle garde « emails ».
- **Le 93 (création ou modification) et le 94 (commande API)** ne sont pas redessinés : ils n'ont pas de maquette, et le diff juge la création conforme.

## Traductions, pas de CHANGELOG

- Cinq textes sont nouveaux : les deux du changement 5, le libellé de la colonne Failed (Q7), le mot court de la pastille et sa phrase (Q9). `src/i18n/locales/catalogues.test.ts` vérifie qu'aucune langue ne manque. Les deux anciens msgid (`Sent via {0}`, `Sent via {0} · {1}`) deviennent obsolètes, puisque l'extraction se fait sans `--clean`.
- Pas de ligne au CHANGELOG. La page transactionnelle refaite est encore dans la version non publiée 44.0, avec sa ligne « Feature ». Ces retouches visuelles ne changent pas ce qu'elle décrit.

## Tests

| Fichier modifié | Fichier de test | Cas |
|---|---|---|
| `components/transactional/TransactionalTable.tsx` | `components/transactional/TransactionalTable.test.tsx` | (a) Ni les boutons de chaque ligne ni aucun de leurs ancêtres jusqu'à la ligne ne portent de classe de masquage (`opacity-`, `invisible`, `hidden`). jsdom n'applique pas Tailwind, donc le vrai contrôle visuel est l'e2e. (b) Le `colgroup` donne à Template, Sent, Delivered, Opens, Clicks, Failed, Bounces, Last sent et actions les largeurs 120, 84, 110, 106, 106, 62, 70, 120 et 96, écrites en littéraux. Notification n'a pas de largeur, et la table a `table-layout: fixed`. |
| `components/transactional/notificationParts.tsx` (Q11) | `components/transactional/notificationParts.test.tsx` | `IntegrationIcon` : rien sans intégration ; le logo Supabase, nommé « Managed by Supabase (supabase integration) » ; une icône générique pour un autre type, ou pour une intégration qui n'existe plus (« Managed by an integration »). Dans la table, la ligne d'une notification gérée porte l'icône et plus le texte « Supabase ». |
| `components/transactional/notificationParts.tsx` (Q9) | `components/transactional/notificationParts.test.tsx` | Sans `short` : « Deleted template » lisible, et la phrase en `sr-only`. Avec `short` : « Deleted » en `aria-hidden`, pas de « Deleted template » visible, et « Deleted template. This notification still sends with it » en `sr-only`. |
| `components/transactional/TransactionalTable.tsx` (Q7, Q8) | `components/transactional/TransactionalTable.test.tsx` | (c) La pastille d'un modèle supprimé est dans le même conteneur `flex-col` que le nom, après lui. (d) La table a la largeur minimale de défilement, 994 px, et la cellule d'actions est épinglée (`ant-table-cell-fix-end`). |
| `pages/TransactionalNotificationsPage.tsx` | `pages/TransactionalNotificationsPage.test.tsx` | Le mock de `UpsertTransactionalNotificationDrawer` rend `buttonProps.icon`. Une assertion `lucide-plus` dans le test existant de la liste (en-tête), une autre dans celui de l'état vide : les deux rendus s'excluent. |
| `components/transactional/UpsertTransactionalNotificationDrawer.tsx` (inchangé, mais rien ne couvre le passage de `buttonProps`) | `components/transactional/UpsertTransactionalNotificationDrawer.test.tsx` | Avec `buttonProps={{ icon: <Plus /> }}`, le bouton d'ouverture contient `svg.lucide-plus`. |
| `pages/TransactionalNotificationsPage.tsx` (changement 5) | `pages/TransactionalNotificationsPage.test.tsx` | Avec un expéditeur, la barre montre « Sent via », puis le nom du fournisseur dans un élément `font-medium text-ink`, puis « · » et l'email. Sans expéditeur, le nom seul, en gras. |
| `components/transactional/TransactionalDetail.tsx` | `components/transactional/TransactionalDetail.test.tsx` | Le bouton « Edit » porte l'icône `lucide-pencil`, et un Edit refusé la garde (le test existant du refus reste vert). |
| `e2e/features/transactional.spec.ts` (fixture `rowAction`) | lui-même | `rowAction` ne survole plus la ligne, et son commentaire est mis à jour. Un nouveau cas vérifie que, sans survol, l'opacité effective du bouton « Send test » de la première ligne vaut 1 : produit des `opacity` calculées du bouton et de tous ses ancêtres, via `evaluate`. `toBeVisible` tient `opacity: 0` pour visible, et `opacity` ne s'hérite pas : un `toHaveCSS` sur le bouton seul passerait déjà. **Contrôle de mutation** : lancer ce cas sur le code d'avant le changement et vérifier qu'il échoue. |

Prettier (`--single-quote --no-semi --print-width 120 --trailing-comma none`) : `TransactionalTable.tsx`, `TransactionalNotificationsPage.tsx`, `TransactionalNotificationsPage.test.tsx` et `TransactionalDetail.tsx` sont déjà formatés, on les formate en entier. `TransactionalTable.test.tsx`, `TransactionalDetail.test.tsx` et `transactional.spec.ts` ne le sont pas : on ne formate que nos lignes.

## Vérification à l'écran

`scratchpad/shots/capture.mjs`, console sur 19273 et API de dev sur 4000, en 1440 et en 1280 :
- 91 : opacité des actions de la première ligne hors survol (attendu 1), largeur de chaque colonne, absence de débordement horizontal, capture ;
- 92 : capture, icône crayon présente ;
- comparaison côte à côte avec les PNG 91 et 92 de la maquette, et liste des écarts restants (ceux de la section « Écarts voulus », et rien d'autre).

## Étape finale : tests

Depuis `console/` :
1. `npx vitest run src/components/transactional src/pages/TransactionalNotificationsPage.test.tsx`
2. `npx tsc -b`
3. `npx eslint src/components/transactional/TransactionalTable.tsx src/components/transactional/TransactionalDetail.tsx src/pages/TransactionalNotificationsPage.tsx` et les tests touchés.
4. `npm run lingui:extract`, puis traduction des deux nouveaux msgid dans les 7 langues, et `npx vitest run src/i18n`.
5. `npx playwright test e2e/features/transactional.spec.ts`. Si elle échoue sur `waitForLoadState` juste après une modification, relancer avant de conclure.
6. `npx vitest run` complet une fois, pour les autres écrans qui importent ces composants.

Puis `/code-review medium` sur le diff final, refait après chaque correction. Commit seulement sur l'accord explicite de Pierre.
