package notifuse_mjml

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderBudgetRemainingAndSpend(t *testing.T) {
	b := newRenderBudget()
	left, score, ok := b.remaining()
	require.True(t, ok)
	assert.InDelta(t, DefaultCompileTimeout.Seconds(), left.Seconds(), 1)
	assert.Equal(t, DefaultCompileRenderScore, score)

	b.spend(DefaultCompileRenderScore - 10)
	_, score, ok = b.remaining()
	require.True(t, ok)
	assert.Equal(t, 10, score)

	b.spend(10)
	_, _, ok = b.remaining()
	assert.False(t, ok, "a budget with no score left is spent")

	late := &renderBudget{deadline: time.Now().Add(-time.Second), score: 1000}
	_, _, ok = late.remaining()
	assert.False(t, ok, "a budget past its deadline is spent")
}

// A render within a budget draws on it, and stops on it: a render that would stay under its
// own limit fails once the compilation's budget is spent.
func TestRenderInBudget(t *testing.T) {
	loop := `{% for i in (1..10000) %}{% endfor %}`

	t.Run("its score is taken off the budget", func(t *testing.T) {
		b := newRenderBudget()
		_, err := liquidHTMLEngine.renderIn(b, loop, map[string]interface{}{}, false)
		require.NoError(t, err)
		assert.Less(t, b.score, DefaultCompileRenderScore)
	})

	t.Run("a budget with too little score left stops the render", func(t *testing.T) {
		b := &renderBudget{deadline: time.Now().Add(DefaultCompileTimeout), score: 1000}
		_, err := liquidHTMLEngine.renderIn(b, loop, map[string]interface{}{}, false)
		require.ErrorIs(t, err, errBudgetExhausted)
	})

	t.Run("a spent budget renders nothing more", func(t *testing.T) {
		b := &renderBudget{deadline: time.Now().Add(DefaultCompileTimeout), score: 0}
		_, err := liquidHTMLEngine.renderIn(b, `{{ 1 }}`, map[string]interface{}{}, false)
		require.ErrorIs(t, err, errBudgetExhausted)
	})

	t.Run("a budget with little time left stops the render", func(t *testing.T) {
		b := &renderBudget{deadline: time.Now().Add(50 * time.Millisecond), score: DefaultCompileRenderScore}
		_, err := liquidHTMLEngine.renderIn(b, `{% for i in (1..9000000) %}{{ i | upcase | downcase }}{% endfor %}`, map[string]interface{}{}, false)
		require.ErrorIs(t, err, errBudgetExhausted)
	})

	t.Run("without a budget, a render keeps its own limits only", func(t *testing.T) {
		out, err := liquidHTMLEngine.renderIn(nil, loop+"done", map[string]interface{}{}, false)
		require.NoError(t, err)
		assert.Equal(t, "done", out)
	})
}

// costlyBlocks is a tree of n text blocks whose Liquid each stays just under the render score
// limit of one render.
func costlyBlocks(t *testing.T, n int) EmailBlock {
	t.Helper()
	blocks := make([]string, n)
	for i := range blocks {
		blocks[i] = fmt.Sprintf(`{"id":"t%d","type":"mj-text","content":"{%% for i in (1..4999990) %%}{%% endfor %%}"}`, i)
	}
	return conditionTree(t, blocks...)
}

// One compilation cannot multiply the limits of a render by its number of blocks: a tree of
// many costly blocks fails on the budget within its time, instead of running each block to
// its own limit.
func TestCompileTemplateStopsOnItsWorkBudget(t *testing.T) {
	start := time.Now()
	resp, err := CompileTemplate(CompileTemplateRequest{
		WorkspaceID:      "ws",
		MessageID:        "m",
		VisualEditorTree: costlyBlocks(t, 100),
		TemplateData:     MapOfAny{"contact": map[string]interface{}{"first_name": "Ann"}},
	})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Contains(t, resp.Error.Message, "work budget of the email")
	assert.Less(t, time.Since(start), DefaultCompileTimeout+2*time.Second)
}

// The templates of the gallery, in each of their languages, compile with their data well within
// the budget: under 1% of its render score.
func TestGalleryTemplatesFitTheWorkBudget(t *testing.T) {
	files, err := filepath.Glob("../../internal/service/demo_gallery/*.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	data := MapOfAny{"contact": map[string]interface{}{"first_name": "Ann", "email": "ann@example.com"}}
	dataJSON, err := json.Marshal(data)
	require.NoError(t, err)

	for _, file := range files {
		raw, err := os.ReadFile(file)
		require.NoError(t, err)
		var fixture struct {
			Languages map[string]struct {
				Tree json.RawMessage `json:"visual_editor_tree"`
			} `json:"languages"`
		}
		require.NoError(t, json.Unmarshal(raw, &fixture))
		require.NotEmpty(t, fixture.Languages, file)
		for lang, content := range fixture.Languages {
			t.Run(filepath.Base(file)+"/"+lang, func(t *testing.T) {
				tree, err := UnmarshalEmailBlock(content.Tree)
				require.NoError(t, err)
				b := newRenderBudget()
				_, err = convertJSONToMJMLWithDataProtectedIn(b, tree, string(dataJSON))
				require.NoError(t, err)
				spent := DefaultCompileRenderScore - b.score
				assert.Positive(t, spent, "the template's Liquid was rendered")
				assert.Less(t, spent, DefaultCompileRenderScore/100, "render score spent: %d", spent)
			})
		}
	}
}
