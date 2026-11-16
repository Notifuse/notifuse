# Broadcasts : remettre en brouillon un broadcast programmé pas encore lancé (`broadcasts.unschedule`)

Plan seulement, rien n'est codé. Écrit le 2026-11-15 sur `feat/contacts-ui-polish` (e935253b), à la demande de Pierre : un broadcast programmé qu'on veut retravailler ne peut aujourd'hui qu'être annulé, et « Cancel » le rend `cancelled` pour toujours. Version 2 : corrections d'une relecture indépendante (Opus, effort élevé) intégrées, chaque référence revérifiée dans le code.

## Constat (vérifié dans le code)

- **Aucun retour en brouillon.** Les appels sont `list`, `estimateAudience`, `get`, `create`, `update`, `schedule`, `pause`, `resume`, `cancel`, `retryFailed`, `sendToIndividual`, `delete`, `getTestResults`, `selectWinner`, `refreshGlobalFeed`, `testRecipientFeed` (`internal/http/broadcast_handler.go`). Aucun ne repasse un broadcast en `draft`.
- **Cancel est définitif.** `BroadcastService.CancelBroadcast` (`internal/service/broadcast_service.go:839`) passe `scheduled`, `paused`, `processing` ou `processed` à `cancelled`, supprime les lignes en file (`DeleteBySourceTx`) et publie `broadcast.cancelled`, que `TaskService.handleBroadcastCancelled` (`internal/service/task_service.go:1628`) reçoit pour sa tâche. Un broadcast `cancelled` ne peut plus être modifié (`UpdateBroadcastRequest.Validate`, contrôle du statut à `internal/domain/broadcast.go:733`) ni programmé (`ScheduleBroadcast` exige `draft`).
- **Pause ne s'applique pas à un programmé** : elle n'accepte que `processing` et `processed` (`PauseBroadcast`, `broadcast_service.go`). Sur un programmé, seul Cancel agit.
- **« Programmé » ne veut pas dire « pas lancé ».** Un broadcast programmé garde le statut `scheduled` pendant que sa tâche envoie (`internal/service/broadcast/orchestrator.go:648`).
- **`started_at` vide ne prouve pas que l'envoi n'a pas commencé.** Chaque run d'envoi le pose s'il est vide, mais un échec de cette écriture est seulement journalisé et l'envoi continue (`orchestrator.go:1204-1225`). La mise à jour v44 ne l'a rempli que pour les statuts autres que `draft` et `scheduled` (`internal/migrations/v44.go`, `v44StartedAtBackfillSQL`) : un envoi programmé déjà en cours à la mise à jour n'a pas de date jusqu'à son run suivant. Entre deux runs, ou après un run échoué qui a encore des essais, la tâche revient à `pending`.
- **La boucle d'envoi ne s'arrête que sur `cancelled` ou `paused`**, jamais sur `draft` (`orchestrator.go`, vers la ligne 1277). Elle ne vérifie un brouillon qu'au début d'un run (`refuseDraft`, `orchestrator.go:1670`).
- **La tâche vit dans la base système, le broadcast dans celle du workspace** : pas de transaction commune. `ScheduleBroadcast` verrouille le broadcast (`GetBroadcastForUpdateTx`) puis écrit la tâche dans sa transaction (`ScheduleBroadcastTask`, `task_service.go:1235`). Cancel écrit le broadcast puis attend `handleBroadcastCancelled`, qui verrouille la tâche. Ordre établi partout : **broadcast, puis tâche**.
- **`refuseDraft`** rend la tâche d'un brouillon pour 30 s tant qu'elle a moins de 10 min, puis la termine sans rien envoyer. `ResetBroadcastTaskTx` remet aussi `created_at` à zéro.
- **Reprogrammer réutilise la tâche** : `ScheduleBroadcastTask` relit la tâche du broadcast et la remet à zéro (`ResetBroadcastTaskTx`) si elle existe, sinon il en crée une.
- **La réservation d'une tâche** est sérialisée par `ExecuteTask` (`GetTx ... FOR UPDATE`, `internal/repository/task_postgres.go:167-177`) et la garde de `MarkAsRunningTx` (`task_postgres.go:761`) ; `GetNextBatch` utilise `FOR UPDATE SKIP LOCKED` mais hors transaction (`task_postgres.go:586-592`). Une tâche supprimée pendant qu'une réservation attend revient « introuvable » : sans envoi. Une tâche « tenue » se lit avec `notHeld` (`task_postgres.go:1454`) : `running` sans bail échu, ou bail encore actif.
- **`GetTaskByBroadcastIDTx`** (`task_postgres.go:1072`) lit avec `FOR UPDATE` et `LIMIT 1`, sans `ORDER BY` ; aucun index unique n'empêche deux tâches `send_broadcast` pour un même broadcast (non confirmé qu'il en existe).
- **`UpdateBroadcast` lit sans verrou** (`broadcast_service.go:241`, `GetBroadcast`) puis réécrit toute la ligne, statut et programmation compris (`internal/repository/broadcast_postgres.go`, vers 263-288).
- **Toute route est classée dans le catalogue d'audit** (`internal/domain/audit_actions.go`, `broadcasts.cancel` à la ligne 159), ce que vérifie `internal/http/audit_routes_test.go`.
- **Pas d'événement sortant** : `EventBroadcastScheduled` est déclaré mais jamais publié ; les webhooks sortants n'ont pas d'événement de broadcast.
- **Le formulaire garde un choix de programmation sur un brouillon** : `sendChoice` vaut `schedule` dès que `scheduled_date` est renseignée (`console/src/components/broadcasts/compose/composeValues.ts:115`). `started_at` est renvoyé par `list` et `get` (`scanBroadcast`) et typé dans `console/src/services/api/broadcast.ts`.
- **Démo** : `broadcasts.schedule` y est bloqué (`restrictedInDemo`, `broadcast_handler.go:53`).

