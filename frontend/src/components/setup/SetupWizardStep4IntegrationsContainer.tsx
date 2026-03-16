import { useState, useCallback } from 'react';

import { feishuApi } from '@/lib/api';
import { SetupWizardStep4Integrations } from './SetupWizardStep4Integrations';

interface SetupWizardStep4IntegrationsContainerProps {
  onNext: () => void;
  onSkip: () => void;
}

export function SetupWizardStep4IntegrationsContainer({
  onNext,
  onSkip,
}: Readonly<SetupWizardStep4IntegrationsContainerProps>) {
  const [feishuEnabled, setFeishuEnabled] = useState(false);
  const [feishuAppId, setFeishuAppId] = useState('');
  const [feishuAppSecret, setFeishuAppSecret] = useState('');
  const [saving, setSaving] = useState(false);

  const handleNext = useCallback(async () => {
    if (!feishuEnabled || (!feishuAppId.trim() && !feishuAppSecret.trim())) {
      onNext();
      return;
    }

    try {
      setSaving(true);
      await feishuApi.updateConfig({
        appId: feishuAppId.trim(),
        appSecret: feishuAppSecret.trim(),
        enabled: true,
      });
      onNext();
    } catch {
      // Allow user to proceed even if save fails;
      // they can reconfigure later in Settings.
      onNext();
    } finally {
      setSaving(false);
    }
  }, [feishuEnabled, feishuAppId, feishuAppSecret, onNext]);

  const handleSkip = useCallback(() => {
    onSkip();
  }, [onSkip]);

  return (
    <SetupWizardStep4Integrations
      feishuEnabled={feishuEnabled}
      feishuAppId={feishuAppId}
      feishuAppSecret={feishuAppSecret}
      onFeishuEnabledChange={setFeishuEnabled}
      onFeishuAppIdChange={setFeishuAppId}
      onFeishuAppSecretChange={setFeishuAppSecret}
      onNext={saving ? () => {} : handleNext}
      onSkip={handleSkip}
    />
  );
}
