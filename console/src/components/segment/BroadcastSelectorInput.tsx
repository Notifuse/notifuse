import React, { useMemo, useRef, useState } from 'react'
import { Select } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { broadcastApi } from '../../services/api/broadcast'

interface BroadcastSelectorInputProps {
  value?: string | null
  onChange?: (value: string | null) => void
  workspaceId: string
  placeholder?: string
  size?: 'small' | 'middle' | 'large'
  style?: React.CSSProperties
}

// BroadcastSelectorInput is a searchable dropdown of the workspace's broadcasts, used to
// scope a timeline segment/automation condition to a specific broadcast. Search is
// server-side (so workspaces with many broadcasts are fully reachable), and the currently
// selected broadcast is resolved by id so its name shows even when it is not on the current
// results page. Controlled via value/onChange so it plugs into an Ant Design Form.Item.
const BroadcastSelectorInput: React.FC<BroadcastSelectorInputProps> = ({
  value,
  onChange,
  workspaceId,
  placeholder,
  size,
  style
}) => {
  const { t } = useLingui()
  const [search, setSearch] = useState('')
  const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const handleSearch = (text: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => setSearch(text), 300)
  }

  const { data, isLoading, isError } = useQuery({
    queryKey: ['broadcasts-selector', workspaceId, search],
    queryFn: () =>
      broadcastApi.list({ workspace_id: workspaceId, search: search || undefined, limit: 50 }),
    enabled: !!workspaceId
  })

  // Resolve the selected broadcast so its name renders even when it is not in the current
  // results page (or was created before this component mounted).
  const { data: selected } = useQuery({
    queryKey: ['broadcast-selected', workspaceId, value],
    queryFn: () => broadcastApi.get({ workspace_id: workspaceId, id: value as string }),
    enabled: !!workspaceId && !!value
  })

  const options = useMemo(() => {
    const list = (data?.broadcasts || []).map((b) => ({ value: b.id, label: b.name }))
    if (selected?.broadcast && !list.some((o) => o.value === selected.broadcast.id)) {
      list.unshift({ value: selected.broadcast.id, label: selected.broadcast.name })
    }
    return list
  }, [data, selected])

  return (
    <Select
      value={value || undefined}
      onChange={(v) => onChange?.(v ?? null)}
      placeholder={placeholder || t`Select a broadcast`}
      loading={isLoading}
      allowClear
      showSearch
      filterOption={false}
      onSearch={handleSearch}
      notFoundContent={isError ? t`Failed to load broadcasts` : undefined}
      size={size}
      style={style}
      options={options}
    />
  )
}

export default BroadcastSelectorInput
