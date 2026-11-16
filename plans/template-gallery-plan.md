# Galerie de modèles à la création d'un modèle

Créer un modèle part d'un point de départ : un modèle de la galerie, un des modèles du workspace, ou « Vierge ». Pour que le modèle « Panier abandonné » liste le panier, les emails d'automatisation reçoivent en plus les données de l'événement qui a déclenché l'automatisation.

## Références (2026-11-15)

- Maquette UX : https://claude.ai/artifact/3XJTfRnAmrTr1SyCGpaXtd (source `scratchpad/template-gallery-mockup.html`).
- Planche des 16 modèles : https://claude.ai/artifact/HHPfPxvo6BQjkEZ28KtSw7 (source `scratchpad/template-gallery-models.html`). Les textes, objets, textes d'aperçu et données de test de la planche sont la version anglaise de référence.
- Tiroir de modèle aligné sur l'écran 125 (capture de Pierre), mesures ramenées au thème : https://claude.ai/artifact/4nwSV9ayKuqwQ9vuRC9fq8 (source `scratchpad/template-drawer-mockup.html`).
- État de départ : `origin/dev`, `VERSION = "44.0"` (non publiée), migration la plus haute `v44.go`.

## Décisions de Pierre (2026-11-15)

| # | Sujet | Choix |
|---|---|---|
| P1 | Où se fait le choix | A · une fenêtre « Nouveau modèle » (Galerie / Vos modèles) avant le tiroir, qui s'ouvre prérempli ; le tiroir garde ses 3 étapes (son cadre est réaligné, partie C) |
| P2 | Langue du contenu des modèles | La langue par défaut du workspace, dans les 8 langues de la console |
| P3 | Où vivent les modèles | Dans la console ; l'API et le MCP ne voient pas la galerie |
| P4 | Liste des modèles | Les 16 de la planche (15 + Vierge) |
| P5 | Panier abandonné | Passer l'événement déclencheur aux emails d'automatisation (partie A) |
| P6 | Images | Servies par l'instance Notifuse elle-même |
| P7 | `components/templates/newsletter.json` | Fichier mort, laissé tel quel pour l'instant |
| T1 | Où montrer le point de départ dans le tiroir | A · une carte « Contenu » sous l'aperçu de boîte de réception : vignette, « Parti du modèle « … » », « Changer » ; en édition la carte reste, sans le point de départ, et mène à l'étape Contenu |
| T2 | À la place de « Non publié » / « Enregistré il y a 8 s » | A · « Nouveau » à la création (bouton « Créer le modèle »), sinon « Enregistré il y a … » tiré de `updated_at`, et « Modifications non enregistrées » dès une modification (bouton « Enregistrer la v{N+1} ») ; pas de brouillon ni d'enregistrement automatique |
| T3 | Passer d'une étape à l'autre | A · navigation libre dès la création ; « Créer » vérifie toutes les étapes, ouvre celle en erreur et la marque d'un point rouge ; plus de Suivant / Précédent |
| T4 | Marquer les champs remplis par le modèle | A · pas de marque ; la carte Contenu dit d'où vient le modèle |

## Décisions de Pierre, suite (2026-11-15)

