import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';
import i18n from '@/i18n';
import { ProductModeSwitch } from './ProductModeSwitch';

function renderAtRoute(route: string) {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={[route]}>
        <ProductModeSwitch />
      </MemoryRouter>
    </I18nextProvider>
  );
}

describe('ProductModeSwitch', () => {
  it('renders both mode buttons', () => {
    renderAtRoute('/board');

    expect(
      screen.getByText(i18n.t('workflow:modeSwitch.manual'))
    ).toBeInTheDocument();
    expect(
      screen.getByText(i18n.t('workflow:modeSwitch.orchestrated'))
    ).toBeInTheDocument();
  });

  it('highlights manual mode when on board route', () => {
    renderAtRoute('/board');

    const manualButton = screen
      .getByText(i18n.t('workflow:modeSwitch.manual'))
      .closest('button');
    expect(manualButton).toHaveClass('text-brand');
  });

  it('highlights orchestrated mode when on workspaces route', () => {
    renderAtRoute('/workspaces/create');

    const orchestratedButton = screen
      .getByText(i18n.t('workflow:modeSwitch.orchestrated'))
      .closest('button');
    expect(orchestratedButton).toHaveClass('text-brand');
  });
});
