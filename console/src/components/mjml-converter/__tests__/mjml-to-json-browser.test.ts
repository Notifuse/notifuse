import { describe, test, expect } from 'vitest'
import { convertMjmlToJsonBrowser, preprocessMjml } from '../mjml-to-json-browser'

describe('MJML Preprocessing', () => {
  test('should escape unescaped ampersands in attributes', () => {
    const input = '<mj-image src="https://example.com?a=1&b=2" />'
    const expected = '<mj-image src="https://example.com?a=1&amp;b=2" />'
    expect(preprocessMjml(input)).toBe(expected)
  })

  test('should not double-escape already escaped ampersands', () => {
    const input = '<mj-image src="https://example.com?a=1&amp;b=2" />'
    expect(preprocessMjml(input)).toBe(input) // Should remain unchanged
  })

  test('should handle multiple unescaped ampersands', () => {
    const input = '<mj-button href="https://example.com?a=1&b=2&c=3" />'
    const expected = '<mj-button href="https://example.com?a=1&amp;b=2&amp;c=3" />'
    expect(preprocessMjml(input)).toBe(expected)
  })

  test('should preserve other XML entities', () => {
    const input = '<mj-text title="Test &lt;tag&gt; &quot;quote&quot; &apos;apos&apos;" />'
    expect(preprocessMjml(input)).toBe(input) // Should remain unchanged
  })

  test('should preserve numeric entities', () => {
    const input = '<mj-text title="Copyright &#169; &#xA9;" />'
    expect(preprocessMjml(input)).toBe(input) // Should remain unchanged
  })

  test('should handle mixed escaped and unescaped ampersands', () => {
    const input = '<mj-image src="https://example.com?safe=1&amp;unsafe=2&bad=3" />'
    const expected = '<mj-image src="https://example.com?safe=1&amp;unsafe=2&amp;bad=3" />'
    expect(preprocessMjml(input)).toBe(expected)
  })

  describe('Duplicate Attribute Handling', () => {
    test('should remove duplicate attributes and keep the last occurrence', () => {
      const input =
        '<mj-section background-color="#ffffff" padding="20px" background-color="#000000">'
      const processed = preprocessMjml(input)

      // Should only have one background-color, and it should be the last one
      expect(processed).toContain('background-color="#000000"')
      expect(processed).not.toContain('background-color="#ffffff"')
      expect(processed).toContain('padding="20px"')
    })

    test('should handle multiple duplicate attributes on same tag', () => {
      const input =
        '<mj-button color="#red" background-color="#fff" color="#blue" background-color="#000">'
      const processed = preprocessMjml(input)

      // Should keep last occurrence of each duplicate
      expect(processed).toContain('color="#blue"')
      expect(processed).toContain('background-color="#000"')
      expect(processed).not.toContain('color="#red"')
      expect(processed).not.toContain('background-color="#fff"')
    })

    test('should handle duplicate attributes on self-closing tags', () => {
      const input = '<mj-spacer height="10px" height="20px" />'
      const processed = preprocessMjml(input)

      expect(processed).toContain('height="20px"')
      expect(processed).not.toContain('height="10px"')
      expect(processed).toContain('/>')
    })

    test('should handle duplicate attributes across multiple tags', () => {
      const input = `
        <mj-section background-color="#fff" background-color="#000">
          <mj-column width="50%" width="100%">
            <mj-text color="#red" color="#blue">Test</mj-text>
          </mj-column>
        </mj-section>
      `
      const processed = preprocessMjml(input)

      // Each tag should have its duplicates removed independently
      expect(processed).toContain('background-color="#000"')
      expect(processed).toContain('width="100%"')
      expect(processed).toContain('color="#blue"')
    })

    test('should not modify tags without duplicate attributes', () => {
      const input = '<mj-section background-color="#ffffff" padding="20px" border-radius="5px">'
      const processed = preprocessMjml(input)

      // Should remain unchanged
      expect(processed).toContain('background-color="#ffffff"')
      expect(processed).toContain('padding="20px"')
      expect(processed).toContain('border-radius="5px"')
    })

    test('should handle tags with no attributes', () => {
      const input = '<mj-section><mj-column></mj-column></mj-section>'
      const processed = preprocessMjml(input)

      // Should remain unchanged
      expect(processed).toBe(input)
    })

    test('should preserve attribute order for non-duplicate attributes', () => {
      const input = '<mj-button href="#" color="#red" padding="10px" color="#blue">'
      const processed = preprocessMjml(input)

      // href and padding should remain in order, color should be deduplicated
      expect(processed.indexOf('href="#"')).toBeLessThan(processed.indexOf('color="#blue"'))
      expect(processed.indexOf('color="#blue"')).toBeLessThan(processed.indexOf('padding="10px"'))
    })

    test('should handle complex real-world case with multiple duplicates', () => {
      const input = `
        <mj-section 
          background-color="#ffffff" 
          padding="20px" 
          background-color="#f0f0f0"
          border-radius="5px"
          padding="30px"
          background-color="#e0e0e0"
        >
      `
      const processed = preprocessMjml(input)

      // Should keep only last occurrence of each duplicate
      expect(processed).toContain('background-color="#e0e0e0"')
      expect(processed).toContain('padding="30px"')
      expect(processed).toContain('border-radius="5px"')
      expect(processed).not.toContain('#ffffff')
      expect(processed).not.toContain('#f0f0f0')
      expect(processed).not.toContain('padding="20px"')
    })
  })
})

