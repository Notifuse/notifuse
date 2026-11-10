import { useState } from 'react'
import { App, Button, Form, Modal } from 'antd'
import { useLingui } from '@lingui/react/macro'
import { useQueryClient } from '@tanstack/react-query'
import { broadcastApi, type Broadcast, type BroadcastVariation } from '../../services/api/broadcast'
import type { Workspace } from '../../services/api/types'
import { VariationSenderSelect } from './VariationSenderSelect'
import { getMarketingSenders, variationsWithMissingSender } from './variationSender'

interface ChangeVariationSenderModalProps {
  workspace: Workspace
  broadcast: Broadcast
  disabled?: boolean
}

interface SenderFormValues {
  senders: Record<string, string | undefined>
}

// ChangeVariationSenderModal replaces, on a paused broadcast, the senders that left the
// marketing integration, so the broadcast can be resumed. It renders nothing when no
// variation needs one.
export function ChangeVariationSenderModal({
  workspace,
  broadcast,
  disabled
}: ChangeVariationSenderModalProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const [form] = Form.useForm<SenderFormValues>()
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)

  const missing = variationsWithMissingSender(workspace, broadcast)
  // Offered only when a variation still to send has lost its sender: a losing variation's
  // does not stop the winner. Once offered, every missing sender is asked for, because the
  // server checks them all on save.
  const blocksSending = missing.some(
    (v) => !broadcast.winning_template || v.template_id === broadcast.winning_template
  )
  if (broadcast.status !== 'paused' || !blocksSending) return null

  const variations = broadcast.test_settings.variations || []
  // Variations sharing a template share a sender (the server refuses two): one field each.
  const asked = missing.filter(
    (v, i) => missing.findIndex((m) => m.template_id === v.template_id) === i
  )
  // The console saves variations without a name: they are numbered as on the broadcast page.
  const variationLabel = (variation: BroadcastVariation) => {
    if (!broadcast.test_settings.enabled && variations.length === 1) return t`Sender`
    const name = variation.variation_name || String(variations.indexOf(variation) + 1)
    return t`Sender of variation ${name}`
  }

  const marketingSenders = getMarketingSenders(workspace)
  const senderRules = [
    {
      validator(_: unknown, value?: string) {
        if (!value || marketingSenders.some((s) => s.id === value)) return Promise.resolve()
        return Promise.reject(
          new Error(
            t`This sender is no longer one of the marketing email integration's. Choose another one or clear the field to use the template's.`
          )
        )
      }
    }
  ]

  const openModal = () => {
    form.setFieldsValue({
      senders: Object.fromEntries(asked.map((v) => [v.template_id, v.sender_id]))
    })
    setOpen(true)
  }

  const save = async () => {
    let values: SenderFormValues
    try {
      values = await form.validateFields()
    } catch {
      return // The form shows what is wrong.
    }
    setSaving(true)
    try {
      const sentVariations = variations.map((variation) => {
        // The joined template is a read-only copy: it is not sent back.
        const sent: BroadcastVariation = {
          variation_name: variation.variation_name,
          template_id: variation.template_id,
          sender_id: variation.sender_id,
          metrics: variation.metrics
        }
        if (variation.template_id in values.senders) {
          sent.sender_id = values.senders[variation.template_id] || undefined
        }
        return sent
      })
      await broadcastApi.update({
        workspace_id: workspace.id,
        id: broadcast.id,
        name: broadcast.name,
        audience: broadcast.audience,
        test_settings: { ...broadcast.test_settings, variations: sentVariations }
      })
      message.success(t`Sender changed. You can now resume the broadcast.`)
      setOpen(false)
      queryClient.invalidateQueries({ queryKey: ['broadcasts', workspace.id] })
    } catch (error) {
      message.error(
        error instanceof Error
          ? t`Failed to change the sender: ${error.message}`
          : t`Failed to change the sender`
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <Button size="small" danger onClick={openModal} disabled={disabled}>
        {t`Change sender`}
      </Button>
      <Modal
        title={t`Change sender`}
        open={open}
        onOk={save}
        onCancel={() => setOpen(false)}
        okText={t`Save`}
        cancelText={t`Cancel`}
        confirmLoading={saving}
        destroyOnHidden
      >
        <p>
          {t`The sender chosen for this broadcast is no longer one of the marketing email integration's senders. Choose another one, or clear the field to send from the template's sender, then resume the broadcast.`}
        </p>
        <Form form={form} layout="vertical">
          {asked.map((variation) => (
            <Form.Item
              key={variation.template_id}
              name={['senders', variation.template_id]}
              label={variationLabel(variation)}
              rules={senderRules}
            >
              <VariationSenderSelect workspace={workspace} templateId={variation.template_id} />
            </Form.Item>
          ))}
        </Form>
      </Modal>
    </>
  )
}

export default ChangeVariationSenderModal
