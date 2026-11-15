package notifuse_mjml

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// panickingValue stands for a bug in liquidgo: reading it panics.
type panickingValue struct{}

func (panickingValue) ToLiquid() interface{} {
	var items []int
	return items[3]
}

// A Go panic during a render fails it with a short error: the stack goes to the
// server log, never to the tenant who reads the error in a preview or a send.
func TestSecureLiquidEngine_PanicIsAnErrorWithoutItsStack(t *testing.T) {
	_, err := NewSecureLiquidEngine().RenderWithTimeout("a{{ x }}b", map[string]interface{}{"x": panickingValue{}})
	require.Error(t, err)
	assert.Equal(t, "liquid rendering failed: internal error", err.Error())
	assert.NotContains(t, err.Error(), "goroutine")
}

// The shared engines render from many goroutines at once: each render gets its
// own assigns and output, and display conditions their own result.
func TestSharedLiquidEngines_ConcurrentRenders(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data := map[string]interface{}{"name": fmt.Sprint("n", i), "admin": i%2 == 0}
			html, err := processLiquidHTML(`{% if admin %}{% assign who = name %}{% endif %}<p>{{ who }}|{{ name }}</p>`, data, "b")
			require.NoError(t, err)
			want := fmt.Sprintf("<p>|n%d</p>", i)
			if i%2 == 0 {
				want = fmt.Sprintf("<p>n%d|n%d</p>", i, i)
			}
			assert.Equal(t, want, html)
			text, err := processLiquidContent(`{% increment c %}-{{ name }}`, data, "s")
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("0-n%d", i), text)
			cond := &DisplayCondition{Rules: []DisplayConditionRule{{Variable: "admin", Operator: DisplayConditionIsTrue}}}
			shown, err := cond.Evaluate(data)
			require.NoError(t, err)
			assert.Equal(t, i%2 == 0, shown, "render %d", i)
		}(i)
	}
	wg.Wait()
}

// The parse cache of the shared engine keeps templates under concurrent use and
// stays within its bound.
func TestSecureLiquidEngine_ParseCacheUnderConcurrency(t *testing.T) {
	engine := NewSecureLiquidEngine()
	var wg sync.WaitGroup
	for i := 0; i < 400; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			src := fmt.Sprintf("{%% if v == %d %%}1{%% endif %%}", i%50)
			out, err := engine.renderCached(src, map[string]interface{}{"v": i % 50})
			require.NoError(t, err)
			assert.Equal(t, "1", out)
		}(i)
	}
	wg.Wait()
	engine.mu.Lock()
	defer engine.mu.Unlock()
	assert.Len(t, engine.parsed, 50)
}
