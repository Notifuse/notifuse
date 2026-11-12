package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// usageQueryRecorder records each query the repository issues and accepts it.
type usageQueryRecorder struct{ issued []string }

func (r *usageQueryRecorder) Match(_, actualSQL string) error {
	r.issued = append(r.issued, strings.Join(strings.Fields(actualSQL), " "))
	return nil
}

func newTemplateUsageRepo(t *testing.T) (*templateRepository, sqlmock.Sqlmock, *usageQueryRecorder) {
	t.Helper()
	rec := &usageQueryRecorder{}
	db, sqlMock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	workspaceRepo := new(MockWorkspaceRepository)
	workspaceRepo.On("GetConnection", mock.Anything, "ws1").Return(db, nil)
	return &templateRepository{workspaceRepo: workspaceRepo}, sqlMock, rec
}

func TestTemplateRepository_GetTemplateUsage_Counts(t *testing.T) {
	repo, sqlMock, rec := newTemplateUsageRepo(t)
	cols := []string{"template_id", "count"}
	sqlMock.ExpectQuery("broadcasts").WillReturnRows(sqlmock.NewRows(cols).AddRow("welcome", 2).AddRow("promo", 1))
	sqlMock.ExpectQuery("transactional").WillReturnRows(sqlmock.NewRows(cols).AddRow("welcome", 1))
	sqlMock.ExpectQuery("automations").WillReturnRows(sqlmock.NewRows(cols))
	sqlMock.ExpectQuery("lists").WillReturnRows(sqlmock.NewRows(cols).AddRow("confirm", 3))

	usage, err := repo.GetTemplateUsage(context.Background(), "ws1", "", domain.TemplateUsageTypes)
	require.NoError(t, err)
	require.NoError(t, sqlMock.ExpectationsWereMet())

	one, two, three, zero := 1, 2, 3, 0
	assert.Equal(t, map[string]*domain.TemplateUsageCounts{
		"welcome": {Broadcasts: &two, TransactionalNotifications: &one, Automations: &zero, Lists: &zero},
		"promo":   {Broadcasts: &one, TransactionalNotifications: &zero, Automations: &zero, Lists: &zero},
		"confirm": {Broadcasts: &zero, TransactionalNotifications: &zero, Automations: &zero, Lists: &three},
	}, usage.Usage)
	assert.Nil(t, usage.References, "no references without template_id")

	require.Len(t, rec.issued, 4)
	broadcasts, transactional, automations, lists := rec.issued[0], rec.issued[1], rec.issued[2], rec.issued[3]

	assert.Contains(t, broadcasts, "COUNT(DISTINCT b.id)")
	assert.Contains(t, broadcasts, "CASE WHEN jsonb_typeof(b.test_settings->'variations') = 'array' THEN b.test_settings->'variations' ELSE '[]'::jsonb END")
	assert.Contains(t, broadcasts, "b.status IN ('draft', 'scheduled', 'processing', 'paused', 'testing', 'test_completed', 'winner_selected')")
	assert.Contains(t, broadcasts, "<> ''")
	for _, done := range []string{"'processed'", "'cancelled'", "'failed'"} {
		assert.NotContains(t, broadcasts, done)
	}

	assert.Contains(t, transactional, "FROM transactional_notifications")
	assert.Contains(t, transactional, "channels->'email'->>'template_id'")
	assert.Contains(t, transactional, "deleted_at IS NULL")
	assert.Contains(t, transactional, "<> ''")

	assert.Contains(t, automations, "COUNT(DISTINCT a.id)")
	assert.Contains(t, automations, "CASE WHEN jsonb_typeof(a.nodes) = 'array' THEN a.nodes ELSE '[]'::jsonb END")
	assert.Contains(t, automations, "n.elem->>'type' = 'email'")
	assert.Contains(t, automations, "n.elem->'config'->>'template_id'")
	assert.Contains(t, automations, "a.trigger_config->>'template_id'")
	assert.Contains(t, automations, "a.deleted_at IS NULL")
	assert.Contains(t, automations, "<> ''")

	assert.Contains(t, lists, "FROM lists")
	assert.Contains(t, lists, "double_optin_template->>'id'")
	assert.Contains(t, lists, "deleted_at IS NULL AND is_double_optin")
	assert.Contains(t, lists, "<> ''")
}

func TestTemplateRepository_GetTemplateUsage_OnlyRequestedTypes(t *testing.T) {
	repo, sqlMock, rec := newTemplateUsageRepo(t)
	sqlMock.ExpectQuery("transactional").WillReturnRows(sqlmock.NewRows([]string{"template_id", "count"}).AddRow("welcome", 1))

	usage, err := repo.GetTemplateUsage(context.Background(), "ws1", "", []domain.TemplateUsageType{domain.TemplateUsageTypeTransactionalNotifications})
	require.NoError(t, err)
	require.NoError(t, sqlMock.ExpectationsWereMet())
	require.Len(t, rec.issued, 1, "a type not requested is not queried")

	one := 1
	assert.Equal(t, map[string]*domain.TemplateUsageCounts{"welcome": {TransactionalNotifications: &one}}, usage.Usage,
		"a type not requested has no counter")
}

