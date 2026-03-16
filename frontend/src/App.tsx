import { useEffect } from 'react';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';
import i18n from '@/i18n';
import { SlashCommands } from '@/pages/SlashCommands';
import { usePostHog } from 'posthog-js/react';
import { useAuth } from '@/hooks';
import { usePreviousPath } from '@/hooks/usePreviousPath';

import { SetupWizardShell } from '@/components/setup';
import { SettingsLayoutContainer } from '@/components/ui-new/containers/SettingsLayoutContainer';
import {
  GeneralSettingsNew,
  ModelsSettingsNew,
  ProjectSettingsNew,
  ReposSettingsNew,
  AgentSettingsNew,
  McpSettingsNew,
  FeishuSettingsNew,
  OrganizationSettingsNew,
} from '@/pages/ui-new/settings';
import { UserSystemProvider, useUserSystem } from '@/components/ConfigProvider';
import { ThemeProvider } from '@/components/ThemeProvider';
import { SearchProvider } from '@/contexts/SearchContext';

import { HotkeysProvider } from 'react-hotkeys-hook';

import { ProjectProvider } from '@/contexts/ProjectContext';
import { ThemeMode } from 'shared/types';
import * as Sentry from '@sentry/react';

import { DisclaimerDialog } from '@/components/dialogs/global/DisclaimerDialog';
import { ReleaseNotesDialog } from '@/components/dialogs/global/ReleaseNotesDialog';
import { ClickedElementsProvider } from './contexts/ClickedElementsProvider';

import { LegacyDesignScope } from '@/components/legacy-design/LegacyDesignScope';
import { NewDesignScope } from '@/components/ui-new/scope/NewDesignScope';
import { NormalLayout } from '@/components/layout/NormalLayout';
import { NewDesignLayout } from '@/components/layout/NewDesignLayout';

// GitCortex pages
import { Board } from '@/pages/Board';
import { Pipeline } from '@/pages/Pipeline';
import { WorkflowDebugPage } from '@/pages/WorkflowDebugPage';
import { Workflows } from '@/pages/Workflows';

// New design pages
import { Workspaces } from '@/pages/ui-new/Workspaces';
import { WorkspacesLanding } from '@/pages/ui-new/WorkspacesLanding';

const SentryRoutes = Sentry.withSentryReactRouterV6Routing(Routes);

function AppContent() {
  const { config, analyticsUserId, updateAndSaveConfig } = useUserSystem();
  const posthog = usePostHog();
  const { isSignedIn } = useAuth();

  usePreviousPath();

  useEffect(() => {
    if (!posthog || !analyticsUserId) return;

    if (config?.analytics_enabled) {
      posthog.opt_in_capturing();
      posthog.identify(analyticsUserId);
      console.log('[Analytics] Analytics enabled and user identified');
    } else {
      posthog.opt_out_capturing();
      console.log('[Analytics] Analytics disabled by user preference');
    }
  }, [config?.analytics_enabled, analyticsUserId, posthog]);

  useEffect(() => {
    if (!config) return;
    let cancelled = false;

    const showNextStep = async () => {
      if (!config.disclaimer_acknowledged) {
        await DisclaimerDialog.show();
        if (!cancelled) {
          await updateAndSaveConfig({ disclaimer_acknowledged: true });
        }
        DisclaimerDialog.hide();
      } else if (!(config as Record<string, unknown>).setup_wizard_completed) {
        // New setup wizard replaces old onboarding dialog
        if (globalThis.location.pathname !== '/setup') {
          globalThis.location.href = '/setup';
        }
        return;
      } else if (config.show_release_notes) {
        await ReleaseNotesDialog.show();
        if (!cancelled) {
          await updateAndSaveConfig({ show_release_notes: false });
        }
        ReleaseNotesDialog.hide();
      }
    };

    showNextStep();

    return () => {
      cancelled = true;
    };
  }, [config, isSignedIn, updateAndSaveConfig]);

  return (
    <I18nextProvider i18n={i18n}>
      <ThemeProvider initialTheme={config?.theme || ThemeMode.SYSTEM}>
        <SearchProvider>
          <SentryRoutes>
            {/* ========== GITCORTEX DESIGN ROUTES ========== */}
            <Route
              path="/"
              element={
                <NewDesignScope>
                  <NewDesignLayout />
                </NewDesignScope>
              }
            >
              {/* Manual Workflow mode */}
              <Route index element={<Navigate to="/board" replace />} />
              <Route path="board" element={<Board />} />
              <Route path="wizard" element={<Workflows />} />
              <Route path="workflows" element={<Workflows />} />
              <Route path="pipeline/:workflowId" element={<Pipeline />} />
              <Route path="debug/:workflowId" element={<WorkflowDebugPage />} />

              {/* Orchestrated Workspace mode */}
              <Route path="workspaces" element={<WorkspacesLanding />} />
              <Route path="workspaces/create" element={<Workspaces />} />
              <Route path="workspaces/:workspaceId" element={<Workspaces />} />
            </Route>

            {/* ========== SETUP WIZARD (full-page, no layout) ========== */}
            <Route
              path="/setup"
              element={
                <NewDesignScope>
                  <SetupWizardShell />
                </NewDesignScope>
              }
            />

            {/* ========== NEW DESIGN SETTINGS ========== */}
            <Route
              path="/settings"
              element={
                <NewDesignScope>
                  <SettingsLayoutContainer />
                </NewDesignScope>
              }
            >
              <Route index element={<Navigate to="/settings/general" replace />} />
              <Route path="general" element={<GeneralSettingsNew />} />
              <Route path="projects" element={<ProjectSettingsNew />} />
              <Route path="repos" element={<ReposSettingsNew />} />
              <Route path="agents" element={<AgentSettingsNew />} />
              <Route path="models" element={<ModelsSettingsNew />} />
              <Route path="mcp" element={<McpSettingsNew />} />
              <Route path="feishu" element={<FeishuSettingsNew />} />
              <Route path="organizations" element={<OrganizationSettingsNew />} />
            </Route>

            {/* ========== LEGACY DESIGN ROUTES ========== */}
            <Route
              element={
                <LegacyDesignScope>
                  <NormalLayout />
                </LegacyDesignScope>
              }
            >
              <Route path="/commands" element={<SlashCommands />} />
              <Route
                path="/mcp-servers"
                element={<Navigate to="/settings/mcp" replace />}
              />
              <Route
                path="/projects"
                element={<Navigate to="/board" replace />}
              />
            </Route>
          </SentryRoutes>
        </SearchProvider>
      </ThemeProvider>
    </I18nextProvider>
  );
}

function App() {
  return (
    <BrowserRouter>
      <UserSystemProvider>
        <ClickedElementsProvider>
          <ProjectProvider>
            <HotkeysProvider initiallyActiveScopes={['*', 'global', 'kanban']}>
              <AppContent />
            </HotkeysProvider>
          </ProjectProvider>
        </ClickedElementsProvider>
      </UserSystemProvider>
    </BrowserRouter>
  );
}

export default App;
