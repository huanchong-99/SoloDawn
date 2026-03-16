import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ArrowsClockwise,
  CheckCircle,
  XCircle,
  SpinnerGap,
  Warning,
} from '@phosphor-icons/react';
import { feishuApi, makeRequest, handleApiResponse } from '@/lib/api';
import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { SettingsToggle } from '@/components/ui-new/primitives/SettingsToggle';
import { SettingsSection } from '@/components/ui-new/primitives/SettingsSection';
import { SettingsInput } from '@/components/ui-new/primitives/SettingsInput';
import { SettingsRow } from '@/components/ui-new/primitives/SettingsRow';
import { ErrorAlert } from '@/components/ui-new/primitives/ErrorAlert';
import { PrimaryButton } from '@/components/ui-new/primitives/PrimaryButton';

type FeishuStatus = Awaited<ReturnType<typeof feishuApi.getStatus>>;

async function updateSystemSettings(
  payload: Record<string, unknown>
): Promise<void> {
  const response = await makeRequest('/api/system-settings', {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
  await handleApiResponse(response);
}

export function FeishuSettingsNew() {
  const { t } = useTranslation(['settings']);

  /* ------------------------------------------------------------------ */
  /*  State                                                              */
  /* ------------------------------------------------------------------ */
  const [status, setStatus] = useState<FeishuStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [togglingEnabled, setTogglingEnabled] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [feishuEnabled, setFeishuEnabled] = useState(false);
  const [appId, setAppId] = useState('');
  const [appSecret, setAppSecret] = useState('');
  const [tenantKey, setTenantKey] = useState('');
  const [baseUrl, setBaseUrl] = useState('https://open.feishu.cn');

  /* ------------------------------------------------------------------ */
  /*  Data fetching                                                      */
  /* ------------------------------------------------------------------ */
  const fetchStatus = useCallback(async () => {
    try {
      setLoading(true);
      const data = await feishuApi.getStatus();
      setStatus(data);
      setFeishuEnabled(data.featureEnabled);
      if (data.configSummary) {
        setAppId(data.configSummary.appId);
        setBaseUrl(data.configSummary.baseUrl);
        setTenantKey(data.configSummary.tenantKey || '');
      }
    } catch {
      setError(t('settings.feishu.loadError'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

  /* ------------------------------------------------------------------ */
  /*  Handlers                                                           */
  /* ------------------------------------------------------------------ */
  const handleToggleEnabled = async (checked: boolean) => {
    try {
      setTogglingEnabled(true);
      setError(null);
      setSuccess(null);
      await updateSystemSettings({ feishu_enabled: checked });
      setFeishuEnabled(checked);
      await fetchStatus();
    } catch {
      setError(t('settings.feishu.form.saveError'));
    } finally {
      setTogglingEnabled(false);
    }
  };

  const handleSave = async () => {
    if (!appId.trim() || !appSecret.trim()) {
      setError(t('settings.feishu.form.requiredFields'));
      return;
    }
    try {
      setSaving(true);
      setError(null);
      setSuccess(null);
      await feishuApi.updateConfig({
        appId: appId.trim(),
        appSecret: appSecret.trim(),
        tenantKey: tenantKey.trim() || undefined,
        baseUrl: baseUrl.trim() || undefined,
        enabled: true,
      });
      setSuccess(t('settings.feishu.form.saveSuccess'));
      setAppSecret('');
      // Trigger backend reconnect with new config
      try {
        await feishuApi.reconnect();
      } catch {
        // Reconnect failure is non-fatal; status refresh will show actual state
      }
      await fetchStatus();
    } catch {
      setError(t('settings.feishu.form.saveError'));
    } finally {
      setSaving(false);
    }
  };

  const handleReconnect = async () => {
    try {
      setReconnecting(true);
      setError(null);
      await feishuApi.reconnect();
      setSuccess(t('settings.feishu.reconnectSuccess'));
      setTimeout(() => fetchStatus(), 2000);
    } catch {
      setError(t('settings.feishu.reconnectError'));
    } finally {
      setReconnecting(false);
    }
  };

  /* ------------------------------------------------------------------ */
  /*  Loading state                                                      */
  /* ------------------------------------------------------------------ */
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-low py-double">
        <SpinnerGap className="size-icon-sm animate-spin" />
        <span className="text-base">{t('settings.general.loading')}</span>
      </div>
    );
  }

  const isConnected = status?.connectionStatus === 'connected';
  const isConfigured = status?.configSummary != null;

  const connectionStatusIndicator = (() => {
    if (isConnected) {
      return (
        <>
          <CheckCircle
            className="size-icon-sm text-success"
            weight="fill"
          />
          <span className="text-base text-success">
            {t('settings:newDesign.feishu.connected')}
          </span>
        </>
      );
    }
    if (isConfigured) {
      return (
        <>
          <XCircle
            className="size-icon-sm text-error"
            weight="fill"
          />
          <span className="text-base text-error">
            {t('settings:newDesign.feishu.disconnected')}
          </span>
        </>
      );
    }
    return (
      <>
        <Warning
          className="size-icon-sm text-low"
          weight="fill"
        />
        <span className="text-base text-low">
          {t('settings:newDesign.feishu.notConfigured')}
        </span>
      </>
    );
  })();

  /* ------------------------------------------------------------------ */
  /*  Render                                                             */
  /* ------------------------------------------------------------------ */
  return (
    <div className="space-y-base">
      {/* Enable / Disable toggle */}
      <SettingsCard
        title={t('settings:newDesign.nav.feishu')}
        description={t('settings.feishu.status.description')}
      >
        <SettingsToggle
          label={t('settings:newDesign.feishu.enableToggle')}
          description={t('settings:newDesign.feishu.enableDescription')}
          checked={feishuEnabled}
          onChange={handleToggleEnabled}
          disabled={togglingEnabled}
        />
      </SettingsCard>

      {/* Connection Status — always visible when enabled */}
      {feishuEnabled && (
        <SettingsCard
          title={t('settings:newDesign.feishu.connectionStatus')}
        >
          <SettingsSection>
            <SettingsRow
              label={t('settings:newDesign.feishu.connectionStatus')}
            >
              <div className="flex items-center gap-2">
                {connectionStatusIndicator}
              </div>
            </SettingsRow>

            {isConfigured && (
              <div className="flex justify-end">
                <PrimaryButton
                  variant="tertiary"
                  onClick={handleReconnect}
                  disabled={reconnecting}
                  actionIcon={reconnecting ? 'spinner' : ArrowsClockwise}
                  value={t('settings:newDesign.feishu.reconnect')}
                />
              </div>
            )}
          </SettingsSection>
        </SettingsCard>
      )}

      {/* Configuration form — only when integration is enabled */}
      {feishuEnabled && (
        <SettingsCard
          title={t('settings.feishu.form.title')}
          description={t('settings.feishu.form.description')}
        >
          {error && <ErrorAlert message={error} className="mb-base" />}

          {success && (
            <div className="mb-base rounded border border-success/30 bg-success/10 p-base text-sm text-success">
              {success}
            </div>
          )}

          <SettingsSection>
            <SettingsInput
              label={t('settings.feishu.form.appId')}
              value={appId}
              onChange={setAppId}
              placeholder="cli_xxxxxxxxxx"
            />

            <SettingsInput
              label={t('settings.feishu.form.appSecret')}
              type="password"
              value={appSecret}
              onChange={setAppSecret}
              placeholder={
                isConfigured
                  ? t('settings.feishu.form.secretPlaceholderExisting')
                  : t('settings.feishu.form.secretPlaceholderNew')
              }
            />

            <SettingsInput
              label={t('settings.feishu.form.tenantKey')}
              value={tenantKey}
              onChange={setTenantKey}
              placeholder={t('settings.feishu.form.tenantKeyPlaceholder')}
            />

            <SettingsInput
              label={t('settings.feishu.form.baseUrl')}
              type="url"
              value={baseUrl}
              onChange={setBaseUrl}
              placeholder="https://open.feishu.cn"
            />
          </SettingsSection>

          <div className="mt-base flex justify-end">
            <PrimaryButton
              onClick={handleSave}
              disabled={saving}
              actionIcon={saving ? 'spinner' : undefined}
              value={t('settings.feishu.form.save')}
            />
          </div>
        </SettingsCard>
      )}
    </div>
  );
}
