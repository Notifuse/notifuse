package notifuse_mjml

import (
	"fmt"
	"time"
)

// DefaultCompileTimeout and DefaultCompileRenderScore bound the Liquid work of one email
// compilation, all its renders together: each block, attribute, display condition, the
// subject, the text and the whole document. Each render keeps its own limits
// (DefaultRenderTimeout, DefaultMaxRenderScore); the budget bounds how many of them a template
// can make costly. A template of the gallery spends well under 1% of it.
const (
	DefaultCompileTimeout     = 10 * time.Second
	DefaultCompileRenderScore = 20_000_000
)

// errBudgetExhausted is the error of a render that the compilation's budget stops, or does not
// start because it is spent.
var errBudgetExhausted = fmt.Errorf("liquid rendering exceeds the work budget of the email (%v and %d of render score for all its renders)",
	DefaultCompileTimeout, DefaultCompileRenderScore)

// renderBudget is the Liquid work left to one compilation. A compilation renders one part
// after the other, so it needs no lock.
type renderBudget struct {
	deadline time.Time
	score    int
}

func newRenderBudget() *renderBudget {
	return &renderBudget{deadline: time.Now().Add(DefaultCompileTimeout), score: DefaultCompileRenderScore}
}

// remaining gives the time and the render score left, and false once either is spent.
func (b *renderBudget) remaining() (time.Duration, int, bool) {
	left := time.Until(b.deadline)
	if left <= 0 || b.score <= 0 {
		return 0, 0, false
	}
	return left, b.score, true
}

// spend takes a render's score off the budget.
func (b *renderBudget) spend(score int) {
	b.score -= score
}