## Comportement voulu

`POST /api/broadcasts.unschedule` avec `workspace_id` et `id`, droit d'écriture des broadcasts, refusé en démo. Un broadcast `scheduled` dont l'envoi n'a pas commencé redevient `draft` : modifiable, supprimable depuis la console, reprogrammable. Si l'envoi a commencé, la requête répond 400 avec un message qui renvoie vers **Cancel** (pas Pause, qui refuse un programmé). Tout autre statut répond 400. Un broadcast inconnu répond 404.

**« L'envoi a commencé »** vaut si l'un de ces faits est vrai (jamais `started_at` seul) :

- `started_at` renseigné ;
- des lignes dans `email_queue` pour ce broadcast (`source_type` broadcast, `source_id`), index `idx_email_queue_source` ;
- des lignes dans `message_history` avec ce `broadcast_id` (index `idx_message_history_broadcast_id`) ; un envoi à une seule personne (`sendToIndividual`) n'en écrit pas, donc pas de faux refus ;
- une tâche tenue (l'inverse de `notHeld` : `running` sans bail échu, ou bail actif), ou `completed` ;
- un état de tâche qui montre une progression : `RecipientOffset` ou `EnqueuedCount` au-dessus de zéro (`internal/domain/task.go:110-113`).

## Décisions (Pierre, 2026-11-15)

- **D1 — Décidé : un appel dédié** `broadcasts.unschedule`, plutôt qu'un `status: "draft"` accepté par `broadcasts.update`.
- **D2 — Décidé : la date gardée** : `schedule.is_scheduled` passe à `false`, mais `scheduled_date`, `scheduled_time`, `timezone` et `use_recipient_timezone` restent. Le formulaire rouvre sur « Schedule » à la même date.
- **D3 — Décidé : la tâche supprimée**, plutôt que terminée. Suppression par `broadcast_id` et `type = 'send_broadcast'`, pour ne laisser aucune tâche en double.
- **D4 — Décidé : « Back to draft »** dans le menu de la ligne du tableau, l'en-tête du détail et le formulaire d'un broadcast programmé, avec un Popconfirm.
- **D5 — Révisé après la revue de code : Cancel reste toujours proposé** sur un broadcast programmé, avec « Back to draft » devant tant que l'envoi n'a pas commencé. Masquer Cancel (première décision) laissait un état bloqué : la console ne charge pas la progression d'un programmé, donc après un refus « envoi commencé » seul « Back to draft » restait, toujours refusé ; et en démo, où le retour en brouillon est bloqué, plus rien n'arrêtait un broadcast programmé. L'appel `broadcasts.cancel` ne change pas.
- **D6 — Décidé : une entrée « Feature » courte** dans la section `44.0 - unreleased` du CHANGELOG.
- **D7 — Décidé : bloqué en démo**, comme `broadcasts.schedule`, pour que les broadcasts programmés de la démo restent intacts.

## Ordre des écritures

Même ordre que Schedule et Cancel : **broadcast, puis tâche**. L'ordre inverse créerait un blocage entre deux bases, que PostgreSQL ne peut pas détecter.

1. Transaction workspace : relire le broadcast verrouillé (`GetBroadcastForUpdateTx`). Exiger `scheduled` ; vérifier `started_at`, `email_queue` et `message_history` (voir plus haut) ; sinon 400.
2. Dans cette transaction ouverte, transaction système : relire la tâche verrouillée (`GetTaskByBroadcastIDTx`). Tenue, `completed`, ou état avec progression → 400. Absente → on continue.
3. Écrire le brouillon (`draft`, `is_scheduled = false`, `updated_at`), puis valider la transaction workspace.
4. Supprimer la tâche (par `broadcast_id` et type), puis valider la transaction système.

Si l'étape 4 échoue, la requête renvoie l'erreur et la tâche reste en attente pour un brouillon : `refuseDraft` la termine sans rien envoyer. **Relancer l'appel** sur un brouillon qui a encore sa tâche supprime cette tâche et répond 200, au lieu de « pas programmé ».

## Course avec « Save changes »

`UpdateBroadcast` passe à une lecture verrouillée (`GetBroadcastForUpdateTx`) dans une transaction, et vérifie à nouveau le statut lu. Sinon, un « Save changes » envoyé en même temps que « Back to draft » réécrirait `scheduled` après la suppression de la tâche, et le broadcast ne partirait jamais. Tests dédiés.

## Fichiers et tests

### Backend

- `internal/domain/broadcast.go` : `UnscheduleBroadcastRequest` (`workspace_id`, `id`) et sa `Validate` ; `UnscheduleBroadcast` dans l'interface `BroadcastService` (ligne 1437), puis `go generate ./internal/domain/`. Tests dans `internal/domain/broadcast_test.go` : champs manquants refusés, requête valide acceptée.
- `internal/domain/audit_actions.go` : `broadcasts.unschedule` à côté de `broadcasts.cancel`. Couvert par `internal/http/audit_routes_test.go`.
- `internal/domain/task.go` et `internal/repository/task_postgres.go` : `DeleteBroadcastTasksTx(ctx, tx, workspace, broadcastID)` (par `broadcast_id` et type), puis `go generate ./internal/domain/`. Tests dans `internal/repository/task_postgres_test.go` (sqlmock) : la requête filtre sur workspace, broadcast et type, et supprime toutes les tâches correspondantes.
- `internal/domain/task.go` (interface `TaskService`) et `internal/service/task_service.go` : `UnscheduleBroadcastTask(ctx, workspaceID, broadcastID, writeDraft func() error)` : ouvre la transaction système, verrouille la tâche, refuse une tâche tenue, `completed` ou avec progression, appelle `writeDraft` (qui valide la transaction workspace), puis supprime la tâche. Tests dans `internal/service/task_service_test.go` : tâche `pending` sans progression supprimée après `writeDraft` ; tâche tenue, `completed`, ou avec `RecipientOffset`/`EnqueuedCount` > 0 refusée sans appeler `writeDraft` ; tâche absente → `writeDraft` appelée ; erreur de `writeDraft` → tâche gardée.
- `internal/repository/email_queue_postgres.go` et le dépôt de l'historique des messages : une lecture « existe-t-il une ligne pour ce broadcast » dans la transaction workspace (réutiliser `CountBySourceAndStatus`, `email_queue_postgres.go:565`, si elle suffit, sinon une `ExistsForSourceTx`). Tests sqlmock à côté des tests existants.
- `internal/service/broadcast_service.go` : `UnscheduleBroadcast`, l'ordre décrit plus haut ; le refus « l'envoi a commencé » est une `domain.ValidationError` (400 via le contrôle existant, `broadcast_handler.go:716-720`). `UpdateBroadcast` passe à la lecture verrouillée. Tests dans `internal/service/broadcast_service_test.go` : sans droit → `PermissionError` ; `draft` avec tâche restante → tâche supprimée, 200 ; `draft` sans tâche, `paused`, `processing`, `cancelled` → refus ; `scheduled` avec `started_at`, avec lignes en file, avec historique, ou avec tâche en progression → refus ; `scheduled` non lancé → `draft`, `is_scheduled` faux, date gardée, tâche supprimée ; `UpdateBroadcast` relit verrouillé et refuse un statut changé entre-temps.
- `internal/http/broadcast_handler.go` : `POST /api/broadcasts.unschedule` avec `restrictedInDemo(requireAuth(...))` (D7) et `HandleUnschedule`. Tests dans `internal/http/broadcast_handler_test.go`, corps JSON bruts : 200, 400 corps invalide, 400 « l'envoi a commencé », 404 broadcast inconnu, 403 sans droit, 405 sur GET, refus en démo.
- **Intégration** : `tests/testutil/client.go` gagne `UnscheduleBroadcast` (à côté de `ScheduleBroadcast` ligne 386 et `CancelBroadcast` ligne 401). Nouveau `tests/integration/broadcast_unschedule_test.go` (`//go:build integration`), cas :
  - programmé, puis `unschedule`, puis la tâche rendue échue et exécutée (`ExecutePendingTasks`, `client.go:623`) : aucun message, statut `draft`, plus de tâche ;
  - reprogrammé ensuite : une tâche neuve, et l'envoi part à l'heure ;
  - envoi commencé avec `started_at` encore vide (lignes en file) : refus, statut inchangé ;
  - tâche restante pour un brouillon (suppression échouée simulée) : `refuseDraft` la termine sans envoi, et relancer `unschedule` la supprime ;
  - concurrence `unschedule` / « Save changes » et `unschedule` / réservation de la tâche, sur le modèle de `tests/integration/broadcast_schedule_concurrent_test.go` ;
  - `paused` et `cancelled` : refus.

### Documentation

- `openapi/paths/broadcasts.yaml` et `openapi/openapi.yaml` : `/api/broadcasts.unschedule` sur le modèle de `/api/broadcasts.cancel` (`broadcasts.yaml:575`), avec les réponses 400 et 404. Puis bundle et copie vers `../docs` (openapi → bundle → `openapi.json` dans `../docs` → `docs.json`), dans une PR du dépôt `docs`.
- `../docs/features/broadcast-campaigns.mdx` : la liste des permissions (vers la ligne 300) et « Edit or cancel scheduled campaigns » (ligne 322) mentionnent le retour en brouillon.

### Console

- `console/src/services/api/broadcast.ts` : `broadcastApi.unschedule`. Test dans `console/src/services/api/broadcast.test.ts`.
- `console/src/components/broadcasts/broadcastStatus.ts` : `availableActions` gagne `unschedule` (`scheduled` et pas `started_at`)  ; `cancel` reste vrai pour tout programmé (D5 révisé) ; `started_at` entre dans le `Pick<Broadcast, …>` de sa signature. Tests dans `broadcastStatus.test.ts`, dont l'assertion existante de la ligne 254 (`['edit', 'cancel']` pour `scheduled`) qui devient `['edit', 'cancel', 'unschedule']`.
- `console/src/components/broadcasts/useBroadcastActions.ts` : `unschedule` rejoint les actions confirmées (`ConfirmedAction`, `run`) ; son `invalidate` rafraîchit déjà la liste, le broadcast et sa tâche. En cas de refus « l'envoi a commencé », recharger le broadcast (D5). Tests dans `useBroadcastActions.test.tsx`.
- `console/src/components/broadcasts/BroadcastsTable.tsx` (menu de ligne), `detail/BroadcastDetail.tsx` (en-tête) et `compose/BroadcastCompose.tsx` (formulaire d'un programmé) : « Back to draft » par le hook partagé. Dans le formulaire, les modifications non enregistrées sont d'abord enregistrées (« Save changes ») avant le retour en brouillon ; s'il y en a, le Popconfirm le dit. Après succès, le formulaire relit le broadcast pour que `scheduled` (`BroadcastCompose.tsx:128`) passe à faux et que l'autosave des brouillons reprenne. Tests dans `BroadcastsTable.test.tsx`, `detail/BroadcastDetail.test.tsx` et `compose/BroadcastCompose.test.tsx` : visible seulement quand `availableActions` le permet, rien n'est appelé avant confirmation, l'appel part avec `workspace_id` et `id`, l'erreur s'affiche, Cancel reste proposé à côté.
- `console/src/components/logs/auditPhrases.ts` : une phrase pour `broadcasts.unschedule` (à côté de la ligne 33). `console/src/services/api/permissions.ts` : l'appel dans la liste des endpoints des broadcasts (à côté de la ligne 586). Tests existants de ces deux fichiers mis à jour s'ils listent les entrées.
- Traductions : `npm run lingui:extract`, puis les 7 langues remplies.

### Autres

- `CHANGELOG.md` : une entrée « Feature » courte dans `44.0 - unreleased` (D6).
- Le dépôt `Notifuse/architecture` n'est pas présent localement : lister dans la description de la PR le flux touché (envoi d'un broadcast : programmation, tâche, statuts, ordre des verrous).
- Branche dédiée depuis `origin/dev`, pas `feat/contacts-ui-polish`.

## Étape finale : tests à lancer

1. `go generate ./internal/domain/` après les changements d'interfaces (le Makefile n'a pas de cible pour les mocks).
2. `make test-domain`, `make test-repo`, `make test-service`, `make test-http`.
3. `docker compose -f tests/compose.test.yaml up -d`, puis seulement les tests d'intégration touchés : `INTEGRATION_TESTS=true go test -tags integration,licdev ./tests/integration/ -run 'TestBroadcastUnschedule|TestBroadcastSchedule_ConcurrentSchedulesOfOneDraft' -v`.
4. Console : `cd console && npx tsc -b && npm run lint && npx vitest run src/components/broadcasts src/components/logs src/services && npm run build`.

