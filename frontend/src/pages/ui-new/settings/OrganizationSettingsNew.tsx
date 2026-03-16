import { useMemo, useState } from 'react';
import {
  Plus,
  UserPlus,
  Trash,
  SpinnerGap,
  CheckCircle,
  Info,
} from '@phosphor-icons/react';
import { useUserOrganizations } from '@/hooks/useUserOrganizations';
import { useOrganizationSelection } from '@/hooks/useOrganizationSelection';
import { useOrganizationMembers } from '@/hooks/useOrganizationMembers';
import { useOrganizationInvitations } from '@/hooks/useOrganizationInvitations';
import { useOrganizationMutations } from '@/hooks/useOrganizationMutations';
import { useUserSystem } from '@/components/ConfigProvider';
import { useAuth } from '@/hooks/auth/useAuth';
import { LoginRequiredPrompt } from '@/components/dialogs/shared/LoginRequiredPrompt';
import { CreateOrganizationDialog } from '@/components/dialogs/org/CreateOrganizationDialog';
import { InviteMemberDialog } from '@/components/dialogs/org/InviteMemberDialog';
import type {
  InviteMemberResult,
  CreateOrganizationResult,
} from '@/components/dialogs';
import { MemberListItem } from '@/components/org/MemberListItem';
import { PendingInvitationItem } from '@/components/org/PendingInvitationItem';
import { RemoteProjectItem } from '@/components/org/RemoteProjectItem';
import type {
  MemberRole,
  Invitation,
  OrganizationMemberWithProfile,
  RemoteProject,
  Project,
} from 'shared/types';
import { MemberRole as MemberRoleEnum } from 'shared/types';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { useProjects } from '@/hooks/useProjects';
import { useOrganizationProjects } from '@/hooks/useOrganizationProjects';
import { useProjectMutations } from '@/hooks/useProjectMutations';
import { ApiError } from '@/lib/api';
import { isRemoteProjectCapabilityUnsupported } from '@/pages/settings/organizationRemoteCapability';
import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { Button } from '@/components/ui-new/primitives/Button';
import { ErrorAlert } from '@/components/ui-new/primitives/ErrorAlert';
import { cn } from '@/lib/utils';

const REMOTE_PROJECT_API_UNSUPPORTED_STATUS = 501;

/* ---------- Inline alert for success ---------- */

function SuccessAlert({
  message,
  className,
}: Readonly<{ message: string; className?: string }>) {
  return (
    <output
      className={cn(
        'relative w-full border border-success bg-success/10 p-base text-sm text-success flex items-center gap-half',
        className
      )}
    >
      <CheckCircle className="size-icon-sm shrink-0" weight="bold" />
      <span>{message}</span>
    </output>
  );
}

/* ---------- Inline alert for info ---------- */

function InfoAlert({
  message,
  className,
}: Readonly<{ message: string; className?: string }>) {
  return (
    <output
      className={cn(
        'relative w-full border border-border bg-secondary p-base text-sm text-normal flex items-center gap-half',
        className
      )}
    >
      <Info className="size-icon-sm shrink-0" weight="bold" />
      <span>{message}</span>
    </output>
  );
}

/* ---------- Loading spinner ---------- */

function LoadingSpinner({ text }: Readonly<{ text: string }>) {
  return (
    <div className="flex items-center justify-center py-double">
      <SpinnerGap className="size-icon-md animate-spin text-low" weight="bold" />
      <span className="ml-half text-low text-sm">{text}</span>
    </div>
  );
}

/* ---------- Empty state ---------- */

function EmptyState({ text }: Readonly<{ text: string }>) {
  return (
    <div className="text-center py-double text-low text-sm">{text}</div>
  );
}

/* ---------- Invitation list ---------- */

function InvitationListContent({
  loadingInvitations,
  invitations,
  onRevoke,
  isRevoking,
  t,
}: Readonly<{
  loadingInvitations: boolean;
  invitations: Invitation[];
  onRevoke: (id: string) => void;
  isRevoking: boolean;
  t: TFunction;
}>) {
  if (loadingInvitations) {
    return <LoadingSpinner text={t('invitationList.loading')} />;
  }

  if (invitations.length === 0) {
    return <EmptyState text={t('invitationList.none')} />;
  }

  return (
    <div className="space-y-base">
      {invitations.map((invitation) => (
        <PendingInvitationItem
          key={invitation.id}
          invitation={invitation}
          onRevoke={onRevoke}
          isRevoking={isRevoking}
        />
      ))}
    </div>
  );
}

