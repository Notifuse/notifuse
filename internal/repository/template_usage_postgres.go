package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/internal/domain"
)

// templateUsageBroadcastStatuses are the broadcasts that may still send their templates:
// processed, cancelled and failed ones are done with them.
var templateUsageBroadcastStatuses = []domain.BroadcastStatus{
	domain.BroadcastStatusDraft,
	domain.BroadcastStatusScheduled,
	domain.BroadcastStatusProcessing,
	domain.BroadcastStatusPaused,
	domain.BroadcastStatusTesting,
	domain.BroadcastStatusTestCompleted,
	domain.BroadcastStatusWinnerSelected,
}

// The JSON arrays read go through a CASE: jsonb_array_elements fails on a scalar such as
// the JSON null an automation without nodes holds, and one bad row would fail the query.
const (
	templateUsageVariations = `jsonb_array_elements(CASE WHEN jsonb_typeof(b.test_settings->'variations') = 'array' THEN b.test_settings->'variations' ELSE '[]'::jsonb END)`
	templateUsageNodes      = `jsonb_array_elements(CASE WHEN jsonb_typeof(a.nodes) = 'array' THEN a.nodes ELSE '[]'::jsonb END)`
)

// templateUsageSource holds the two queries of one object type. countSQL returns
// (template_id, count) for every template used; refsSQL takes the template ID as $1 and
// returns (id, name, status, total), total being the count before the limit.
type templateUsageSource struct {
	ref      domain.TemplateUsageReferenceType
	countSQL string
	refsSQL  string
}

func templateUsageSources() map[domain.TemplateUsageType]templateUsageSource {
	statuses := make([]string, len(templateUsageBroadcastStatuses))
	for i, s := range templateUsageBroadcastStatuses {
		statuses[i] = "'" + string(s) + "'"
	}
	broadcastStatus := "b.status IN (" + strings.Join(statuses, ", ") + ")"
	limit := fmt.Sprintf("LIMIT %d", domain.TemplateUsageReferenceLimit)

	return map[domain.TemplateUsageType]templateUsageSource{
		domain.TemplateUsageTypeBroadcasts: {
			ref: domain.TemplateUsageReferenceBroadcast,
			countSQL: `SELECT v.elem->>'template_id', COUNT(DISTINCT b.id)
				FROM broadcasts b CROSS JOIN LATERAL ` + templateUsageVariations + ` AS v(elem)
				WHERE ` + broadcastStatus + ` AND v.elem->>'template_id' <> ''
				GROUP BY 1`,
			refsSQL: `SELECT b.id, b.name, b.status, COUNT(*) OVER ()
				FROM broadcasts b
				WHERE ` + broadcastStatus + `
				  AND EXISTS (SELECT 1 FROM ` + templateUsageVariations + ` AS v(elem) WHERE v.elem->>'template_id' = $1)
				ORDER BY b.name, b.id ` + limit,
		},
		domain.TemplateUsageTypeTransactionalNotifications: {
			ref: domain.TemplateUsageReferenceTransactionalNotification,
			countSQL: `SELECT channels->'email'->>'template_id', COUNT(*)
				FROM transactional_notifications
				WHERE deleted_at IS NULL AND channels->'email'->>'template_id' <> ''
				GROUP BY 1`,
			refsSQL: `SELECT id, name, '', COUNT(*) OVER ()
				FROM transactional_notifications
				WHERE deleted_at IS NULL AND channels->'email'->>'template_id' = $1
				ORDER BY name, id ` + limit,
		},
		domain.TemplateUsageTypeAutomations: {
			ref: domain.TemplateUsageReferenceAutomation,
			// An automation using the template in several places counts once.
			countSQL: `SELECT r.template_id, COUNT(DISTINCT a.id)
				FROM automations a CROSS JOIN LATERAL (
					SELECT n.elem->'config'->>'template_id' AS template_id
					FROM ` + templateUsageNodes + ` AS n(elem)
					WHERE n.elem->>'type' = 'email'
					UNION
					SELECT a.trigger_config->>'template_id'
				) r
				WHERE a.deleted_at IS NULL AND r.template_id <> ''
				GROUP BY 1`,
			refsSQL: `SELECT a.id, a.name, COALESCE(a.status, ''), COUNT(*) OVER ()
				FROM automations a
				WHERE a.deleted_at IS NULL
				  AND (a.trigger_config->>'template_id' = $1 OR EXISTS (
					SELECT 1 FROM ` + templateUsageNodes + ` AS n(elem)
					WHERE n.elem->>'type' = 'email' AND n.elem->'config'->>'template_id' = $1))
				ORDER BY a.name, a.id ` + limit,
		},
		domain.TemplateUsageTypeLists: {
			ref: domain.TemplateUsageReferenceList,
			countSQL: `SELECT double_optin_template->>'id', COUNT(*)
				FROM lists
				WHERE deleted_at IS NULL AND is_double_optin AND double_optin_template->>'id' <> ''
				GROUP BY 1`,
			refsSQL: `SELECT id, name, '', COUNT(*) OVER ()
				FROM lists
				WHERE deleted_at IS NULL AND is_double_optin AND double_optin_template->>'id' = $1
				ORDER BY name, id ` + limit,
		},
	}
}