## Implémentation (2026-11-15, branche `feat/broadcast-unschedule`)

Fait comme décrit, à ces écarts près, chacun plus simple ou plus sûr que le plan :

- **Verrou de la tâche dans `BroadcastService`**, qui a déjà `taskRepo`, plutôt qu'une `TaskService.UnscheduleBroadcastTask` : pas de nouvelle méthode d'interface de service. La transaction système s'ouvre en premier sans rien verrouiller ; la transaction workspace verrouille le broadcast, puis la tâche est verrouillée, le brouillon écrit et la transaction workspace validée ; la tâche est supprimée et la transaction système validée ensuite. Verrous : broadcast puis tâche ; validations : workspace puis système.
- **Une seule lecture des envois** : `BroadcastRepository.HasSendActivityTx` (un `EXISTS` sur `email_queue` et un sur `message_history`, dans la base du workspace), plutôt que deux dépôts.
- **Tâche « commencée »** : `running` ou `completed`, bail actif, état avec progression, et aussi un run postérieur à la (re)programmation (`last_run_at` après `created_at`). `ResetBroadcastTaskTx` garde `last_run_at` mais remet `created_at` : un vieux run ne bloque donc pas un broadcast reprogrammé.
- **Console** : la confirmation reprend la fenêtre `modal.confirm` du hook partagé (comme Pause, Resume et Cancel), plutôt qu'un Popconfirm. Dans le formulaire, « Back to draft » est désactivé tant qu'il y a des modifications non enregistrées (« Save or discard your changes first »), plutôt que de les enregistrer d'abord. `availableActions` tient aussi compte de la progression (e-mails en file, envoyés ou échoués) pour ne plus proposer « Back to draft » quand l'envoi a commencé ; Cancel reste toujours proposé (D5 révisé).
- **Test d'intégration** : le cas « envoi commencé sans date » supprime aussi la tâche, pour que seuls les e-mails en file ou envoyés prouvent le départ (sinon la tâche suffisait et la vérification n'était pas couverte).

Vérifié : `make test-domain`, `test-repo`, `test-service`, `test-http` ; `TestBroadcastUnschedule` et `TestBroadcastSchedule_ConcurrentSchedulesOfOneDraft` en intégration ; console `tsc -b`, lint, 6 619 tests, build ; spec OpenAPI valide (`redocly lint`), bundle en ajouts seuls. Contrôles par mutation : sans le contrôle de la tâche, sans le contrôle des envois, suppression de la tâche avant le brouillon, lecture non verrouillée de `UpdateBroadcast` : chacun fait échouer les tests.

Revue de code (Opus, effort moyen) : backend jugé sain (ordre des verrous et des validations, erreurs). Corrigé ensuite : D5 révisé (Cancel toujours proposé) ; suppression des tâches faite sous le verrou du broadcast, pour qu'une programmation lancée juste après ne perde pas sa nouvelle tâche ; le test concurrent exige un 200 de chaque retour en brouillon.
