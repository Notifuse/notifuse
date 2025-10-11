# Plan : Utiliser les labels personnalisés des custom fields dans le tableau des contacts

## Contexte

Sur la page des contacts (`console/src/pages/ContactsPage.tsx`), lorsque des custom fields sont sélectionnées dans le sélecteur de colonnes, elles affichent des labels génériques comme "Custom String 1", "Custom Number 1", etc.

Le workspace dispose d'une fonctionnalité permettant de définir des labels personnalisés pour ces champs dans `workspace.settings.custom_field_labels`, mais ces labels ne sont pas utilisés de manière cohérente dans le tableau.

## Analyse du problème

### État actuel

1. **Dans `allColumns` (lignes 240-274)** : Les titres utilisent **déjà** `getCustomFieldLabel()` ✓
   - Ces colonnes sont passées au `ContactColumnsSelector`
   - Le sélecteur affiche donc les bons labels personnalisés

2. **Dans les colonnes de la Table (lignes 620-763)** : Les titres utilisent des **chaînes codées en dur** ✗
   - Par exemple : `title: 'Custom String 1'`, `title: 'Custom Number 1'`, etc.
   - Ces colonnes sont utilisées pour afficher les données dans la table Ant Design
   - Elles n'utilisent pas les labels personnalisés du workspace

### Conséquence

Il y a une **incohérence** entre :
- Le sélecteur de colonnes qui montre les labels personnalisés
- Le tableau qui affiche les labels génériques

## Solution

Utiliser `getCustomFieldLabel()` pour les titres des colonnes du tableau, en cohérence avec ce qui est fait pour `allColumns`.

## Fichiers impactés

### 1. `console/src/pages/ContactsPage.tsx`

**Lignes à modifier** : 620-763 (définitions des colonnes custom fields)

#### Avant (exemple pour custom_string_1)
```typescript
{
  title: 'Custom String 1',
  dataIndex: 'custom_string_1',
  key: 'custom_string_1',
  hidden: !visibleColumns.custom_string_1
}
```

#### Après (exemple pour custom_string_1)
```typescript
{
  title: getCustomFieldLabel('custom_string_1', currentWorkspace),
  dataIndex: 'custom_string_1',
  key: 'custom_string_1',
  hidden: !visibleColumns.custom_string_1
}
```

### Custom fields à mettre à jour

**Custom String Fields (5)** : Lignes 620-648
- `custom_string_1` (ligne 621)
- `custom_string_2` (ligne 627)
- `custom_string_3` (ligne 633)
- `custom_string_4` (ligne 639)
- `custom_string_5` (ligne 645)

**Custom Number Fields (5)** : Lignes 650-678
- `custom_number_1` (ligne 651)
- `custom_number_2` (ligne 657)
- `custom_number_3` (ligne 663)
- `custom_number_4` (ligne 669)
- `custom_number_5` (ligne 675)

**Custom Datetime Fields (5)** : Lignes 680-718
- `custom_datetime_1` (ligne 681)
- `custom_datetime_2` (ligne 689)
- `custom_datetime_3` (ligne 697)
- `custom_datetime_4` (ligne 705)
- `custom_datetime_5` (ligne 713)

**Custom JSON Fields (5)** : Lignes 720-763
- `custom_json_1` (ligne 721)
- `custom_json_2` (ligne 730)
- `custom_json_3` (ligne 739)
- `custom_json_4` (ligne 748)
- `custom_json_5` (ligne 757)

**Total** : 20 colonnes à mettre à jour

## Étapes d'implémentation

### Étape 1 : Mise à jour des colonnes Custom String
Remplacer les titres codés en dur par `getCustomFieldLabel()` pour les 5 champs custom_string_X

### Étape 2 : Mise à jour des colonnes Custom Number
Remplacer les titres codés en dur par `getCustomFieldLabel()` pour les 5 champs custom_number_X

### Étape 3 : Mise à jour des colonnes Custom Datetime
Remplacer les titres codés en dur par `getCustomFieldLabel()` pour les 5 champs custom_datetime_X

### Étape 4 : Mise à jour des colonnes Custom JSON
Remplacer les titres codés en dur par `getCustomFieldLabel()` pour les 5 champs custom_json_X

### Étape 5 : Tests
Vérifier que :
1. Les labels personnalisés s'affichent correctement dans les en-têtes de colonnes
2. Les labels par défaut s'affichent quand aucun label personnalisé n'est défini
3. Le sélecteur de colonnes et le tableau affichent les mêmes labels
4. Les colonnes restent fonctionnelles (tri, filtres, etc.)

## Dépendances existantes

- `getCustomFieldLabel()` est déjà importé (ligne 35)
- `currentWorkspace` est déjà disponible (ligne 84)
- Le hook fonctionne déjà correctement pour `allColumns` (lignes 254-273)

## Test manuel

1. Aller dans les workspace settings
2. Définir un label personnalisé pour un custom field (ex : "Customer ID" pour `custom_string_1`)
3. Aller sur la page des contacts
4. Sélectionner la colonne dans le sélecteur de colonnes
5. Vérifier que :
   - Le sélecteur affiche "Customer ID" (déjà fonctionnel)
   - L'en-tête de colonne affiche "Customer ID" (nouveau comportement)

## Notes techniques

- La fonction `getCustomFieldLabel()` est définie dans `console/src/hooks/useCustomFieldLabel.ts`
- Elle retourne automatiquement le label par défaut si aucun label personnalisé n'est défini
- Format du label par défaut : "Custom {Type} {Number}" (ex: "Custom String 1")
- Le workspace stocke les labels dans `workspace.settings.custom_field_labels` sous forme de Record<string, string>

## Avantages de cette solution

1. **Cohérence** : Les mêmes labels apparaissent dans le sélecteur et le tableau
2. **Réutilisation** : Utilise la fonction existante `getCustomFieldLabel()`
3. **Maintenabilité** : Les labels sont centralisés dans les workspace settings
4. **UX améliorée** : Les utilisateurs voient des noms de champs significatifs

## Risques et considérations

- **Aucun risque de régression** : Le comportement par défaut reste identique si aucun label n'est défini
- **Compatibilité** : Fonctionne avec les workspaces existants qui n'ont pas encore défini de labels
- **Performance** : `getCustomFieldLabel()` est une fonction simple qui ne fait pas d'appel API

## Alternatives considérées

1. ❌ **Modifier uniquement les colonnes sans toucher allColumns** : allColumns utilise déjà la bonne approche
2. ❌ **Créer une nouvelle fonction** : getCustomFieldLabel() existe déjà et fonctionne bien
3. ✅ **Utiliser getCustomFieldLabel() pour les colonnes du tableau** : Solution simple et cohérente

## Références

- Fichier principal : `console/src/pages/ContactsPage.tsx`
- Hook utilisé : `console/src/hooks/useCustomFieldLabel.ts`
- Type workspace : `console/src/services/api/workspace.ts` (ligne 25)
- Configuration des labels : Composant `CustomFieldsConfiguration` dans les workspace settings
