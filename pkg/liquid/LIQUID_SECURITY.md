# Liquid Template Security

## Overview

This document outlines the security measures implemented for Liquid template rendering in Notifuse.

## Architecture

Notifuse uses two different Liquid template implementations:

### Blog Themes: V8 + LiquidJS

- **Engine**: V8 JavaScript engine (`rogchap/v8go`) running LiquidJS (browser build)
- **Purpose**: Blog theme rendering (home, post, category pages)
- **Features**: Full support for `render` tag with parameters
- **Context Pool**: 10 pre-warmed V8 contexts for performance

### MJML Emails: Go Liquid

- **Engine**: `github.com/osteele/liquid` (pure Go implementation)
- **Purpose**: Email template variable interpolation
- **Features**: Basic liquid syntax without parameterized render/include

## ⚠️ Security Threats

### 1. **Code Execution**

- **Risk**: Malicious code execution on server or client
- **Mitigation**: Sandboxed Liquid engine with no access to filesystem or system

### 2. **Resource Exhaustion**

- **Risk**: Infinite loops, excessive memory usage, CPU exhaustion
- **Mitigation** (V8 + LiquidJS):
  - Render timeout: 5 seconds (`renderLimit: 5000`)
  - Template size limit: 100KB (`parseLimit: 102400`)
  - Memory limit: 10MB (`memoryLimit: 10485760`)
  - Context pool: Max 10 concurrent renders

### 3. **Cross-Site Scripting (XSS)**

