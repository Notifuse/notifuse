import React, { useEffect, useState } from "react";
import { Alert, App, Button, Card, Form, Input, Space, Typography } from "antd";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faCopy } from "@fortawesome/free-regular-svg-icons";
import { Trans, useLingui } from "@lingui/react/macro";
import {
  workspaceService,
  type Integration,
} from "../../services/api/workspace";

const { Text } = Typography;

export const ZAPIER_DOCUMENTATION_URL =
  "https://docs.notifuse.com/integrations/zapier";

/**
 * What the label field is seeded with on a fresh connection.
 *
 * A fixed default is safe now: the server derives the key address from the label and appends
 * random hex, so two workspaces both connecting as "Zapier" get two distinct addresses.
 */
const DEFAULT_LABEL = "Zapier";

/**
 * The API base URL a user pastes into Zapier's connection form.
 *
 * It is the plain API origin. It is deliberately NOT `{workspaceId}.{apiHost}`: the server has no
 * per-workspace host, and a workspace is selected inside each Zap through the `workspace_id`
 * dropdown rather than through the URL.
 */
function zapierApiBaseUrl(): string {
  const configured = window.API_ENDPOINT?.trim();
  // Trailing slashes are the single most reported cause of broken self-hosted Zapier
  // connections across comparable products, so never print one.
  return (configured || window.location.origin).replace(/\/+$/, "");
}

interface ZapierFormValues {
  label: string;
}

export interface ZapierSettingsProps {
  workspaceId: string;
  /**
   * The card being renamed. Absent puts the body in connect mode, which mints an API key;
   * present puts it in edit mode, which never does — offering the connect action on a rename
   * would leave the workspace with a second key nobody asked for.
   */
  integration?: Integration;
  /** Edit mode only. Persists the rename; the drawer footer submits the form through `formRef`. */
  onSave?: (integration: Integration) => Promise<void>;
  /**
   * Connect mode only. Called once the key exists, so the caller can refetch the workspace and
   * show the new card. It must not close this drawer: the token is in exactly one response and
   * nothing can reissue it, so the panel stands until the user dismisses it. A failed refetch is
   * the caller's to report — the connection itself already succeeded.
   */
  onConnected?: () => void;
  /** Connect mode only. The user dismissed the token panel; the token is unrecoverable now. */
  onDone?: () => void;
  /**
   * Defaults to denied. The server refuses a non-owner outright, so the action is disabled
   * rather than answered with a 403 toast.
   */
  isOwner?: boolean;
  formRef?: React.RefObject<{ submit: () => void } | null>;
}

