import { useCallback, useMemo } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  GearSixIcon,
  FolderSimpleIcon,
  GitBranchIcon,
  RobotIcon,
  BrainIcon,
  PlugsIcon,
  ChatTeardropDotsIcon,
  BuildingsIcon,
} from '@phosphor-icons/react';
import { useUserSystem } from '@/components/ConfigProvider';
import { SettingsLayout, type SettingsNavItem } from '../views/SettingsLayout';

export function SettingsLayoutContainer() {
  const { t } = useTranslation(['settings']);
  const location = useLocation();
  const navigate = useNavigate();
  const { remoteFeaturesEnabled, updateAndSaveConfig } = useUserSystem();

  const navItems: SettingsNavItem[] = useMemo(
    () => [
      {
        path: 'general',
        label: t('settings:newDesign.nav.general'),
        icon: GearSixIcon,
      },
      {
        path: 'projects',
        label: t('settings:newDesign.nav.projects'),
        icon: FolderSimpleIcon,
      },
      {
        path: 'repos',
        label: t('settings:newDesign.nav.repos'),
        icon: GitBranchIcon,
      },
      {
        path: 'agents',
        label: t('settings:newDesign.nav.agents'),
        icon: RobotIcon,
      },
      {
        path: 'models',
        label: t('settings:newDesign.nav.models'),
        icon: BrainIcon,
      },
      {
        path: 'mcp',
        label: t('settings:newDesign.nav.mcp'),
        icon: PlugsIcon,
      },
      {
        path: 'feishu',
        label: t('settings:newDesign.nav.feishu'),
        icon: ChatTeardropDotsIcon,
      },
      {
        path: 'organizations',
        label: t('settings:newDesign.nav.organizations'),
        icon: BuildingsIcon,
        visible: remoteFeaturesEnabled,
      },
    ],
    [t, remoteFeaturesEnabled]
  );

  const handleNavigate = useCallback(
    (path: string) => {
      navigate(path);
    },
    [navigate]
  );

  const handleRerunSetup = useCallback(async () => {
    await updateAndSaveConfig({
      onboarding_acknowledged: false,
    });
    navigate('/');
  }, [updateAndSaveConfig, navigate]);

  return (
    <SettingsLayout
      navItems={navItems}
      currentPath={location.pathname}
      onNavigate={handleNavigate}
      onRerunSetup={handleRerunSetup}
      rerunSetupLabel={t('settings:newDesign.layout.rerunSetup')}
      title={t('settings:newDesign.layout.title')}
    >
      <Outlet />
    </SettingsLayout>
  );
}
