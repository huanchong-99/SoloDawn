import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  SpinnerGap,
  Check,
  CaretLeft,
  CaretRight,
  Warning,
} from '@phosphor-icons/react';
import { BaseCodingAgent, McpConfig } from 'shared/types';
import { useUserSystem } from '@/components/ConfigProvider';
import { ApiError, mcpServersApi } from '@/lib/api';
import { McpConfigStrategyGeneral } from '@/lib/mcpStrategies';
import { JSONEditor } from '@/components/ui/json-editor';
import { SettingsCard } from '@/components/ui-new/primitives/SettingsCard';
import { SettingsSection } from '@/components/ui-new/primitives/SettingsSection';
import { SettingsSelect } from '@/components/ui-new/primitives/SettingsSelect';
import { Button } from '@/components/ui-new/primitives/Button';
import { ErrorAlert } from '@/components/ui-new/primitives/ErrorAlert';
import { Label } from '@/components/ui-new/primitives/Label';
import { cn } from '@/lib/utils';

const MCP_NOT_SUPPORTED_ERROR_CODE = 'MCP_NOT_SUPPORTED';

interface McpUiError {
  code: string | null;
  message: string;
}

function isMcpUiErrorData(
  data: unknown
): data is { code?: string | null; message?: string } {
  if (!data || typeof data !== 'object') {
    return false;
  }

  const candidate = data as { code?: unknown; message?: unknown };
  const codeOk =
    candidate.code === undefined ||
    candidate.code === null ||
    typeof candidate.code === 'string';
  const messageOk =
    candidate.message === undefined || typeof candidate.message === 'string';

  return codeOk && messageOk;
}

export const buildMcpServersPayload = (
  editorValue: string,
  mcpConfig: McpConfig
): McpConfig['servers'] => {
  if (!editorValue.trim()) {
    return {};
  }

  const fullConfig = JSON.parse(editorValue);
  McpConfigStrategyGeneral.validateFullConfig(mcpConfig, fullConfig);
  return McpConfigStrategyGeneral.extractServersForApi(mcpConfig, fullConfig);
};

