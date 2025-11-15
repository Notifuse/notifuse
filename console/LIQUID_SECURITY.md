# Liquid Template Security

## Overview

This document outlines the security measures implemented for Liquid template rendering in the blog theme system.

## ⚠️ Security Threats

### 1. **Code Execution**
- **Risk**: Malicious code execution on server or client
- **Mitigation**: Sandboxed Liquid engine with no access to filesystem or system

### 2. **Resource Exhaustion**
- **Risk**: Infinite loops, excessive memory usage, CPU exhaustion
- **Mitigation**: 
  - Timeout limits (5 seconds)
  - Iteration limits (10,000 max)
  - Template size limits (100KB max)

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
- **Mitigation**: Disabled `include`, `layout`, and `render` tags

## 🚫 Disabled/Restricted Features

### Tags (Disabled)
```liquid
{% include "other-file" %}   ❌ BLOCKED
{% layout "base" %}           ❌ BLOCKED  
{% render "partial" %}        ❌ BLOCKED
{% raw %}...{% endraw %}      ❌ BLOCKED (in some implementations)
```

### Why These Are Blocked
- **include/layout/render**: Could access files outside user's scope
- **raw**: Could bypass security processing

## ✅ Allowed Features

All standard Liquid features are allowed EXCEPT the disabled tags above:

### Safe Tags
```liquid
{% if condition %}...{% endif %}          ✅ SAFE
{% for item in items %}...{% endfor %}   ✅ SAFE
{% assign var = value %}                 ✅ SAFE
{% case variable %}...{% endcase %}      ✅ SAFE
{% comment %}...{% endcomment %}         ✅ SAFE
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
- [ ] No disabled tags (include/layout/render)
- [ ] Nesting depth < 20 levels
- [ ] Balanced opening/closing tags
- [ ] Valid Liquid syntax
- [ ] No suspicious patterns

## 🔧 Backend Implementation Notes

### Go Liquid Library Recommendations

**Option 1: github.com/osteele/liquid** (Recommended)
```go
engine := liquid.NewEngine()
engine.RegisterTag("include", nil)  // Disable
engine.RegisterTag("layout", nil)   // Disable  
engine.RegisterTag("render", nil)   // Disable
```

**Option 2: Use a sandboxed environment**
- Docker container with resource limits
- Read-only filesystem
- No network access
- CPU/memory limits via cgroups

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

**Last Updated**: November 15, 2024  
**Version**: 1.0  
**Owner**: Security Team

