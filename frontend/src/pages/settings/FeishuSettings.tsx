import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshCw, CheckCircle2, XCircle, Loader2 } from 'lucide-react';
import { feishuApi } from '@/lib/api';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';

type FeishuStatus = Awaited<ReturnType<typeof feishuApi.getStatus>>;

export function FeishuSettings() {
  const { t } = useTranslation(['settings']);

  const [status, setStatus] = useState<FeishuStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [appId, setAppId] = useState('');
  const [appSecret, setAppSecret] = useState('');
  const [tenantKey, setTenantKey] = useState('');
  const [baseUrl, setBaseUrl] = useState('https://open.feishu.cn');

  const fetchStatus = useCallback(async () => {
    try {
      setLoading(true);
      const data = await feishuApi.getStatus();
      setStatus(data);
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

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground py-8">
        <Loader2 className="h-4 w-4 animate-spin" />
        {t('settings.general.loading')}
      </div>
    );
  }

  const isConnected = status?.connectionStatus === 'connected';
  const isConfigured = status?.configSummary != null;

  return (
    <div className="space-y-6">
      {/* Connection Status */}
      <Card>
        <CardHeader>
          <CardTitle>{t('settings.feishu.status.title')}</CardTitle>
          <CardDescription>
            {t('settings.feishu.status.description')}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {!status?.featureEnabled && (
            <Alert>
              <AlertDescription>
                {t('settings.feishu.status.featureDisabled')}
              </AlertDescription>
            </Alert>
          )}

          <div className="flex items-center gap-3">
            {isConnected ? (
              <CheckCircle2 className="h-5 w-5 text-green-500" />
            ) : (
              <XCircle className="h-5 w-5 text-destructive" />
            )}
            <span className="text-sm font-medium">
              {isConnected
                ? t('settings.feishu.status.connected')
                : isConfigured
                  ? t('settings.feishu.status.disconnected')
                  : t('settings.feishu.status.notConfigured')}
            </span>
          </div>

          {isConfigured && (
            <Button
              variant="outline"
              size="sm"
              onClick={handleReconnect}
              disabled={reconnecting || !status?.featureEnabled}
            >
              {reconnecting ? (
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
              ) : (
                <RefreshCw className="h-4 w-4 mr-2" />
              )}
              {t('settings.feishu.status.reconnect')}
            </Button>
          )}
        </CardContent>
      </Card>

      {/* Configuration Form */}
      <Card>
        <CardHeader>
          <CardTitle>{t('settings.feishu.form.title')}</CardTitle>
          <CardDescription>
            {t('settings.feishu.form.description')}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          {success && (
            <Alert>
              <AlertDescription>{success}</AlertDescription>
            </Alert>
          )}

          <div className="space-y-2">
            <Label htmlFor="feishu-app-id">
              {t('settings.feishu.form.appId')}
            </Label>
            <Input
              id="feishu-app-id"
              value={appId}
              onChange={(e) => setAppId(e.target.value)}
              placeholder="cli_xxxxxxxxxx"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="feishu-app-secret">
              {t('settings.feishu.form.appSecret')}
            </Label>
            <Input
              id="feishu-app-secret"
              type="password"
              value={appSecret}
              onChange={(e) => setAppSecret(e.target.value)}
              placeholder={
                isConfigured
                  ? t('settings.feishu.form.secretPlaceholderExisting')
                  : t('settings.feishu.form.secretPlaceholderNew')
              }
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="feishu-tenant-key">
              {t('settings.feishu.form.tenantKey')}
            </Label>
            <Input
              id="feishu-tenant-key"
              value={tenantKey}
              onChange={(e) => setTenantKey(e.target.value)}
              placeholder={t('settings.feishu.form.tenantKeyPlaceholder')}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="feishu-base-url">
              {t('settings.feishu.form.baseUrl')}
            </Label>
            <Input
              id="feishu-base-url"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              placeholder="https://open.feishu.cn"
            />
          </div>

          <Button onClick={handleSave} disabled={saving}>
            {saving && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
            {t('settings.feishu.form.save')}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
