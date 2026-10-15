import { App, Button, Card, Typography } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { useAuth } from '../../contexts/AuthContext'
import { workspaceService, type PendingInvitation } from '../../services/api/workspace'

const { Text } = Typography

const pendingInvitationsKey = 'pendingInvitations'

// The invitations addressed to the signed-in user's email. Someone who signs in with single
// sign-on before opening the emailed link arrives with an account and no workspace: this is
// where they join. Joining is a choice made here, not something signing in does for them.
export function PendingInvitations() {
  const { t } = useLingui()
  const { message } = App.useApp()
  const { user, refreshWorkspaces } = useAuth()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  // Keyed by user: the query client outlives a sign-out, and the next account must not be
  // shown the previous one's invitations while its own load.
  const { data } = useQuery({
    queryKey: [pendingInvitationsKey, user?.id],
    queryFn: () => workspaceService.pendingInvitations(),
    enabled: !!user
  })
  const refreshList = () => queryClient.invalidateQueries({ queryKey: [pendingInvitationsKey] })

  const join = useMutation({
    mutationFn: (invitation: PendingInvitation) => workspaceService.joinInvitation(invitation.id),
    onSuccess: async (response, invitation) => {
      const workspaceName = invitation.workspace_name
      message.success(t`You joined ${workspaceName}`)
      void refreshList()
      // The join is done whatever happens next. The workspace list lives in the auth context,
      // not in react-query; if refreshing it fails, staying here is better than opening a
      // workspace the console does not know yet — and better than reporting the join failed.
      try {
        await refreshWorkspaces()
      } catch {
        return
      }
      navigate({
        to: '/console/workspace/$workspaceId',
        params: { workspaceId: response.workspace_id }
      })
    },
    onError: (error) => {
      // The server's sentence, not a fixed one: "no longer valid" and "limit reached" are
      // different things to do next.
      message.error(
        error instanceof Error && error.message ? error.message : t`Failed to join the workspace`
      )
      // A refusal usually means the list is stale — joined in another tab, revoked, expired.
      void refreshList()
    }
  })

  const invitations = data?.invitations ?? []
  if (invitations.length === 0) {
    return null
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginBottom: '16px' }}>
      <Text type="secondary">{t`Pending invitations`}</Text>
      {invitations.map((invitation) => {
        const inviterName = invitation.inviter_name
        return (
          <Card key={invitation.id} size="small">
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: '8px'
              }}
            >
              <div style={{ minWidth: 0 }}>
                <div style={{ fontWeight: 500 }}>{invitation.workspace_name}</div>
                {inviterName && (
                  <Text type="secondary" style={{ fontSize: '11px' }} ellipsis>
                    {t`Invited by ${inviterName}`}
                  </Text>
                )}
              </div>
              <Button
                type="primary"
                size="small"
                loading={join.isPending && join.variables?.id === invitation.id}
                disabled={join.isPending}
                onClick={() => join.mutate(invitation)}
              >
                {t`Join`}
              </Button>
            </div>
          </Card>
        )
      })}
    </div>
  )
}
