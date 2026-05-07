import { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Sparkles, Code, ChevronDown, HandMetal } from 'lucide-react';
import { BaseCodingAgent, EditorType } from 'shared/types';
import type { EditorConfig, ExecutorProfileId } from 'shared/types';
import { useUserSystem } from '@/components/ConfigProvider';

import { toPrettyCase } from '@/utils/string';
import NiceModal, { useModal } from '@ebay/nice-modal-react';
import { defineModal, type NoProps } from '@/lib/modals';
import { useTranslation } from 'react-i18next';
import { useEditorAvailability } from '@/hooks/useEditorAvailability';
import { EditorAvailabilityIndicator } from '@/components/EditorAvailabilityIndicator';
import { useAgentAvailability } from '@/hooks/useAgentAvailability';
import { AgentAvailabilityIndicator } from '@/components/AgentAvailabilityIndicator';

export type OnboardingResult = {
  profile: ExecutorProfileId;
  editor: EditorConfig;
};

const OnboardingDialogImpl = NiceModal.create<NoProps>(() => {
  const modal = useModal();
  const { profiles, config } = useUserSystem();
  const { t } = useTranslation(['common', 'settings']);
  const defaultLabel = t('defaultLabel', { ns: 'settings' });

  const [profile, setProfile] = useState<ExecutorProfileId>(
    config?.executor_profile || {
      executor: BaseCodingAgent.CLAUDE_CODE,
      variant: null,
    }
  );
  const [editorType, setEditorType] = useState<EditorType>(EditorType.VS_CODE);
  const [customCommand, setCustomCommand] = useState<string>('');

  const editorAvailability = useEditorAvailability(editorType);
  const agentAvailability = useAgentAvailability(profile.executor);

  const handleComplete = () => {
    modal.resolve({
      profile,
      editor: {
        editorType,
        customCommand:
          editorType === EditorType.CUSTOM ? customCommand || null : null,
        remoteSshHost: null,
        remoteSshUser: null,
      },
    } as OnboardingResult);
  };

  const isValid =
    editorType !== EditorType.CUSTOM ||
    (editorType === EditorType.CUSTOM && customCommand.trim() !== '');

  return (
    <Dialog open={modal.visible} uncloseable={true}>
      <DialogContent className="sm:max-w-[600px] space-y-4">
        <DialogHeader>
          <div className="flex items-center gap-3">
            <HandMetal className="h-6 w-6 text-primary text-primary-foreground" />
            <DialogTitle>{t('onboarding.title')}</DialogTitle>
          </div>
          <DialogDescription className="text-left pt-2">
            {t('onboarding.description')}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <h2 className="text-xl flex items-center gap-2">
            <Sparkles className="h-4 w-4" />
            {t('onboarding.chooseAgent')}
          </h2>
          <div className="space-y-2">
            <Label htmlFor="profile">{t('onboarding.defaultAgent')}</Label>
            <div className="flex gap-2">
              <Select
                value={profile.executor}
                onValueChange={(v) =>
                  setProfile({ executor: v as BaseCodingAgent, variant: null })
                }
              >
                <SelectTrigger id="profile" className="flex-1">
                  <SelectValue placeholder={t('onboarding.selectAgentPlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {profiles &&
                    (Object.keys(profiles) as BaseCodingAgent[])
                      .sort((a, b) => a.localeCompare(b))
                      .map((agent) => (
                        <SelectItem key={agent} value={agent}>
                          {agent}
                        </SelectItem>
                      ))}
                </SelectContent>
              </Select>

              {/* Show variant selector if selected profile has variants */}
              {(() => {
                const selectedProfile = profiles?.[profile.executor];
                const hasVariants =
                  selectedProfile && Object.keys(selectedProfile).length > 0;

                if (hasVariants) {
                  return (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="outline"
                          className="w-24 px-2 flex items-center justify-between"
                        >
                          <span className="text-xs truncate flex-1 text-left">
                            {profile.variant || defaultLabel.toUpperCase()}
                          </span>
                          <ChevronDown className="h-3 w-3 ml-1 flex-shrink-0" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent>
                        {Object.keys(selectedProfile).map((variant) => (
                          <DropdownMenuItem
                            key={variant}
                            onClick={() =>
                              setProfile({
                                ...profile,
                                variant: variant,
                              })
                            }
                            className={
                              profile.variant === variant ? 'bg-accent' : ''
                            }
                          >
                            {variant}
                          </DropdownMenuItem>
                        ))}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  );
                } else if (selectedProfile) {
                  // Show disabled button when profile exists but has no variants
                  return (
                    <Button
                      variant="outline"
                      className="w-24 px-2 flex items-center justify-between"
                      disabled
                    >
                      <span className="text-xs truncate flex-1 text-left">
                        {defaultLabel}
                      </span>
                    </Button>
                  );
                }
                return null;
              })()}
            </div>
            <AgentAvailabilityIndicator availability={agentAvailability} />
          </div>
        </div>

        <div className="space-y-2">
          <h2 className="text-xl flex items-center gap-2">
            <Code className="h-4 w-4" />
            {t('onboarding.chooseEditor')}
          </h2>

          <div className="space-y-2">
            <Label htmlFor="editor">{t('onboarding.preferredEditor')}</Label>
            <Select
              value={editorType}
              onValueChange={(value: EditorType) => setEditorType(value)}
            >
              <SelectTrigger id="editor">
                <SelectValue placeholder={t('onboarding.selectEditorPlaceholder')} />
              </SelectTrigger>
              <SelectContent>
                {Object.values(EditorType).map((type) => (
                  <SelectItem key={type} value={type}>
                    {toPrettyCase(type)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {/* Editor availability status indicator */}
            {editorType !== EditorType.CUSTOM && (
              <EditorAvailabilityIndicator availability={editorAvailability} />
            )}

            <p className="text-sm text-muted-foreground">
              {t('onboarding.editorDescription')}
            </p>

            {editorType === EditorType.CUSTOM && (
              <div className="space-y-2">
                <Label htmlFor="custom-command">{t('onboarding.customCommand')}</Label>
                <Input
                  id="custom-command"
                  placeholder={t('onboarding.customCommandPlaceholder')}
                  value={customCommand}
                  onChange={(e) => setCustomCommand(e.target.value)}
                />
                <p className="text-sm text-muted-foreground">
                  {t('onboarding.customCommandDescription')}
                </p>
              </div>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button
            onClick={handleComplete}
            disabled={!isValid}
            className="w-full"
          >
            {t('buttons.continue')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
});

export const OnboardingDialog = defineModal<void, OnboardingResult>(
  OnboardingDialogImpl
);
