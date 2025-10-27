# Notifuse Implementation Plans

This directory contains implementation plans for features and architectural changes.

## Current Plans

### ✅ Active Implementation Plans

1. **[connection-manager-implementation.md](./connection-manager-implementation.md)** - **MAIN PLAN**
   - **Status:** Ready for implementation
   - **Purpose:** Solve "too many connections" errors with shared connection pool architecture
   - **Key feature:** Support unlimited workspaces with fixed 100 connection limit
   - **Approach:** Small pools (2-3 connections) per workspace database with LRU eviction
   
### 📚 Supporting Documentation

2. **[connection-pooling-vs-per-query.md](./connection-pooling-vs-per-query.md)**
   - **Purpose:** Technical analysis comparing connection pooling vs per-query connections
   - **Key finding:** Connection pooling is 10-100x faster than creating connections per query
   - **Includes:** Benchmarks, load test results, cost analysis

### 🗄️ Archived Plans

3. **[connection-manager-singleton-OLD.md](./connection-manager-singleton-OLD.md)**
   - **Status:** Superseded by connection-manager-implementation.md
   - **Why archived:** Original approach didn't scale (reserved too many connections per workspace)
   - **Keep for:** Historical reference and to understand evolution of solution

## Plan Status Legend

- ✅ **Active** - Ready for or currently being implemented
- 📚 **Documentation** - Supporting technical analysis
- 🗄️ **Archived** - Superseded or historical reference

## How to Use Plans

1. **For implementation:** Use the active plan (connection-manager-implementation.md)
2. **For understanding:** Read the supporting documentation
3. **For history:** Review archived plans to see what changed and why

## Plan Creation Guidelines

When creating new plans:
1. Use descriptive kebab-case filenames (e.g., `feature-name-plan.md`)
2. Include clear status at the top (active, draft, archived)
3. Update this README when adding new plans
4. Archive old plans instead of deleting them