func TestTemplateRepository_GetTemplateUsage_OneTemplate(t *testing.T) {
	repo, sqlMock, rec := newTemplateUsageRepo(t)
	cols := []string{"id", "name", "status", "total"}
	sqlMock.ExpectQuery("broadcasts").WithArgs("welcome").
		WillReturnRows(sqlmock.NewRows(cols).AddRow("bc1", "Black Friday", "scheduled", 60).AddRow("bc2", "Cyber Monday", "draft", 60))
	sqlMock.ExpectQuery("transactional").WithArgs("welcome").WillReturnRows(sqlmock.NewRows(cols))
	sqlMock.ExpectQuery("automations").WithArgs("welcome").
		WillReturnRows(sqlmock.NewRows(cols).AddRow("au1", "Onboarding", "live", 1))
	sqlMock.ExpectQuery("lists").WithArgs("welcome").
		WillReturnRows(sqlmock.NewRows(cols).AddRow("l1", "Newsletter", "", 1))

	usage, err := repo.GetTemplateUsage(context.Background(), "ws1", "welcome", domain.TemplateUsageTypes)
	require.NoError(t, err)
	require.NoError(t, sqlMock.ExpectationsWereMet())

	sixty, one, zero := 60, 1, 0
	assert.Equal(t, map[string]*domain.TemplateUsageCounts{
		"welcome": {Broadcasts: &sixty, TransactionalNotifications: &zero, Automations: &one, Lists: &one},
	}, usage.Usage, "the count is the window total, exact beyond the rows returned")
	require.NotNil(t, usage.References)
	assert.Equal(t, []domain.TemplateUsageReference{
		{Type: domain.TemplateUsageReferenceBroadcast, ID: "bc1", Name: "Black Friday", Status: "scheduled"},
		{Type: domain.TemplateUsageReferenceBroadcast, ID: "bc2", Name: "Cyber Monday", Status: "draft"},
		{Type: domain.TemplateUsageReferenceAutomation, ID: "au1", Name: "Onboarding", Status: "live"},
		{Type: domain.TemplateUsageReferenceList, ID: "l1", Name: "Newsletter"},
	}, *usage.References)

	require.Len(t, rec.issued, 4)
	for _, q := range rec.issued {
		assert.Contains(t, q, "COUNT(*) OVER ()")
		assert.Contains(t, q, "LIMIT 50")
		assert.Contains(t, q, "= $1")
	}
	assert.Contains(t, rec.issued[0], "EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(b.test_settings->'variations') = 'array'")
	assert.Contains(t, rec.issued[0], "ORDER BY b.name, b.id")
	assert.Contains(t, rec.issued[1], "deleted_at IS NULL")
	assert.Contains(t, rec.issued[1], "ORDER BY name, id")
	assert.Contains(t, rec.issued[2], "a.trigger_config->>'template_id' = $1 OR EXISTS")
	assert.Contains(t, rec.issued[2], "a.deleted_at IS NULL")
	assert.Contains(t, rec.issued[2], "ORDER BY a.name, a.id")
	assert.Contains(t, rec.issued[3], "deleted_at IS NULL AND is_double_optin")
	assert.Contains(t, rec.issued[3], "ORDER BY name, id")
}

func TestTemplateRepository_GetTemplateUsage_UnusedTemplate(t *testing.T) {
	repo, sqlMock, _ := newTemplateUsageRepo(t)
	sqlMock.ExpectQuery("lists").WithArgs("welcome").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status", "total"}))

	usage, err := repo.GetTemplateUsage(context.Background(), "ws1", "welcome", []domain.TemplateUsageType{domain.TemplateUsageTypeLists})
	require.NoError(t, err)
	assert.Empty(t, usage.Usage, "an unused template is absent from usage")
	require.NotNil(t, usage.References)
	assert.Empty(t, *usage.References)
}

func TestTemplateRepository_GetTemplateUsage_Errors(t *testing.T) {
	t.Run("query error", func(t *testing.T) {
		repo, sqlMock, _ := newTemplateUsageRepo(t)
		sqlMock.ExpectQuery("broadcasts").WillReturnError(errors.New("boom"))
		_, err := repo.GetTemplateUsage(context.Background(), "ws1", "", domain.TemplateUsageTypes)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "broadcasts")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("connection error", func(t *testing.T) {
		workspaceRepo := new(MockWorkspaceRepository)
		workspaceRepo.On("GetConnection", mock.Anything, "ws1").Return(nil, errors.New("no db"))
		repo := &templateRepository{workspaceRepo: workspaceRepo}
		_, err := repo.GetTemplateUsage(context.Background(), "ws1", "", domain.TemplateUsageTypes)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no db")
	})
}
