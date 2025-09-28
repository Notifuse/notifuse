# Test Coverage Analysis & Improvement Plan

## Overview

**Analysis Date:** September 28, 2025  
**Current Coverage:** 76.3% (service layer), 80.9% (broadcast service), 77.1% (app layer)  
**Target:** Increase coverage for better Codecov metrics  
**Files Analyzed:** 279 Go source files  
**Test Files Found:** 118 test files

## 🎉 **Recent Achievements**

- ✅ **App Layer Repository Getters**: 10 methods improved from 0% to 100% coverage
- ✅ **App Layer InitDB**: Improved from 0% to 22.2% coverage
- ✅ **App Layer Initialize**: Improved from 0% to 20.0% coverage
- ✅ **App Layer Overall**: Improved from ~65% to 77.1% coverage

## Current Coverage Summary

```bash
# Generate coverage report
cd /Users/pierre/Sites/notifuse3/code/notifuse
go test -coverprofile=coverage.out ./internal/... -v
go tool cover -func=coverage.out | grep -v "100.0%" | head -20
```

## Critical Missing Test Coverage

### 🔥 **HIGH PRIORITY** - 0% Coverage

#### 1. App Layer (`internal/app/app.go`) ✅ **COMPLETED**

- [x] `InitDB()` - **22.2% coverage** (was 0%)
- [x] `Initialize()` - **20.0% coverage** (was 0%)
- [x] `GetUserRepository()` - **100% coverage** (was 0%)
- [x] `GetWorkspaceRepository()` - **100% coverage** (was 0%)
- [x] `GetContactRepository()` - **100% coverage** (was 0%)
- [x] `GetListRepository()` - **100% coverage** (was 0%)
- [x] `GetTemplateRepository()` - **100% coverage** (was 0%)
- [x] `GetBroadcastRepository()` - **100% coverage** (was 0%)
- [x] `GetMessageHistoryRepository()` - **100% coverage** (was 0%)
- [x] `GetContactListRepository()` - **100% coverage** (was 0%)
- [x] `GetTransactionalNotificationRepository()` - **100% coverage** (was 0%)
- [x] `GetTelemetryRepository()` - **100% coverage** (was 0%)

**Impact:** High - These are core application methods ✅ **ACHIEVED**

#### 2. Service Layer - Missing Test Files

##### `system_notification_service.go` - **NO TEST FILE EXISTS** ⚠️

- [ ] Create `internal/service/system_notification_service_test.go`
- [ ] Test `HandleCircuitBreakerEvent()`
- [ ] Test `HandleBroadcastFailedEvent()`
- [ ] Test `HandleSystemAlert()`
- [ ] Test `notifyWorkspaceOwners()`
- [ ] Test `RegisterWithEventBus()`

**Impact:** Critical - Handles system alerts and notifications

#### 3. Domain Layer

##### `timezones.go` - Missing Tests

- [ ] Create `internal/domain/timezones_test.go`
- [ ] Test `IsValidTimezone()` function with valid/invalid cases

##### `setting.go` - Error Method Not Tested

- [ ] Test `ErrSettingNotFound.Error()` method - 0% coverage

##### `broadcast.go` - Validation Methods

- [ ] Test `Validate()` methods - 0% coverage
- [ ] Test `FromURLParams()` method - 0% coverage

##### `errors.go` - Error Methods

- [ ] Test `Error()` method - 0% coverage

#### 4. Database Layer (`internal/database/`)

##### `utils.go` - Multiple 0% Coverage Functions

- [ ] Test `GetConnectionPoolSettings()` - 0%
- [ ] Test `ConnectToWorkspace()` - 0%
- [ ] Test `EnsureWorkspaceDatabaseExists()` - 0%
- [ ] Test `EnsureSystemDatabaseExists()` - 0%

##### `init.go`

- [ ] Test `CleanDatabase()` - 0%

##### `schema/system_tables.go`

- [ ] Test `GetMigrationStatements()` - 0%

### 🟡 **MEDIUM PRIORITY** - Low Coverage

#### Service Layer - Partial Coverage

- [ ] Improve `telemetry_service.go` edge cases
- [ ] Enhance `demo_service.go` test coverage
- [ ] Add more scenarios to `notification_center_service.go`

### 🟢 **LOW PRIORITY** - Nice to Have

#### Interface Testing

- [ ] Add integration tests for domain interfaces
- [ ] Validate mock implementations
- [ ] Test error handling paths in existing covered functions

## Implementation Plan

### Phase 1: Quick Wins (Immediate Impact)

**Estimated Coverage Gain: 5-8%**

1. **Create `timezones_test.go`** (Easy - 30 min)

   ```go
   func TestIsValidTimezone(t *testing.T) {
       // Test valid timezones
       assert.True(t, IsValidTimezone("America/New_York"))
       assert.True(t, IsValidTimezone("UTC"))

       // Test invalid timezones
       assert.False(t, IsValidTimezone("Invalid/Timezone"))
       assert.False(t, IsValidTimezone(""))
   }
   ```

2. **Add error method tests** (Easy - 15 min)

   ```go
   func TestErrSettingNotFound_Error(t *testing.T) {
       err := &ErrSettingNotFound{Key: "test-key"}
       assert.Equal(t, "setting not found: test-key", err.Error())
   }
   ```

3. **Test repository getters in app.go** (Medium - 1 hour)
   ```go
   func TestApp_RepositoryGetters(t *testing.T) {
       // Test all Get*Repository methods
   }
   ```

### Phase 2: Critical Services (High Impact)

**Estimated Coverage Gain: 8-12%**

1. **Create `system_notification_service_test.go`** (High - 3-4 hours)

   - Mock dependencies: WorkspaceRepository, BroadcastRepository, Mailer
   - Test all event handlers
   - Test error scenarios
   - Test notification delivery