| # | Sujet | Choix |
|---|---|---|
| Q1 | Variable Liquid de l'événement | `trigger_event` |
| Q2 | Traductions préremplies | Oui, une par autre langue du workspace couverte par la galerie |
| Q3 | Copie depuis « Vos modèles » | Comme « Dupliquer » : « {nom} copy », `…-copy`, mode d'édition gardé |
| Q4 | Vignettes | Rendu réel ; « Vos modèles » ne compile que les lignes visibles |
| Q5 | Livraison | Tout en local, dans un worktree (`.claude/worktrees/template-gallery`, branche `feat/template-gallery` partie d'`origin/dev`), sans PR pour l'instant |
| Q6 | Envoyer un test avec des modifications | Confirmation : « Enregistrer la v{N+1} et envoyer » ou « Envoyer la v{N} » |
| Q7 | Bureau / mobile de l'aperçu | Texte, comme la newsletter |

## Écarts au plan, apparus pendant le code

- **Vierge** laisse vides le nom, l'objet et le texte d'aperçu, comme une création l'a toujours fait ; seuls les blocs (logo, paragraphe, pied de page) viennent du modèle.
- **Libellés** : « Email subject » devient « Subject » et « Subject preview » devient « Preview text », comme la maquette (« Objet », « Texte d'aperçu »). Les messages d'erreur Liquid de l'objet, jusqu'ici non traduits, passent par Lingui.
- **Images** : JPEG (moins de 9 Ko chacune), le logo en PNG. Générées par `console/scripts/generate-gallery-images.py`.
- **Lignes de commande et de panier** : un bloc `mj-liquid` (boucle `{% for %}` en MJML), que l'éditeur affiche en code et ne réécrit pas.
- **`FullDrawer`** reçoit `barCenter` (les étapes) et `keyboard` (le tiroir de modèle ne se ferme pas avec Échap, comme avant).
- **Migration, décision de Pierre (2026-11-15)** : tout va dans la v44, encore non publiée (pas de v45, `VERSION` reste 44.0). v44 installe les deux fonctions d'inscription en fin de liste, puis remplace seulement le **corps** de la fonction de chaque trigger d'automatisation active (`CREATE OR REPLACE FUNCTION`, sans DROP ni CREATE TRIGGER) : aucun verrou sur `contact_timeline` pendant que v44 tient `message_history` et des lignes d'`automations`, et aucun trigger ne peut être élargi. La boucle de v38 est sortie dans `internal/migrations/automation_trigger_regen.go` (option `bodiesOnly`) ; v38 génère les corps d'avant 44 (`generator.WithoutTriggerEvent()`), sinon ses triggers appelleraient une fonction qui n'existe qu'après v44.
- **Bases déjà en 44** (développement local, toute instance qui tourne sur `dev`) : elles n'exécuteront pas cette étape. Il faut y appliquer les deux fonctions et les corps à la main, ou recréer la base.

- **Logo et nom du workspace (Pierre, 2026-11-16)** : les modèles prennent `settings.logo_url` à la place du logo de la galerie, et le nom du workspace à la place de « Acme » dans tous les textes et toutes les langues (`withBrand`, qui retire `< > { }` du nom) ; sans logo ni nom, l'exemple reste.

## Partie A · L'événement déclencheur dans les emails d'automatisation (serveur)

Constat (rapport d'exploration du 2026-11-15) :
- Le trigger généré par automatisation (`internal/service/automation_trigger_generator.go:273-326`) appelle `automation_enroll_contact(id, email, root_node_id, frequency)` sans transmettre `NEW`.
- La fonction d'inscription (`internal/database/schema/automation_triggers.go:18-126`) n'écrit rien de l'événement.
- `contact_automations.context JSONB DEFAULT '{}'` existe depuis v20 et n'est écrit par aucun code. Il est réclamé avec la ligne (`automation_postgres.go:1189-1273`) et arrive dans `params.Contact.Context` pour chaque étape.
- Pour un événement personnalisé, la ligne `contact_timeline` ne porte pas les propriétés : elles sont dans `custom_events.properties` (clé `event_name` = `substring(kind from 14)`, `external_id` = `entity_id`).
- L'email d'une automatisation reçoit aujourd'hui seulement le contact, la liste et `automation_id` / `automation_name` (`automation_node_executor.go:314-327`).

Changements :
1. **Fonction d'inscription** · `internal/database/schema/automation_triggers.go` : une surcharge à 5 arguments `automation_enroll_contact(..., p_trigger JSONB)`, qui écrit `context = jsonb_build_object('trigger', p_trigger)`. La version à 4 arguments reste telle quelle : les triggers installés l'appellent par son nom tant qu'ils ne sont pas régénérés, et v20, v21 et v25 gardent leur copie d'origine.
2. **Générateur** · `internal/service/automation_trigger_generator.go` : le corps généré construit `jsonb_build_object('kind', NEW.kind, 'entity_type', NEW.entity_type, 'entity_id', NEW.entity_id, 'changes', NEW.changes, 'occurred_at', NEW.created_at)`. Pour un déclencheur `custom_event.*`, le générateur ajoute `'name'` et `'properties'`, lus dans `custom_events` par `(event_name, external_id)`. Puis il appelle la surcharge à 5 arguments.
3. **Exécuteur** · `internal/service/automation_node_executor.go` : `ProvidedData["trigger_event"] = params.Contact.Context["trigger"]` quand la clé existe. Une inscription antérieure à la migration n'a pas la clé, et la variable est alors absente : le modèle « Panier abandonné » teste `{% if trigger_event.properties.items %}`.
4. **Base neuve** · `internal/database/init.go:829` : installe la surcharge à côté de la fonction actuelle.
5. **Migration** · dans `internal/migrations/v44.go` (décision de Pierre ; voir « Écarts au plan ») : sur chaque base de workspace, installe la surcharge, puis régénère le trigger de chaque automatisation active avec le vrai générateur. Chaque condition est sondée avant l'installation, avec un savepoint par automatisation, sur le modèle de `V38Migration.healAutomationTriggerConditions`. La migration est idempotente : `CREATE OR REPLACE`, et la régénération réécrit le même corps.
6. **Version et changelog** : `config/config.go` `VERSION` passe au majeur de la migration. Une ligne **Feature** dans `CHANGELOG.md` : « Automation emails can use the event that started the automation, with `{{ trigger_event }}`: a custom event's properties, a list or segment change, or the clicked link. »
7. **Prompt de l'assistant email** · `console/src/components/email_builder/email-ai-system-prompt.ts` : ajouter `trigger_event` à la liste des variables d'automatisation.
8. **Docs** (`../docs`, PR à part) : la page des variables Liquid des automatisations, avec un exemple de panier.

Points à vérifier pendant le code :
- Taille : un `properties` volumineux est copié par inscription. On vérifie la limite de taille d'un événement personnalisé à l'ingestion. S'il n'y en a pas, on n'en ajoute pas : la copie n'est qu'une ligne par inscription.
- Rétention et effacement : la suppression d'un contact doit supprimer ses `contact_automations` (cascade ou suppression explicite). Le plan de rétention doit savoir que `context` contient désormais une copie des propriétés.
- `every_time` et la mise à jour d'un événement : chaque mise à jour réinscrit le contact (`domain/custom_event.go:272-277`). Chaque inscription garde l'état de l'événement à ce moment-là, ce qui est le comportement voulu pour un panier.
- Le dépôt `../architecture` ne couvre pas les automatisations. Rien à y mettre à jour, sauf si une fiche les mentionne : on vérifie avec grep.

Tests (partie A) :
- `internal/database/schema/automation_triggers_test.go` : la surcharge à 5 arguments écrit `context.trigger`, et la version à 4 arguments laisse `context = '{}'`.
- `internal/service/automation_trigger_generator_test.go` : le corps généré pour `custom_event.cart_abandoned` lit `custom_events.properties` et appelle la surcharge. Pour `list.subscribed`, il ne fait pas de lecture dans `custom_events`. Un nom d'événement contenant une apostrophe est bien échappé.
- `internal/service/automation_node_executor_test.go` : avec `Context = {"trigger": {...}}`, les données passées à `BuildTemplateData` contiennent `trigger_event`. Avec `{}`, elles n'ont pas la clé.
- `internal/migrations/v44_test.go` : la surcharge est installée et les triggers des automatisations actives sont régénérés. Une automatisation dont la condition échoue est sautée sans annuler les autres, et un second passage ne change rien.
- Intégration · `tests/integration/automation_trigger_event_test.go` : une automatisation déclenchée par `custom_event.cart_abandoned` avec `properties.items` envoie un email. On lit cet email dans Mailpit et on vérifie qu'il liste les articles (mémoire : tests qui lisent la vraie sortie). Le même test, sur un contact inscrit avant la migration, envoie un email sans les articles et sans erreur.

## Partie B · La galerie dans la console

### B1 · Les modèles

Les fichiers sont dans `console/src/components/templates/gallery/` :
- `types.ts` : `GalleryModel` (id, cas d'usage, catégorie, qui l'envoie, `build(content, assets)`), `GalleryContent` (les textes d'un modèle, typés par modèle) et `GalleryLanguage` (les 8 langues).
- `blocks.ts` : les briques qui fabriquent un arbre `EmailBlock` : en-tête, hero, texte, bouton, code, lignes clé/valeur, articles, grille de produits, code promo, article, événement, étapes, citation, et les pieds de page marketing, transactionnel et adresse seule. Les attributs par défaut (`mj-attributes`) portent la couleur d'accent, pour qu'on la change à un seul endroit. Les ID des blocs sont stables (`<modèle>-<brique>-<n>`).
- `models.ts` : les 16 modèles de la planche, chacun avec sa catégorie, son objet, son texte d'aperçu et ses données de test. Les lignes de commande et le panier passent par une boucle `{% for item in … %}` dans un bloc `mj-raw`/`mj-text`. On vérifie que l'éditeur visuel affiche ce bloc sans le casser.
- `content/{en,fr,es,de,ca,pt-BR,ja,it}.ts` : les textes, l'objet et le texte d'aperçu par modèle. `GalleryContent` rend une langue incomplète impossible à compiler. Ce sont des contenus d'email, pas des chaînes d'interface, donc pas de Lingui.
- `resolveGalleryLanguage.ts` : de la langue par défaut du workspace à la langue du contenu. `pt` donne `pt-BR`, une langue hors des 8 donne `en`.
- `buildStartingPoint.ts` : construit, à partir d'un modèle et du workspace, ce que le tiroir prérempli attend (nom, ID, catégorie, objet, texte d'aperçu, arbre, données de test). Les données du modèle passent par-dessus `buildDefaultTestData`. Il ajoute une traduction par autre langue du workspace couverte par la galerie (Q2).
- « Vierge » remplace `email-template.json` comme contenu de secours (`createDefaultBlocks` dans `CreateTemplateDrawer.tsx`). On supprime `email-template.json` quand plus rien ne l'importe.

### B2 · Les images

- Des PNG neutres (dégradés et formes, sans texte) dans `console/public/gallery/`. Un script `console/scripts/generate-gallery-images.py` les génère avec Pillow, à moins de 60 Ko chacune, et les fichiers générés sont commités. Le SVG est exclu, car Gmail le bloque.
- `vite.config.ts` a `base: '/console/'` et le serveur Go sert la console sous `/console` (`internal/http/root_handler.go:205-229`). L'URL d'une image est donc `${API_ENDPOINT}/console/gallery/<fichier>.png`, que `buildStartingPoint` reçoit en paramètre (`assets.baseUrl`).
- L'URL est figée dans le modèle à sa création. Si le domaine de l'instance change, ou si l'instance n'est pas joignable depuis Internet, les images ne s'affichent pas chez les destinataires. Les liens de suivi ont déjà ce besoin. On n'ajoute rien au code, une phrase dans la doc.
- En dev, Vite sert `public/` sous `/console/`, et `API_ENDPOINT` pointe sur localapi : on vérifie que l'image s'affiche dans l'éditeur en local.

### B3 · La fenêtre « Nouveau modèle »

`console/src/components/templates/TemplateStartModal.tsx` :
- Onglets « Galerie » et « Vos modèles ». « Vos modèles » est masqué sans le droit de lecture des modèles.
- La galerie a une colonne de cas d'usage avec leurs comptes, une recherche, et des cartes avec vignette, nom et pastille de catégorie. « Vierge » vient en premier.
- Un clic sur une carte ouvre l'aperçu, avec la bascule Bureau / Mobile, la fiche (catégorie, objet, sections, variables) et le bouton « Utiliser ce modèle ». « ← Galerie » garde le filtre et la recherche. Un double-clic utilise directement le modèle.
- « Vos modèles » a une recherche et un filtre de catégorie, trie par date de mise à jour, et propose « Aperçu » et « Partir de ce modèle ».
- Avec `forceCategory`, on ne voit que les modèles de cette catégorie et « Vierge », sans colonne de filtres, avec le bandeau « Catégorie … imposée par … ».
- Vignettes (Q4) : `templates.compile` (droit de lecture des modèles) avec TanStack Query, clé `['gallery-thumb', workspaceId, lang, modelId]`, rendu réduit dans un `iframe` `sandbox`. Pour « Vos modèles », seulement les lignes visibles (IntersectionObserver).
- Clavier : Échap ferme, les flèches parcourent les cartes, Entrée ouvre l'aperçu. Toutes les chaînes passent par Lingui (`console/CLAUDE.md`).

### B4 · Le tiroir prérempli

`console/src/components/templates/CreateTemplateDrawer.tsx` :
- La création sans `template` ni `fromTemplate` ouvre d'abord `TemplateStartModal`. Elle ouvre le tiroir directement si la fenêtre renvoie « Vierge ». Comme les 5 points d'entrée passent par ce composant, aucun appelant ne change, hormis `TemplatesPage` en mode contrôlé (`openDrawer({ mode: 'create' })`), qui reçoit le même traitement.
- Nouvelle prop interne `startingPoint` (modèle de la galerie). Un modèle choisi dans « Vos modèles » passe par le chemin `fromTemplate` existant (Q3).
- Le point de départ s'affiche dans la carte Contenu de la colonne de droite (T1, partie C), avec « Changer ». Si l'arbre a été modifié, « Changer » demande confirmation avant de remplacer le contenu.
- Les champs préremplis n'ont pas de marque (T4). L'expéditeur n'est jamais prérempli.
- ID : `kebabCase` du nom, comme aujourd'hui. Si l'ID est déjà pris dans la liste des modèles chargée, on ajoute `_2`, `_3`, etc. Sans droit de lecture, on laisse le serveur refuser, comme aujourd'hui.

### B5 · Changelog et docs

- Une ligne **Feature** dans `CHANGELOG.md` (dans la même version que la partie A si elles partent ensemble) : « Creating a template starts from a gallery of 15 ready-made emails (verification code, receipt, order confirmation, newsletter…) written in the workspace's language, or from one of your templates. »
- `../docs`, PR à part : la page Modèles, avec la galerie, et la phrase sur les images servies par l'instance.
- `console/e2e/features/templates.spec.ts` : « New template » ouvre maintenant la fenêtre. Le parcours de l'e2e passe par « Vierge ».

Tests (partie B) :
- `gallery/models.test.ts` :
  - chaque modèle construit, dans chaque langue, un arbre valide (racine `mjml`, ID uniques, aucun texte vide) ;
  - chaque modèle Marketing ou Bienvenue porte `{{ unsubscribe_url }}` ;
  - aucun modèle Transactionnel, Opt-in ou Désabonnement ne le porte ;
  - l'Opt-in porte `{{ confirm_subscription_url }}` ;
  - chaque variable Liquid de l'arbre est présente dans ses données de test (ou fournie par l'envoi : `contact`, `list`, `unsubscribe_url`…).
- `gallery/resolveGalleryLanguage.test.ts` : `fr` donne `fr`, `pt` donne `pt-BR`, `nl` donne `en`, et une valeur vide donne `en`.
- `gallery/buildStartingPoint.test.ts` :
  - les données du modèle passent par-dessus les données par défaut ;
  - les traductions ne sont créées que pour les langues du workspace couvertes ;
  - les URL d'images partent de `assets.baseUrl` ;
  - l'objet du modèle « Commande » est `Your order #{{ order.number }} is confirmed` en anglais.
- `TemplateStartModal.test.tsx` :
  - les filtres et leurs comptes, et la recherche ;
  - « Vierge » en premier ;
  - `forceCategory` masque les autres catégories ;
  - « Vos modèles » est absent sans droit de lecture ;
  - l'aperçu, puis « ← Galerie », garde le filtre ;
  - Échap ferme sans rien créer ;
  - Entrée ouvre l'aperçu.
- `CreateTemplateDrawer.test.tsx` :
  - la création ouvre la fenêtre, et l'édition ne l'ouvre pas ;
  - « Vierge » ouvre le tiroir avec l'arbre Vierge ;
  - un modèle préremplit nom, ID, catégorie, objet, texte d'aperçu et données de test ;
  - un ID déjà pris reçoit `_2` ;
  - « Changer » après une modification demande confirmation ;
  - `forceCategory` est gardé.
- `pages/TemplatesPage.test.tsx` : « New template » ouvre la fenêtre.
- Intégration (partie B, console seule) : pas de nouveau test Go. Le test d'intégration de la partie A couvre l'envoi du panier.
- Vérification dans la console locale (notifusedev.com:5173) :
  - créer un modèle depuis chacun des 5 points d'entrée ;
  - envoyer un test du modèle « Commande » ;
  - ouvrir l'email reçu dans Mailpit et vérifier que les images se chargent ;
  - refaire le parcours sur un workspace en `fr`.

## Partie C · Le tiroir de modèle aligné sur l'écran 125 (console)

Le cadre du tiroir et son étape Paramètres, laissés hors périmètre par `plans/templates-mockup-alignment-plan.md`, sont alignés sur la capture de Pierre. Les mesures suivent le thème et non la capture, qui n'est pas à l'échelle (tableau en tête de la maquette du tiroir) : champs et boutons de 28 px, texte de 13 px, barre de 40 px, `Panel` à en-tête de 40 px, colonne de droite de 320 px. Il ne faut aucune valeur entre crochets pour une couleur, une taille de texte ou un rayon, et pas de classe Tailwind sur un composant antd (`console/CLAUDE.md`).

Changements dans `console/src/components/templates/CreateTemplateDrawer.tsx` (à découper en sous-composants, voir plus bas) :
- **Cadre** : `FullDrawer` remplace le `Drawer` antd, comme `BroadcastCompose`, avec :
  - `backLabel` « Templates » ;
  - `barTitle` : le nom, `v{N}` en édition, puis l'état (T2) ;
  - au centre, le `Segmented` des trois étapes ;
  - `topActions` : « Envoyer un test » et le bouton principal.

  `FullDrawer` n'a pas d'emplacement central : on en ajoute un (prop `barCenter`), avec ses tests.
- **État (T2)** :
  - un `Pill` « Nouveau » (ton primary) à la création ;
  - « Enregistré il y a … », tiré de `updated_at` avec `dayjs().fromNow()`, comme le `SaveStatus` de la newsletter ;
  - un `Pill` « Modifications non enregistrées » (ton warning) dès que `dirtyRef` passe à vrai.

  Le bouton principal dit « Créer le modèle » ou « Enregistrer la v{N+1} ».
- **Étapes (T3)** : navigation libre. « Suivant » et « Précédent » disparaissent, ainsi que `goNext`. L'enregistrement valide le formulaire entier. Si un champ d'une autre étape est en erreur, on ouvre la première étape en erreur, et le `Segmented` la marque d'un point rouge, accompagné d'un libellé accessible.
- **Paramètres** : deux `Panel`.
  - **Identité** : nom, ID (`utm_content`, aide « Ne pourra plus changer après la création », grisé en édition), catégorie, et le mode d’édition en deux cartes (Visuel / Code (MJML), 28 px comme les autres champs).
  - **Enveloppe**, avec « Langue par défaut : … » en `extra` : objet et texte d'aperçu (suffixés de la langue quand il y a des traductions), puis expéditeur avec son aide existante et répondre à (ordre demandé par Pierre).

  Les champs sont en `grid grid-cols-2 gap-3` avec `Form.itemMarginBottom: 0`, comme `AudienceCard`.
- **Objet** : un bouton `{ }` dans le champ ouvre la liste des variables des données de test et insère `{{ … }}` au curseur. L'aide dit « Liquid accepté, par exemple `{{ contact.first_name }}` ».
- **Colonne de droite** (`w-aside`, collante comme dans `BroadcastCompose`) :
  - un `Panel` « Aperçu dans la boîte de réception », avec bureau / mobile en `Segmented` petit dans l'en-tête. Il montre la ligne de boîte de réception, objet et texte d'aperçu rendus avec les données de test par `templates.compile` (comme `PreviewPanel`), au-dessus d'une ligne d'exemple fixe. Il remplace `IphoneEmailPreview` dans cette étape ;
  - un `Panel` « Contenu » avec la vignette du rendu réel, « Modifier → » qui ouvre l'étape Contenu, et, à la création depuis la galerie, « Parti du modèle « … » » et « Changer » (T1).
- **Envoyer un test** : nouveau dans le tiroir, il ouvre `SendTemplateModal`. L'envoi de test ne connaît qu'un modèle enregistré (`TestTemplateRequest.TemplateID`, `internal/domain/transactional.go:163`). Le bouton est donc :
  - grisé avant la création, avec l'infobulle « Créez le modèle pour l'envoyer en test » ;
  - avec des modifications non enregistrées, il ouvre d'abord une confirmation : « Le test enverra la v3 enregistrée », avec les boutons « Enregistrer la v4 et envoyer » et « Envoyer la v3 ».

  Il demande les mêmes droits que dans la liste.
- **Fermeture** : aujourd'hui la croix vide le formulaire sans prévenir (`handleClose`). Elle demandera confirmation quand `dirtyRef` est vrai, avec le `Modal.confirm` déjà utilisé ailleurs.
- **Barre de l'éditeur d'email** (maquette https://claude.ai/artifact/Agsc6kszxVgmdHwnDuYQxk, décisions E1-E3 de Pierre : A, A, A) : 40 px, fond `bg-subtle`, filet `line-soft`, marges 12 px, contrôles de 28 px ; Annuler / Rétablir en icônes lucide ; Aide et Importer / Exporter en boutons par défaut (plus de violet « ghost ») ; même traitement dans l'éditeur de code MJML (bascule Ordinateur / Mobile en icônes centrées).
- **Découpage** : le fichier dépasse 1 500 lignes. L'étape Paramètres sort dans `TemplateSettingsStep.tsx`, la barre dans `TemplateDrawerBar.tsx`, et la colonne de droite dans `TemplateSettingsAside.tsx`. La logique d'enregistrement, de conflit (409) et de traductions ne bouge pas.
- **Lingui** : nouvelles chaînes extraites et traduites dans les 8 `.po`. Elles passent en dernier dans l'ordre des PR, car tous les `.po` entrent en conflit entre PR parallèles (mémoire du projet).
- **Changelog** : la ligne Feature de la galerie (B5) mentionne aussi le nouveau tiroir, l'envoi de test depuis le tiroir et la confirmation à la fermeture. Il n'y a pas de ligne Fix : la fermeture sans confirmation n'était pas un défaut publié, et elle est améliorée dans la même version.

Tests (partie C) :
- `components/layout/FullDrawer.test.tsx` : `barCenter` s'affiche au centre de la barre, et son absence ne change rien aux écrans existants.
- `TemplateDrawerBar.test.tsx` :
  - « Nouveau » et « Créer le modèle » à la création ;
  - « v3 », « Enregistré il y a … » et « Enregistrer la v4 » en édition ;
  - « Modifications non enregistrées » après une modification ;
  - le point rouge et son libellé accessible sur l'étape en erreur.
- `TemplateSettingsStep.test.tsx` :
  - l'ID est grisé en édition ;
  - le mode d'édition passe par les deux cartes (clavier compris) ;
  - le bouton `{ }` insère la variable au curseur ;
  - les libellés portent la langue quand il y a des traductions.
- `TemplateSettingsAside.test.tsx` :
  - la ligne de boîte de réception montre l'objet rendu ;
  - « Changer » n'apparaît qu'à la création depuis la galerie, et demande confirmation si l'arbre a changé ;
  - « Modifier → » ouvre l'étape Contenu.
- `CreateTemplateDrawer.test.tsx` (en plus de B) :
  - navigation libre dès la création ;
  - l'enregistrement avec un champ manquant d'une autre étape ouvre cette étape ;
  - « Envoyer un test » est grisé avant la création ;
  - avec des modifications, il propose « Enregistrer la v4 et envoyer » ;
  - la croix demande confirmation si le tiroir est modifié, et ferme directement sinon ;
  - le conflit 409 se comporte comme avant.
- `console/e2e/features/templates.spec.ts` : le parcours de création n'a plus de « Suivant ». Les sélecteurs suivent le nouveau nom accessible du tiroir.
- Vérification visuelle :
  - captures Playwright à 1440 px et 1200 px de large ;
  - à comparer avec la maquette du tiroir sur une planche, comme pour les écrans 127-129.

## Commandes de test

Partie A :
- `docker compose -f tests/compose.test.yaml up -d`
- `make test-domain test-service test-repo test-migrations`
- `go test ./internal/database/schema/...`
- `INTEGRATION_TESTS=true go test -tags integration,licdev ./tests/integration/ -run 'TestAutomationTriggerEvent'` : seulement les tests touchés.

Parties B et C (console) :
- `cd console && npm run lint && npx tsc --noEmit && npm test`
- `cd console && npm run lingui:extract`, puis compléter les 8 `.po`.
- `cd console && npx playwright test e2e/features/templates.spec.ts`.