export function ZapierSettings({
  workspaceId,
  integration,
  onSave,
  onConnected,
  onDone,
  isOwner = false,
  formRef,
}: ZapierSettingsProps) {
  const { t } = useLingui();
  const { message } = App.useApp();
  const [form] = Form.useForm<ZapierFormValues>();
  const [connecting, setConnecting] = useState(false);
  const [connected, setConnected] = useState<{
    token: string;
    email: string;
  } | null>(null);

  const isEditing = Boolean(integration);
  const apiBaseUrl = zapierApiBaseUrl();

  // Hand the form up so the drawer footer's Save can submit it, the way the sibling bodies do.
  useEffect(() => {
    if (formRef) {
      (
        formRef as React.MutableRefObject<{ submit: () => void } | null>
      ).current = form;
    }
  }, [form, formRef]);

  useEffect(() => {
    form.setFieldsValue({ label: integration?.name ?? DEFAULT_LABEL });
  }, [integration, form]);

  const copy = async (value: string, label: string) => {
    await navigator.clipboard.writeText(value);
    message.success(t`${label} copied to clipboard`);
  };

  const handleFinish = async (values: ZapierFormValues) => {
    // The whitespace rule below refuses an all-blank label, but "  Marketing  " passes it, and
    // both paths write the label verbatim — as the card name here, and as the seed of the key
    // address on the server, which trims neither.
    const label = values.label.trim();

    if (integration) {
      try {
        await onSave?.({
          ...integration,
          name: label,
          updated_at: new Date().toISOString(),
        });
      } catch {
        // The caller reports its own failure; catching here only keeps the rejection from
        // escaping antd's onFinish as an unhandled one.
      }
      return;
    }

    // The button disables itself while in flight, but the same form can be submitted through
    // formRef by whatever chrome the caller wraps this in, and a second connect mints a second
    // key rather than being refused.
    if (connecting) {
      return;
    }

    setConnecting(true);
    try {
      const response = await workspaceService.connectZapier({
        workspace_id: workspaceId,
        label,
      });
      setConnected({ token: response.token, email: response.email });
    } catch (error) {
      const fallback = t`Failed to connect Zapier`;
      message.error(
        error instanceof Error && error.message ? error.message : fallback,
      );
      return;
    } finally {
      setConnecting(false);
    }

    // Deliberately after the token is on screen, and outside the try: the key exists whatever
    // the refresh does, and a refetch that throws must not read as a failed connection.
    onConnected?.();
  };

  const labelField = (
    <Form.Item
      label={t`Label`}
      name="label"
      rules={[
        {
          required: true,
          whitespace: true,
          message: t`Please enter a label for this connection`,
        },
      ]}
      extra={
        isEditing
          ? t`Renaming this connection does not change the address of the API key it already minted.`
          : t`Names this connection in Notifuse, and the API key it creates in Settings → Team.`
      }
    >
      <Input
        placeholder={t`e.g. Marketing`}
        maxLength={64}
        disabled={!isOwner}
      />
    </Form.Item>
  );

  const apiUrlCard = (
    <Card title={t`API URL`} className="!mb-6" size="small">
      <div className="text-gray-500 mb-2">
        {t`Paste this URL into the API URL field when you connect your Notifuse account in Zapier.`}
      </div>
      <Space.Compact style={{ width: "100%" }}>
        <Input value={apiBaseUrl} readOnly aria-label={t`API URL`} />
        <Button
          icon={<FontAwesomeIcon icon={faCopy} />}
          onClick={() => copy(apiBaseUrl, t`API URL`)}
        >
          {t`Copy`}
        </Button>
      </Space.Compact>
    </Card>
  );

  if (isEditing) {
    return (
      <Form
        form={form}
        layout="vertical"
        onFinish={handleFinish}
        disabled={!isOwner}
      >
        {labelField}
      </Form>
    );
  }

  return (
    <>
      <div className="text-gray-600 mb-6">
        <Trans>
          Connecting Zapier lets a Zap react to Notifuse events — a new contact,
          a list subscription, a segment a contact joined — and lets a Zap
          create or update contacts and subscribe them to your lists. Each Zap
          you turn on registers its own webhook subscription, which you can see
          in Settings → Webhooks.
        </Trans>{" "}
        <a
          href={ZAPIER_DOCUMENTATION_URL}
          target="_blank"
          rel="noopener noreferrer"
        >
          {t`Read the Zapier setup guide`}
        </a>
      </div>

      {connected ? (
        // The panel replaces the form and stays until it is dismissed. Every sibling drawer
        // closes itself on success; this one cannot, because closing discards the only copy of
        // the token that will ever exist.
        <>
          {apiUrlCard}
          <Card title={t`Zapier API key`} size="small">
            <Alert
              title={t`API key created`}
              description={t`This token is displayed once and cannot be retrieved again. Copy it now and paste it into Zapier.`}
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
            />
            <Input.TextArea
              value={connected.token}
              autoSize={{ minRows: 3, maxRows: 5 }}
              readOnly
              aria-label={t`API key token`}
            />
            <Text type="secondary" className="block mt-3">
              {t`The key is listed in Settings → Team as ${connected.email}, where its permissions can be widened.`}
            </Text>
            <div className="flex justify-end gap-2 mt-3">
              <Button
                icon={<FontAwesomeIcon icon={faCopy} />}
                onClick={() => copy(connected.token, t`API key`)}
              >
                {t`Copy`}
              </Button>
              <Button
                type="primary"
                onClick={() => {
                  setConnected(null);
                  onDone?.();
                }}
              >
                {t`Done`}
              </Button>
            </div>
          </Card>
        </>
      ) : (
        <Form form={form} layout="vertical" onFinish={handleFinish}>
          {labelField}
          {apiUrlCard}
          <Button
            type="primary"
            block
            loading={connecting}
            // Nothing server-side blocks a second connect — several connections per workspace
            // are allowed by design — so a double-click would mint two keys and two cards.
            disabled={!isOwner || connecting}
            onClick={() => form.submit()}
          >
            {t`Connect Zapier`}
          </Button>
        </Form>
      )}
    </>
  );
}