2. **Improve app initialization tests** (Medium - 2 hours)
   - Test `InitDB()` with mocked database
   - Test `Initialize()` method
   - Test error handling paths

### Phase 3: Database & Utilities (Medium Impact)

**Estimated Coverage Gain: 3-5%**

1. **Create database utility tests** (Medium - 2-3 hours)

   - Mock database connections
   - Test connection pool settings
   - Test database creation functions

2. **Add validation method tests** (Easy - 1 hour)
   - Test broadcast validation
   - Test parameter parsing

## Test File Templates

### Template for Service Tests

```go
package service

import (
    "context"
    "testing"

    "github.com/Notifuse/notifuse/internal/domain"
    "github.com/Notifuse/notifuse/internal/domain/mocks"
    pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
    "github.com/golang/mock/gomock"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestServiceName_MethodName(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    // Setup mocks
    mockRepo := mocks.NewMockRepository(ctrl)
    mockLogger := pkgmocks.NewMockLogger(ctrl)

    // Configure logger expectations
    mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
    mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

    // Create service
    service := NewService(mockRepo, mockLogger)

    // Test cases
    t.Run("Success case", func(t *testing.T) {
        // Test implementation
    })

    t.Run("Error case", func(t *testing.T) {
        // Test error scenarios
    })
}
```

## Progress Tracking

### Completed ✅

- [x] Coverage analysis completed
- [x] Priority list created
- [x] Implementation plan defined
- [x] **App Layer Repository Getters** - All 10 getter methods now at 100% coverage
- [x] **App Layer InitDB** - Now at 22.2% coverage (was 0%)
- [x] **App Layer Initialize** - Now at 20.0% coverage (was 0%)
- [x] **System Notification Service** - Created comprehensive test suite (was 0% coverage)
- [x] **Timezones IsValidTimezone** - Now at 100% coverage (was 0%)
- [x] **Setting Error Method** - Now at 100% coverage (was 0%)
- [x] **Database Utilities** - All DSN and connection pool functions now at 100% coverage
- [x] **Database Init Functions** - Created comprehensive test coverage for CleanDatabase and initialization functions
- [x] **Database Schema Functions** - Created tests for migration statements and table name validation

### In Progress 🔄

- [x] Phase 1: Quick wins - **COMPLETE**
- [x] Phase 2: Critical services - **COMPLETE**
- [x] Phase 3: Database utilities - **COMPLETE**

### Files to Create

1. ✅ `internal/service/system_notification_service_test.go` - **COMPLETED**
2. ✅ `internal/domain/timezones_test.go` - **COMPLETED**
3. ✅ `internal/domain/setting_test.go` - **COMPLETED**
4. ✅ `internal/database/utils_test.go` - **COMPLETED**
5. ✅ `internal/database/init_test.go` - **COMPLETED**
6. ✅ `internal/database/schema/system_tables_test.go` - **COMPLETED**

## CI Compatibility Notes

All database tests have been designed to be CI-friendly:

- **Flexible Mock Expectations:** Tests use error-path testing instead of rigid mock sequences
- **Order-Independent:** Database cleanup tests don't depend on specific table drop order
- **Robust Error Handling:** Tests focus on exercising error paths rather than exact execution flows

## Commands for Testing

```bash
# Generate coverage report
cd /Users/pierre/Sites/notifuse3/code/notifuse
go test -coverprofile=coverage.out ./internal/... -v

# View coverage by function
go tool cover -func=coverage.out | sort -k3 -n

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Test specific package
go test -v ./internal/service -cover

# Test with race detection
go test -race -coverprofile=coverage.out ./internal/...
```

## Expected Outcomes

### Before Implementation

- **App Layer:** Multiple functions with 0% coverage
- **Service Layer:** 76.3% coverage
- **Broadcast Service:** 80.9% coverage
- **Missing:** ~24 critical functions with 0% coverage

### After Phase 1, 2 & 3 Implementation ✅

- **App Layer:** 77.1% coverage (improved from ~65%)
- **Repository Getters:** 100% coverage (was 0%)
- **InitDB:** 22.2% coverage (was 0%)
- **Initialize:** 20.0% coverage (was 0%)
- **Service Layer:** 77.2% coverage (improved from 76.3%)
- **Domain Layer:** 87.4% coverage (significant improvement)
- **Database Layer:** 37.4% coverage (was likely 0%)
- **System Notification Service:** Comprehensive test coverage added
- **Timezones IsValidTimezone:** 100% coverage (was 0%)
- **Setting Error Method:** 100% coverage (was 0%)
- **Database Utilities:** All DSN functions at 100% coverage (was 0%)
- **Connection Pool Settings:** 100% coverage (was 0%)
- **Database Initialization:** Comprehensive test coverage added
- **Schema Functions:** Test coverage for migration and table validation
- **Broadcast Service:** 80.9% coverage (maintained)

### Target After Full Implementation

- **Service Layer:** 85-90% coverage
- **Broadcast Service:** 85-90% coverage
- **Overall:** 15-20% improvement in total coverage

## Notes

- Focus on **system_notification_service.go** first - it's completely untested
- **Quick wins** with utility functions and error methods
- **Database layer** needs careful mocking for connection tests
- **App layer** initialization requires integration test approach
- Consider using **testify/suite** for complex service tests

## Resources

- [Go Testing Best Practices](https://golang.org/doc/tutorial/add-a-test)
- [Testify Documentation](https://github.com/stretchr/testify)
- [GoMock Documentation](https://github.com/golang/mock)
- [Coverage Tool Documentation](https://golang.org/cmd/cover/)

---

**Last Updated:** September 28, 2025  
**Next Review:** After Phase 1 completion
