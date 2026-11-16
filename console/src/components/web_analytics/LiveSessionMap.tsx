import { ReactNode, useMemo } from 'react'
import * as echarts from 'echarts'
import ReactECharts from 'echarts-for-react'
import { Spin } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { PRIMARY_COLOR } from './lib/types'
import { numberFormat } from '../layout'
import { colors, over } from '../../theme/tokens'
import worldGeoJson from './lib/worldGeo.json'

// Same lazy registration as CountryMapView: parsing the 240 KB collection is
// only worth it once a map is actually on screen.
let worldRegistered = false
function ensureWorldMap(): void {
  if (worldRegistered) return
  echarts.registerMap('world', worldGeoJson as Parameters<typeof echarts.registerMap>[1])
  worldRegistered = true
}

export interface LiveSessionLocation {
  latitude: number | null
  longitude: number | null
  city: string | null
  country: string | null
  sessions: number
}

/** Land in a faint primary tint, its borders and a hovered country a shade deeper. */
const LAND = over(colors.primary[500], 0.11)
const LAND_EDGE = over(colors.primary[500], 0.29)

interface LiveSessionMapProps {
  data: LiveSessionLocation[]
  loading?: boolean
}

interface TooltipParams {
  name?: string
  value?: [number, number, number]
}

/**
 * Where the visitors on the site right now are, one rippling dot per place.
 * Unlike the dashboard's choropleth this plots coordinates, so two cities in
 * the same country stay two marks — which is the point of watching live.
 */
export function LiveSessionMap(props: LiveSessionMapProps): ReactNode {
  const { t, i18n } = useLingui()
  // The options are rebuilt on a language change, for the counts of the tooltip
  const locale = i18n.locale

  const points = useMemo(
    () =>
      props.data
        .filter((row) => row.latitude != null && row.longitude != null)
        .map((row) => ({
          name: row.city ? `${row.city}, ${row.country ?? ''}`.replace(/, $/, '') : row.country || t`Unknown`,
          value: [row.longitude as number, row.latitude as number, row.sessions]
        })),
    [props.data, t]
  )

  const option = useMemo(() => {
    ensureWorldMap()
    return {
      geo: {
        map: 'world',
        roam: false,
        left: 10,
        right: 10,
        top: 10,
        bottom: 10,
        itemStyle: {
          areaColor: LAND,
          borderColor: LAND_EDGE,
          borderWidth: 0.5
        },
        emphasis: { itemStyle: { areaColor: LAND_EDGE } }
      },
      tooltip: {
        trigger: 'item',
        formatter: (params: TooltipParams) => {
          if (!params.value) return ''
          const sessions = params.value[2]
          return `${params.name}: ${numberFormat(locale, i18n).int(sessions)}`
        }
      },
      series: [
        {
          type: 'effectScatter',
          coordinateSystem: 'geo',
          data: points,
          // Small enough that a busy city doesn't swallow its neighbours, and
          // never smaller than a dot you can aim a cursor at.
          symbolSize: (value: [number, number, number]) => Math.min(20, Math.max(6, 4 + value[2] * 2)),
          showEffectOn: 'render',
          rippleEffect: { brushType: 'stroke', scale: 3, period: 3 },
          itemStyle: { color: PRIMARY_COLOR, shadowBlur: 10, shadowColor: PRIMARY_COLOR },
          zlevel: 1
        }
      ]
    }
  }, [points, locale, i18n])

  if (props.loading && props.data.length === 0) {
    return (
      <div className="flex aspect-[2/1] items-center justify-center">
        <Spin />
      </div>
    )
  }

  // Rendered even with nothing on it: an empty world map still says "nobody is
  // here right now", where an empty state would read as a broken widget.
  return (
    <div className="aspect-[2/1] overflow-hidden">
      <ReactECharts
        option={option}
        notMerge
        style={{ height: '100%', width: '100%' }}
        opts={{ renderer: 'svg' }}
      />
    </div>
  )
}
