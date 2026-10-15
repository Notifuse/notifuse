package notifuse_mjml

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessLiquidTemplate(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		templateData map[string]interface{}
		context      string
		expected     string
		expectError  bool
	}{
		{
			name:         "no liquid tags",
			content:      "Hello World",
			templateData: map[string]interface{}{"name": "John"},
			context:      "test",
			expected:     "Hello World",
			expectError:  false,
		},
		{
			name:         "simple variable interpolation",
			content:      "Hello {{name}}!",
			templateData: map[string]interface{}{"name": "John"},
			context:      "test",
			expected:     "Hello John!",
			expectError:  false,
		},
		{
			name:         "multiple variables",
			content:      "Hello {{name}}, welcome to {{company}}!",
			templateData: map[string]interface{}{"name": "John", "company": "ACME Corp"},
			context:      "test",
			expected:     "Hello John, welcome to ACME Corp!",
			expectError:  false,
		},
		{
			name:         "conditional content",
			content:      "{% if isPremium %}Premium Member{% else %}Standard Member{% endif %}",
			templateData: map[string]interface{}{"isPremium": true},
			context:      "test",
			expected:     "Premium Member",
			expectError:  false,
		},
		{
			name:         "conditional content false",
			content:      "{% if isPremium %}Premium Member{% else %}Standard Member{% endif %}",
			templateData: map[string]interface{}{"isPremium": false},
			context:      "test",
			expected:     "Standard Member",
			expectError:  false,
		},
		{
			name:         "liquid filters",
			content:      "Hello {{name | upcase}}!",
			templateData: map[string]interface{}{"name": "john"},
			context:      "test",
			expected:     "Hello JOHN!",
			expectError:  false,
		},
		{
			name:         "empty template data",
			content:      "Hello {{name | default: 'Guest'}}!",
			templateData: nil,
			context:      "test",
			expected:     "Hello Guest!",
			expectError:  false,
		},
		{
			name:         "undefined variable with default",
			content:      "Hello {{unknown | default: 'Guest'}}!",
			templateData: map[string]interface{}{"name": "John"},
			context:      "test",
			expected:     "Hello Guest!",
			expectError:  false,
		},
		{
			name:    "nested contact.email access",
			content: "<p>Your email: {{ contact.email }}</p>",
			templateData: map[string]interface{}{
				"contact": map[string]interface{}{
					"email":      "john@example.com",
					"first_name": "John",
				},
			},
			context:     "test",
			expected:    "<p>Your email: john@example.com</p>",
			expectError: false,
		},
		{
			name:    "contact.email with HTML entity nbsp",
			content: "<p>Email: {{&nbsp;contact.email&nbsp;}}</p>",
			templateData: map[string]interface{}{
				"contact": map[string]interface{}{
					"email": "jane@example.com",
				},
			},
			context:     "test",
			expected:    "<p>Email: jane@example.com</p>",
			expectError: false,
		},
		{
			name:    "contact.email with numeric entity nbsp",
			content: "<p>Email: {{&#160;contact.email&#160;}}</p>",
			templateData: map[string]interface{}{
				"contact": map[string]interface{}{
					"email": "test@example.com",
				},
			},
			context:     "test",
			expected:    "<p>Email: test@example.com</p>",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessLiquidTemplate(tt.content, tt.templateData, tt.context)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestCleanLiquidTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no liquid tags unchanged",
			input:    "<p>Hello world</p>",
			expected: "<p>Hello world</p>",
		},
		{
			name:     "clean liquid unchanged",
			input:    "{{ contact.email }}",
			expected: "{{ contact.email }}",
		},
		{
			name:     "HTML entity nbsp in variable",
			input:    "{{&nbsp;contact.email&nbsp;}}",
			expected: "{{ contact.email }}",
		},
		{
			name:     "numeric entity nbsp in variable",
			input:    "{{&#160;contact.email&#160;}}",
			expected: "{{ contact.email }}",
		},
		{
			name:     "hex entity nbsp in variable",
			input:    "{{&#xa0;contact.email&#xa0;}}",
			expected: "{{ contact.email }}",
		},
		{
			name:     "hex entity uppercase nbsp in variable",
			input:    "{{&#xA0;contact.email&#xA0;}}",
			expected: "{{ contact.email }}",
		},
		{
			name:     "unicode non-breaking space in variable becomes a space",
			input:    "{{\u00a0contact.email\u00a0}}",
			expected: "{{ contact.email }}",
		},
		{
			name:     "mixed HTML entities and text",
			input:    "<p>Email: {{&nbsp;contact.email&nbsp;}}</p>",
			expected: "<p>Email: {{ contact.email }}</p>",
		},
		{
			name:     "block tag with nbsp",
			input:    "{%&nbsp;if contact.email&nbsp;%}yes{%&nbsp;endif&nbsp;%}",
			expected: "{% if contact.email %}yes{% endif %}",
		},
		{
			name:     "zero-width space in variable",
			input:    "{{\u200bcontact.email\u200b}}",
			expected: "{{contact.email}}",
		},
		{
			name:     "nbsp and curly quotes together, as the text editor stores them",
			input:    "<p>Hello {{&nbsp;first_name | default: \u2018Valued User\u2019&nbsp;}}!</p>",
			expected: "<p>Hello {{ first_name | default: 'Valued User' }}!</p>",
		},
		{
			name:     "tag containing a percent sign",
			input:    "{%&nbsp;assign year = 'now' | date: '%Y'&nbsp;%}",
			expected: "{% assign year = 'now' | date: '%Y' %}",
		},
		{
			name:     "raw non-breaking space between a tag name and its condition",
			input:    "{% if\u00a0contact.discount contains '%' %}",
			expected: "{% if contact.discount contains '%' %}",
		},
		{
			name:     "raw body is left as Liquid outputs it",
			input:    "{% raw %}{{ a &#60; b | default: \u2018x\u2019 }}{% endraw %}{{ y | default: \u2018z\u2019 }}",
			expected: "{% raw %}{{ a &#60; b | default: \u2018x\u2019 }}{% endraw %}{{ y | default: 'z' }}",
		},
		{
			name:     "raw tags with whitespace control and a non-breaking space",
			input:    "{%- raw&nbsp;-%}{{ \u2018x\u2019 }}{%&nbsp;endraw&nbsp;%}{{ \u2018y\u2019 }}",
			expected: "{%- raw -%}{{ \u2018x\u2019 }}{% endraw %}{{ 'y' }}",
		},
		{
			name:     "endraw with whitespace control does not end a raw body, as in liquidgo",
			input:    "{% raw %}{{ \u2018x\u2019 }}{%- endraw -%}{{ \u2018y\u2019 }}{% endraw %}{{ \u2018z\u2019 }}",
			expected: "{% raw %}{{ \u2018x\u2019 }}{%- endraw -%}{{ \u2018y\u2019 }}{% endraw %}{{ 'z' }}",
		},
		{
			name:     "endraw followed by more markup ends a raw body, as in liquidgo",
			input:    "{% raw %}{{ \u2018x\u2019 }}{% endraw foo %}{{ \u2018y\u2019 }}",
			expected: "{% raw %}{{ \u2018x\u2019 }}{% endraw foo %}{{ 'y' }}",
		},
		{
			name:     "other spellings of a non-breaking space",
			input:    "{%&#x00A0;if x&#XA0;%}",
			expected: "{% if x %}",
		},
		{
			name:     "curly quotes and entities outside Liquid markup unchanged",
			input:    "<p>\u2018Hi\u2019 &gt; &quot;there&quot; {{ x }}</p>",
			expected: "<p>\u2018Hi\u2019 &gt; &quot;there&quot; {{ x }}</p>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanLiquidTemplate(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestNormalizeLiquidMarkup covers the spellings the console's rich text editor
// (Tiptap Typography + HTML serialization) gives to Liquid syntax typed into a text
// block. The curly characters are written as escapes so they cannot be mistaken for
// ASCII quotes when reading the table.
func TestNormalizeLiquidMarkup(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "curly single quotes around a filter argument",
			input:    "{{ first_name | default: \u2018Valued User\u2019 }}",
			expected: "{{ first_name | default: 'Valued User' }}",
		},
		{
			name:     "curly double quotes around a filter argument",
			input:    "{{ first_name | default: \u201cValued User\u201d }}",
			expected: "{{ first_name | default: \"Valued User\" }}",
		},
		{
			name:     "entity-encoded quotes from MJML attribute escaping",
			input:    "{{ url | default: &#39;https://a.test&#39; | append: &quot;?x&quot; | append: &#34;y&#34; | append: &apos;z&apos; }}",
			expected: "{{ url | default: 'https://a.test' | append: \"?x\" | append: \"y\" | append: 'z' }}",
		},
		{
			name:     "not-equal sign becomes the != operator",
			input:    "{% if contact.first_name \u2260 blank %}",
			expected: "{% if contact.first_name != blank %}",
		},
		{
			name:     "entity-encoded comparison operators",
			input:    "{% if a &gt; 1 and b &lt; 2 and c &gt;= 3 and d &#62; 4 and e &#60;= 5 %}",
			expected: "{% if a > 1 and b < 2 and c >= 3 and d > 4 and e <= 5 %}",
		},
		{
			name:     "operator and curly quotes combined",
			input:    "{% if contact.country \u2260 \u201cFR\u201d %}",
			expected: "{% if contact.country != \"FR\" %}",
		},
		{
			name:     "ASCII-quoted literal keeps a curly apostrophe",
			input:    "{{ 'It\u2019s here' | upcase }}",
			expected: "{{ 'It\u2019s here' | upcase }}",
		},
		{
			name:     "ASCII-quoted literal keeps entities and the not-equal sign",
			input:    "{{ \"a &gt; b \u2260 c &#39;d&#39;\" }}",
			expected: "{{ \"a &gt; b \u2260 c &#39;d&#39;\" }}",
		},
		{
			name:     "apostrophe the editor curled inside a curly literal is straightened",
			input:    "{{ x | default: \u201cWe\u2019re glad\u201d }}",
			expected: "{{ x | default: \"We're glad\" }}",
		},
		{
			name:     "curly double-quoted literal keeps an ASCII apostrophe",
			input:    "{{ x | default: \u201cWe're glad\u201d }}",
			expected: "{{ x | default: \"We're glad\" }}",
		},
		{
			name:     "literal opened by a curly quote closes on an ASCII quote",
			input:    "{{ x | default: \u2018Guest' }}",
			expected: "{{ x | default: 'Guest' }}",
		},
		{
			name:     "markup ending inside a curly literal is left unchanged",
			input:    "{{ x | default: \u2018Guest }}",
			expected: "{{ x | default: \u2018Guest }}",
		},
		{
			name:     "lone curly quote is left unchanged",
			input:    "{{ \u2019 }}",
			expected: "{{ \u2019 }}",
		},
		{
			name:     "ASCII opener closed by a curly quote",
			input:    "{{ x | default: 'Guest\u2019 }}",
			expected: "{{ x | default: 'Guest' }}",
		},
		{
			name:     "ASCII opener closed by a curly quote, followed by another literal",
			input:    "{% if a == 'x\u2019 or b == 'y' %}",
			expected: "{% if a == 'x' or b == 'y' %}",
		},
		{
			name:     "two half-curled literals in one markup",
			input:    "{{ c | append: 'a b\u2019 | prepend: '10%\u2019 }}",
			expected: "{{ c | append: 'a b' | prepend: '10%' }}",
		},
		{
			name:     "ASCII literal holding apostrophes that end words is left as typed",
			input:    "{{ 'the dogs\u2019 and cats\u2019 toys' }}",
			expected: "{{ 'the dogs\u2019 and cats\u2019 toys' }}",
		},
		{
			name:     "double quotes inside a curly literal are kept",
			input:    "{{ x | default: \u2018say \u201chi\u201d\u2019 | append: \u2018 &quot;ok&quot;\u2019 }}",
			expected: "{{ x | default: 'say \u201chi\u201d' | append: ' &quot;ok&quot;' }}",
		},
		{
			name:     "entity-encoded ASCII literal keeps a curly apostrophe",
			input:    "{{ x | default: &#39;It\u2019s you&#39; }}",
			expected: "{{ x | default: 'It\u2019s you' }}",
		},
		{
			name:     "name with an apostrophe in a curly literal",
			input:    "{% if contact.last_name == \u201cO\u2019Brien\u201d %}",
			expected: "{% if contact.last_name == \"O'Brien\" %}",
		},
		{
			name:     "other entity spellings of quotes",
			input:    "{{ a | default: &#x27;A&#x27; | append: &#039;B&#039; | append: &lsquo;C&rsquo; | append: &ldquo;D&rdquo; | append: &#8216;E&#8217; }}",
			expected: "{{ a | default: 'A' | append: 'B' | append: 'C' | append: \"D\" | append: 'E' }}",
		},
		{
			name:     "German low quotes",
			input:    "{{ x | default: \u201eGast\u201c }}",
			expected: "{{ x | default: \"Gast\" }}",
		},
		{
			name:     "other spellings of comparison operators",
			input:    "{% if a \u2265 1 and b \u2264 2 and c &ne; 3 and d &#x3E; 4 and e &ge; 5 %}",
			expected: "{% if a >= 1 and b <= 2 and c != 3 and d > 4 and e >= 5 %}",
		},
		{
			name:     "entities that are neither quotes nor operators are copied",
			input:    "{{ a | append: &amp; | append: &hellip; }}",
			expected: "{{ a | append: &amp; | append: &hellip; }}",
		},
		{
			name:     "valid markup is unchanged",
			input:    "{% if contact.age >= 18 and contact.name != 'Jo' %}",
			expected: "{% if contact.age >= 18 and contact.name != 'Jo' %}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeLiquidMarkup(tt.input); got != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, got)
			}
		})
	}
}

