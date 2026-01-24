import React from 'react'
import { Table, Spin, Alert } from 'antd'
import { useLingui } from '@lingui/react/macro'
import * as echarts from 'echarts/core'
import { LineChart, BarChart, PieChart } from 'echarts/charts'
import {
  TitleComponent,
  TooltipComponent,
  GridComponent,
  LegendComponent
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import ReactEChartsCore from 'echarts-for-react/lib/core'
import { AnalyticsQuery, AnalyticsResponse } from '../../services/api/analytics'

// Register the required components
echarts.use([
  LineChart,
  BarChart,
  PieChart,
  TitleComponent,
  TooltipComponent,
  GridComponent,
  LegendComponent,
  CanvasRenderer
])

interface ChartVisualizationProps {
  data: AnalyticsResponse | null
  chartType: 'line' | 'bar' | 'pie' | 'table'
  query: AnalyticsQuery
  loading?: boolean
  error?: string | null
  height?: number
  showLegend?: boolean
  colors?: Record<string, string>
  measureTitles?: Record<string, string>
}

export const ChartVisualization: React.FC<ChartVisualizationProps> = ({
  data,
  chartType,
  query,
  loading = false,
  error = null,
  height = 300,
  showLegend = true,
  colors = {},
  measureTitles = {}
}) => {
  const { t } = useLingui()
  // Helper function to format dates from ISO format to readable format
  const formatDate = (dateString: string) => {
    if (!dateString) return dateString
    try {
      const date = new Date(dateString)
      return date.toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric',
        year: date.getFullYear() !== new Date().getFullYear() ? 'numeric' : undefined
      })
    } catch {
      return dateString
    }
  }

  const getChartOption = () => {
    if (!data || !data.data.length) return {}

    const chartData = data.data

    switch (chartType) {
      case 'line':
        return getLineChartOption(chartData, query)
      case 'bar':
        return getBarChartOption(chartData, query)
      case 'pie':
        return getPieChartOption(chartData, query)
      default:
        return {}
    }
  }

  const getLineChartOption = (chartData: Record<string, unknown>[], query: AnalyticsQuery) => {
    // Handle time dimension data
    const timeDimension = query.timeDimensions?.[0]?.dimension
    const measures = query.measures
    const dimensions = query.dimensions

    if (timeDimension) {
      // Time series chart
      // Try different possible field names for time dimension
      const granularity = query.timeDimensions?.[0]?.granularity
      const timeField = granularity ? `${timeDimension}_${granularity}` : timeDimension
      const xAxisData = chartData.map((item) => {
        const rawDate = item[timeField] || item[timeDimension] || item.created_at
        return formatDate(String(rawDate))
      })
      const series = measures.map((measure) => ({
        name: measure,
        type: 'line',
        data: chartData.map((item) => item[measure] || 0),
        smooth: true,
        symbol: 'none', // Hide dots by default
        symbolSize: 6,
        emphasis: {
          focus: 'series',
          symbol: 'circle', // Show dots on hover
          symbolSize: 8
        },
        ...(colors[measure] && { itemStyle: { color: colors[measure] } })
      }))

      return {
        animation: false, // Remove animations
        tooltip: {
          trigger: 'axis',
          axisPointer: {
            type: 'cross'
          },
          formatter: (params: unknown) => {
            if (!Array.isArray(params)) return ''

            let result = `<div style="margin-bottom: 4px; font-weight: 600;">${(params[0] as { axisValue?: string })?.axisValue || ''}</div>`

            params.forEach((param: { seriesName?: string; value?: number; color?: string }) => {
              const measureName = param.seriesName || ''
              const title = measureTitles[measureName] || measureName
              const value = param.value || 0
              const color = param.color || '#000'

              result += `<div style="display: flex; align-items: center; margin: 2px 0;">
                <span style="display: inline-block; width: 10px; height: 10px; background-color: ${color}; border-radius: 50%; margin-right: 8px;"></span>
                <span style="font-weight: 500;">${title}:</span>
                <span style="margin-left: 8px; font-weight: 600;">${value.toLocaleString()}</span>
              </div>`
            })

            return result
          }
        },
        ...(showLegend ? {
          legend: {
            data: measures
          }
        } : {}),
        grid: {
          left: '3%',
          right: '4%',
          bottom: '3%',
          containLabel: true
        },
        xAxis: {
          type: 'category',
          boundaryGap: false,
          data: xAxisData
        },
        yAxis: {
          type: 'value'
        },
        series
      }
    } else if (dimensions.length > 0) {
      // Categorical line chart
      const xAxisData = chartData.map((item) => {
        const value = item[dimensions[0]]
        // If it looks like a date, format it
        if (typeof value === 'string' && (value.includes('T') || value.includes('-'))) {
          return formatDate(value)
        }
        return value
      })
      const series = measures.map((measure) => ({
        name: measure,
        type: 'line',
        data: chartData.map((item) => item[measure] || 0),
        symbol: 'none', // Hide dots by default
        symbolSize: 6,
        emphasis: {
          focus: 'series',
          symbol: 'circle', // Show dots on hover
          symbolSize: 8
        },
        ...(colors[measure] && { itemStyle: { color: colors[measure] } })
      }))

      return {
        animation: false, // Remove animations
        tooltip: {
          trigger: 'axis',
          formatter: (params: unknown) => {
            if (!Array.isArray(params)) return ''

            let result = `<div style="margin-bottom: 4px; font-weight: 600;">${(params[0] as { axisValue?: string })?.axisValue || ''}</div>`

            params.forEach((param: { seriesName?: string; value?: number; color?: string }) => {
              const measureName = param.seriesName || ''
              const title = measureTitles[measureName] || measureName
              const value = param.value || 0
              const color = param.color || '#000'

              result += `<div style="display: flex; align-items: center; margin: 2px 0;">
                <span style="display: inline-block; width: 10px; height: 10px; background-color: ${color}; border-radius: 50%; margin-right: 8px;"></span>
                <span style="font-weight: 500;">${title}:</span>
                <span style="margin-left: 8px; font-weight: 600;">${value.toLocaleString()}</span>
              </div>`
            })

            return result
          }
        },
        ...(showLegend ? {
          legend: {
            data: measures
          }
        } : {}),
        xAxis: {
          type: 'category',
          data: xAxisData
        },
        yAxis: {
          type: 'value'
        },
        series
      }
    }

    return {}
  }

  const getBarChartOption = (chartData: Record<string, unknown>[], query: AnalyticsQuery) => {
    const timeDimension = query.timeDimensions?.[0]?.dimension
    const measures = query.measures
    const dimensions = query.dimensions

    if (timeDimension) {
      // Time series stacked bar chart
      const barGranularity = query.timeDimensions?.[0]?.granularity
      const timeField = barGranularity ? `${timeDimension}_${barGranularity}` : timeDimension
      const xAxisData = chartData.map((item) => {
        const rawDate = item[timeField] || item[timeDimension] || item.created_at
        return formatDate(String(rawDate))
      })
      const series = measures.map((measure) => ({
        name: measure,
        type: 'bar',
        data: chartData.map((item) => item[measure] || 0)
      }))

      return {
        tooltip: {
          trigger: 'axis',
          axisPointer: {
            type: 'shadow'
          }
        },
        ...(showLegend ? {
          legend: {
            data: measures
          }
        } : {}),
        grid: {
          left: '3%',
          right: '4%',
          bottom: '3%',
          containLabel: true
        },
        xAxis: {
          type: 'category',
          data: xAxisData
        },
        yAxis: {
          type: 'value'
        },
        series
      }
    } else if (dimensions.length > 0) {
      // Categorical stacked bar chart
      const xAxisData = chartData.map((item) => {
        const value = item[dimensions[0]]
        // If it looks like a date, format it
        if (typeof value === 'string' && (value.includes('T') || value.includes('-'))) {
          return formatDate(value)
        }
        return value
      })
      const series = measures.map((measure) => ({
        name: measure,
        type: 'bar',
        data: chartData.map((item) => item[measure] || 0)
      }))

      return {
        tooltip: {
          trigger: 'axis',
          axisPointer: {
            type: 'shadow'
          }
        },
        ...(showLegend ? {
          legend: {
            data: measures
          }
        } : {}),
        grid: {
          left: '3%',
          right: '4%',
          bottom: '3%',
          containLabel: true
        },
        xAxis: {
          type: 'category',
          data: xAxisData
        },
        yAxis: {
          type: 'value'
        },
        series
      }
    }

    return {}
  }

  const getPieChartOption = (chartData: Record<string, unknown>[], query: AnalyticsQuery) => {
    const dimensions = query.dimensions
    const measures = query.measures

    if (dimensions.length > 0 && measures.length > 0) {
      const data = chartData.map((item) => ({
        name: item[dimensions[0]],
        value: item[measures[0]] || 0
      }))

      return {
        tooltip: {
          trigger: 'item'
        },
        legend: {
          orient: 'vertical',
          left: 'left'
        },
        series: [
          {
            name: measures[0],
            type: 'pie',
            radius: '50%',
            data,
            emphasis: {
              itemStyle: {
                shadowBlur: 10,
                shadowOffsetX: 0,
                shadowColor: 'rgba(0, 0, 0, 0.5)'
              }
            }
          }
        ]
      }
    }

    return {}
  }

  const getTableColumns = () => {
    if (!data || !data.data.length) return []

    const firstRow = data.data[0]
    return Object.keys(firstRow).map((key) => ({
      title: key,
      dataIndex: key,
      key,
      sorter: (a: Record<string, unknown>, b: Record<string, unknown>) => {
        if (typeof a[key] === 'number' && typeof b[key] === 'number') {
          return (a[key] as number) - (b[key] as number)
        }
        return String(a[key]).localeCompare(String(b[key]))
      }
    }))
  }

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '50px' }}>
        <Spin size="large" />
      </div>
    )
  }

  if (error) {
    return <Alert message={t`Error`} description={error} type="error" showIcon />
  }

  if (!data || !data.data.length) {
    return (
      <div style={{ textAlign: 'center', padding: '50px', color: '#999' }}>{t`No data available`}</div>
    )
  }

  if (chartType === 'table') {
    return (
      <Table
        columns={getTableColumns()}
        dataSource={data.data.map((item, index) => ({ ...item, key: index }))}
        pagination={{ pageSize: 10 }}
        size="small"
      />
    )
  }

  return (
    <ReactEChartsCore
      echarts={echarts}
      option={getChartOption()}
      style={{ height: `${height}px` }}
      notMerge={true}
      lazyUpdate={true}
    />
  )
}
