import { screen, waitFor, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';

const listSourcesMock = vi.fn();
const syncSourceMock = vi.fn();
const updateSourceMock = vi.fn();
const removeSourceMock = vi.fn();
const makeRequestMock = vi.fn();

vi.mock('@/lib/api', () => ({
  architectureApi: {
    listSources: (...args: unknown[]) => listSourcesMock(...args),
    createSource: vi.fn(),
    updateSource: (...args: unknown[]) => updateSourceMock(...args),
    removeSource: (...args: unknown[]) => removeSourceMock(...args),
    syncSource: (...args: unknown[]) => syncSourceMock(...args),
    listEntries: vi.fn(),
  },
  makeRequest: (...args: unknown[]) => makeRequestMock(...args),
  handleApiResponse: async (value: unknown) => value,
}));

import { ArchitectureSettingsNew } from '../ArchitectureSettingsNew';
import { renderWithI18n, setTestLanguage } from '@/test/renderWithI18n';

const BUILTIN_SOURCE = {
  id: 'src-1',
  name: 'Awesome Architecture',
  owner: 'study8677',
  repo: 'awesome-architecture',
  branch: 'main',
  includePaths: ['templates/'],
  enabled: true,
  builtin: true,
  lastSyncedAt: '2026-07-04T00:00:00Z',
  lastSyncStatus: 'ok',
  entryCount: 26,
};

describe('ArchitectureSettingsNew', () => {
  beforeEach(async () => {
    await setTestLanguage('en');
    vi.clearAllMocks();
    listSourcesMock.mockResolvedValue([BUILTIN_SOURCE]);
    makeRequestMock.mockResolvedValue({});
    syncSourceMock.mockResolvedValue(BUILTIN_SOURCE);
    updateSourceMock.mockResolvedValue(BUILTIN_SOURCE);
  });

  it('renders the builtin source with coordinates and entry count', async () => {
    renderWithI18n(<ArchitectureSettingsNew />);

    await waitFor(() =>
      expect(screen.getByText('Awesome Architecture')).toBeInTheDocument()
    );
    expect(
      screen.getByText('study8677/awesome-architecture@main')
    ).toBeInTheDocument();
    expect(screen.getByText(/26 entries/)).toBeInTheDocument();
    // Builtin sources cannot be deleted.
    expect(screen.queryByText('Delete')).not.toBeInTheDocument();
  });

  it('defaults the guidance toggle to on when the setting is absent', async () => {
    renderWithI18n(<ArchitectureSettingsNew />);
    await waitFor(() =>
      expect(screen.getByText('Awesome Architecture')).toBeInTheDocument()
    );

    const toggles = screen.getAllByRole('switch');
    // First switch on the page is the guidance toggle.
    expect(toggles[0]).toHaveAttribute('aria-checked', 'true');
  });

  it('respects a stored "false" guidance setting', async () => {
    makeRequestMock.mockResolvedValue({
      architecture_guidance_enabled: 'false',
    });
    renderWithI18n(<ArchitectureSettingsNew />);
    await waitFor(() =>
      expect(screen.getByText('Awesome Architecture')).toBeInTheDocument()
    );

    const toggles = screen.getAllByRole('switch');
    expect(toggles[0]).toHaveAttribute('aria-checked', 'false');
  });

  it('triggers a manual sync', async () => {
    renderWithI18n(<ArchitectureSettingsNew />);
    await waitFor(() =>
      expect(screen.getByText('Awesome Architecture')).toBeInTheDocument()
    );

    fireEvent.click(screen.getByText('Sync now'));
    await waitFor(() => expect(syncSourceMock).toHaveBeenCalledWith('src-1'));
  });
});
