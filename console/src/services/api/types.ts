/**
 * Central type exports for API services
 *
 * This file re-exports types from their respective service files,
 * providing a convenient single import location for consumers.
 * The actual type definitions are co-located with their service implementations.
 */

// Re-export from auth.ts
export type {
  SignInRequest,
  SignInResponse,
  VerifyCodeRequest,
  VerifyResponse,
  GetCurrentUserResponse
} from './auth'

// Re-export from workspace.ts
export type {
  TemplateBlock,
  WorkspaceSettings,
  FileManagerSettings,
  EmailProviderKind,
  Sender,
  EmailProvider,
  AmazonSES,
  SMTPSettings,
  SparkPostSettings,
  PostmarkSettings,
  MailgunSettings,
  MailjetSettings,
  IntegrationType,
  Integration,
  SupabaseAuthEmailHookSettings,
  SupabaseUserCreatedHookSettings,
  SupabaseIntegrationSettings,
  CreateWorkspaceRequest,
  Workspace,
  CreateWorkspaceResponse,
  ListWorkspacesResponse,
  GetWorkspaceResponse,
  UpdateWorkspaceRequest,
  UpdateWorkspaceResponse,
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
  RemoveMemberRequest,
  RemoveMemberResponse,
  DeleteWorkspaceRequest,
  DeleteWorkspaceResponse,
  CreateIntegrationRequest,
  UpdateIntegrationRequest,
  DeleteIntegrationRequest,
  CreateIntegrationResponse,
  UpdateIntegrationResponse,
  DeleteIntegrationResponse,
  WorkspaceMember,
  GetWorkspaceMembersResponse,
  InviteMemberRequest,
  InviteMemberResponse,
  ResourcePermissions,
  UserPermissions,
  SetUserPermissionsRequest,
  SetUserPermissionsResponse,
  WorkspaceInvitation,
  User,
  VerifyInvitationTokenResponse,
  AcceptInvitationResponse,
  DeleteInvitationRequest,
  DeleteInvitationResponse
} from './workspace'

// Re-export from list.ts
export type {
  TemplateReference,
  List,
  CreateListRequest,
  GetListsRequest,
  GetListRequest,
  UpdateListRequest,
  DeleteListRequest,
  GetListsResponse,
  GetListResponse,
  CreateListResponse,
  UpdateListResponse,
  DeleteListResponse,
  ListStats,
  GetListStatsRequest,
  GetListStatsResponse,
  ContactListTotalType,
  SubscribeToListsRequest
} from './list'

// Re-export from template.ts
export type {
  Template,
  EmailTemplate,
  GetTemplatesRequest,
  GetTemplateRequest,
  CreateTemplateRequest,
  UpdateTemplateRequest,
  DeleteTemplateRequest,
  GetTemplatesResponse,
  GetTemplateResponse,
  CreateTemplateResponse,
  UpdateTemplateResponse,
  DeleteTemplateResponse,
  MjmlErrorDetail,
  MjmlCompileError,
  TrackingSettings,
  CompileTemplateRequest,
  CompileTemplateResponse,
  TestEmailProviderRequest,
  TestEmailProviderResponse,
  TestTemplateRequest,
  TestTemplateResponse
} from './template'

// Re-export from template_blocks.ts
export type {
  GetTemplateBlockRequest,
  ListTemplateBlocksRequest,
  CreateTemplateBlockRequest,
  UpdateTemplateBlockRequest,
  DeleteTemplateBlockRequest,
  GetTemplateBlockResponse,
  ListTemplateBlocksResponse,
  CreateTemplateBlockResponse,
  UpdateTemplateBlockResponse,
  DeleteTemplateBlockResponse
} from './template_blocks'

// Re-export from segment.ts
export type {
  SegmentStatus,
  BooleanOperator,
  TreeNode,
  Segment,
  CreateSegmentRequest,
  GetSegmentsRequest,
  GetSegmentRequest,
  UpdateSegmentRequest,
  DeleteSegmentRequest,
  RebuildSegmentRequest,
  PreviewSegmentRequest,
  GetSegmentContactsRequest,
  GetSegmentsResponse,
  GetSegmentResponse,
  CreateSegmentResponse,
  UpdateSegmentResponse,
  DeleteSegmentResponse,
  RebuildSegmentResponse,
  PreviewSegmentResponse,
  GetSegmentContactsResponse
} from './segment'

// Re-export from task.ts
export type {
  TaskStatus,
  SendBroadcastState,
  TaskState,
  Task,
  GetTaskResponse,
  ListTasksResponse
} from './task'

// Re-export from notification_center.ts
export type {
  NotificationCenterRequest,
  NotificationCenterResponse,
  UnsubscribeFromListsRequest
} from './notification_center'

// Re-export from contacts.ts
export type { Contact } from './contacts'

// Re-export from transactional_notifications.ts
export type { EmailOptions } from './transactional_notifications'
