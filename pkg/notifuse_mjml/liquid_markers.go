package notifuse_mjml

import (
	"encoding/json"
	"strings"
)

// The MJML export can mark what the visual editor's MJML import cannot rebuild from the
// markup alone: the bounds of an mj-liquid block, whose content is written as is, and the
// display condition of a block, written as the Liquid that sending evaluates. The marks
// are Liquid comments, which render to nothing: the marked MJML sends as the unmarked one.
// The import (console/src/components/mjml-converter) reads the same strings.
const (
	liquidCommentStart = "{% comment %}"
	liquidCommentEnd   = "{% endcomment %}"

	// LiquidBlockMarkerOpen and LiquidBlockMarkerClose surround the content of an mj-liquid block.
	LiquidBlockMarkerOpen  = liquidCommentStart + "notifuse:liquid" + liquidCommentEnd
	LiquidBlockMarkerClose = liquidCommentStart + "/notifuse:liquid" + liquidCommentEnd

	// A block with a display condition is preceded by conditionMarkerPrefix, the condition
	// as JSON and liquidCommentEnd, and followed by ConditionMarkerClose.
	conditionMarkerPrefix = liquidCommentStart + "notifuse:condition "
	ConditionMarkerClose  = liquidCommentStart + "/notifuse:condition" + liquidCommentEnd
)

// conditionMarkerOpen is the mark written before a block with condition. In its JSON, %, <,
// > and & are escaped (\u0025…): it can neither end the comment nor open a tag for an HTML parser.
func conditionMarkerOpen(condition *DisplayCondition) string {
	raw, err := json.Marshal(condition) // escapes <, > and &
	if err != nil {
		return ""
	}
	return conditionMarkerPrefix + strings.ReplaceAll(string(raw), "%", `\u0025`) + liquidCommentEnd
}
