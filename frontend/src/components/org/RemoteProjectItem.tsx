import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Unlink } from 'lucide-react';
import type { Project, RemoteProject } from 'shared/types';
import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@/components/ui-new/dialogs/ConfirmDialog';

interface RemoteProjectItemProps {
  remoteProject: RemoteProject;
  linkedLocalProject?: Project;
  availableLocalProjects: Project[];
  onLink: (remoteProjectId: string, localProjectId: string) => void;
  onUnlink: (localProjectId: string) => void;
  isLinking: boolean;
  isUnlinking: boolean;
  disabled?: boolean;
  disabledReason?: string;
}

export function RemoteProjectItem({
  remoteProject,
  linkedLocalProject,
  availableLocalProjects,
  onLink,
  onUnlink,
  isLinking,
  isUnlinking,
  disabled = false,
  disabledReason,
}: Readonly<RemoteProjectItemProps>) {
  const { t } = useTranslation('organization');
  const handleUnlinkClick = async () => {
    if (!linkedLocalProject || disabled) return;

    try {
      const result = await ConfirmDialog.show({
        title: t('sharedProjects.confirmUnlinkTitle'),
        message: t('sharedProjects.confirmUnlink', { projectName: linkedLocalProject.name }),
        variant: 'destructive',
      });
      if (result === 'confirmed') {
        onUnlink(linkedLocalProject.id);
      }
    } catch {
      // User cancelled
    }
  };

  const handleLinkSelect = (localProjectId: string) => {
    if (disabled) return;
    if (!availableLocalProjects.some((p) => p.id === localProjectId)) return;
    onLink(remoteProject.id, localProjectId);
  };

  return (
    <div className="flex items-center justify-between p-3 border rounded-lg">
      <div className="flex items-center gap-3 flex-1 min-w-0">
        <div className="flex-1 min-w-0">
          <div className="font-medium text-sm">{remoteProject.name}</div>
          {linkedLocalProject ? (
            <div className="text-xs text-muted-foreground">
              {t('sharedProjects.linkedTo', {
                projectName: linkedLocalProject.name,
              })}
            </div>
          ) : (
            <div className="text-xs text-muted-foreground">
              {t('sharedProjects.notLinked')}
            </div>
          )}
        </div>
        {linkedLocalProject && (
          <Badge variant="default">{t('sharedProjects.linked')}</Badge>
        )}
        {disabled && (
          <Badge variant="outline">
            {t('sharedProjects.actionsUnavailable', {
              defaultValue: 'Actions unavailable',
            })}
          </Badge>
        )}
      </div>
      <div className="flex items-center gap-2">
        {linkedLocalProject ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={handleUnlinkClick}
            disabled={isUnlinking || disabled}
            title={disabled ? disabledReason : undefined}
          >
            <Unlink className="h-4 w-4 text-destructive" />
          </Button>
        ) : (
          <Select
            onValueChange={handleLinkSelect}
            disabled={
              isLinking || availableLocalProjects.length === 0 || disabled
            }
          >
            <SelectTrigger
              className="w-[180px]"
              title={disabled ? disabledReason : undefined}
            >
              <SelectValue placeholder={t('sharedProjects.linkProject')} />
            </SelectTrigger>
            <SelectContent>
              {availableLocalProjects.length === 0 ? (
                <SelectItem value="no-projects" disabled>
                  {t('sharedProjects.noAvailableProjects')}
                </SelectItem>
              ) : (
                availableLocalProjects.map((project) => (
                  <SelectItem key={project.id} value={project.id}>
                    {project.name}
                  </SelectItem>
                ))
              )}
            </SelectContent>
          </Select>
        )}
      </div>
    </div>
  );
}
