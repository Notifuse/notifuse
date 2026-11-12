package domain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MessageFilter mirrors the filter fields of MessageListParams: same names, types and JSON
// tags, and MessageListParams has nothing else but Cursor and Limit.
func TestMessageFilter_MirrorsMessageListParams(t *testing.T) {
	filterType := reflect.TypeOf(MessageFilter{})
	paramsType := reflect.TypeOf(MessageListParams{})

	filterFields := map[string]bool{}
	for i := 0; i < filterType.NumField(); i++ {
		f := filterType.Field(i)
		filterFields[f.Name] = true
		p, ok := paramsType.FieldByName(f.Name)
		require.True(t, ok, "MessageListParams has no field %s", f.Name)
		assert.Equal(t, f.Type, p.Type, f.Name)
		assert.Equal(t, f.Tag.Get("json"), p.Tag.Get("json"), f.Name)
	}
	for i := 0; i < paramsType.NumField(); i++ {
		name := paramsType.Field(i).Name
		if name == "Cursor" || name == "Limit" {
			continue
		}
		assert.True(t, filterFields[name], "MessageFilter has no field %s", name)
	}
}

// Every filter field travels through Filter() and back through setFilter.
func TestMessageFilter_RoundTrip(t *testing.T) {
	var params MessageListParams
	v := reflect.ValueOf(&params).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if field.Name == "Cursor" || field.Name == "Limit" {
			continue
		}
		target := v.Field(i)
		switch target.Interface().(type) {
		case string:
			target.SetString("value-" + field.Name)
		case *bool:
			value := i%2 == 0
			target.Set(reflect.ValueOf(&value))
		case *time.Time:
			value := time.Date(2026, 11, 1, 0, 0, i, 0, time.UTC)
			target.Set(reflect.ValueOf(&value))
		default:
			t.Fatalf("field %s has a type this test does not fill: %s", field.Name, field.Type)
		}
	}

	filter := params.Filter()
	fv := reflect.ValueOf(filter)
	for i := 0; i < fv.NumField(); i++ {
		name := fv.Type().Field(i).Name
		assert.Equal(t, v.FieldByName(name).Interface(), fv.Field(i).Interface(), "Filter() carries %s", name)
	}

	var back MessageListParams
	back.setFilter(filter)
	bv := reflect.ValueOf(back)
	for i := 0; i < fv.NumField(); i++ {
		name := fv.Type().Field(i).Name
		assert.Equal(t, fv.Field(i).Interface(), bv.FieldByName(name).Interface(), "setFilter carries %s", name)
	}
}

