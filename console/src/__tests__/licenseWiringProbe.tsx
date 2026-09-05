import { useLicense } from '../hooks/useLicense'

// Stands in for the router's output so the App under test renders its real provider stack
// around a consumer of useLicense. Kept in its own module because the mock factory that
// installs it is hoisted above every import.
export function LicenceProbe() {
  const { entitlements, has } = useLicense()
  return (
    <div>
      <span data-testid="tier">{entitlements?.tier ?? 'unknown'}</span>
      <span data-testid="has-rbac">{String(has('rbac'))}</span>
      <span data-testid="has-sso">{String(has('sso'))}</span>
    </div>
  )
}