/* ---------- Member list ---------- */

function MemberListContent({
  loadingMembers,
  members,
  currentUserId,
  isAdmin,
  onRemove,
  onRoleChange,
  isRemoving,
  isRoleChanging,
  t,
}: Readonly<{
  loadingMembers: boolean;
  members: OrganizationMemberWithProfile[];
  currentUserId: string | null;
  isAdmin: boolean;
  onRemove: (userId: string) => void;
  onRoleChange: (userId: string, role: MemberRole) => void;
  isRemoving: boolean;
  isRoleChanging: boolean;
  t: TFunction;
}>) {
  if (loadingMembers) {
    return <LoadingSpinner text={t('memberList.loading')} />;
  }

  if (members.length === 0) {
    return <EmptyState text={t('memberList.none')} />;
  }

  return (
    <div className="space-y-base">
      {members.map((member) => (
        <MemberListItem
          key={member.user_id}
          member={member}
          currentUserId={currentUserId}
          isAdmin={isAdmin}
          onRemove={onRemove}
          onRoleChange={onRoleChange}
          isRemoving={isRemoving}
          isRoleChanging={isRoleChanging}
        />
      ))}
    </div>
  );
}

/* ---------- Remote projects ---------- */

function RemoteProjectsContent({
  loadingProjects,
  loadingRemoteProjects,
  isRemoteProjectUnsupported,
  remoteProjectsError,
  remoteProjects,
  allProjects,
  availableLocalProjects,
  onLink,
  onUnlink,
  isLinking,
  isUnlinking,
  remoteProjectUnsupportedMessage,
  loadRemoteProjectsErrorMessage,
  t,
}: Readonly<{
  loadingProjects: boolean;
  loadingRemoteProjects: boolean;
  isRemoteProjectUnsupported: boolean;
  remoteProjectsError: unknown;
  remoteProjects: RemoteProject[];
  allProjects: Project[];
  availableLocalProjects: Project[];
  onLink: (remoteId: string, localId: string) => void;
  onUnlink: (projectId: string) => void;
  isLinking: boolean;
  isUnlinking: boolean;
  remoteProjectUnsupportedMessage: string;
  loadRemoteProjectsErrorMessage: string;
  t: TFunction;
}>) {
  if (loadingProjects || loadingRemoteProjects) {
    return <LoadingSpinner text={t('sharedProjects.loading')} />;
  }

  if (isRemoteProjectUnsupported) {
    return <InfoAlert message={remoteProjectUnsupportedMessage} />;
  }

  if (remoteProjectsError) {
    return (
      <ErrorAlert
        message={
          remoteProjectsError instanceof Error
            ? remoteProjectsError.message
            : loadRemoteProjectsErrorMessage
        }
      />
    );
  }

  if (remoteProjects.length === 0) {
    return <EmptyState text={t('sharedProjects.noProjects')} />;
  }

  return (
    <div className="space-y-base">
      {remoteProjects.map((remoteProject) => {
        const linkedLocalProject = allProjects.find(
          (p) => p.remoteProjectId === remoteProject.id
        );

        return (
          <RemoteProjectItem
            key={remoteProject.id}
            remoteProject={remoteProject}
            linkedLocalProject={linkedLocalProject}
            availableLocalProjects={availableLocalProjects}
            onLink={onLink}
            onUnlink={onUnlink}
            isLinking={isLinking}
            isUnlinking={isUnlinking}
            disabled={isRemoteProjectUnsupported}
            disabledReason={remoteProjectUnsupportedMessage}
          />
        );
      })}
    </div>
  );
}

/* ---------- Organization select (native) ---------- */

function OrgSelect({
  value,
  onChange,
  organizations,
  placeholder,
}: Readonly<{
  value: string;
  onChange: (value: string) => void;
  organizations: Array<{ id: string; name: string }>;
  placeholder: string;
}>) {
  return (
    <div className="relative">
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={cn(
          'appearance-none w-full rounded border border-border bg-secondary px-base py-half text-base text-normal',
          'focus:outline-none focus:ring-1 focus:ring-brand'
        )}
      >
        <option value="" disabled>
          {placeholder}
        </option>
        {organizations.length > 0 ? (
          organizations.map((org) => (
            <option key={org.id} value={org.id}>
              {org.name}
            </option>
          ))
        ) : (
          <option value="no-orgs" disabled>
            No organizations
          </option>
        )}
      </select>
    </div>
  );
}