func TestMessageListParams_FromQuery_OriginAndCreatedRange(t *testing.T) {
	t.Run("origin, created_after and created_before are read", func(t *testing.T) {
		var p MessageListParams
		require.NoError(t, p.FromQuery(url.Values{
			"origin":         {"automation"},
			"created_after":  {"2026-11-01T00:00:00Z"},
			"created_before": {"2026-11-02T00:00:00+01:00"},
		}))
		assert.Equal(t, MessageOriginAutomation, p.Origin)
		require.NotNil(t, p.CreatedAfter)
		assert.True(t, p.CreatedAfter.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)))
		require.NotNil(t, p.CreatedBefore)
		assert.True(t, p.CreatedBefore.Equal(time.Date(2026, 11, 1, 23, 0, 0, 0, time.UTC)))
	})

	t.Run("each origin is accepted", func(t *testing.T) {
		for _, origin := range MessageOrigins {
			var p MessageListParams
			assert.NoError(t, p.FromQuery(url.Values{"origin": {origin}}), origin)
		}
	})

	t.Run("an unknown origin is refused", func(t *testing.T) {
		var p MessageListParams
		err := p.FromQuery(url.Values{"origin": {"api"}})
		require.Error(t, err)
		assert.Equal(t, "origin must be broadcast, automation, transactional or other", err.Error())
	})

	t.Run("created_after after created_before is refused", func(t *testing.T) {
		var p MessageListParams
		err := p.FromQuery(url.Values{"created_after": {"2026-11-02T00:00:00Z"}, "created_before": {"2026-11-01T00:00:00Z"}})
		require.Error(t, err)
		assert.Equal(t, "created_after must be before created_before", err.Error())
	})

	t.Run("an unreadable created_after is refused", func(t *testing.T) {
		var p MessageListParams
		err := p.FromQuery(url.Values{"created_after": {"yesterday"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid created_after time format")
	})
}

func TestMessageFilter_Validate_DoesNotChangeTheFilter(t *testing.T) {
	f := MessageFilter{Channel: "email", Origin: MessageOriginOther}
	before := f
	require.NoError(t, f.Validate())
	assert.Equal(t, before, f)
}

func TestMessageFilter_RequireBound(t *testing.T) {
	after := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	t.Run("refused without created_after nor an indexed filter", func(t *testing.T) {
		for name, f := range map[string]MessageFilter{
			"nothing":       {},
			"list_id":       {ListID: "l1"},
			"external_id":   {ExternalID: "e1"},
			"origin":        {Origin: MessageOriginBroadcast},
			"is_opened":     {IsOpened: boolPtr(true)},
			"sent_after":    {SentAfter: &after},
			"created_until": {CreatedBefore: &after},
		} {
			err := f.RequireBound()
			require.Error(t, err, name)
			assert.Equal(t, "messages.count needs created_after or an indexed filter", err.Error(), name)
		}
	})

	t.Run("accepted with created_after or each indexed filter", func(t *testing.T) {
		for name, f := range map[string]MessageFilter{
			"created_after":                 {CreatedAfter: &after},
			"id":                            {ID: "m1"},
			"contact_email":                 {ContactEmail: "a@example.com"},
			"broadcast_id":                  {BroadcastID: "b1"},
			"transactional_notification_id": {TransactionalNotificationID: "t1"},
			"template_id":                   {TemplateID: "tpl1"},
		} {
			assert.NoError(t, f.RequireBound(), name)
		}
	})
}

func TestParseMessageCountQuery(t *testing.T) {
	t.Run("parses, ignores cursor and limit", func(t *testing.T) {
		f, err := ParseMessageCountQuery(url.Values{
			"created_after": {"2026-11-01T00:00:00Z"},
			"origin":        {"broadcast"},
			"is_opened":     {"true"},
			"cursor":        {"not-a-cursor"},
			"limit":         {"not-a-number"},
		})
		require.NoError(t, err)
		assert.Equal(t, MessageOriginBroadcast, f.Origin)
		require.NotNil(t, f.IsOpened)
		assert.True(t, *f.IsOpened)
	})

	t.Run("an invalid filter is refused before the bound", func(t *testing.T) {
		_, err := ParseMessageCountQuery(url.Values{"origin": {"api"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "origin must be")
	})

	t.Run("an unreadable value is refused", func(t *testing.T) {
		_, err := ParseMessageCountQuery(url.Values{"broadcast_id": {"b1"}, "is_opened": {"yes"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid is_opened value")
	})

	t.Run("an unbounded filter is refused", func(t *testing.T) {
		_, err := ParseMessageCountQuery(url.Values{"list_id": {"l1"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs created_after")
	})
}

func TestInboundWebhookEventListParams_Source(t *testing.T) {
	t.Run("each accepted source is read and accepted", func(t *testing.T) {
		for _, source := range WebhookSources {
			var p InboundWebhookEventListParams
			require.NoError(t, p.FromQuery(url.Values{"workspace_id": {"ws1"}, "source": {string(source)}}), source)
			assert.Equal(t, source, p.Source)
		}
	})

	t.Run("an unknown source is refused", func(t *testing.T) {
		var p InboundWebhookEventListParams
		err := p.FromQuery(url.Values{"workspace_id": {"ws1"}, "source": {"brevo"}})
		require.Error(t, err)
		assert.Equal(t, "invalid source: brevo", err.Error())
	})

	t.Run("no source, no filter", func(t *testing.T) {
		var p InboundWebhookEventListParams
		require.NoError(t, p.FromQuery(url.Values{"workspace_id": {"ws1"}}))
		assert.Empty(t, p.Source)
	})
}

// WebhookSources lists every WebhookSource constant declared in inbound_webhook_event.go, so
// a new provider is accepted by the filter as soon as it is declared there.
func TestWebhookSources_HasEveryConstant(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "inbound_webhook_event.go", nil, 0)
	require.NoError(t, err)

	var declared []WebhookSource
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			if ident, ok := vs.Type.(*ast.Ident); !ok || ident.Name != "WebhookSource" {
				continue
			}
			for _, value := range vs.Values {
				lit := value.(*ast.BasicLit)
				declared = append(declared, WebhookSource(lit.Value[1:len(lit.Value)-1]))
			}
		}
	}
	require.NotEmpty(t, declared)
	assert.ElementsMatch(t, declared, WebhookSources)
}
