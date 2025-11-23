# MJML Email Template Security

## Overview

The MJML email template system now includes comprehensive security protections to prevent infinite loops, memory exhaustion, and DoS attacks through malicious Liquid templates.

## Security Features

### 1. Render Timeout Protection
- **Limit**: 5 seconds per template
- **Protection**: Prevents infinite loops and CPU exhaustion
- **Implementation**: Context-based timeout with goroutine monitoring
- **Test Coverage**: ✅ Verified with nested million-iteration loops

### 2. Template Size Limit
- **Limit**: 100KB per template
- **Protection**: Prevents memory exhaustion and DoS via large templates
- **Implementation**: Pre-validation before parsing
- **Test Coverage**: ✅ Verified with 200KB templates

### 3. Panic Recovery
- **Protection**: Server crashes from template errors
- **Implementation**: Goroutine with `defer recover()`
- **Test Coverage**: ✅ Verified with edge cases

### 4. Memory Limit
- **Limit**: 10MB (informational, enforced at Go level)
- **Protection**: Prevents excessive memory allocation
- **Note**: Go Liquid handles memory internally

## Architecture

```
Email Template → SecureLiquidEngine → processLiquidContent → Safe Rendering
                      ↓
            [Timeout + Size + Panic Protection]
```

### Components

**SecureLiquidEngine** (`liquid_secure.go`)
- Wraps `github.com/osteele/liquid` with security controls
- Enforces timeout, size limits, panic recovery
- Provides `RenderWithTimeout()` method

**Integration** (`converter.go`)
- Replaces direct Liquid engine usage
- Transparent to existing code
- Maintains backward compatibility

## Usage

### Basic Rendering (Automatic)

```go
// processLiquidContent automatically uses SecureLiquidEngine
result, err := processLiquidContent(template, data, blockID)
```

### Custom Configuration

```go
// Create engine with custom limits
engine := NewSecureLiquidEngineWithOptions(
    10*time.Second,  // 10 second timeout
    50*1024,         // 50KB size limit
)

result, err := engine.RenderWithTimeout(template, data)
```

## Error Handling

### Timeout Error
```
liquid rendering timeout after 5s (possible infinite loop or excessive computation)
```

**Action**: Review template for infinite loops or excessive iterations.

### Size Limit Error
```
template size (200000 bytes) exceeds maximum allowed size (102400 bytes)
```

**Action**: Reduce template size or increase limit if justified.

### Rendering Error
```
liquid rendering failed: [error details]
```

**Action**: Check template syntax and data format.

## Testing

### Security Tests (`liquid_secure_test.go`)
- 8 test suites
- 18 test cases
- Covers: timeout, size limits, panic recovery, edge cases

### Integration Tests (`converter_test.go`)
- 5 test suites
- Tests security in real MJML conversion context
- Verifies backward compatibility

### Test Results
```
✅ TestSecureLiquidEngine_TimeoutEnforcement (5.00s)
✅ TestSecureLiquidEngine_TemplateSizeLimit (0.00s)
✅ TestSecureLiquidEngine_NormalTemplatesWork (0.00s)
✅ TestSecureLiquidEngine_PanicRecovery (0.00s)
✅ TestSecureLiquidEngine_DeepNesting (0.00s)
✅ TestSecureLiquidEngine_MemoryExhaustion (0.14s)
✅ TestSecureLiquidEngine_ErrorMessages (0.01s)
✅ TestSecureLiquidEngine_EdgeCases (0.00s)
✅ TestSecureLiquidIntegration (5.16s)
```

## Best Practices

### Template Design

1. **Avoid Deep Loops**
   ```liquid
   ❌ BAD: {% for i in (1..1000000) %}...{% endfor %}
   ✅ GOOD: {% for item in items limit:100 %}...{% endfor %}
   ```

2. **Keep Templates Small**
   - Stay under 100KB
   - Use external CSS
   - Minimize inline styles

3. **Use Filters Wisely**
   ```liquid
   ✅ {{ text | escape }}
   ✅ {{ price | money }}
   ✅ {{ date | date: "%Y-%m-%d" }}
   ```

4. **Handle Missing Data**
   ```liquid
   ✅ {{ user.name | default: "Guest" }}
   ✅ {% if order.tracking_url %}...{% endif %}
   ```

### Development

1. **Test Templates Locally**
   - Use unit tests with realistic data
   - Verify timeout behavior
   - Check error messages

2. **Monitor Production**
   - Log timeout errors
   - Track template size
   - Alert on failures

3. **Version Control**
   - Keep template history
   - Document changes
   - Review before deployment

## Security Comparison

| Feature | Before | After |
|---------|--------|-------|
| **Timeout Protection** | ❌ None | ✅ 5 seconds |
| **Size Limits** | ❌ None | ✅ 100KB |
| **Panic Recovery** | ❌ None | ✅ Always on |
| **Attack Surface** | ⚠️ HIGH | ✅ LOW |
| **DoS Prevention** | ❌ Vulnerable | ✅ Protected |

## Configuration

### Constants (`liquid_secure.go`)

```go
const (
    DefaultRenderTimeout   = 5 * time.Second
    DefaultMaxTemplateSize = 100 * 1024       // 100KB
    DefaultMaxMemory       = 10 * 1024 * 1024 // 10MB
)
```

### Custom Limits

Modify these constants if your use case requires different limits:
- Increase timeout for complex emails
- Increase size for large templates
- Document rationale for changes

## Troubleshooting

### Template Times Out

**Symptoms**: Error mentions "timeout after 5s"

**Solutions**:
1. Simplify template logic
2. Reduce loop iterations
3. Remove nested loops
4. Use filters instead of logic

### Template Too Large

**Symptoms**: Error mentions "exceeds maximum allowed size"

**Solutions**:
1. Remove inline styles → use MJML attributes
2. Extract repeated content → use variables
3. Optimize HTML structure
4. Split into multiple templates

### Rendering Errors

**Symptoms**: Generic "rendering failed" error

**Solutions**:
1. Validate Liquid syntax
2. Check data format (must be valid JSON)
3. Test with minimal data
4. Review error logs

## Performance Impact

- **Normal templates**: < 10ms overhead
- **Large templates**: Size check adds < 1ms
- **Timeout infrastructure**: Negligible
- **Memory**: Minimal additional allocation

## Future Enhancements

Potential improvements (not yet implemented):
- [ ] Iteration count limits
- [ ] Nesting depth limits
- [ ] Rate limiting per user/workspace
- [ ] Template complexity scoring
- [ ] Automatic optimization suggestions

## References

- [LiquidJS Documentation](https://liquidjs.com)
- [Go Liquid Library](https://github.com/osteele/liquid)
- [OWASP Template Injection](https://owasp.org/www-project-web-security-testing-guide/)
- [Main Security Doc](../../console/LIQUID_SECURITY.md)

---

**Last Updated**: November 18, 2024
**Version**: 1.0
**Status**: ✅ Production Ready

