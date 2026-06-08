import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router';
import { AppShell } from './components/layout/app-shell.tsx';
import { OverviewPage } from './pages/overview-page.tsx';
import { ObservationsPage } from './pages/observations-page.tsx';
import { SessionsPage } from './pages/sessions-page.tsx';
import { SessionDetailPage } from './pages/session-detail-page.tsx';
import { SyncHealthPage } from './pages/sync-health-page.tsx';
import { SyncProjectPage } from './pages/sync-project-page.tsx';
import { PromptsPage } from './pages/prompts-page.tsx';
import { TopicsPage } from './pages/topics-page.tsx';
import { SettingsPage } from './pages/settings-page.tsx';
import { ProjectsPage } from './pages/projects-page.tsx';
import { ProjectDetailPage } from './pages/project-detail-page.tsx';
import { OrphansPage } from './pages/orphans-page.tsx';
import { BrainPage } from './pages/brain-page.tsx';
const rootRoute = createRootRoute({ component: AppShell });

const overviewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: OverviewPage,
});

export interface ObservationsSearch {
  q?: string;
  project?: string[];
  type?: string[];
  tool_name?: string[];
  scope?: 'project' | 'personal' | 'all';
  topic_key?: string;
  deleted?: 'active' | 'all' | 'only';
}

function parseStringArray(input: unknown): string[] | undefined {
  if (Array.isArray(input)) {
    const arr = input.filter((v): v is string => typeof v === 'string' && v.length > 0);
    return arr.length === 0 ? undefined : arr;
  }
  if (typeof input === 'string' && input.length > 0) return [input];
  return undefined;
}

function parseScope(input: unknown): ObservationsSearch['scope'] {
  if (input === 'project' || input === 'personal' || input === 'all') return input;
  return undefined;
}

function parseDeleted(input: unknown): ObservationsSearch['deleted'] {
  if (input === 'active' || input === 'all' || input === 'only') return input;
  return undefined;
}

export const observationsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/observations',
  validateSearch: (raw: Record<string, unknown>): ObservationsSearch => {
    const out: ObservationsSearch = {};
    if (typeof raw.q === 'string' && raw.q.length > 0) out.q = raw.q;
    const project = parseStringArray(raw.project);
    if (project) out.project = project;
    const type = parseStringArray(raw.type);
    if (type) out.type = type;
    const tool_name = parseStringArray(raw.tool_name);
    if (tool_name) out.tool_name = tool_name;
    const scope = parseScope(raw.scope);
    if (scope) out.scope = scope;
    if (typeof raw.topic_key === 'string' && raw.topic_key.length > 0) out.topic_key = raw.topic_key;
    const deleted = parseDeleted(raw.deleted);
    if (deleted) out.deleted = deleted;
    return out;
  },
  component: ObservationsPage,
});

const sessionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/sessions',
  component: SessionsPage,
});

const sessionDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/sessions/$id',
  component: SessionDetailPage,
});

const syncRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/sync',
  component: SyncHealthPage,
});

const syncProjectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/sync/$project',
  component: SyncProjectPage,
});

const projectsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/projects',
  component: ProjectsPage,
});

const orphansRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/projects/_orphans',
  component: OrphansPage,
});

const projectDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/projects/$project',
  component: ProjectDetailPage,
});

const promptsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/prompts',
  component: PromptsPage,
});

const topicsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/topics',
  component: TopicsPage,
});

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: SettingsPage,
});

const brainRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/brain',
  component: BrainPage,
});

const routeTree = rootRoute.addChildren([
  overviewRoute,
  observationsRoute,
  sessionsRoute,
  sessionDetailRoute,
  projectsRoute,
  orphansRoute,
  projectDetailRoute,
  syncRoute,
  syncProjectRoute,
  promptsRoute,
  topicsRoute,
  settingsRoute,
  brainRoute,
]);

export const router = createRouter({ routeTree, defaultPreload: 'intent' });

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
