# Modèles (écrans 127, 128, 129, 129b de la planche v2) : alignement sur « Nuit claire »

Branche `feat/templates-mockup`, partie de `origin/dev` (46c9f803, qui contient la #587). Front seulement : aucun changement de backend, de SQL ni de variable d'environnement.

## Référence et état de départ (2026-11-15)

- **La maquette** : la vraie, relancée depuis `maquette-console-v2.patch` dans un worktree jetable (`scratchpad/mockup-app`, détaché sur `65813b8`) avec son faux backend, sur le port 19274. Ses écrans s'ouvrent par les états `tpl-preview`, `tpl-test`, `tpl-translations`, `tpl-import` et `tpl-conflict`. Source : `MissingTemplates.tsx` et `MissingKit.tsx` (129b).
- **La console** : cette branche sur le workspace de démo, captures Playwright en 1440. Rien n'est écrit : `templates.update` répond 409, et `templates.create` et `email.testTemplate` répondent 400, interceptés au réseau.
- **La planche** de comparaison : `scratchpad/planche/modeles.html`.
- **Hors périmètre** (consigne de Pierre) :
  - l'éditeur d'email (125b), l'enregistrement d'un bloc et la condition d'affichage (125c) ;
  - le cadre du formulaire de modèle et son étape Paramètres (125) : la maquette des traductions est dessinée dans un nouveau cadre (Paramètres / Contenu / Traductions, « Enregistrer la v4 »), qui ne change pas ici.

## Décisions de Pierre (2026-11-15)

| # | Écran | Choix |
|---|---|---|
| Q1 | 127 Aperçu | A · aligner sur la maquette, sans « Cci » (un modèle n'a pas de Cci) |
| Q2 | 128 Test | A · un tiroir, options visibles |
| Q2b | Portée de 128 (composant partagé avec les Newsletters et le Transactionnel) | partout ; prévenir toto |
| Q3 | 129 Traductions | A · maître-détail, à l'intérieur de l'étape Traductions |
| Q4 | 129b Import | A · restyler avec l'extrait du fichier ; le conflit ne change pas |
| Q5 | Le tiroir de test ouvert depuis une newsletter fait glisser son plein écran de 180 px (`push` de rc-drawer) | toto ajoute `push={false}` aux deux `FullDrawer` des Newsletters dans sa #588 ; rien dans cette branche |

## Changements

### 1. Aperçu d'un modèle (127) · `components/templates/TemplatePreviewDrawer.tsx`

Appelants : `TemplatesPage`, `TransactionalNotificationsPage`, `ContactMessagesTab` (avec `messageHistory`) et `ContactActivityFeed`, en mode contrôlé ; `BroadcastAside`, `AbTestPanel`, `ListHeader` et `TemplateSelectorInput`, par `children` ; `MessageHistoryTable`, qui n'est importé nulle part. Tous leurs tests simulent le tiroir. Le détail d'un message des Logs a sa propre vue et ne l'utilise pas. Toute la logique reste :
- la compilation et ses erreurs ;
- l'expéditeur d'une newsletter ;
- Cc et Cci du message ;
- les métadonnées ;
- la version texte et sa source.

Ne change que la présentation :
- **En-tête** : un en-tête maison, `closable={false}` et le `CloseButton` de `DrawerControls` à droite comme dans la maquette et `SidePanel`.
  - Le titre : le nom, puis en gris « v{version} · {catégorie} » quand la version existe.
  - La catégorie passe par `TEMPLATE_CATEGORY_LABELS`, avec `isTemplateCategory` ; une catégorie inconnue s'affiche brute.
  - La langue passe en `Select` à droite, avant la croix, au lieu du `Segmented` des descriptions. Elle n'apparaît que s'il y a plusieurs langues, comme aujourd'hui.
  - Le nom accessible du tiroir devient « {nom} v{N} · {catégorie} » : `e2e/features/templates.spec.ts` (l. 138, `exact: true`) passe à une expression `^Welcome Email`.
- **Enveloppe** : `KeyValueList` (`components/layout`), qui est déjà cette grille, remplace `Descriptions`. Lignes :
  - De ;
  - Répondre à : toujours affichée, avec « Not set » passé explicitement (`KeyValueList` mettrait « — »), comme la maquette. Pour un message dont le Reply-To diffère de celui du modèle, la mention « (original: … ) » reste ;
  - Objet, en gras ;
  - Texte d'aperçu, en gris, quand il y en a un (libellé existant « Subject preview ») ;
  - Cc et Cci : seulement pour un message (`messageHistory`), comme aujourd'hui.
- **Vues** : un `Segmented` compact remplace les `Tabs` soulignés, dans l'ordre de la maquette : HTML, MJML, Texte, Données, puis Metadata pour un message. Mêmes clés, mêmes conditions d'apparition. La vue montrée est la vue active si elle existe, sinon la première disponible : lors d'une erreur MJML, il n'y a pas de vue HTML, et le `Segmented` ne doit pas rester vide.
- **Email** : sur un fond `bg-subtle`, l'`iframe` dans une carte blanche arrondie et bordée, sans le motif pointillé, qui demanderait une valeur entre crochets.
- **Largeur** : 760 px au lieu de 650, comme la maquette.

### 2. Envoyer un email de test (128) · `components/templates/SendTemplateModal.tsx`

Partagé avec `BroadcastCompose`, `BroadcastDetail` et `TransactionalNotificationsPage` : le changement vaut partout (Q2b). Le nom du composant et ses props ne changent pas, et aucun appelant n'est modifié.
- **Forme** : `Modal` devient `SidePanel` (`components/layout`), le panneau maison de 520 px à droite, avec `copyLink={false}`, la croix à droite et le `footer`. Le pied porte « Cancel » et « Send Test Email » (primaire, icône `Send`), avec les mêmes conditions de désactivation et de chargement. `withCCAndBCC` reste, puisqu'il sert aux dépendances de la remise à zéro.
- **Glissement** : ouvert dans le plein écran d'une newsletter, le tiroir le fait glisser de 180 px. toto le corrige dans sa #588 (Q5).
- **Disposition** : la phrase d'introduction en gris. « Email Integration » et « Sender » côte à côte (`grid-cols-2`), puis Destinataire, Liste (avec son aide selon le cas) et Langue, comme aujourd'hui.
- **Options toujours visibles** sous un titre de section « Options ». Le lien « + add from name, CC, BCC, reply-to, attachments » et l'état `showAdvancedOptions` disparaissent. Les champs restent tous, avec les mêmes conditions :
  - Nom de l'expéditeur, avec son aide ;
  - CC et BCC côte à côte, seulement avec `canReadContacts` ;
  - Reply-To ;
  - les pièces jointes, avec leurs limites (3 Mo, 10 Mo, 20 fichiers) et leur liste.
- **Icônes** : `UploadOutlined` et `DeleteOutlined` (antd) deviennent `Paperclip` et `X` (lucide), comme le reste de la console.

### 3. Traductions (129) · `components/templates/TemplateTranslationsTab.tsx`

Toute la logique reste :
- les états par langue ;
- `handleToggle`, avec la réactivation qui garde les données ;
- le verrou de licence `template_i18n` ;
- la réinitialisation, avec sa confirmation ;
- le tiroir d'éditeur par langue, en visuel ou en code ;
- la version texte propre à chaque langue ;
- Import / Export par langue.

La présentation devient un maître-détail :
- **Noms des langues** : affichés dans la langue de la console (`Intl.DisplayNames`), avec repli sur `SUPPORTED_LANGUAGES`. Aujourd'hui, « Edit {0} translation » donne « Modifier la traduction French ».
- **À gauche** (`aside` de 280 px environ), « Workspace languages » :
  - la langue par défaut, avec « Default » en gris, non sélectionnable ;
  - puis chaque langue de traduction : son code, son nom et son `Switch` (même désactivation par licence) ;
  - la langue choisie est surlignée. La ligne est un `<button>` (code et nom), et le `Switch` est son voisin : jamais un bouton dans un bouton. Le `Switch` a un `aria-label` par langue (« Translate into {langue} »), n'a plus de libellés Enabled / Disabled (petit, comme la maquette), et ne change pas la langue choisie ;
  - en bas, une note : « Languages come from the workspace settings ».
  - Par défaut, la première langue active est choisie, sinon la première langue. Si la langue choisie disparaît de la liste (l'onglet reste monté), le choix revient à ce défaut. Une liste vide ne plante pas : sans langue de traduction, l'onglet n'est de toute façon pas monté.
- **À droite**, une carte pour la langue choisie :
  - titre « {Langue} translation », et dans `extra` le bouton « Reset », avec sa `Popconfirm` actuelle (il remplace le menu déroulant à côté d'« Open email editor ») ;
  - deux paires « source → traduction » : à gauche, en gris sur fond `bg-subtle`, l'objet et le texte d'aperçu de la langue par défaut (`defaultSubject`, `defaultSubjectPreview`, déjà passés au composant) ; à droite, les champs Subject et Subject preview ;
  - une section « Content » : l'aperçu téléphone actuel (objet et aperçu, réduit à 80 % dans un cadre de 320 px, mesuré pour qu'il ne soit pas coupé) à gauche, à la place de la vignette de la maquette, qui demanderait une compilation par langue ; à droite, la version texte, la phrase sur la réinitialisation et « Open email editor » (primaire).
- **Langue désactivée** : la carte dit « Enable this translation to edit it », avec un bouton « Enable » (pas de second `Switch`). Sous le verrou de licence, le bouton est désactivé et la phrase renvoie à la notice de licence.
- **Sous verrou** : « Reset », « Open email editor » et les champs restent désactivés, comme aujourd'hui.
- `LicenceGateNotice` reste en haut.

### 4. Erreurs d'import (129b) · `components/templates/ImportErrorsModal.tsx` et `pages/TemplatesPage.tsx`

L'import ne traite qu'un fichier, donc un seul modèle : la liste par modèle de la maquette (« 2 modèles sur 5 ») n'a pas de données, et reste un écart voulu.

- **Le modal** :
  - garde son titre (« MJML Syntax Error », etc.) ;
  - liste chaque erreur sur une ligne, avec une icône `X` rouge et le texte en police fixe ;
  - montre, si le fichier est fourni et qu'une erreur cite une ligne (`line N`), l'extrait du fichier de N−2 à N, avec la marge des numéros et la ligne N marquée ;
  - remplace les quatre conseils par une phrase courte, comme la maquette ;
  - a pour pied « Close », plus « Retry with a corrected file » (primaire) quand `onRetry` est fourni.
- **Nouvelles props**, optionnelles : `source?: string`, `line?: number` et `onRetry?: () => void`. Le calcul de l'extrait, `excerptLines`, est dans `importExcerpt.ts` : un composant ne doit exporter que des composants (rechargement à chaud).
- **Montage** : la page ne monte le modal que pendant qu'il est ouvert, comme ses autres fenêtres. Un nouvel essai repart ainsi d'un modal neuf.
- **Le numéro de ligne** se lit dans `importTemplateFile.ts`, et non dans le modal. Le résultat d'échec d'une erreur de syntaxe MJML porte `source` (le texte déjà lu par `FileReader`) et `line`. Trois formes sont reconnues, chacune pour la première occurrence :
  - Chrome et Safari (libxml2) : « error on line N » ;
  - Firefox : « Line Number N » ;
  - jsdom : « N:C: » juste après « Invalid MJML syntax: ».
  Les erreurs de structure (`validateEmailTree`) n'ont pas de ligne, et une erreur de syntaxe JSON part en toast : pas d'extrait pour elles. Le prétraitement MJML peut décaler un numéro dans de rares cas ; l'extrait reste alors indicatif.
- **`TemplatesPage`** garde `kind`, `source` et `line` dans son état `importErrors`, et passe `onRetry`. Celui-ci ferme le modal, puis clique le même sélecteur (`jsonInput` ou `mjmlInput`). Pas de `file.text()`, qui manque sous jsdom.
- **`ImportExportButton`** (barre de l'éditeur d'email, y compris par langue) ne change pas de code, mais son modal prend le nouveau style (« Close » au lieu de « OK », sans extrait ni « Retry »). C'est un effet assumé, à dire à Pierre.
- **Le conflit d'édition** (`useTemplateConflictModal`) ne change pas : il dit déjà plus que la maquette (les numéros de version).

## Prettier

Seul `ImportErrorsModal.tsx` était formaté sur dev : il l'est en entier, comme ses nouveaux fichiers. Ailleurs, seules mes lignes le sont. Dans `SendTemplateModal.tsx`, le bloc de rendu (`return (…)`), réécrit presque entièrement, est formaté d'un bloc : retirer le fragment des options y a désindenté des lignes voisines. Le reste du fichier garde sa forme de dev.

## Écarts voulus

- **127** :
  - pas de ligne « Cci » pour un modèle (pas de champ) ;
  - fond uni au lieu du motif pointillé.
- **128** : aucun écart de champ. Dans une demi-colonne de 520 px, un expéditeur long est tronqué (« Notifuse Demo <demo@notif… »), et la liste l'affiche en entier.
- **129** :
  - le cadre du formulaire (125) ne change pas ;
  - l'aperçu téléphone à la place de la vignette de contenu ;
  - les champs requis restent vérifiés à l'enregistrement, et non par une erreur rouge sous le champ comme dans la maquette.
- **129b** :
  - pas de liste par modèle (import d'un seul fichier) ;
  - l'extrait ne s'affiche que si une erreur cite une ligne ;
  - le conflit ne change pas.

## Traductions et CHANGELOG

- Textes déjà au catalogue, à réutiliser : Default, Content, Close, Not set, Reset, Reset translation, Cancel, Language, Subject preview, Reply to, Attachments, Retry. Textes à ajouter : Options, Workspace languages, les libellés de source (« Subject · {language} », « Subject preview · {language} ») et l'`aria-label` des interrupteurs.
- Les nouveaux textes sont extraits par `npm run lingui:extract`, sans `--clean`, puis traduits dans les 7 langues, avec l'apostrophe typographique en français. Ce sont :
  - « Options » (si le msgid n'existe pas déjà) ;
  - « Languages come from the workspace settings » ;
  - « {Langue} translation » ;
  - « Default » (si absent) ;
  - « Content » (si absent) ;
  - la phrase de réinitialisation ;
  - « Enable this translation to edit it » ;
  - « Retry with a corrected file » ;
  - « Close » (si absent) ;
  - la phrase de conseil de l'import.
- **CHANGELOG** : pas de ligne. Ce sont des retouches de présentation de la console refaite dans la 44.0, non publiée, et elles ne changent pas ce que ses lignes décrivent.

## Tests

| Fichier modifié | Fichier de test | Cas |
|---|---|---|
| `TemplatePreviewDrawer.tsx` | `TemplatePreviewDrawer.test.tsx` | (a) Le sous-titre « v{N} · {catégorie} », et une catégorie inconnue brute. (b) La langue en `Select` dans l'en-tête : on ouvre la liste, on choisit, et l'aperçu recompile dans cette langue ; pas de `Select` avec une seule langue. Réécrit les cas des l. 216-238, qui cliquaient « French » dans le `Segmented`. (c) Répondre à affiché, avec « Not set » sans valeur, et « (original: … ) » pour un message ; le texte d'aperçu affiché. (d) Les vues en `Segmented` dans l'ordre HTML, MJML, Text, Data, et un clic change la vue. Réécrit le cas des l. 94-105, qui lisait `.ant-tabs-tab`. (e) Erreur MJML : la vue MJML est montrée. (f) Cc et Cci d'un message affichés ; pas de Cci pour un modèle seul. (g) La croix ferme le tiroir. Les autres cas restent : erreurs, version texte, métadonnées, expéditeur d'une newsletter. |
| `SendTemplateModal.tsx` | `SendTemplateModal.test.tsx` | (a) Un tiroir (`.ant-drawer`), pas un `.ant-modal`. (b) Les options sont visibles sans clic : From Name, Reply-To, Attachments, et CC et BCC avec `canReadContacts`. (c) Sans `canReadContacts`, ni CC ni BCC. (d) L'envoi passe toujours tous les champs : les cas des l. 315, 324 et 346 n'ouvrent plus le lien. |
| `TemplateTranslationsTab.tsx` | `TemplateTranslationsTab.test.tsx` | (a) La liste montre la langue par défaut (« Default ») et chaque langue de traduction avec son interrupteur nommé. (b) La première langue active est choisie, et cliquer une autre ligne affiche sa carte. (c) La carte montre le texte source de la langue par défaut à côté des champs. (d) Le `Switch` d'une ligne active ou désactive sans changer de langue choisie. (e) « Reset » demande confirmation, puis remet les valeurs par défaut. (f) Sous verrou de licence : activation, « Enable », « Reset », éditeur et champs désactivés. (g) Une langue désactivée montre « Enable », qui l'active. Réécrits : l'aide `switchFor` (l. 72-73) vise l'`aria-label` de l'interrupteur ; les cas des l. 85-107, 164, 234-258 et 265 ; le cas des l. 277-286, qui choisit fr puis de. Les autres restent : réactivation qui garde les données, éditeur par langue. |
| `importTemplateFile.ts` | `importTemplateFile.test.ts` | Le numéro de ligne se lit dans chacune des trois formes, sur des chaînes littérales : libxml2, avec son second « line 5 » ignoré ; Firefox ; jsdom. Un vrai MJML invalide sous jsdom renvoie `source` et `line`. Une erreur de structure n'a pas de `line`. |
| `importExcerpt.ts` (nouveau) | `importExcerpt.test.ts` (nouveau) | La ligne fautive et les deux d'avant, numérotées depuis 1 ; le début du fichier ; les fins de ligne Windows ; rien pour une ligne hors du fichier. |
| `ImportErrorsModal.tsx` | `ImportErrorsModal.test.tsx` (nouveau) | (a) Chaque erreur est sur une ligne. (b) Avec `source` et `line: 6`, l'extrait montre les lignes 4 à 6 et marque la 6 ; avec `line: 1`, il commence à la 1. (c) Sans `line` ou sans `source`, pas d'extrait. (d) « Retry with a corrected file » appelle `onRetry` ; sans `onRetry`, pas de bouton. |
| `pages/TemplatesPage.tsx` | `pages/TemplatesPage.test.tsx` | Un import MJML invalide ouvre le modal avec l'extrait du fichier ; « Retry » ferme le modal et clique le sélecteur MJML. Le cas existant de la l. 414 reste. |

## Vérification à l'écran

`scratchpad/tpl/capture.mjs` (console sur 19273, rien d'écrit), en 1440 et 1280 :
- 127 dans chaque vue ;
- 128 ;
- 129, en choisissant une autre langue ;
- 129b avec un MJML invalide.

Comparaison avec les captures de la maquette (`scratchpad/mockshots/m-tpl-*.png`), et mesure des recouvrements : en 128, les colonnes doubles en 1280 ; en 129, la largeur de la liste et de la carte.

## Étape finale : tests

Depuis `console/` :
1. `npx vitest run src/components/templates src/pages/TemplatesPage.test.tsx src/components/broadcasts src/pages/TransactionalNotificationsPage.test.tsx src/components/contacts src/components/lists src/components/segment src/components/messages src/i18n` : les appelants de 127 et 128 compris.
2. `npx tsc -b`
3. `npx eslint` sur les fichiers touchés.
4. `npm run lingui:extract`, puis les traductions des 7 langues.
5. E2e, avec une config temporaire qui vise le port 19273 : `e2e/features/templates.spec.ts`, dont la l. 138 est adaptée, et `e2e/features/transactional.spec.ts`. Aucun e2e de newsletter n'ouvre le test (d'après toto).

Puis une `/code-review medium` sur le diff final (3 au plus pour ce chantier), et un commit seulement sur l'accord de Pierre.

## Issue de la revue (une sur trois, sur le diff non commité)

**Corrigé, puis vérifié à la main** (tests du fichier, `tsc`, eslint, Prettier sur mes lignes, e2e Modèles et Transactionnel : 40 sur 40) :
- une ligne citée au-delà de la fin du fichier montre les dernières lignes, au lieu de rien ;
- la liste des langues filtre sur la même langue par défaut qu'elle affiche, si bien qu'un workspace sans `default_language` ne liste plus « en » deux fois ;
- la langue affichée est fixée à l'ouverture (la première active, sinon la première) : en activer une autre ne fait plus sauter le détail ;
- les vues de l'aperçu restent montées, masquées par `hidden`, comme avec les anciens onglets : revenir au HTML ne recharge pas l'`iframe` ;
- `Intl.DisplayNames` est créé une fois par langue de la console (`useMemo`) ;
- le nom du fichier joint n'a plus qu'une variable.

Chaque correction a son test.

**Laissé, et pourquoi :**
- **Prétraitement MJML** : il peut décaler le numéro de ligne (attributs en double réunis sur une ligne). C'est rare et déjà écrit dans le plan ; l'extrait reste indicatif.
- **Import de l'éditeur** : l'import de la barre de l'éditeur d'email n'a ni extrait ni « Retry », parce que l'éditeur ne change pas (consigne).
- **« Reply to : Not set »** : la ligne apparaît aussi dans l'aperçu d'un message envoyé sans Reply-To, comme dans la maquette, qui la montre toujours.
- **Modal d'import** : il n'est monté que pendant qu'il est ouvert, donc sans animation de fermeture, comme les autres fenêtres de la page.