// GetTemplateUsage runs one grouped query per requested type, in the order of
// domain.TemplateUsageTypes. Templates referenced inside segment trees or trigger
// conditions are not counted.
func (r *templateRepository) GetTemplateUsage(ctx context.Context, workspaceID string, templateID string, types []domain.TemplateUsageType) (*domain.TemplateUsage, error) {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	requested := make(map[domain.TemplateUsageType]bool, len(types))
	for _, t := range types {
		requested[t] = true
	}
	sources := templateUsageSources()

	usage := domain.NewTemplateUsage(templateID != "")
	counts := map[string]map[domain.TemplateUsageType]int{}
	var queried []domain.TemplateUsageType

	for _, typ := range domain.TemplateUsageTypes {
		if !requested[typ] {
			continue
		}
		queried = append(queried, typ)
		src := sources[typ]

		if templateID == "" {
			rows, err := workspaceDB.QueryContext(ctx, src.countSQL)
			if err != nil {
				return nil, fmt.Errorf("failed to count template usage by %s: %w", typ, err)
			}
			for rows.Next() {
				var id string
				var n int
				if err := rows.Scan(&id, &n); err != nil {
					_ = rows.Close()
					return nil, fmt.Errorf("failed to scan template usage of %s: %w", typ, err)
				}
				if counts[id] == nil {
					counts[id] = map[domain.TemplateUsageType]int{}
				}
				counts[id][typ] = n
			}
			err = rows.Err()
			_ = rows.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read template usage of %s: %w", typ, err)
			}
			continue
		}

		rows, err := workspaceDB.QueryContext(ctx, src.refsSQL, templateID)
		if err != nil {
			return nil, fmt.Errorf("failed to list template usage by %s: %w", typ, err)
		}
		total := 0
		for rows.Next() {
			ref := domain.TemplateUsageReference{Type: src.ref}
			if err := rows.Scan(&ref.ID, &ref.Name, &ref.Status, &total); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("failed to scan template usage of %s: %w", typ, err)
			}
			*usage.References = append(*usage.References, ref)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read template usage of %s: %w", typ, err)
		}
		if total > 0 {
			if counts[templateID] == nil {
				counts[templateID] = map[domain.TemplateUsageType]int{}
			}
			counts[templateID][typ] = total
		}
	}

	// Every queried type gets a counter on each used template, 0 included.
	for id, byType := range counts {
		entry := &domain.TemplateUsageCounts{}
		for _, typ := range queried {
			entry.Set(typ, byType[typ])
		}
		usage.Usage[id] = entry
	}

	return &usage, nil
}
