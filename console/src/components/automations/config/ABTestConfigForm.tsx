import React from 'react'
import { Form, Input, InputNumber, Button, Alert } from 'antd'
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons'
import type { ABTestNodeConfig, ABTestVariant } from '../../../services/api/automation'

interface ABTestConfigFormProps {
  config: ABTestNodeConfig
  onChange: (config: ABTestNodeConfig) => void
}

const VARIANT_IDS = ['A', 'B', 'C', 'D']
const MAX_VARIANTS = 4
const MIN_VARIANTS = 2

const DEFAULT_VARIANTS: ABTestVariant[] = [
  { id: 'A', name: 'Variant A', weight: 50, next_node_id: '' },
  { id: 'B', name: 'Variant B', weight: 50, next_node_id: '' }
]

export const ABTestConfigForm: React.FC<ABTestConfigFormProps> = ({ config, onChange }) => {
  const variants = config?.variants?.length > 0 ? config.variants : DEFAULT_VARIANTS

  // Initialize with defaults if empty (using ref to run only once)
  const initializedRef = React.useRef(false)
  React.useEffect(() => {
    if (!initializedRef.current && (!config?.variants || config.variants.length === 0)) {
      initializedRef.current = true
      onChange({ variants: DEFAULT_VARIANTS })
    }
  }, [config?.variants, onChange])

  const totalWeight = variants.reduce((sum, v) => sum + (v.weight || 0), 0)
  const isWeightValid = totalWeight === 100

  const handleVariantChange = (index: number, field: keyof ABTestVariant, value: string | number) => {
    const updatedVariants = variants.map((v, i) => {
      if (i === index) {
        return { ...v, [field]: value }
      }
      return v
    })
    onChange({ ...config, variants: updatedVariants })
  }

  const handleAddVariant = () => {
    if (variants.length >= MAX_VARIANTS) return

    // Find next available ID
    const usedIds = new Set(variants.map((v) => v.id))
    const nextId = VARIANT_IDS.find((id) => !usedIds.has(id)) || `V${variants.length + 1}`

    const newVariant: ABTestVariant = {
      id: nextId,
      name: `Variant ${nextId}`,
      weight: 0,
      next_node_id: ''
    }

    onChange({ ...config, variants: [...variants, newVariant] })
  }

  const handleRemoveVariant = (index: number) => {
    if (variants.length <= MIN_VARIANTS) return

    const updatedVariants = variants.filter((_, i) => i !== index)
    onChange({ ...config, variants: updatedVariants })
  }

  const handleDistributeEvenly = () => {
    const count = variants.length
    const baseWeight = Math.floor(100 / count)
    const remainder = 100 % count

    const updatedVariants = variants.map((v, i) => ({
      ...v,
      weight: baseWeight + (i < remainder ? 1 : 0)
    }))

    onChange({ ...config, variants: updatedVariants })
  }

  return (
    <Form layout="vertical" className="nodrag">
      <Form.Item
        label="Test Variants"
        required
        extra="Split contacts between variants. Weights must total 100%."
      >
        <div className="space-y-2">
          {variants.map((variant, index) => (
            <div key={variant.id} className="flex items-center gap-2">
              <div className="w-8 h-8 flex items-center justify-center bg-blue-100 text-blue-600 rounded font-medium text-sm">
                {variant.id}
              </div>
              <Input
                value={variant.name}
                onChange={(e) => handleVariantChange(index, 'name', e.target.value)}
                placeholder="Variant name"
                style={{ flex: 1 }}
              />
              <InputNumber
                min={1}
                max={99}
                value={variant.weight}
                onChange={(value) => handleVariantChange(index, 'weight', value || 0)}
                formatter={(value) => `${value}%`}
                parser={(value) => parseInt(value?.replace('%', '') || '0', 10)}
                style={{ width: 80 }}
              />
              <Button
                type="text"
                icon={<DeleteOutlined />}
                onClick={() => handleRemoveVariant(index)}
                disabled={variants.length <= MIN_VARIANTS}
                danger
              />
            </div>
          ))}
        </div>
      </Form.Item>

      <div className="flex items-center justify-between mb-4">
        {variants.length < MAX_VARIANTS && (
          <Button type="dashed" onClick={handleAddVariant} icon={<PlusOutlined />} size="small">
            Add Variant
          </Button>
        )}
        <Button type="link" onClick={handleDistributeEvenly} size="small">
          Distribute evenly
        </Button>
      </div>

      <div className={`text-sm ${isWeightValid ? 'text-green-600' : 'text-red-500'}`}>
        Total: {totalWeight}%
        {!isWeightValid && ' (must equal 100%)'}
      </div>

      {!isWeightValid && (
        <Alert
          type="warning"
          message={`Weights must sum to 100%. Currently: ${totalWeight}%`}
          className="mt-2"
          showIcon
        />
      )}
    </Form>
  )
}
