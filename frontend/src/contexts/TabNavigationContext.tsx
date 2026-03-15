import { createContext, useContext, type ReactNode } from 'react';
import type { TabType } from '@/types/tabs';

interface TabNavContextType {
  activeTab: TabType;
  setActiveTab: (tab: TabType) => void;
}

export const TabNavContext = createContext<TabNavContextType | null>(null);

interface TabNavigationProviderProps {
  children: ReactNode;
  value: TabNavContextType;
}

export function TabNavigationProvider({ children, value }: Readonly<TabNavigationProviderProps>) {
  return (
    <TabNavContext.Provider value={value}>{children}</TabNavContext.Provider>
  );
}

export function useTabNavigation(): TabNavContextType {
  const context = useContext(TabNavContext);
  if (!context) {
    throw new Error('useTabNavigation must be used within a TabNavigationProvider');
  }
  return context;
}