- **Risk**: Injecting malicious JavaScript into rendered pages
- **Mitigation**:
  - HTML escaping (note: Liquid doesn't auto-escape by default)
  - Content Security Policy headers
  - Input validation

### 4. **Server-Side Request Forgery (SSRF)**

- **Risk**: Making unauthorized network requests
- **Mitigation**: No network access from templates

### 5. **File System Access**

- **Risk**: Reading sensitive files or including unauthorized templates
- **Mitigation**: Disabled `layout` and `render` tags; `include` tag is allowed for template reusability

## 🚫 Disabled/Restricted Features

### Blog Themes (V8 + LiquidJS)

```liquid
{% render "partial", param: value %}   ✅ ALLOWED (with parameters)
{% include "other-file" %}             ✅ ALLOWED (basic syntax)
{% layout "base" %}                    ❌ BLOCKED (no file system access)
{% raw %}...{% endraw %}               ✅ ALLOWED (safe in V8 sandbox)
```

### MJML Emails (Go Liquid)

```liquid
{% render "partial" %}                 ❌ NOT SUPPORTED (library limitation)
{% include "other-file" %}             ✅ ALLOWED (registered partials only)
{% layout "base" %}                    ❌ BLOCKED
```

### Why These Are Blocked

- **layout**: Could access files outside user's scope; not needed for single-page rendering
- File system access is controlled through custom file system implementations

## ✅ Allowed Features

All standard Liquid features are allowed EXCEPT the disabled tags above:

### Safe Tags

```liquid
{% if condition %}...{% endif %}          ✅ SAFE
{% for item in items %}...{% endfor %}   ✅ SAFE
{% assign var = value %}                 ✅ SAFE
{% case variable %}...{% endcase %}      ✅ SAFE
{% comment %}...{% endcomment %}         ✅ SAFE
{% include "template-name" %}            ✅ SAFE (template reusability)
```

### Safe Filters

```liquid
{{ text | upcase }}                      ✅ SAFE
{{ text | downcase }}                    ✅ SAFE
{{ array | join: ', ' }}                 ✅ SAFE
{{ number | plus: 5 }}                   ✅ SAFE
{{ date | date: "%Y-%m-%d" }}           ✅ SAFE
{{ text | strip_html }}                  ✅ SAFE
{{ text | escape }}                      ✅ SAFE (recommended!)
```

## 🔒 Security Limits

### Frontend (liquidjs)

```javascript
{
  maxTemplateSize: 100000,      // 100KB per template
  maxExecutionTime: 5000,       // 5 seconds
  maxIterations: 10000,         // 10k loop iterations
  maxNestingDepth: 20,          // 20 levels of nesting
  strictFilters: true,          // Throw on undefined filters
  strictVariables: false,       // Graceful on undefined vars
}
```

### Backend (Go implementation)

```go
// Recommended limits for production
MaxTemplateSize:    100 * 1024  // 100KB
MaxExecutionTime:   5 * time.Second
MaxLoopIterations:  10000
MaxRecursionDepth:  20
```

## 🛡️ Security Best Practices

### 1. **Escape User Content**

```liquid
❌ BAD:  {{ user_input }}
✅ GOOD: {{ user_input | escape }}
```

### 2. **Validate Before Save**

All templates are validated before saving:

- Syntax check
- Security scan for disabled tags
- Size limit check
- Nesting depth check

### 3. **Content Security Policy**

When serving blog pages, use strict CSP headers:

```
Content-Security-Policy:
  default-src 'self';
  script-src 'self';
  style-src 'self' 'unsafe-inline';
  img-src 'self' https:;
  font-src 'self';
```

### 4. **Rate Limiting**

Implement rate limits:

- Template compilation: 10 per minute per workspace
- Preview requests: 30 per minute per user
- Publish operations: 5 per hour per workspace

### 5. **Audit Logging**

Log all template operations:

- Template creation/updates (who, when, what changed)
- Template publications
- Template errors/timeouts
- Security validation failures

### 6. **Version Control**

- Keep history of all template versions
- Allow rollback to previous safe versions
- Lock published templates (read-only)

## 📋 Pre-Save Validation Checklist

Before saving a template, we validate:

- [ ] Template size < 100KB
- [ ] No disabled tags (layout/render)
- [ ] Nesting depth < 20 levels
- [ ] Balanced opening/closing tags
- [ ] Valid Liquid syntax
- [ ] No suspicious patterns

## 🔧 Backend Implementation Notes

### Blog Themes: V8 + LiquidJS

**Architecture**:

- V8 JavaScript engine (`rogchap/v8go`)
- LiquidJS browser UMD build (~305KB embedded)
- Context pool of 10 pre-warmed V8 contexts
- Custom file system for in-memory partials

**Code**:

```go
// V8 context pool (lazy initialization)
pool := liquid.GetPool()
ctx := pool.Acquire()
defer pool.Release(ctx)

// Render with partials
html, err := liquid.RenderBlogTemplate(template, data, partials)
```

**Security**:

- V8 sandboxing: No file system, no network access
- Custom file system: Only registered partials accessible
- Resource limits: Managed by V8 isolate
- Template validation: Syntax checked before rendering

**Build Process**:

1. `cd console && npm install` - Install console dependencies (includes liquidjs)
2. `npm run bundle-liquid` - Copy browser UMD build from console to `../pkg/liquid/`
3. `go build` - Embeds bundle via `//go:embed` directive

Note: liquidjs is already a dependency of the console, so no separate package.json is needed.

### MJML Emails: github.com/osteele/liquid

**Code**:

```go
engine := liquid.NewEngine()
// Register partials
engine.RegisterTemplateStore(store)
html, err := engine.ParseAndRenderString(template, data)
```

**Security**:

- Pure Go implementation
- No file system access (memory-only partials)
- Basic liquid syntax only

### Resource Limits

**V8 + LiquidJS Implementation**:

1. **Render Timeout**: 5 seconds (`renderLimit: 5000` ms)
   - Prevents infinite loops and excessive computation
   - Enforced by LiquidJS during template rendering
   - ✅ Test coverage: Verified with nested million-iteration loops

2. **Template Size Limit**: 100KB (`parseLimit: 102400` bytes)
   - Prevents DoS attacks via large template uploads
   - Enforced during template parsing phase
   - ✅ Test coverage: Verified with 200KB templates

3. **Memory Limit**: 10MB (`memoryLimit: 10485760` bytes)
   - Prevents excessive memory consumption
   - Enforced by LiquidJS during parsing/rendering
   - Protects against memory exhaustion attacks

4. **Context Pool**: 10 pre-warmed contexts (pool-level limit)
   - Max 10 concurrent template renders
   - Prevents unbounded resource growth
   - Contexts reused for performance (no initialization overhead)

5. **Bundle Size**: ~305KB (one-time cost)
   - LiquidJS browser UMD build embedded in Go binary
   - Loaded once per V8 context at initialization
   - Shared across all renders in same context

### Template Execution

```go
// Pseudocode for secure execution
func renderTemplate(template string, data map[string]interface{}) (string, error) {
    // 1. Validate security
    if err := validateTemplateSecurity(template); err != nil {
        return "", err
    }

    // 2. Parse with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // 3. Render in goroutine with panic recovery
    resultChan := make(chan string, 1)
    errorChan := make(chan error, 1)

    go func() {
        defer func() {
            if r := recover(); r != nil {
                errorChan <- fmt.Errorf("template panic: %v", r)
            }
        }()

        result, err := engine.ParseAndRender(template, data)
        if err != nil {
            errorChan <- err
        } else {
            resultChan <- result
        }
    }()

    // 4. Wait for result or timeout
    select {
    case result := <-resultChan:
        return result, nil
    case err := <-errorChan:
        return "", err
    case <-ctx.Done():
        return "", errors.New("template execution timeout")
    }
}
```

## 🚨 Incident Response

If a security issue is detected:

1. **Immediate Actions**
   - Disable affected template(s)
   - Roll back to last known good version
   - Notify workspace owner

2. **Investigation**
   - Review audit logs
   - Check for similar patterns in other templates
   - Assess impact scope

3. **Remediation**
   - Fix security issue
   - Update validation rules
   - Add regression test

4. **Communication**
   - Notify affected users
   - Document incident
   - Update security guidelines

## 📚 Additional Resources

- [Liquid Documentation](https://shopify.github.io/liquid/)
- [liquidjs GitHub](https://github.com/harttle/liquidjs)
- [OWASP Template Injection](https://owasp.org/www-project-web-security-testing-guide/latest/4-Web_Application_Security_Testing/07-Input_Validation_Testing/18-Testing_for_Server-side_Template_Injection)

## ✅ Security Checklist for Deployment

- [ ] All disabled tags are blocked
- [ ] Timeout protection is active (5s max)
- [ ] Template size limits enforced (100KB max)
- [ ] Iteration limits in place (10k max)
- [ ] Security validation runs on every template save
- [ ] CSP headers configured for blog serving
- [ ] Rate limiting implemented
- [ ] Audit logging enabled
- [ ] Template version control active
- [ ] Incident response plan documented
- [ ] Security testing completed
- [ ] Penetration testing performed

---

**Last Updated**: November 18, 2024  
**Version**: 2.0 (V8 + LiquidJS for blog themes)  
**Owner**: Security Team
