import { screen, waitFor, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';

const listMock = vi.fn();
const createMock = vi.fn();
const updateMock = vi.fn();
const removeMock = vi.fn();
const makeRequestMock = vi.fn();

vi.mock('@/lib/api', () => ({
  designStylesApi: {
    list: (...args: unknown[]) => listMock(...args),
    create: (...args: unknown[]) => createMock(...args),
    update: (...args: unknown[]) => updateMock(...args),
    remove: (...args: unknown[]) => removeMock(...args),
  },
  makeRequest: (...args: unknown[]) => makeRequestMock(...args),
  handleApiResponse: async (value: unknown) => value,
}));

import { DesignStylesSettingsNew } from '../DesignStylesSettingsNew';
import { renderWithI18n, setTestLanguage } from '@/test/renderWithI18n';

const BUILTIN_STYLE = {
  id: 'style-1',
  slug: 'anthropic-frontend-design',
  name: 'Anthropic Frontend Design',
  description: 'Distinctive production-grade UI direction.',
  content: 'Approach this as the design lead...',
  sourceName: 'anthropics/skills — frontend-design',
  sourceUrl: 'https://github.com/anthropics/skills',
  license: 'Apache-2.0',
  builtin: true,
  enabled: true,
  createdAt: '2026-07-04T00:00:00Z',
  updatedAt: '2026-07-04T00:00:00Z',
};

const CUSTOM_STYLE = {
  id: 'style-2',
  slug: 'my-style',
  name: 'My Style',
  description: 'Personal direction.',
  content: 'Use warm monochrome.',
  sourceName: null,
  sourceUrl: null,
  license: null,
  builtin: false,
  enabled: true,
  createdAt: '2026-07-04T00:00:00Z',
  updatedAt: '2026-07-04T00:00:00Z',
};

describe('DesignStylesSettingsNew', () => {
  beforeEach(async () => {
    await setTestLanguage('en');
    vi.clearAllMocks();
    listMock.mockResolvedValue([BUILTIN_STYLE, CUSTOM_STYLE]);
    // System settings GET (default style not set).
    makeRequestMock.mockResolvedValue({ default_design_style: '' });
    updateMock.mockResolvedValue(CUSTOM_STYLE);
    createMock.mockResolvedValue(CUSTOM_STYLE);
    removeMock.mockResolvedValue(undefined);
  });

  it('renders builtin and custom styles with their badges and attribution', async () => {
    renderWithI18n(<DesignStylesSettingsNew />);

    // Style names appear both as default-style <option>s and as card titles.
    await waitFor(() =>
      expect(
        screen.getAllByText('Anthropic Frontend Design').length
      ).toBeGreaterThan(0)
    );
    expect(screen.getAllByText('My Style').length).toBeGreaterThan(0);
    expect(screen.getByText('None (no design direction)')).toBeInTheDocument();
    expect(screen.getByText('Built-in')).toBeInTheDocument();
    expect(screen.getByText('Custom')).toBeInTheDocument();
    expect(screen.getByText('Apache-2.0')).toBeInTheDocument();
    expect(
      screen.getByText('Source: anthropics/skills — frontend-design')
    ).toBeInTheDocument();
  });

  it('only offers edit/delete for custom styles', async () => {
    renderWithI18n(<DesignStylesSettingsNew />);
    await waitFor(() =>
      expect(screen.getAllByText('My Style').length).toBeGreaterThan(0)
    );

    // One custom style → exactly one Edit and one Delete action.
    expect(screen.getAllByText('Edit')).toHaveLength(1);
    expect(screen.getAllByText('Delete')).toHaveLength(1);
    // Both styles can be duplicated.
    expect(screen.getAllByText('Duplicate')).toHaveLength(2);
  });

  it('duplicates a builtin style into a custom copy', async () => {
    renderWithI18n(<DesignStylesSettingsNew />);
    await waitFor(() =>
      expect(
        screen.getAllByText('Anthropic Frontend Design').length
      ).toBeGreaterThan(0)
    );

    fireEvent.click(screen.getAllByText('Duplicate')[0]);

    await waitFor(() => expect(createMock).toHaveBeenCalledTimes(1));
    expect(createMock).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Anthropic Frontend Design (copy)',
        content: BUILTIN_STYLE.content,
      })
    );
  });

  it('expands style content on demand', async () => {
    renderWithI18n(<DesignStylesSettingsNew />);
    await waitFor(() =>
      expect(screen.getAllByText('My Style').length).toBeGreaterThan(0)
    );

    expect(
      screen.queryByText('Use warm monochrome.')
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getAllByText('View content')[1]);
    expect(screen.getByText('Use warm monochrome.')).toBeInTheDocument();
  });
});