/* ========== Main component ========== */

export function OrganizationSettingsNew() {
  const { t } = useTranslation('organization');
  const {
    loginStatus,
    remoteFeaturesEnabled,
    loading: systemLoading,
  } = useUserSystem();
  const { isSignedIn, isLoaded } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  // Fetch all organizations
  const {
    data: orgsResponse,
    isLoading: orgsLoading,
    error: orgsError,
    refetch: refetchOrgs,
  } = useUserOrganizations();

  // Organization selection with URL sync
  const { selectedOrgId, selectedOrg, handleOrgSelect } =
    useOrganizationSelection({
      organizations: orgsResponse,
      onSelectionChange: () => {
        setSuccess(null);
        setError(null);
      },
    });

  // Get current user's role and ID
  const currentUserRole = selectedOrg?.user_role;
  const isAdmin = currentUserRole === MemberRoleEnum.ADMIN;
  const isPersonalOrg = selectedOrg?.is_personal ?? false;
  const currentUserId =
    loginStatus?.status === 'loggedin' ? loginStatus.profile.user_id : null;

  // Fetch members using query hook
  const { data: members = [], isLoading: loadingMembers } =
    useOrganizationMembers(selectedOrgId);

  // Fetch invitations using query hook (admin only)
  const { data: invitations = [], isLoading: loadingInvitations } =
    useOrganizationInvitations({
      organizationId: selectedOrgId || null,
      isAdmin,
      isPersonal: isPersonalOrg,
    });

  // Organization mutations
  const {
    removeMember,
    updateMemberRole,
    revokeInvitation,
    deleteOrganization,
  } = useOrganizationMutations({
    onRevokeSuccess: () => {
      setSuccess('Invitation revoked successfully');
      setTimeout(() => setSuccess(null), 3000);
    },
    onRevokeError: (err) => {
      setError(
        err instanceof Error ? err.message : 'Failed to revoke invitation'
      );
    },
    onRemoveSuccess: () => {
      setSuccess('Member removed successfully');
      setTimeout(() => setSuccess(null), 3000);
    },
    onRemoveError: (err) => {
      setError(err instanceof Error ? err.message : 'Failed to remove member');
    },
    onRoleChangeSuccess: () => {
      setSuccess('Member role updated successfully');
      setTimeout(() => setSuccess(null), 3000);
    },
    onRoleChangeError: (err) => {
      setError(
        err instanceof Error ? err.message : 'Failed to update member role'
      );
    },
    onDeleteSuccess: () => {
      setSuccess(t('settings.deleteSuccess'));
      setTimeout(() => setSuccess(null), 3000);
      refetchOrgs()
        .then(() => {
          if (orgsResponse?.organizations) {
            const personalOrg = orgsResponse.organizations.find(
              (org) => org.is_personal
            );
            if (personalOrg) {
              handleOrgSelect(personalOrg.id);
            }
          }
        })
        .catch(console.error);
    },
    onDeleteError: (err) => {
      setError(err instanceof Error ? err.message : t('settings.deleteError'));
    },
  });

  // Fetch all local projects
  const { projects: allProjects = [], isLoading: loadingProjects } =
    useProjects();

  // Fetch remote projects for the selected organization
  const {
    data: remoteProjects = [],
    isLoading: loadingRemoteProjects,
    error: remoteProjectsError,
  } = useOrganizationProjects(selectedOrgId);

  const remoteProjectUnsupportedMessage = t(
    'sharedProjects.remoteProjectUnsupported',
    {
      defaultValue:
        'Remote project linking is unavailable because this backend version does not support remote project APIs.',
    }
  );

  const loadRemoteProjectsErrorMessage = t('sharedProjects.loadError', {
    defaultValue: 'Failed to load remote projects.',
  });

  const remoteProjectsApiUnsupported = useMemo(() => {
    if (!remoteProjectsError) {
      return false;
    }

    if (
      remoteProjectsError instanceof ApiError &&
      remoteProjectsError.status === REMOTE_PROJECT_API_UNSUPPORTED_STATUS
    ) {
      return true;
    }

    return isRemoteProjectCapabilityUnsupported(remoteProjectsError);
  }, [remoteProjectsError]);

  const isRemoteProjectUnsupported =
    !remoteFeaturesEnabled || remoteProjectsApiUnsupported;

  // Calculate available local projects (not linked to any remote project)
  const availableLocalProjects = allProjects.filter(
    (project) => !project.remoteProjectId
  );

  // Project mutations
  const { linkToExisting, unlinkProject } = useProjectMutations({
    onLinkSuccess: () => {
      setSuccess('Project linked successfully');
      setTimeout(() => setSuccess(null), 3000);
    },
    onLinkError: (err) => {
      if (isRemoteProjectCapabilityUnsupported(err)) {
        setError(remoteProjectUnsupportedMessage);
        return;
      }
      setError(err instanceof Error ? err.message : 'Failed to link project');
    },
    onUnlinkSuccess: () => {
      setSuccess('Project unlinked successfully');
      setTimeout(() => setSuccess(null), 3000);
    },
    onUnlinkError: (err) => {
      if (isRemoteProjectCapabilityUnsupported(err)) {
        setError(remoteProjectUnsupportedMessage);
        return;
      }
      setError(
        err instanceof Error ? err.message : 'Failed to unlink project'
      );
    },
  });

  const handleCreateOrganization = async () => {
    try {
      const result: CreateOrganizationResult =
        await CreateOrganizationDialog.show();

      if (result.action === 'created' && result.organizationId) {
        handleOrgSelect(result.organizationId ?? '');
        setSuccess('Organization created successfully');
        setTimeout(() => setSuccess(null), 3000);
      }
    } catch (err) {
      console.debug('Organization dialog cancelled or error occurred', err);
    }
  };

  const handleInviteMember = async () => {
    if (!selectedOrgId) return;

    try {
      const result: InviteMemberResult = await InviteMemberDialog.show({
        organizationId: selectedOrgId,
      });

      if (result.action === 'invited') {
        setSuccess('Member invited successfully');
        setTimeout(() => setSuccess(null), 3000);
      }
    } catch (err) {
      console.debug('Organization dialog cancelled or error occurred', err);
    }
  };

  const handleRevokeInvitation = (invitationId: string) => {
    if (!selectedOrgId) return;

    setError(null);
    revokeInvitation.mutate({ orgId: selectedOrgId, invitationId });
  };

  const handleRemoveMember = async (userId: string) => {
    if (!selectedOrgId) return;

    const confirmed = globalThis.confirm(t('confirmRemoveMember'));
    if (!confirmed) return;

    setError(null);
    removeMember.mutate({ orgId: selectedOrgId, userId });
  };

  const handleRoleChange = async (userId: string, newRole: MemberRole) => {
    if (!selectedOrgId) return;

    setError(null);
    updateMemberRole.mutate({ orgId: selectedOrgId, userId, role: newRole });
  };

  const handleDeleteOrganization = async () => {
    if (!selectedOrgId || !selectedOrg) return;

    const confirmed = globalThis.confirm(
      t('settings.confirmDelete', { orgName: selectedOrg.name })
    );
    if (!confirmed) return;

    setError(null);
    deleteOrganization.mutate(selectedOrgId);
  };

  const handleLinkProject = (
    remoteProjectId: string,
    localProjectId: string
  ) => {
    if (isRemoteProjectUnsupported) {
      setError(remoteProjectUnsupportedMessage);
      return;
    }

    setError(null);
    linkToExisting.mutate({
      localProjectId,
      data: { remote_project_id: remoteProjectId },
    });
  };

  const handleUnlinkProject = (projectId: string) => {
    if (isRemoteProjectUnsupported) {
      setError(remoteProjectUnsupportedMessage);
      return;
    }

    setError(null);
    unlinkProject.mutate(projectId);
  };

  /* ---------- Early returns ---------- */

  if (!isLoaded || orgsLoading || systemLoading) {
    return <LoadingSpinner text={t('settings.loadingOrganizations')} />;
  }

  if (!remoteFeaturesEnabled) {
    return (
      <div className="py-double">
        <InfoAlert
          message={t('settings.remoteDisabled', {
            defaultValue: 'Remote features are disabled in this build.',
          })}
        />
      </div>
    );
  }

  if (!isSignedIn) {
    return (
      <div className="py-double">
        <LoginRequiredPrompt
          title={t('loginRequired.title')}
          description={t('loginRequired.description')}
          actionLabel={t('loginRequired.action')}
        />
      </div>
    );
  }

  if (orgsError) {
    return (
      <div className="py-double">
        <ErrorAlert
          message={
            orgsError instanceof Error
              ? orgsError.message
              : t('settings.loadError')
          }
        />
      </div>
    );
  }

  const organizations = orgsResponse?.organizations ?? [];

  /* ---------- Main render ---------- */

  return (
    <div className="space-y-base">
      {/* Global alerts */}
      {error && <ErrorAlert message={error} />}
      {success && <SuccessAlert message={success} />}

      {/* Organization selector card */}
      <SettingsCard
        title={t('settings.title')}
        description={t('settings.description')}
      >
        <div className="flex items-start justify-between gap-base mb-base">
          <div className="flex-1" />
          <Button
            variant="secondary"
            size="sm"
            onClick={handleCreateOrganization}
          >
            <Plus className="size-icon-xs" weight="bold" />
            {t('createDialog.createButton')}
          </Button>
        </div>

        <div className="space-y-half">
          <label
            htmlFor="org-selector-new"
            className="text-normal text-base font-medium"
          >
            {t('settings.selectLabel')}
          </label>
          <OrgSelect
            value={selectedOrgId}
            onChange={handleOrgSelect}
            organizations={organizations}
            placeholder={t('settings.selectPlaceholder')}
          />
          <p className="text-low text-sm">{t('settings.selectHelper')}</p>
        </div>
      </SettingsCard>

      {/* Pending invitations (admin only, non-personal) */}
      {selectedOrg && isAdmin && !isPersonalOrg && (
        <SettingsCard
          title={t('invitationList.title')}
          description={t('invitationList.description', {
            orgName: selectedOrg.name,
          })}
        >
          <InvitationListContent
            loadingInvitations={loadingInvitations}
            invitations={invitations}
            onRevoke={handleRevokeInvitation}
            isRevoking={revokeInvitation.isPending}
            t={t}
          />
        </SettingsCard>
      )}

      {/* Members */}
      {selectedOrg && (
        <SettingsCard
          title={t('memberList.title')}
          description={t('memberList.description', {
            orgName: selectedOrg.name,
          })}
        >
          {isAdmin && !isPersonalOrg && (
            <div className="flex justify-end mb-base">
              <Button
                variant="secondary"
                size="sm"
                onClick={handleInviteMember}
              >
                <UserPlus className="size-icon-xs" weight="bold" />
                {t('memberList.inviteButton')}
              </Button>
            </div>
          )}
          <MemberListContent
            loadingMembers={loadingMembers}
            members={members}
            currentUserId={currentUserId}
            isAdmin={isAdmin}
            onRemove={handleRemoveMember}
            onRoleChange={handleRoleChange}
            isRemoving={removeMember.isPending}
            isRoleChanging={updateMemberRole.isPending}
            t={t}
          />
        </SettingsCard>
      )}

      {/* Shared projects */}
      {selectedOrg && (
        <SettingsCard
          title={t('sharedProjects.title')}
          description={t('sharedProjects.description', {
            orgName: selectedOrg.name,
          })}
        >
          <RemoteProjectsContent
            loadingProjects={loadingProjects}
            loadingRemoteProjects={loadingRemoteProjects}
            isRemoteProjectUnsupported={isRemoteProjectUnsupported}
            remoteProjectsError={remoteProjectsError}
            remoteProjects={remoteProjects}
            allProjects={allProjects}
            availableLocalProjects={availableLocalProjects}
            onLink={handleLinkProject}
            onUnlink={handleUnlinkProject}
            isLinking={linkToExisting.isPending}
            isUnlinking={unlinkProject.isPending}
            remoteProjectUnsupportedMessage={remoteProjectUnsupportedMessage}
            loadRemoteProjectsErrorMessage={loadRemoteProjectsErrorMessage}
            t={t}
          />
        </SettingsCard>
      )}

      {/* Danger zone */}
      {selectedOrg && isAdmin && !isPersonalOrg && (
        <SettingsCard
          title={t('settings.dangerZone')}
          description={t('settings.dangerZoneDescription')}
          className="border-error"
        >
          <div className="flex items-center justify-between gap-double">
            <div className="flex-1 min-w-0">
              <p className="text-normal text-base font-medium">
                {t('settings.deleteOrganization')}
              </p>
              <p className="text-low text-sm mt-0.5">
                {t('settings.deleteOrganizationDescription')}
              </p>
            </div>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => {
                void handleDeleteOrganization();
              }}
              disabled={deleteOrganization.isPending}
            >
              {deleteOrganization.isPending ? (
                <SpinnerGap
                  className="size-icon-xs animate-spin"
                  weight="bold"
                />
              ) : (
                <Trash className="size-icon-xs" weight="bold" />
              )}
              {t('common:buttons.delete')}
            </Button>
          </div>
        </SettingsCard>
      )}
    </div>
  );
}
