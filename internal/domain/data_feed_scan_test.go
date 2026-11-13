package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataFeedSettings_Scan(t *testing.T) {
	t.Run("stored JSON that does not decode is refused", func(t *testing.T) {
		var global GlobalFeedSettings
		var recipient RecipientFeedSettings
		var settings DataFeedSettings

		assert.Error(t, global.Scan([]byte("{not json")))
		assert.Error(t, recipient.Scan([]byte("{not json")))
		assert.Error(t, settings.Scan([]byte("{not json")))
	})

	// The console reads the header list without a nil check.
	t.Run("feeds stored without headers get an empty list", func(t *testing.T) {
		var settings DataFeedSettings
		require.NoError(t, settings.Scan([]byte(`{"global_feed": {"enabled": true}, "recipient_feed": {"enabled": true}}`)))

		require.NotNil(t, settings.GlobalFeed)
		require.NotNil(t, settings.RecipientFeed)
		assert.NotNil(t, settings.GlobalFeed.Headers)
		assert.Empty(t, settings.GlobalFeed.Headers)
		assert.NotNil(t, settings.RecipientFeed.Headers)
		assert.Empty(t, settings.RecipientFeed.Headers)
	})
}

// The recipient feed receives every custom field that holds a value, and none that is null.
func TestBuildRecipientFeedContact_CustomFields(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	num := func(f float64) *NullableFloat64 { return &NullableFloat64{Float64: f} }
	date := func(d int) *NullableTime { return &NullableTime{Time: at.AddDate(0, 0, d)} }
	data := func(v string) *NullableJSON { return &NullableJSON{Data: map[string]interface{}{"v": v}} }

	contact := &Contact{
		Email:         "a@example.com",
		CustomNumber2: num(2), CustomNumber3: num(3), CustomNumber4: num(4), CustomNumber5: num(5),
		CustomDatetime2: date(2), CustomDatetime3: date(3), CustomDatetime4: date(4), CustomDatetime5: date(5),
		CustomJSON2: data("2"), CustomJSON3: data("3"), CustomJSON4: data("4"), CustomJSON5: data("5"),
	}

	got := BuildRecipientFeedContact(contact)

	assert.Equal(t, []*float64{&contact.CustomNumber2.Float64, &contact.CustomNumber3.Float64, &contact.CustomNumber4.Float64, &contact.CustomNumber5.Float64},
		[]*float64{got.CustomNumber2, got.CustomNumber3, got.CustomNumber4, got.CustomNumber5})
	dates := []*string{got.CustomDatetime2, got.CustomDatetime3, got.CustomDatetime4, got.CustomDatetime5}
	for i, d := range dates {
		require.NotNil(t, d, "custom_datetime_%d", i+2)
		assert.Equal(t, at.AddDate(0, 0, i+2).Format(time.RFC3339), *d)
	}
	assert.Equal(t, []interface{}{contact.CustomJSON2.Data, contact.CustomJSON3.Data, contact.CustomJSON4.Data, contact.CustomJSON5.Data},
		[]interface{}{got.CustomJSON2, got.CustomJSON3, got.CustomJSON4, got.CustomJSON5})

	nulls := BuildRecipientFeedContact(&Contact{
		Email:         "b@example.com",
		CustomNumber2: &NullableFloat64{IsNull: true}, CustomDatetime2: &NullableTime{IsNull: true}, CustomJSON2: &NullableJSON{IsNull: true},
	})
	assert.Nil(t, nulls.CustomNumber2)
	assert.Nil(t, nulls.CustomDatetime2)
	assert.Nil(t, nulls.CustomJSON2)
}
