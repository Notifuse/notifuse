import { ReactNode, useMemo } from 'react'
import * as echarts from 'echarts'
import ReactECharts from 'echarts-for-react'
import { Empty, Spin } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { formatValue } from './lib/format'
import { ISO2_TO_ISO3, ISO3_TO_ISO2, countryName } from './lib/isoCountries'
import { PRIMARY_COLOR } from './lib/types'
import worldGeoJson from './lib/worldGeo.json'

interface GeoFeature {
  id: string
  properties: { name: string }
}

const features = (worldGeoJson as { features: GeoFeature[] }).features

// ECharts matches map data by feature name, not by the alpha-3 feature id, so
// both directions of that translation are needed: one to place a value on the
// map, one to turn a click back into the country code the filters speak.
const ISO3_TO_GEO_NAME: Record<string, string> = {}
const GEO_NAME_TO_ISO3: Record<string, string> = {}
for (const feature of features) {
  if (!feature.id || !feature.properties?.name) continue
  ISO3_TO_GEO_NAME[feature.id] = feature.properties.name
  GEO_NAME_TO_ISO3[feature.properties.name] = feature.id
}

// Registering parses the whole 240 KB collection, so it waits until a map is
// actually rendered rather than running for every page that pulls in a table.
let worldRegistered = false
function ensureWorldMap(): void {
  if (worldRegistered) return
  echarts.registerMap('world', worldGeoJson as Parameters<typeof echarts.registerMap>[1])
  worldRegistered = true
}

export interface CountryMapDatum {
  dimension_value: string
  [metric: string]: string | number | undefined
}

interface CountryMapViewProps {
  data: CountryMapDatum[]
  /** Measure the choropleth is shaded by, e.g. `sessions`. */
  metric: string
  loading?: boolean
  /** Receives the ISO 3166-1 alpha-2 code of the country that was clicked. */
  onSelect?: (iso2: string) => void
}

interface TooltipParams {
  name?: string
  data?: { value?: number }
}

export function CountryMapView(props: CountryMapViewProps): ReactNode {
  const { data, metric, loading, onSelect } = props
  const { t } = useLingui()

  ensureWorldMap()

  const metricLabel = useMemo(() => {
    const labels: Record<string, string> = {
      sessions: t`Sessions`,
      users: t`Users`,
      pageviews: t`Pageviews`,
      goals: t`Goals`,
      goal_conversions: t`Conversions`
    }
    return labels[metric] ?? metric
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `t` is a new function on every render
  }, [metric])

  const mapData = useMemo(
    () =>
      data
        .map((row) => {
          const iso2 = String(row.dimension_value ?? '').toUpperCase()
          const geoName = ISO3_TO_GEO_NAME[ISO2_TO_ISO3[iso2] ?? iso2]
          // Micro-states and territories have no feature to shade; they still
          // appear in the country table next to the map.
          if (!geoName) return null
          const value = row[metric]
          return { name: geoName, value: typeof value === 'number' ? value : Number(value) || 0 }
        })
        .filter((entry): entry is { name: string; value: number } => entry !== null),
    [data, metric]
  )

  const maxValue = useMemo(
    () => mapData.reduce((max, entry) => Math.max(max, entry.value), 0) || 1,
    [mapData]
  )

  const option = useMemo(
    () => ({
      tooltip: {
        trigger: 'item',
        formatter: (params: TooltipParams) => {
          const iso3 = params.name ? GEO_NAME_TO_ISO3[params.name] : undefined
          const label = iso3 ? countryName(iso3) : (params.name ?? '')
          const value = params.data?.value
          if (value === undefined || value === null || Number.isNaN(value)) return label
          return `<div style="font-weight:500">${label}</div><div>${metricLabel}: ${formatValue(
            value,
            'number'
          )}</div>`
        }
      },
      geo: {
        map: 'world',
        roam: false,
        left: 10,
        right: 10,
        top: 10,
        bottom: 10,
        itemStyle: {
          areaColor: '#f9fafb',
          borderColor: '#e5e7eb',
          borderWidth: 0.5
        },
        emphasis: {
          itemStyle: { areaColor: PRIMARY_COLOR },
          label: { show: true, fontSize: 10 }
        }
      },
      visualMap: {
        min: 0,
        max: maxValue,
        show: false,
        inRange: {
          color: [
            'rgba(119, 99, 241, 0.2)',
            'rgba(119, 99, 241, 0.4)',
            'rgba(119, 99, 241, 0.6)',
            'rgba(119, 99, 241, 0.8)',
            'rgba(119, 99, 241, 1)'
          ]
        }
      },
      series: [{ name: 'country', type: 'map', geoIndex: 0, data: mapData }]
    }),
    [mapData, maxValue, metricLabel]
  )

  const onEvents = useMemo(() => {
    if (!onSelect) return undefined
    return {
      click: (params: { name?: string }) => {
        const iso3 = params.name ? GEO_NAME_TO_ISO3[params.name] : undefined
        const iso2 = iso3 ? ISO3_TO_ISO2[iso3] : undefined
        if (iso2) onSelect(iso2)
      }
    }
  }, [onSelect])

  if (loading && data.length === 0) {
    return (
      <div className="flex items-center justify-center aspect-[2/1]">
        <Spin />
      </div>
    )
  }

  if (data.length === 0) {
    return (
      <Empty description={t`No country data`} image={Empty.PRESENTED_IMAGE_SIMPLE} className="py-8" />
    )
  }

  return (
    <div className="aspect-[2/1]">
      <ReactECharts
        option={option}
        style={{ height: '100%', width: '100%', cursor: onSelect ? 'pointer' : 'default' }}
        opts={{ renderer: 'svg' }}
        onEvents={onEvents}
      />
    </div>
  )
}
