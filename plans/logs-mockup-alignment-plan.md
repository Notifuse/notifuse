# Logs (maquettes 101 et 104) : alignement sur « Nuit claire »

Branche `feat/logs-mockup`, partie de `origin/dev` (fe3ba2f6). Front seulement : aucun changement de backend, de SQL ni de variable d'environnement.

## État de départ (mesuré le 2026-11-15)

Console de cette branche sur le workspace de démo, en compte root, avec Playwright en 1440 et 1280 (`deviceScaleFactor: 2`). Le journal d'audit de la démo est vide : ses entrées sont injectées au niveau du réseau (`page.route` sur `auditLogs.list`), sans rien écrire en base. Ce sont des actions que le serveur audite vraiment.

- **101 Messages**
  - **Recherche :** « Email ▾ Valeur exacte » (un champ parmi Email, ID de message, ID externe), là où la maquette a une boîte unique « Email, ID de message ou ID externe ». L'API ne cherche que sur un champ à la fois, en valeur exacte ; un « @ » tapé bascule déjà sur Email.
  - **Pas de colonne Fournisseur :** `message_history` n'a pas de champ fournisseur (comme pour la fiche 92 de la #587).
  - **Compteur Livré :** il affiche « — » avec l'info-bulle « Delivery isn't reported by SMTP providers » (`countsDeliveries`, `messageProvider.ts`). La démo envoie en SMTP. Déjà traité.
  - **Heure :** en UTC, comme le workspace de démo (« Last 7 days · UTC »). Déjà juste.
  - Ce qui diffère encore est listé dans les écarts voulus. Le reste suit la maquette : les compteurs par statut cliquables, le parcours, l'erreur, le panneau de détail (102, conforme).
- **104 Journal d'audit**
  - **Barre de filtres :** sur deux lignes en 1440 pour un compte root. La recherche fait 300 px (260 dans la maquette : `.cx-search { width: 260px }`), et le choix d'étendue « This workspace / Whole deployment », réservé au root, prend 245 px en deux boutons. « + Filter » passe à la ligne.
  - Le reste : Quand, Acteur (avatar ou clé), Action en phrase avec la cible en gras, Ressource, Origine, et le pied « Kept … · Change in Settings ». Écarts relevés par la relecture : l'ordre des filtres (Période d'abord), la Ressource en texte gris (étiquette dans la maquette), l'Origine dans une boîte à police fixe, la loupe absente, l'Export bordé.

## Décisions de Pierre (2026-11-15, planche `logs.html` sur le port 19281)

| # | Question | Choix |
|---|---|---|
| Q1 | Recherche de 101 | A · garder « Champ ▾ Valeur exacte » (écart voulu) |
| Q2 | Barre de filtres de 104 sur deux lignes | B · le choix d'étendue en menu « View ▾ », recherche plus étroite |
| Q3 | Largeur de la recherche, posée à nouveau après la relecture : la maquette fait 260 px et non ~230, le chiffre de Q2 était faux | A · 260 px. Mesuré : une ligne en 1440, en anglais comme en français |
| Q4 | Petits écarts de 104 (cumulables) | D · ordre Acteur, Ressource, Période ; E · Ressource en étiquette grise ; Exporter sans bordure, comme la maquette. Non retenus : F (Origine en texte simple) et G (loupe) |

## Changements

### 1. Barre du journal d'audit (Q2)

`console/src/components/logs/AuditLogTab.tsx` :
- **Recherche :** `style={{ width: 300 }}` devient `260`, la largeur de la maquette (Q3).
- **Choix d'étendue :** le `Segmented` des deux boutons devient un `FilterChip` « View This workspace ▾ ». C'est le même composant que Period, Actor et Resource, avec un menu `selectable` des deux étendues.
  - Le menu écrit `scope` dans l'URL comme avant (`deployment`, ou rien pour le workspace) et efface `id` et `actor_id`. Il reste réservé au root.
  - Recliquer sur l'étendue en cours ne fait rien (`if (key === scope) return`), comme le radio du Segmented, qui ne déclenchait pas `onChange`. Sans ce garde, le filtre Acteur et l'entrée ouverte étaient effacés, et une entrée d'historique écrite (bloquant de la relecture).
  - Le choix en cours n'est porté que visuellement dans le menu ; la puce fermée l'annonce (« View This workspace »), comme Period, Actor et Resource.
- **Ordre des filtres (Q4 · D) :** Actor, Resource, puis Period, comme la maquette.
- **Ressource (Q4 · E) :** la colonne rend `Pill tone="neutral" dot={false}`. Mesuré dans les 8 langues : l'étiquette la plus longue (« Registro de auditoría », 131 px) élargit la colonne, en largeur automatique et `scroll.x: 'max-content'`, sans débordement.
- **Exporter (Q4) :** `type="text"`, sans bordure comme la maquette. Le chevron reste, car le bouton ouvre le menu CSV ou NDJSON.
- **Libellés :** `scopeLabels` regroupe « This workspace » et « Whole deployment » (textes existants), pour la valeur de la puce et pour le menu.
- **Libellé de la puce :** `t({ message: 'View', context: 'audit log scope' })`. Le msgid « View » sans contexte existe déjà, mais il appartient à l'éditeur d'automatisation : le contexte évite d'en dépendre. Traductions : les mêmes noms que ce msgid (« Vue », « Ansicht », « Vista », « 表示 », « Visualização »).
- **Import :** `Segmented` est retiré des imports.
- **Prettier :** avec les options du prompt (largeur 120), le fichier n'était pas formaté sur dev, alors qu'avec une largeur de 100 il l'est. Je ne formate que mes lignes.

