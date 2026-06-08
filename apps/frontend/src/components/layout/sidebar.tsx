import { Link } from '@tanstack/react-router';
import {
  Activity,
  Database,
  FileText,
  Folders,
  LayoutDashboard,
  ListTodo,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Tags,
} from 'lucide-react';
import type { ReactNode, JSX } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '../../lib/cn.ts';
import { SidebarFooter } from './sidebar-footer.tsx';

interface NavLink {
  to: string;
  labelKey: string;
  icon: ReactNode;
}

interface SidebarProps {
  collapsed: boolean;
  onToggle: () => void;
}

const links: NavLink[] = [
  { to: '/', labelKey: 'sidebar.overview', icon: <LayoutDashboard size={16} /> },
  { to: '/brain', labelKey: 'sidebar.knowledge', icon: <Network size={16} /> },
  { to: '/projects', labelKey: 'sidebar.projects', icon: <Folders size={16} /> },
  { to: '/observations', labelKey: 'sidebar.observations', icon: <Database size={16} /> },
  { to: '/sessions', labelKey: 'sidebar.sessions', icon: <Activity size={16} /> },
  { to: '/sync', labelKey: 'sidebar.syncHealth', icon: <ListTodo size={16} /> },
  { to: '/prompts', labelKey: 'sidebar.prompts', icon: <FileText size={16} /> },
  { to: '/topics', labelKey: 'sidebar.topics', icon: <Tags size={16} /> },
  { to: '/settings', labelKey: 'sidebar.settings', icon: <Settings size={16} /> },
];

export function Sidebar({ collapsed, onToggle }: SidebarProps): JSX.Element {
  const { t } = useTranslation();
  return (
    <aside
      className={cn(
        'flex shrink-0 flex-col border-r border-border bg-surface py-5 transition-[width] duration-200',
        collapsed ? 'w-16 px-2' : 'w-56 px-3',
      )}
    >
      <div className={cn('flex items-center pb-5', collapsed ? 'justify-center px-0' : 'justify-between px-2')}>
        {!collapsed && (
          <div>
            <p className="text-xs uppercase tracking-wider text-fg-muted">{t('sidebar.brand')}</p>
            <h1 className="text-base font-semibold text-fg">{t('sidebar.subtitle')}</h1>
          </div>
        )}
        <button
          type="button"
          onClick={onToggle}
          aria-label={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
          title={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
          className="rounded-md p-2 text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg"
        >
          {collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
        </button>
      </div>
      <nav className="flex flex-1 flex-col gap-1" aria-label={t('sidebar.primaryNav')}>
        {links.map((link) => (
          <Link
            key={link.to}
            to={link.to}
            activeOptions={{ exact: link.to === '/' }}
            title={collapsed ? t(link.labelKey) : undefined}
            className={cn(
              'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
              'text-fg-muted hover:bg-surface-2 hover:text-fg',
              collapsed && 'justify-center px-0',
            )}
            activeProps={{ className: 'bg-surface-2 text-fg' }}
          >
            {link.icon}
            {!collapsed && <span>{t(link.labelKey)}</span>}
          </Link>
        ))}
      </nav>
      <SidebarFooter collapsed={collapsed} />
    </aside>
  );
}