// TestProcessLiquidTemplate_EditorRewrittenMarkup renders Liquid exactly as the
// console's text editor stores it once Typography has curled the quotes and HTML
// serialization has encoded the operators. Unnormalized, a curly-quoted default
// renders nothing, and ≠ or &gt; silently reduce the comparison to a truthiness test
// of its left operand; the remaining cases guard against normalizing too much.
func TestProcessLiquidTemplate_EditorRewrittenMarkup(t *testing.T) {
	const greeting = "Hello {{ first_name | default: \u2018Valued User\u2019 }}!"

	tests := []struct {
		name         string
		content      string
		templateData map[string]interface{}
		expected     string
	}{
		{
			name:         "default applies when the variable is null",
			content:      greeting,
			templateData: map[string]interface{}{"first_name": nil},
			expected:     "Hello Valued User!",
		},
		{
			name:         "default applies when the variable is missing",
			content:      greeting,
			templateData: map[string]interface{}{"contact": map[string]interface{}{"email": "a@b.test"}},
			expected:     "Hello Valued User!",
		},
		{
			name:         "default applies when the variable is an empty string",
			content:      greeting,
			templateData: map[string]interface{}{"first_name": ""},
			expected:     "Hello Valued User!",
		},
		{
			name:         "default is ignored when the variable has a value",
			content:      greeting,
			templateData: map[string]interface{}{"first_name": "Jo"},
			expected:     "Hello Jo!",
		},
		{
			name:         "curly double quotes on a nested contact field",
			content:      "Hi {{ contact.first_name | default: \u201cfriend\u201d }},",
			templateData: map[string]interface{}{"contact": map[string]interface{}{"first_name": nil}},
			expected:     "Hi friend,",
		},
		{
			name:         "not-equal sign compares instead of testing truthiness",
			content:      "{% if contact.name \u2260 \u2018Jo\u2019 %}other{% else %}jo{% endif %}",
			templateData: map[string]interface{}{"contact": map[string]interface{}{"name": "Jo"}},
			expected:     "jo",
		},
		{
			name:         "entity-encoded greater-than compares instead of testing truthiness",
			content:      "<p>{% if contact.age &gt; 18 %}adult{% else %}minor{% endif %}</p>",
			templateData: map[string]interface{}{"contact": map[string]interface{}{"age": 10}},
			expected:     "<p>minor</p>",
		},
		{
			name:         "tag containing a percent sign",
			content:      "{% assign year = \u20182024-03-05\u2019 | date: \u2018%Y\u2019 %}{{ year }}",
			templateData: map[string]interface{}{"contact": map[string]interface{}{}},
			expected:     "2024",
		},
		{
			name:         "contains with a curly-quoted percent sign",
			content:      "{% if contact.discount contains \u2018%\u2019 %}percent{% else %}amount{% endif %}",
			templateData: map[string]interface{}{"contact": map[string]interface{}{"discount": "10%"}},
			expected:     "percent",
		},
		{
			name:         "name with an apostrophe compares equal",
			content:      "{% if contact.last_name == \u201cO\u2019Brien\u201d %}match{% else %}nomatch{% endif %}",
			templateData: map[string]interface{}{"contact": map[string]interface{}{"last_name": "O'Brien"}},
			expected:     "match",
		},
		{
			name:         "greater-or-equal sign from a word processor compares",
			content:      "{% if contact.age \u2265 30 %}old{% else %}young{% endif %}",
			templateData: map[string]interface{}{"contact": map[string]interface{}{"age": 20}},
			expected:     "young",
		},
		{
			name:         "lone curly quote still renders nothing",
			content:      "[{{ \u2019 }}]",
			templateData: map[string]interface{}{"contact": map[string]interface{}{}},
			expected:     "[]",
		},
		{
			name:         "raw body ends where liquidgo ends it",
			content:      "{% raw %}{{ \u2018x\u2019 }}{%- endraw -%}{{ \u2018y\u2019 }}{% endraw %}{{ x | default: \u2018z\u2019 }}",
			templateData: map[string]interface{}{"contact": map[string]interface{}{}},
			expected:     "{{ \u2018x\u2019 }}{%- endraw -%}{{ \u2018y\u2019 }}z",
		},
		{
			name:         "curly quotes outside Liquid markup are left alone",
			content:      "<p>\u2018{{ first_name | default: \u2018there\u2019 }}\u2019 &gt; you</p>",
			templateData: map[string]interface{}{"first_name": nil},
			expected:     "<p>\u2018there\u2019 &gt; you</p>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessLiquidTemplate(tt.content, tt.templateData, "test")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// corruptLiquidLikeEditor rewrites the Liquid markup of a template the way the console's
// text editor stored it before it left Liquid alone: Typography curled every typed
// quote (opening after a space or an opening bracket, closing otherwise) and turned
// != into ≠, HTML serialization encoded > and <, and some spaces became non-breaking.
func corruptLiquidLikeEditor(rng *rand.Rand, template string) string {
	return liquidMarkupRegex.ReplaceAllStringFunc(template, func(markup string) string {
		var b strings.Builder
		prev := ' '
		for i := 0; i < len(markup); i++ {
			c := markup[i]
			opens := strings.ContainsRune(" ({[<'\"", prev)
			switch {
			case c == '\'' && opens:
				b.WriteString("\u2018")
			case c == '\'':
				b.WriteString("\u2019")
			case c == '"' && opens:
				b.WriteString("\u201c")
			case c == '"':
				b.WriteString("\u201d")
			case c == '!' && i+1 < len(markup) && markup[i+1] == '=':
				b.WriteString("\u2260")
				i++
			case c == '>':
				b.WriteString("&gt;")
			case c == '<':
				b.WriteString("&lt;")
			case c == ' ' && rng.Intn(5) == 0:
				b.WriteString([]string{"&nbsp;", "\u00a0"}[rng.Intn(2)])
			default:
				b.WriteByte(c)
			}
			prev = rune(c)
		}
		return b.String()
	})
}

// randomLiquidTemplate builds a template from the Liquid a visual-editor user writes:
// defaults, filters with string arguments, string and number comparisons, dates.
func randomLiquidTemplate(rng *rand.Rand) string {
	pick := func(options ...string) string { return options[rng.Intn(len(options))] }
	literal := func() string {
		text := pick("Guest", "Valued User", "10%", "%Y", "a b", "It's", "O'Brien")
		if strings.Contains(text, "'") {
			return `"` + text + `"`
		}
		quote := pick("'", `"`)
		return quote + text + quote
	}
	field := pick("contact.first_name", "first_name", "contact.country", "contact.last_name", "contact.discount")
	switch rng.Intn(5) {
	case 0:
		return "Hello {{ " + field + " | default: " + literal() + " }}!"
	case 1:
		return "{{ " + field + " | append: " + literal() + " | prepend: " + literal() + " }}"
	case 2:
		return "{% if " + field + " " + pick("==", "!=", "contains") + " " + literal() + " %}A{% else %}B{% endif %}"
	case 3:
		return "{% if contact.age " + pick(">", "<", ">=", "<=", "!=") + " " + pick("18", "20") + " and " + field + " != " + literal() + " %}A{% else %}B{% endif %}"
	default:
		return "{% assign day = '2024-03-05' | date: " + pick(`'%Y'`, `"%d %B"`) + " %}[{{ day }}]"
	}
}

var liquidPropertyData = []map[string]interface{}{
	{"first_name": nil, "contact": map[string]interface{}{"first_name": nil, "country": "FR", "last_name": "O'Brien", "discount": "10%", "age": 20}},
	{"first_name": "Jo", "contact": map[string]interface{}{"first_name": "Sam", "country": "DE", "last_name": "Smith", "discount": "5", "age": 18}},
}

// TestCleanLiquidTemplate_EditorCorruptionRendersAsTyped is the promise of the
// normalization: Liquid as the text editor stored it renders exactly like the Liquid
// the user typed, and cleaning touches nothing outside the markup and is idempotent.
func TestCleanLiquidTemplate_EditorCorruptionRendersAsTyped(t *testing.T) {
	rng := rand.New(rand.NewSource(438))
	for i := 0; i < 1500; i++ {
		typed := randomLiquidTemplate(rng)
		stored := corruptLiquidLikeEditor(rng, typed)
		cleaned := cleanLiquidTemplate(stored)
		require.Equal(t, cleaned, cleanLiquidTemplate(cleaned), "cleaning must be idempotent for %q", stored)
		require.Equal(t, liquidMarkupRegex.ReplaceAllString(stored, ""), liquidMarkupRegex.ReplaceAllString(cleaned, ""),
			"text outside the markup must not change for %q", stored)
		for _, data := range liquidPropertyData {
			want, wantErr := ProcessLiquidTemplate(typed, data, "typed")
			got, gotErr := ProcessLiquidTemplate(stored, data, "stored")
			require.Equal(t, wantErr == nil, gotErr == nil, "typed %q, stored %q", typed, stored)
			require.Equal(t, want, got, "typed %q, stored %q, cleaned %q", typed, stored, cleaned)
		}
	}
}

// TestCleanLiquidTemplate_ASCIILiteralsKeepTheirTypography guards code-mode and API
// templates: an ASCII-quoted literal that deliberately holds curly quotes, dashes or
// entities renders exactly as Liquid renders it without any normalization.
func TestCleanLiquidTemplate_ASCIILiteralsKeepTheirTypography(t *testing.T) {
	texts := []string{"It\u2019s here", "the dogs\u2019 toys", "dogs\u2019 and cats\u2019 toys", "rock \u2019n\u2019 roll", "say \u2018hi\u2019 now", "\u201cquoted\u201d word", "\u2019tis", "a \u2260 b", "x &gt; y", "Today \u2014 \u201cnew\u201d"}
	rng := rand.New(rand.NewSource(7))
	engine := NewSecureLiquidEngine()
	for i := 0; i < 600; i++ {
		first, second := texts[rng.Intn(len(texts))], texts[rng.Intn(len(texts))]
		template := []string{
			"{{ contact.first_name | default: '" + first + "' }}",
			"{{ contact.country | append: '" + first + "' | prepend: '" + second + "' }}",
			"{% if contact.country == '" + first + "' or contact.country != '" + second + "' %}A{% else %}B{% endif %}",
			"{% assign v = '" + first + "' %}{{ v | append: '" + second + "' }}",
		}[rng.Intn(4)]
		for _, data := range liquidPropertyData {
			want, err := engine.Render(template, data)
			require.NoError(t, err)
			got, err := ProcessLiquidTemplate(template, data, "test")
			require.NoError(t, err)
			require.Equal(t, want, got, "template %q", template)
		}
	}
}

func TestProcessLiquidTemplateEmailSubjects(t *testing.T) {
	// Specific tests for email subject line scenarios
	tests := []struct {
		name         string
		subject      string
		templateData map[string]interface{}
		expected     string
	}{
		{
			name:         "personalized subject",
			subject:      "Welcome {{firstName}}!",
			templateData: map[string]interface{}{"firstName": "John", "lastName": "Doe"},
			expected:     "Welcome John!",
		},
		{
			name:         "company and user subject",
			subject:      "{{firstName}}, your {{company}} order is ready",
			templateData: map[string]interface{}{"firstName": "Jane", "company": "ACME Corp"},
			expected:     "Jane, your ACME Corp order is ready",
		},
		{
			name:         "conditional urgency",
			subject:      "{% if urgent %}URGENT: {% endif %}Your order update",
			templateData: map[string]interface{}{"urgent": true},
			expected:     "URGENT: Your order update",
		},
		{
			name:         "non-urgent conditional",
			subject:      "{% if urgent %}URGENT: {% endif %}Your order update",
			templateData: map[string]interface{}{"urgent": false},
			expected:     "Your order update",
		},
		{
			name:         "order count simple",
			subject:      "You have {{orderCount}} order(s)",
			templateData: map[string]interface{}{"orderCount": 1},
			expected:     "You have 1 order(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessLiquidTemplate(tt.subject, tt.templateData, "email_subject")

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestLiquidInHrefAttributes(t *testing.T) {
	tests := []struct {
		name         string
		block        EmailBlock
		templateData string
		expectedHref string
		expectError  bool
	}{
		{
			name: "button with liquid href",
			block: func() EmailBlock {
				b := NewBaseBlock("btn1", MJMLComponentMjButton)
				b.Attributes["href"] = "{{ contact.profile_url }}"
				b.Attributes["backgroundColor"] = "#007bff"
				b.Content = stringPtr("Click me!")
				return &MJButtonBlock{BaseBlock: b}
			}(),
			templateData: `{"contact": {"profile_url": "https://example.com/profile/123"}}`,
			expectedHref: `href="https://example.com/profile/123"`,
			expectError:  false,
		},
		{
			name: "image with liquid src",
			block: func() EmailBlock {
				b := NewBaseBlock("img1", MJMLComponentMjImage)
				b.Attributes["src"] = "{{ user.avatar_url }}"
				b.Attributes["alt"] = "User avatar"
				return &MJImageBlock{BaseBlock: b}
			}(),
			templateData: `{"user": {"avatar_url": "https://example.com/avatars/user123.jpg"}}`,
			expectedHref: `src="https://example.com/avatars/user123.jpg"`,
			expectError:  false,
		},
		{
			name: "social element with liquid href",
			block: func() EmailBlock {
				b := NewBaseBlock("social1", MJMLComponentMjSocialElement)
				b.Attributes["href"] = "{{ company.linkedin_url }}"
				b.Attributes["name"] = "linkedin"
				return &MJSocialElementBlock{BaseBlock: b}
			}(),
			templateData: `{"company": {"linkedin_url": "https://linkedin.com/company/acme"}}`,
			expectedHref: `href="https://linkedin.com/company/acme"`,
			expectError:  false,
		},
		{
			name: "social element with noshare name preserves href",
			block: func() EmailBlock {
				b := NewBaseBlock("social2", MJMLComponentMjSocialElement)
				b.Attributes["href"] = "https://facebook.com/mypage"
				b.Attributes["name"] = "facebook-noshare"
				return &MJSocialElementBlock{BaseBlock: b}
			}(),
			templateData: `{}`,
			expectedHref: `name="facebook-noshare"`,
			expectError:  false,
		},
		{
			name: "non-URL attributes should not be processed",
			block: func() EmailBlock {
				b := NewBaseBlock("text1", MJMLComponentMjText)
				b.Attributes["fontSize"] = "{{ font_size }}" // Not a URL attribute
				b.Attributes["href"] = "{{ link_url }}"      // URL attribute
				b.Content = stringPtr("Hello world")
				return &MJTextBlock{BaseBlock: b}
			}(),
			templateData: `{"font_size": "18px", "link_url": "https://example.com"}`,
			expectedHref: `font-size="{{ font_size }}"`, // Should NOT be processed
			expectError:  false,
		},
		{
			name: "background-url with liquid",
			block: func() EmailBlock {
				b := NewBaseBlock("section1", MJMLComponentMjSection)
				b.Attributes["backgroundUrl"] = "{{ campaign.background_image }}"
				return &MJSectionBlock{BaseBlock: b}
			}(),
			templateData: `{"campaign": {"background_image": "https://example.com/bg.jpg"}}`,
			expectedHref: `background-url="https://example.com/bg.jpg"`,
			expectError:  false,
		},
		{
			name: "liquid with conditional logic",
			block: func() EmailBlock {
				b := NewBaseBlock("btn2", MJMLComponentMjButton)
				b.Attributes["href"] = "{% if user.is_premium %}{{ premium_url }}{% else %}{{ regular_url }}{% endif %}"
				b.Content = stringPtr("Get Started")
				return &MJButtonBlock{BaseBlock: b}
			}(),
			templateData: `{"user": {"is_premium": true}, "premium_url": "https://premium.example.com", "regular_url": "https://example.com"}`,
			expectedHref: `href="https://premium.example.com"`,
			expectError:  false,
		},
		{
			name: "empty template data preserves liquid syntax",
			block: func() EmailBlock {
				b := NewBaseBlock("btn3", MJMLComponentMjButton)
				b.Attributes["href"] = "{{ fallback_url | default: 'https://fallback.com' }}"
				b.Content = stringPtr("Fallback")
				return &MJButtonBlock{BaseBlock: b}
			}(),
			templateData: `{}`,
			// With empty template data, Liquid syntax is preserved (issue #225, #226)
			expectedHref: `href="{{ fallback_url | default: &#39;https://fallback.com&#39; }}"`,
			expectError:  false,
		},
		{
			name: "undefined variable with default filter",
			block: func() EmailBlock {
				b := NewBaseBlock("btn3b", MJMLComponentMjButton)
				b.Attributes["href"] = "{{ fallback_url | default: 'https://fallback.com' }}"
				b.Content = stringPtr("Fallback")
				return &MJButtonBlock{BaseBlock: b}
			}(),
			// When template data has some values (but not fallback_url), default filter is used
			templateData: `{"other_var": "value"}`,
			expectedHref: `href="https://fallback.com"`,
			expectError:  false,
		},
		{
			name: "liquid with non-breaking space should be cleaned",
			block: func() EmailBlock {
				b := NewBaseBlock("btn_nbsp", MJMLComponentMjButton)
				b.Attributes["href"] = "{{ \u00a0confirm_subscription_url }}"
				b.Content = stringPtr("Confirm")
				return &MJButtonBlock{BaseBlock: b}
			}(),
			templateData: `{"confirm_subscription_url": "https://example.com/confirm"}`,
			expectedHref: `href="https://example.com/confirm"`,
			expectError:  false,
		},
		{
			name: "invalid liquid syntax should return original",
			block: func() EmailBlock {
				b := NewBaseBlock("btn4", MJMLComponentMjButton)
				b.Attributes["href"] = "{{ invalid syntax"
				b.Content = stringPtr("Error Test")
				return &MJButtonBlock{BaseBlock: b}
			}(),
			templateData: `{"url": "https://test.com"}`,
			expectedHref: `href="{{ invalid syntax"`, // Should return original on error
			expectError:  false,                      // We don't error, just log warning
		},
		{
			name: "image with liquid alt text",
			block: func() EmailBlock {
				b := NewBaseBlock("img2", MJMLComponentMjImage)
				b.Attributes["src"] = "https://example.com/product.jpg"
				b.Attributes["alt"] = "{{ product.name }} - {{ product.category }}"
				return &MJImageBlock{BaseBlock: b}
			}(),
			templateData: `{"product": {"name": "Blue Widget", "category": "Electronics"}}`,
			expectedHref: `alt="Blue Widget - Electronics"`,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertJSONToMJMLWithData(tt.block, tt.templateData)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !strings.Contains(result, tt.expectedHref) {
				t.Errorf("Expected result to contain '%s', got: %s", tt.expectedHref, result)
			}
		})
	}
}

func TestProcessAttributeValue(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		attributeKey string
		templateData map[string]interface{}
		blockID      string
		expected     string
	}{
		{
			name:         "href attribute with liquid",
			value:        "{{ base_url }}/profile",
			attributeKey: "href",
			templateData: map[string]interface{}{"base_url": "https://example.com"},
			blockID:      "test",
			expected:     "https://example.com/profile",
		},
		{
			name:         "src attribute with liquid",
			value:        "{{ cdn_url }}/image.jpg",
			attributeKey: "src",
			templateData: map[string]interface{}{"cdn_url": "https://cdn.example.com"},
			blockID:      "test",
			expected:     "https://cdn.example.com/image.jpg",
		},
		{
			name:         "non-url attribute should not process",
			value:        "{{ font_size }}",
			attributeKey: "fontSize",
			templateData: map[string]interface{}{"font_size": "18px"},
			blockID:      "test",
			expected:     "{{ font_size }}", // Should return original
		},
		{
			name:         "action attribute with liquid",
			value:        "{{ form_action }}",
			attributeKey: "action",
			templateData: map[string]interface{}{"form_action": "https://api.example.com/submit"},
			blockID:      "test",
			expected:     "https://api.example.com/submit",
		},
		{
			name:         "custom-url attribute with liquid",
			value:        "{{ custom_value }}",
			attributeKey: "my-custom-url",
			templateData: map[string]interface{}{"custom_value": "https://custom.example.com"},
			blockID:      "test",
			expected:     "https://custom.example.com",
		},
		{
			name:         "nil template data",
			value:        "{{ some_var }}",
			attributeKey: "href",
			templateData: nil,
			blockID:      "test",
			expected:     "{{ some_var }}", // Should return original
		},
		{
			name:         "alt attribute with liquid",
			value:        "{{ product.name }} image",
			attributeKey: "alt",
			templateData: map[string]interface{}{"product": map[string]interface{}{"name": "Blue Widget"}},
			blockID:      "test",
			expected:     "Blue Widget image",
		},
		// Issue #226: Liquid-only src that would render to empty when variable is missing
		{
			name:         "src with liquid-only value when variable missing from template data",
			value:        "{{ postImage }}",
			attributeKey: "src",
			templateData: map[string]interface{}{"other_var": "value"}, // postImage is NOT in data
			blockID:      "test",
			expected:     "[undefined: postImage]", // Should show debug message, NOT render to empty
		},
		{
			name:         "href with liquid-only value when variable missing from template data",
			value:        "{{ link_url }}",
			attributeKey: "href",
			templateData: map[string]interface{}{"contact": map[string]interface{}{"email": "test@example.com"}}, // link_url is NOT in data
			blockID:      "test",
			expected:     "[undefined: link_url]", // Should show debug message, NOT render to empty
		},
		{
			name:         "src with mixed content when variable missing",
			value:        "https://cdn.example.com/{{ image_path }}",
			attributeKey: "src",
			templateData: map[string]interface{}{"other_var": "value"}, // image_path is NOT in data
			blockID:      "test",
			// When some part of the URL is static, the result will be non-empty
			expected: "https://cdn.example.com/",
		},
		{
			name:         "src with liquid and default filter when variable missing",
			value:        "{{ image_url | default: 'https://placeholder.com/img.jpg' }}",
			attributeKey: "src",
			templateData: map[string]interface{}{"other_var": "value"}, // image_url is NOT in data
			blockID:      "test",
			expected:     "https://placeholder.com/img.jpg", // Default should be used
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processAttributeValue(tt.value, tt.attributeKey, tt.templateData, tt.blockID, false)

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestSecureLiquidIntegration tests the security features in the MJML converter context
func TestSecureLiquidIntegration(t *testing.T) {
	t.Run("timeout protection in MJML conversion", func(t *testing.T) {
		// Create a text block with template that would timeout
		block := func() EmailBlock {
			b := NewBaseBlock("text1", MJMLComponentMjText)
			b.Content = stringPtr(`
				{% for i in (1..1000000) %}
					{% for j in (1..1000000) %}
						<div>{{ i }} - {{ j }}</div>
					{% endfor %}
				{% endfor %}
			`)
			return &MJTextBlock{BaseBlock: b}
		}()

		templateData := `{}`

		// Should complete (even if it returns original content due to timeout)
		result, err := ConvertJSONToMJMLWithData(block, templateData)

		// Should not hang or crash - either returns result or logs warning
		if err != nil {
			// Error is acceptable (conversion might fail if liquid fails)
			t.Logf("Got error (acceptable): %v", err)
		}

		// The important thing is we didn't hang - test passes if we get here
		_ = result
	})

	t.Run("normal email templates work correctly", func(t *testing.T) {
		// Create a realistic email template
		block := func() EmailBlock {
			b := NewBaseBlock("text3", MJMLComponentMjText)
			b.Content = stringPtr(`
				<h1>Hello {{ user.name }}!</h1>
				<p>Thank you for your order #{{ order.id }}.</p>
				{% if order.tracking_url %}
					<p>Track your order: <a href="{{ order.tracking_url }}">Click here</a></p>
				{% endif %}
			`)
			return &MJTextBlock{BaseBlock: b}
		}()

		templateData := `{
			"user": {"name": "John Doe"},
			"order": {
				"id": "12345",
				"tracking_url": "https://example.com/track/12345"
			}
		}`

		result, err := ConvertJSONToMJMLWithData(block, templateData)

		if err != nil {
			t.Fatalf("Expected no error for normal template, got: %v", err)
		}

		// Verify content was rendered
		if !strings.Contains(result, "John Doe") {
			t.Error("Expected rendered username in result")
		}
		if !strings.Contains(result, "12345") {
			t.Error("Expected order ID in result")
		}
		if !strings.Contains(result, "https://example.com/track/12345") {
			t.Error("Expected tracking URL in result")
		}
	})

	t.Run("backward compatibility with existing templates", func(t *testing.T) {
		// Test that existing tests still pass with secure engine
		testCases := []struct {
			content  string
			data     string
			expected string
		}{
			{"Hello {{ name }}", `{"name": "World"}`, "Hello World"},
			{"Price: ${{ price }}", `{"price": 99.99}`, "Price: $99.99"},
		}

		for _, tc := range testCases {
			block := func() EmailBlock {
				b := NewBaseBlock("test", MJMLComponentMjText)
				b.Content = stringPtr(tc.content)
				return &MJTextBlock{BaseBlock: b}
			}()

			result, err := ConvertJSONToMJMLWithData(block, tc.data)
			if err != nil {
				t.Errorf("Unexpected error for %q: %v", tc.content, err)
				continue
			}

			if !strings.Contains(result, tc.expected) {
				t.Errorf("Expected %q in result for %q, got: %s", tc.expected, tc.content, result)
			}
		}
	})

	t.Run("realistic email with multiple blocks", func(t *testing.T) {
		// Test a more complex email structure
		section := func() EmailBlock {
			s := NewBaseBlock("section1", MJMLComponentMjSection)

			// Add text block
			text := NewBaseBlock("text1", MJMLComponentMjText)
			text.Content = stringPtr("Welcome {{ user.name }}!")
			s.Children = []EmailBlock{&MJTextBlock{BaseBlock: text}}
			return &MJSectionBlock{BaseBlock: s}
		}()

		templateData := `{"user": {"name": "Alice"}}`
		result, err := ConvertJSONToMJMLWithData(section, templateData)

		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if !strings.Contains(result, "Welcome Alice!") {
			t.Error("Expected rendered content in result")
		}
	})
}

func TestFormatSingleAttribute(t *testing.T) {
	// Test formatSingleAttribute - this was at 0% coverage
	// formatSingleAttribute is a wrapper that calls formatSingleAttributeWithLiquid with nil template data
	tests := []struct {
		name     string
		key      string
		value    interface{}
		expected string
	}{
		{
			name:     "string value",
			key:      "href",
			value:    "https://example.com",
			expected: ` href="https://example.com"`,
		},
		{
			name:     "boolean true",
			key:      "disabled",
			value:    true,
			expected: " disabled",
		},
		{
			name:     "boolean false",
			key:      "disabled",
			value:    false,
			expected: "",
		},
		{
			name:     "empty string",
			key:      "title",
			value:    "",
			expected: "",
		},
		{
			name:     "numeric value",
			key:      "width",
			value:    100,
			expected: ` width="100"`,
		},
		{
			name:     "camelCase to kebab-case",
			key:      "backgroundColor",
			value:    "#ffffff",
			expected: ` background-color="#ffffff"`,
		},
		{
			name:     "pointer to string",
			key:      "src",
			value:    stringPtr("image.jpg"),
			expected: ` src="image.jpg"`,
		},
		{
			name:     "nil pointer",
			key:      "optional",
			value:    (*string)(nil),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSingleAttribute(tt.key, tt.value)
			if result != tt.expected {
				t.Errorf("formatSingleAttribute(%q, %v) = %q, want %q", tt.key, tt.value, result, tt.expected)
			}
		})
	}
}

func TestEndToEnd_MjAttributesGlobalsPreserved(t *testing.T) {
	// Build full tree via JSON: mjml > head > mj-attributes > [mj-all, mj-text] + body > section > column > mj-text
	fullJSON := `{
		"id": "mjml-1",
		"type": "mjml",
		"children": [
			{
				"id": "head-1",
				"type": "mj-head",
				"children": [
					{
						"id": "attrs-1",
						"type": "mj-attributes",
						"children": [
							{"id":"all-1","type":"mj-all","attributes":{"fontFamily":"Helvetica"}},
							{"id":"text-def","type":"mj-text","attributes":{"color":"#333333"}}
						]
					}
				]
			},
			{
				"id": "body-1",
				"type": "mj-body",
				"children": [
					{
						"id": "section-1",
						"type": "mj-section",
						"children": [
							{
								"id": "column-1",
								"type": "mj-column",
								"children": [
									{"id":"text-1","type":"mj-text","content":"Hello"}
								]
							}
						]
					}
				]
			}
		]
	}`

	block, err := UnmarshalEmailBlock([]byte(fullJSON))
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Convert to MJML string
	mjmlOutput := ConvertJSONToMJML(block)

	// Behavior: mj-all appears in the MJML output with its attributes
	if !strings.Contains(mjmlOutput, `<mj-all font-family="Helvetica"`) {
		t.Errorf("Expected mj-all with font-family in output, got:\n%s", mjmlOutput)
	}

	// Behavior: body mj-text with no stored attributes produces a tag without inline attributes
	// (so mj-attributes globals can take effect at render time)
	if !strings.Contains(mjmlOutput, `<mj-text>Hello</mj-text>`) {
		t.Errorf("Expected body mj-text without inline attributes, got:\n%s", mjmlOutput)
	}
}

func TestMJLiquidDirectOutput(t *testing.T) {
	content := `{% for item in items %}<mj-column><mj-text>{{ item.name }}</mj-text></mj-column>{% endfor %}`
	base := NewBaseBlock("liq", MJMLComponentMjLiquid)
	base.Content = stringPtr(content)

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("sec", MJMLComponentMjSection)}
	section.Children = []EmailBlock{&MJLiquidBlock{BaseBlock: base}}

	result := ConvertJSONToMJMLRaw(section)
	if strings.Contains(result, "<mj-liquid") {
		t.Errorf("Result should not contain <mj-liquid tag, got: %s", result)
	}
	if strings.Contains(result, "</mj-liquid>") {
		t.Errorf("Result should not contain </mj-liquid> tag, got: %s", result)
	}
	if !strings.Contains(result, "{% for item in items %}") {
		t.Errorf("Result should contain Liquid for-loop, got: %s", result)
	}
	if !strings.Contains(result, "<mj-section") {
		t.Errorf("Result should contain <mj-section, got: %s", result)
	}
}

func TestMJLiquidEmptyContent(t *testing.T) {
	base := NewBaseBlock("liq", MJMLComponentMjLiquid)
	block := &MJLiquidBlock{BaseBlock: base}

	column := &MJColumnBlock{BaseBlock: NewBaseBlock("col", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{block}

	result := ConvertJSONToMJMLRaw(column)
	if strings.Contains(result, "mj-liquid") {
		t.Errorf("Result should not contain mj-liquid, got: %s", result)
	}
}

func TestMJLiquidInsideColumn(t *testing.T) {
	base := NewBaseBlock("liq", MJMLComponentMjLiquid)
	base.Content = stringPtr(`{% if show %}<mj-image src="test.jpg" />{% endif %}`)

	column := &MJColumnBlock{BaseBlock: NewBaseBlock("col", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{&MJLiquidBlock{BaseBlock: base}}

	result := ConvertJSONToMJMLRaw(column)
	if !strings.Contains(result, "{% if show %}") {
		t.Errorf("Result should contain Liquid conditional, got: %s", result)
	}
	if strings.Contains(result, "<mj-liquid") {
		t.Errorf("Result should not contain <mj-liquid tag, got: %s", result)
	}
}

func TestMJLiquidMixedWithRegularBlocks(t *testing.T) {
	text := &MJTextBlock{BaseBlock: NewBaseBlock("txt", MJMLComponentMjText)}
	text.Content = stringPtr("Hello World")

	liqBase := NewBaseBlock("liq", MJMLComponentMjLiquid)
	liqBase.Content = stringPtr(`{% if extra %}<mj-text>Extra</mj-text>{% endif %}`)

	column := &MJColumnBlock{BaseBlock: NewBaseBlock("col", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{text, &MJLiquidBlock{BaseBlock: liqBase}}

	result := ConvertJSONToMJMLRaw(column)
	if !strings.Contains(result, "Hello World</mj-text>") {
		t.Errorf("Result should contain regular text block content, got: %s", result)
	}
	if !strings.Contains(result, "{% if extra %}") {
		t.Errorf("Result should contain Liquid conditional, got: %s", result)
	}
	if strings.Contains(result, "<mj-liquid") {
		t.Errorf("Result should not contain <mj-liquid tag, got: %s", result)
	}
}

func TestMJLiquidEmptyStringContent(t *testing.T) {
	base := NewBaseBlock("liq", MJMLComponentMjLiquid)
	base.Content = stringPtr("")
	block := &MJLiquidBlock{BaseBlock: base}

	column := &MJColumnBlock{BaseBlock: NewBaseBlock("col", MJMLComponentMjColumn)}
	column.Children = []EmailBlock{block}

	result := ConvertJSONToMJMLRaw(column)
	if strings.Contains(result, "mj-liquid") {
		t.Errorf("Result should not contain mj-liquid, got: %s", result)
	}
}

func TestMJLiquidContentWithSpecialChars(t *testing.T) {
	base := NewBaseBlock("liq", MJMLComponentMjLiquid)
	base.Content = stringPtr(`{% if show %}<mj-text>Price: $5 &amp; free shipping</mj-text>{% endif %}`)

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("sec", MJMLComponentMjSection)}
	section.Children = []EmailBlock{&MJLiquidBlock{BaseBlock: base}}

	result := ConvertJSONToMJMLRaw(section)
	if !strings.Contains(result, "&amp;") {
		t.Errorf("Result should preserve HTML entities in content, got: %s", result)
	}
	if !strings.Contains(result, "{% if show %}") {
		t.Errorf("Result should preserve Liquid syntax, got: %s", result)
	}
	if strings.Contains(result, "<mj-liquid") {
		t.Errorf("Result should not contain <mj-liquid tag, got: %s", result)
	}
}

func TestMJLiquidNotPerBlockProcessed(t *testing.T) {
	base := NewBaseBlock("liq", MJMLComponentMjLiquid)
	base.Content = stringPtr(`{% for item in items %}<mj-text>{{ item.name }}</mj-text>{% endfor %}`)

	section := &MJSectionBlock{BaseBlock: NewBaseBlock("sec", MJMLComponentMjSection)}
	section.Children = []EmailBlock{&MJLiquidBlock{BaseBlock: base}}

	body := &MJBodyBlock{BaseBlock: NewBaseBlock("body", MJMLComponentMjBody)}
	body.Children = []EmailBlock{section}

	root := &MJMLBlock{BaseBlock: NewBaseBlock("mjml", MJMLComponentMjml)}
	root.Children = []EmailBlock{body}

	result, err := ConvertJSONToMJMLWithData(root, `{"items": [{"name": "A"}]}`)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Liquid should NOT be rendered per-block — still raw
	if !strings.Contains(result, "{% for item in items %}") {
		t.Errorf("Liquid syntax should be preserved (not per-block processed), got: %s", result)
	}
	if strings.Contains(result, "<mj-liquid") {
		t.Errorf("Result should not contain <mj-liquid tag, got: %s", result)
	}
}

func BenchmarkProcessLiquidContent(b *testing.B) {
	content := "Hello {{ contact.first_name }}, welcome to {% if contact.language == 'en' %}our newsletter{% endif %}!"
	data := map[string]interface{}{
		"contact": map[string]interface{}{
			"first_name": "Alice",
			"language":   "en",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := processLiquidContent(content, data, "bench-block"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProcessLiquidContentParallel(b *testing.B) {
	content := "Hello {{ contact.first_name }}!"
	data := map[string]interface{}{
		"contact": map[string]interface{}{"first_name": "Alice"},
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := processLiquidContent(content, data, "bench-block"); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCamelToKebab(b *testing.B) {
	for i := 0; i < b.N; i++ {
		camelToKebab("backgroundColor")
	}
}

func TestProtectRestoreLiquidDelimiters_RoundTrips(t *testing.T) {
	cases := []string{
		"Hi {{ x }}",
		"{% if a %}b{% endif %}",
		"{{ a | default: '{% raw %}' }}",
		"50% }} off {{",
		"plain text, no delimiters",
		"",
		"{{{%}}%}",
	}
	for _, s := range cases {
		if got := restoreLiquidDelimiters(protectRenderedLiquidDelimiters(s)); got != s {
			t.Errorf("round-trip changed %q -> %q", s, got)
		}
	}
}

func TestProtectRemovesAllLiveDelimiters(t *testing.T) {
	for _, s := range []string{"{{ x }}", "{% if a %}", "a }} b %} c", "{{{%"} {
		p := protectRenderedLiquidDelimiters(s)
		for _, delim := range []string{"{{", "}}", "{%", "%}"} {
			if strings.Contains(p, delim) {
				t.Errorf("protect left live delimiter %q in %q (-> %q)", delim, s, p)
			}
		}
	}
}

func TestRestoreIsIdempotentAndSafeWithoutSentinels(t *testing.T) {
	plain := "no sentinels {{ still literal }}"
	if restoreLiquidDelimiters(plain) != plain {
		t.Errorf("restore altered a string with no sentinels: %q", restoreLiquidDelimiters(plain))
	}
	p := protectRenderedLiquidDelimiters("{{ a }}{% b %}")
	once := restoreLiquidDelimiters(p)
	if restoreLiquidDelimiters(once) != once {
		t.Errorf("restore is not idempotent")
	}
}

func TestProtectStripsPreexistingSentinels(t *testing.T) {
	// Data that already contains a sentinel must not forge a delimiter on restore.
	planted := "danger" + liquidSentinelOpenVar + "evil" + liquidSentinelCloseTag
	out := restoreLiquidDelimiters(protectRenderedLiquidDelimiters(planted))
	for _, delim := range []string{"{{", "}}", "{%", "%}"} {
		if strings.Contains(out, delim) {
			t.Errorf("planted sentinel forged delimiter %q: %q", delim, out)
		}
	}
}