Mesuré sur la console réelle, sur captures :
- en 1440, une ligne, en anglais et en français ;
- en 1280 et en root, « Period » et « + Filter » passent à la ligne (la recherche à 260 et le nouvel ordre ajoutent un contrôle au retour à la ligne accepté avec Q2) ;
- un membre non root n'a pas la puce « View », et sa barre est plus courte encore.

Le texte d'aide de la recherche est tronqué (« IP address, actor email or tar… »), et son info-bulle donne la règle en entier.

## Écarts voulus (restent après ce chantier)

- **101 · recherche :** « Champ ▾ Valeur exacte » au lieu d'une boîte unique (Q1). La recherche reste exacte et dit sur quoi elle porte ; une boîte unique demanderait de deviner le champ ou un nouveau paramètre de l'API.
- **101 · colonne Fournisseur :** absente. Un message n'enregistre pas le fournisseur qui l'a envoyé, et afficher le fournisseur actuel serait faux pour les envois passés. C'est la même décision que pour la fiche 92 (#587, Q3).
- **101 · « Colonnes » :** la console a un bouton icône avec info-bulle, la maquette un bouton avec libellé. Laissé tel quel.
- **104 · Origine :** l'adresse IP seule. La maquette ajoute la ville (« · Paris »), mais aucune géolocalisation n'est enregistrée.
- **104 · Quand :** l'heure de la console a les secondes, et le journal d'audit les garde.
- **101 · puce « Statut Tous » :** absente. Le statut se filtre par les compteurs cliquables et par « + Filter ».
- **101 et 104 · loupe dans la recherche :** absente (Q4 · G non retenu ; en 101, la recherche est « Champ ▾ Valeur »).
- **104 · Origine :** l'adresse reste dans une boîte à police fixe (Q4 · F non retenu).
- **104 · contrôles en plus :** « + Filter » (filtres d'action et de résultat), Rafraîchir, le menu d'export CSV ou NDJSON, et le compteur « N entries » du pied servent des fonctions réelles, et la maquette ne les montre pas.
- **105 Webhooks sortants et 106 Détail d'une entrée d'audit :** sans maquette, pas redessinés.

## Traductions, pas de CHANGELOG

- **Nouveau texte :** un seul, « View » dans le contexte `audit log scope`, traduit dans les 7 langues.
- **CHANGELOG :** pas de ligne. La refonte des Logs est dans la version non publiée 44.0, avec sa ligne « Feature » (« The audit log reads as sentences… filtered by period, actor, resource, action or outcome »), et ce changement de disposition ne change pas ce qu'elle décrit.

## Tests

| Fichier modifié | Fichier de test | Cas |
|---|---|---|
| `components/logs/AuditLogTab.tsx` | `components/logs/AuditLogTab.test.tsx` | (a) « offers the deployment view to a root only… » réécrit : un membre non root ne voit ni « This workspace » ni « Whole deployment », quel que soit le nom de la puce. Un root, parti de `actor_id` et `id` dans l'URL, reclique « This workspace » sans aucune navigation (contrôle de mutation fait : sans le garde, le cas échoue) ; puis « Whole deployment » écrit exactement `{ tab: 'audit', scope: 'deployment' }`. La recherche garde 260 px : le style du premier ancêtre qui en a un, sans preuve de la ligne unique, vérifiée sur capture. (b) « lays out the toolbar and the resource as the mockup does » : l'ordre Actor, Resource, Period dans le DOM, Export en `ant-btn-text`, et la Ressource en pastille `bg-neutral-bg` sans point. |

## Vérification à l'écran

`scratchpad/logs/auditbar2.mjs` (root, entrées injectées) : captures de la barre en 1440 et 1280, comparées à la maquette 104. Le nombre de lignes se lit sur la capture : les contrôles de droite, centrés entre deux lignes, faussent un comptage par position.

## Étape finale : tests

Depuis `console/` :
1. `npx vitest run src/components/logs src/i18n`
2. `npx tsc -b`
3. `npx eslint src/components/logs/AuditLogTab.tsx src/components/logs/AuditLogTab.test.tsx`
4. `npm run lingui:extract`, et vérifier que seul le nouveau msgid apparaît, traduit dans les 7 langues.
5. Il n'y a pas d'e2e propre aux Logs (`e2e/features` n'en a pas, et aucune spec ne vise le journal d'audit) : `npx playwright test e2e/protected-pages.spec.ts`, avec une config temporaire qui vise le port 19273. Cette spec vérifie seulement que la page charge sur l'onglet Messages ; la barre d'audit se vérifie sur captures (`scratchpad/logs/final-en.mjs`, `final-fr.mjs`).

Puis une `/code-review medium` sur le diff final (budget : 3 au plus pour ce chantier ; une faite, sur le diff non commité). Après elle, deux petites retouches, vérifiées à la main (relecture du diff, tests du fichier, `tsc`, eslint) : les éléments du menu « View » dérivés de `scopeLabels`, et l'absence de l'étendue pour un non-root testée sur ses deux options. Commit seulement sur l'accord de Pierre.