describe('MJML to JSON Browser Converter', () => {
  describe('Basic Conversion', () => {
    test('should convert simple MJML to EmailBlock format', () => {
      const mjmlInput = `
        <mjml>
          <mj-body>
            <mj-section>
              <mj-column>
                <mj-text>Hello World</mj-text>
              </mj-column>
            </mj-section>
          </mj-body>
        </mjml>
      `

      const result = convertMjmlToJsonBrowser(mjmlInput)

      expect(result.type).toBe('mjml')
      expect(result.id).toBeDefined()
      expect(result.children).toBeDefined()
      expect(result.children?.length).toBe(1)

      const bodyBlock = result.children?.[0]
      expect(bodyBlock?.type).toBe('mj-body')
      expect(bodyBlock?.children?.length).toBe(1)

      const sectionBlock = bodyBlock?.children?.[0]
      expect(sectionBlock?.type).toBe('mj-section')
      expect(sectionBlock?.children?.length).toBe(1)

      const columnBlock = sectionBlock?.children?.[0]
      expect(columnBlock?.type).toBe('mj-column')
      expect(columnBlock?.children?.length).toBe(1)

      const textBlock = columnBlock?.children?.[0]
      expect(textBlock?.type).toBe('mj-text')
      // Plain text should be wrapped in <p> tags for consistency with Tiptap editor
      expect((textBlock as { content?: string })?.content).toBe('<p>Hello World</p>')
    })

    test('should handle MJML with attributes', () => {
      const mjmlInput = `
        <mjml>
          <mj-body width="600px" background-color="#ffffff">
            <mj-section padding="20px">
              <mj-column>
                <mj-text font-size="16px" color="#333333">Styled Text</mj-text>
              </mj-column>
            </mj-section>
          </mj-body>
        </mjml>
      `

      const result = convertMjmlToJsonBrowser(mjmlInput)

      const bodyBlock = result.children?.[0]
      expect((bodyBlock?.attributes as Record<string, unknown>)?.width).toBe('600px')
      expect((bodyBlock?.attributes as Record<string, unknown>)?.backgroundColor).toBe('#ffffff')

      const sectionBlock = bodyBlock?.children?.[0]
      expect((sectionBlock?.attributes as Record<string, unknown>)?.padding).toBe('20px')

      const textBlock = sectionBlock?.children?.[0]?.children?.[0]
      expect((textBlock?.attributes as Record<string, unknown>)?.fontSize).toBe('16px')
      expect((textBlock?.attributes as Record<string, unknown>)?.color).toBe('#333333')
      // Plain text should be wrapped in <p> tags for consistency with Tiptap editor
      expect((textBlock as { content?: string })?.content).toBe('<p>Styled Text</p>')
    })

    test('should handle self-closing elements', () => {
      const mjmlInput = `
        <mjml>
          <mj-body>
            <mj-section>
              <mj-column>
                <mj-spacer height="20px" />
                <mj-divider border-width="1px" border-color="#ccc" />
              </mj-column>
            </mj-section>
          </mj-body>
        </mjml>
      `

      const result = convertMjmlToJsonBrowser(mjmlInput)

      const columnBlock = result.children?.[0]?.children?.[0]?.children?.[0]
      expect(columnBlock?.children?.length).toBe(2)

      const spacerBlock = columnBlock?.children?.[0]
      expect(spacerBlock?.type).toBe('mj-spacer')
      expect((spacerBlock?.attributes as Record<string, unknown>)?.height).toBe('20px')
      expect(spacerBlock?.children).toBeUndefined()

      const dividerBlock = columnBlock?.children?.[1]
      expect(dividerBlock?.type).toBe('mj-divider')
      expect((dividerBlock?.attributes as Record<string, unknown>)?.borderWidth).toBe('1px')
      expect((dividerBlock?.attributes as Record<string, unknown>)?.borderColor).toBe('#ccc')
    })
  })

  describe('Duplicate Attribute Integration Tests', () => {
    test('should successfully convert MJML with duplicate background-color attribute', () => {
      const mjmlWithDuplicate = `
        <mjml>
          <mj-body>
            <mj-section background-color="#ffffff" padding="20px" background-color="#000000">
              <mj-column>
                <mj-text>Content</mj-text>
              </mj-column>
            </mj-section>
          </mj-body>
        </mjml>
      `

      // Should not throw - preprocessing should fix the duplicate
      const result = convertMjmlToJsonBrowser(mjmlWithDuplicate)

      expect(result.type).toBe('mjml')
      const sectionBlock = result.children?.[0]?.children?.[0]
      expect(sectionBlock?.type).toBe('mj-section')

      // Should have the last value of background-color
      expect((sectionBlock?.attributes as Record<string, unknown>)?.backgroundColor).toBe('#000000')
      expect((sectionBlock?.attributes as Record<string, unknown>)?.padding).toBe('20px')
    })

    test('should handle real-world error case from Sentry (line 291 error)', () => {
      // Simulating the error reported in Sentry
      const mjmlWithError = `
        <mjml>
          <mj-body>
            <mj-section background-color="#ffffff" background-color="#000000">
              <mj-column>
                <mj-text>This would previously fail on line with duplicate attribute</mj-text>
              </mj-column>
            </mj-section>
          </mj-body>
        </mjml>
      `

      // Should NOT throw "Attribute background-color redefined" error
      expect(() => convertMjmlToJsonBrowser(mjmlWithError)).not.toThrow()

      const result = convertMjmlToJsonBrowser(mjmlWithError)
      expect(result.type).toBe('mjml')

      const sectionBlock = result.children?.[0]?.children?.[0]
      expect((sectionBlock?.attributes as Record<string, unknown>)?.backgroundColor).toBe('#000000')
    })

    test('should combine ampersand escaping with duplicate attribute removal', () => {
      const complexMjml = `
        <mjml>
          <mj-body>
            <mj-section background-color="#fff" background-color="#000">
              <mj-column>
                <mj-image 
                  src="https://example.com/img.jpg?w=500&h=300" 
                  width="100px"
                  src="https://example.com/img2.jpg?a=1&b=2"
                  width="200px"
                />
              </mj-column>
            </mj-section>
          </mj-body>
        </mjml>
      `

      // Should handle both issues: unescaped ampersands AND duplicate attributes
      const result = convertMjmlToJsonBrowser(complexMjml)

      expect(result.type).toBe('mjml')

      const sectionBlock = result.children?.[0]?.children?.[0]
      expect((sectionBlock?.attributes as Record<string, unknown>)?.backgroundColor).toBe('#000')

      const columnBlock = sectionBlock?.children?.[0]

      const imageBlock = columnBlock?.children?.[0]
      // Should use the last src value (with properly escaped ampersands)
      expect((imageBlock?.attributes as Record<string, unknown>)?.src).toBe('https://example.com/img2.jpg?a=1&b=2')
      expect((imageBlock?.attributes as Record<string, unknown>)?.width).toBe('200px')
    })
  })

  describe('mj-text Content Normalization', () => {
    test('should wrap plain text in <p> tags', () => {
      const mjmlInput = `
        <mjml>
          <mj-body>
            <mj-section>
              <mj-column>
                <mj-text>Plain text content</mj-text>
              </mj-column>
            </mj-section>
          </mj-body>
        </mjml>
      `

      const result = convertMjmlToJsonBrowser(mjmlInput)
      const textBlock = result.children?.[0]?.children?.[0]?.children?.[0]?.children?.[0]
      expect((textBlock as { content?: string })?.content).toBe('<p>Plain text content</p>')
    })

    test('should preserve content already wrapped in HTML tags', () => {
      const mjmlInput = `
        <mjml>
          <mj-body>
            <mj-section>
              <mj-column>
                <mj-text><p>Already wrapped</p></mj-text>
              </mj-column>
            </mj-section>
          </mj-body>
        </mjml>
      `

      const result = convertMjmlToJsonBrowser(mjmlInput)
      const textBlock = result.children?.[0]?.children?.[0]?.children?.[0]?.children?.[0]
      expect((textBlock as { content?: string })?.content).toBe('<p>Already wrapped</p>')
    })

    test('should preserve complex HTML content', () => {
      const mjmlInput = `
        <mjml>
          <mj-body>
            <mj-section>
              <mj-column>
                <mj-text><p>Paragraph 1</p><p>Paragraph 2</p><strong>Bold</strong></mj-text>
              </mj-column>
            </mj-section>
          </mj-body>
        </mjml>
      `

      const result = convertMjmlToJsonBrowser(mjmlInput)
      const textBlock = result.children?.[0]?.children?.[0]?.children?.[0]?.children?.[0]
      expect((textBlock as { content?: string })?.content).toBe('<p>Paragraph 1</p><p>Paragraph 2</p><strong>Bold</strong>')
    })

    test('should not wrap mj-button content in <p> tags', () => {
      const mjmlInput = `
        <mjml>
          <mj-body>
            <mj-section>
              <mj-column>
                <mj-button>Click Me</mj-button>
              </mj-column>
            </mj-section>
          </mj-body>
        </mjml>
      `

      const result = convertMjmlToJsonBrowser(mjmlInput)
      const buttonBlock = result.children?.[0]?.children?.[0]?.children?.[0]?.children?.[0]
      expect(buttonBlock?.type).toBe('mj-button')
      // Button content should NOT be wrapped (normalization only applies to mj-text)
      expect((buttonBlock as { content?: string })?.content).toBe('Click Me')
    })
  })
})