export function McpSettingsNew() {
  const { t } = useTranslation('settings');
  const { config, profiles } = useUserSystem();
  const [mcpServers, setMcpServers] = useState('{}');
  const [mcpConfig, setMcpConfig] = useState<McpConfig | null>(null);
  const [mcpError, setMcpError] = useState<McpUiError | null>(null);
  const [mcpLoading, setMcpLoading] = useState(true);
  const [selectedProfileKey, setSelectedProfileKey] =
    useState<BaseCodingAgent | null>(null);
  const [mcpApplying, setMcpApplying] = useState(false);
  const [mcpConfigPath, setMcpConfigPath] = useState<string>('');
  const [success, setSuccess] = useState(false);
  const [carouselOffset, setCarouselOffset] = useState(0);

  const toMcpUiError = (err: unknown, fallbackMessage: string): McpUiError => {
    if (
      err instanceof ApiError &&
      err.error_data &&
      isMcpUiErrorData(err.error_data)
    ) {
      if (err.error_data.code || err.error_data.message) {
        return {
          code: err.error_data.code ?? null,
          message: err.error_data.message ?? err.message,
        };
      }
    }

    if (err instanceof Error) {
      return { code: null, message: err.message };
    }

    return { code: null, message: fallbackMessage };
  };

  // Initialize selected profile when config loads
  useEffect(() => {
    if (!profiles || selectedProfileKey) {
      return;
    }

    const currentExecutor = config?.executor_profile?.executor;
    if (currentExecutor && profiles[currentExecutor]) {
      setSelectedProfileKey(currentExecutor as BaseCodingAgent);
      return;
    }

    const firstProfileKey = Object.keys(profiles)[0];
    if (firstProfileKey) {
      setSelectedProfileKey(firstProfileKey as BaseCodingAgent);
    }
  }, [config?.executor_profile, profiles, selectedProfileKey]);

  // Load existing MCP configuration when selected profile changes
  useEffect(() => {
    const loadMcpServersForProfile = async (profileKey: BaseCodingAgent) => {
      setMcpLoading(true);
      setMcpError(null);
      setMcpConfigPath('');

      try {
        const result = await mcpServersApi.load({
          executor: profileKey,
        });
        setMcpConfig(result.mcp_config);
        const fullConfig = McpConfigStrategyGeneral.createFullConfig(
          result.mcp_config
        );
        const configJson = JSON.stringify(fullConfig, null, 2);
        setMcpServers(configJson);
        setMcpConfigPath(result.config_path);
      } catch (err: unknown) {
        setMcpError(toMcpUiError(err, t('settings.mcp.errors.loadFailed')));
        console.error('Error loading MCP servers:', err);
      } finally {
        setMcpLoading(false);
      }
    };

    if (selectedProfileKey) {
      loadMcpServersForProfile(selectedProfileKey);
    }
  }, [selectedProfileKey, t]);

  const handleMcpServersChange = (value: string) => {
    setMcpServers(value);
    setMcpError(null);

    if (value.trim() && mcpConfig) {
      try {
        const parsedConfig = JSON.parse(value);
        McpConfigStrategyGeneral.validateFullConfig(mcpConfig, parsedConfig);
      } catch (err) {
        if (err instanceof SyntaxError) {
          setMcpError({
            code: null,
            message: t('settings.mcp.errors.invalidJson'),
          });
        } else {
          setMcpError({
            code: null,
            message:
              err instanceof Error
                ? err.message
                : t('settings.mcp.errors.validationError'),
          });
        }
      }
    }
  };

  const handleApplyMcpServers = async () => {
    if (!selectedProfileKey || !mcpConfig) return;

    setMcpApplying(true);
    setMcpError(null);

    try {
      try {
        const mcpServersConfig = buildMcpServersPayload(mcpServers, mcpConfig);

        await mcpServersApi.save(
          {
            executor: selectedProfileKey,
          },
          { servers: mcpServersConfig }
        );

        setSuccess(true);
        setTimeout(() => setSuccess(false), 3000);
      } catch (mcpErr) {
        if (mcpErr instanceof SyntaxError) {
          setMcpError({
            code: null,
            message: t('settings.mcp.errors.invalidJson'),
          });
        } else {
          setMcpError(
            toMcpUiError(mcpErr, t('settings.mcp.errors.saveFailed'))
          );
        }
      }
    } catch (err) {
      setMcpError({
        code: null,
        message: t('settings.mcp.errors.applyFailed'),
      });
      console.error('Error applying MCP servers:', err);
    } finally {
      setMcpApplying(false);
    }
  };

  const addServer = (key: string) => {
    if (!mcpConfig) {
      return;
    }

    try {
      const existing = mcpServers.trim() ? JSON.parse(mcpServers) : {};
      const updated = McpConfigStrategyGeneral.addPreconfiguredToConfig(
        mcpConfig,
        existing,
        key
      );
      setMcpServers(JSON.stringify(updated, null, 2));
      setMcpError(null);
    } catch (err) {
      console.error(err);
      setMcpError({
        code: null,
        message:
          err instanceof Error
            ? err.message
            : t('settings.mcp.errors.addServerFailed'),
      });
    }
  };

  const isMcpUnsupported = mcpError?.code === MCP_NOT_SUPPORTED_ERROR_CODE;

  const preconfiguredObj = (mcpConfig?.preconfigured || {}) as Record<
    string,
    unknown
  >;
  const meta =
    typeof preconfiguredObj.meta === 'object' && preconfiguredObj.meta !== null
      ? (preconfiguredObj.meta as Record<
          string,
          { name?: string; description?: string; url?: string; icon?: string }
        >)
      : {};
  const servers = Object.fromEntries(
    Object.entries(preconfiguredObj).filter(([k]) => k !== 'meta')
  ) as Record<string, unknown>;
  const getMetaFor = (key: string) => meta[key] || {};

  const serverEntries = Object.entries(servers);
  const visibleCount = 4;
  const maxOffset = Math.max(0, serverEntries.length - visibleCount);
  const canScrollLeft = carouselOffset > 0;
  const canScrollRight = carouselOffset < maxOffset;

  if (!config) {
    return (
      <div className="py-double">
        <ErrorAlert message={t('settings.mcp.errors.loadFailed')} />
      </div>
    );
  }

  const profileOptions = profiles
    ? Object.keys(profiles)
        .sort((a, b) => a.localeCompare(b))
        .map((profileKey) => ({
          value: profileKey,
          label: profileKey,
        }))
    : [];

  return (
    <div className="space-y-base">
      {mcpError && (
        <ErrorAlert
          message={t('settings.mcp.errors.mcpError', {
            error: mcpError.message,
          })}
        />
      )}

      {success && (
        <output
          className="relative w-full border border-success bg-success/10 p-base text-sm text-success block"
        >
          {t('settings.mcp.save.successMessage')}
        </output>
      )}

      <SettingsCard
        title={t('settings.mcp.title')}
        description={t('settings.mcp.description')}
      >
        <SettingsSection>
          <SettingsSelect
            label={t('settings.mcp.labels.agent')}
            description={t('settings.mcp.labels.agentHelper')}
            value={selectedProfileKey ?? ''}
            onChange={(value: string) => {
              if (!profiles?.[value]) return;
              setSelectedProfileKey(value as BaseCodingAgent);
            }}
            options={profileOptions}
            placeholder={t('settings.mcp.labels.agentPlaceholder')}
          />

          {isMcpUnsupported ? (
            <div className="rounded border border-[hsl(40,80%,50%)]/30 bg-[hsl(40,80%,50%)]/10 p-base">
              <div className="flex gap-base">
                <Warning
                  className="size-icon-sm text-[hsl(40,80%,50%)] shrink-0 mt-0.5"
                  weight="fill"
                />
                <div>
                  <h3 className="text-sm font-medium text-high">
                    {t('settings.mcp.errors.notSupported')}
                  </h3>
                  <div className="mt-1 text-sm text-low">
                    <p>{mcpError?.message}</p>
                    <p className="mt-1">
                      {t('settings.mcp.errors.supportMessage')}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          ) : (
            <div className="space-y-base">
              <div className="space-y-1">
                <Label htmlFor="mcp-servers">
                  {t('settings.mcp.labels.serverConfig')}
                </Label>
                <JSONEditor
                  id="mcp-servers"
                  placeholder={
                    mcpLoading
                      ? t('settings.mcp.save.loading')
                      : '{\n  "server-name": {\n    "type": "stdio",\n    "command": "your-command",\n    "args": ["arg1", "arg2"]\n  }\n}'
                  }
                  value={
                    mcpLoading
                      ? t('settings.mcp.loading.jsonEditor')
                      : mcpServers
                  }
                  onChange={handleMcpServersChange}
                  disabled={mcpLoading}
                  minHeight={300}
                />
                {mcpError && !isMcpUnsupported && (
                  <p className="text-sm text-error">{mcpError.message}</p>
                )}
                <div className="text-sm text-low">
                  {mcpLoading ? (
                    t('settings.mcp.loading.configuration')
                  ) : (
                    <span>
                      {t('settings.mcp.labels.saveLocation')}
                      {mcpConfigPath && (
                        <span className="ml-half font-ibm-plex-mono text-xs">
                          {mcpConfigPath}
                        </span>
                      )}
                    </span>
                  )}
                </div>
              </div>

              {mcpConfig?.preconfigured &&
                typeof mcpConfig.preconfigured === 'object' && (
                  <div className="pt-base">
                    <Label>{t('settings.mcp.labels.popularServers')}</Label>
                    <p className="text-sm text-low mb-base">
                      {t('settings.mcp.labels.serverHelper')}
                    </p>

                    <div className="relative overflow-hidden rounded border border-border bg-secondary">
                      <div className="w-full px-base py-base">
                        <div
                          className="flex gap-base transition-transform duration-300"
                          style={{
                            transform: `translateX(-${carouselOffset * (100 / visibleCount)}%)`,
                          }}
                        >
                          {serverEntries.map(([key]) => {
                            const metaObj = getMetaFor(key) as {
                              name?: string;
                              description?: string;
                              url?: string;
                              icon?: string;
                            };
                            const name = metaObj.name || key;
                            const description =
                              metaObj.description || 'No description';
                            const icon = metaObj.icon
                              ? `/${metaObj.icon}`
                              : null;

                            return (
                              <button
                                key={name}
                                type="button"
                                onClick={() => addServer(key)}
                                aria-label={`Add ${name} to config`}
                                className={cn(
                                  'group flex-none text-left outline-none',
                                  'rounded border border-border bg-panel p-base',
                                  'hover:border-brand/40 hover:shadow-soft transition-all duration-200',
                                  'focus-visible:ring-1 focus-visible:ring-brand'
                                )}
                                style={{
                                  width: `calc((100% - ${(visibleCount - 1) * 12}px) / ${visibleCount})`,
                                }}
                              >
                                <div className="flex items-center gap-base mb-1">
                                  <div className="size-6 rounded border border-border bg-secondary grid place-items-center overflow-hidden shrink-0">
                                    {icon ? (
                                      <img
                                        src={icon}
                                        alt=""
                                        className="w-full h-full object-cover"
                                      />
                                    ) : (
                                      <span className="text-sm font-semibold text-normal">
                                        {name.slice(0, 1).toUpperCase()}
                                      </span>
                                    )}
                                  </div>
                                  <span className="text-base font-medium text-high truncate">
                                    {name}
                                  </span>
                                </div>
                                <p className="text-sm text-low line-clamp-3">
                                  {description}
                                </p>
                              </button>
                            );
                          })}
                        </div>
                      </div>

                      {canScrollLeft && (
                        <button
                          type="button"
                          onClick={() =>
                            setCarouselOffset((prev) => Math.max(0, prev - 1))
                          }
                          className="absolute left-1 top-1/2 -translate-y-1/2 size-7 rounded-full border border-border bg-panel/80 backdrop-blur grid place-items-center text-low hover:text-normal transition-colors duration-200"
                          aria-label="Previous servers"
                        >
                          <CaretLeft
                            className="size-icon-xs"
                            weight="bold"
                          />
                        </button>
                      )}
                      {canScrollRight && (
                        <button
                          type="button"
                          onClick={() =>
                            setCarouselOffset((prev) =>
                              Math.min(maxOffset, prev + 1)
                            )
                          }
                          className="absolute right-1 top-1/2 -translate-y-1/2 size-7 rounded-full border border-border bg-panel/80 backdrop-blur grid place-items-center text-low hover:text-normal transition-colors duration-200"
                          aria-label="Next servers"
                        >
                          <CaretRight
                            className="size-icon-xs"
                            weight="bold"
                          />
                        </button>
                      )}
                    </div>
                  </div>
                )}
            </div>
          )}
        </SettingsSection>
      </SettingsCard>

      {/* Sticky Save Button */}
      <div className="sticky bottom-0 z-10 bg-panel/80 backdrop-blur border-t border-border py-base">
        <div className="flex justify-end">
          <Button
            variant="primary"
            size="md"
            onClick={handleApplyMcpServers}
            disabled={mcpApplying || mcpLoading || !!mcpError || success}
          >
            {mcpApplying && (
              <SpinnerGap
                className="size-icon-xs animate-spin"
                weight="bold"
              />
            )}
            {success && (
              <Check className="size-icon-xs" weight="bold" />
            )}
            {success
              ? t('settings.mcp.save.success')
              : t('settings.mcp.save.button')}
          </Button>
        </div>
      </div>
    </div>
  );
}
