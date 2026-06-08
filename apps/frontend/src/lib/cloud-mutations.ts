import { useMutation, useQueryClient, type UseMutationResult } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { api, ApiRequestError, type CloudSyncProjectResponse } from './api.ts';

function extractErrorMessage(err: unknown): string {
  if (err instanceof ApiRequestError) return err.api.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

/**
 * Invalidate every query that depends on cloud sync state. Centralised so
 * each mutation hook stays consistent (a missed key here causes stale UI
 * after enroll/unenroll).
 */
function invalidateSyncQueries(queryClient: ReturnType<typeof useQueryClient>): void {
  void queryClient.invalidateQueries({ queryKey: ['projects'] });
  void queryClient.invalidateQueries({ queryKey: ['sync-projects'] });
  void queryClient.invalidateQueries({ queryKey: ['sync-project'] });
  void queryClient.invalidateQueries({ queryKey: ['sync-issues'] });
  void queryClient.invalidateQueries({ queryKey: ['overview'] });
  void queryClient.invalidateQueries({ queryKey: ['health'] });
}

interface EnrollResponse {
  ok: true;
  project: string;
  output: string;
}

export function useEnrollProject(project: string): UseMutationResult<EnrollResponse, unknown, void> {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  return useMutation<EnrollResponse, unknown, void>({
    mutationFn: () => api.enrollProject(project),
    onSuccess: () => {
      toast.success(t('projects.toast.enrollSuccess', { project }));
      invalidateSyncQueries(queryClient);
    },
    onError: (err) => {
      toast.error(t('projects.toast.enrollError', { project, error: extractErrorMessage(err) }));
    },
  });
}

export function useUnenrollProject(project: string): UseMutationResult<EnrollResponse, unknown, void> {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  return useMutation<EnrollResponse, unknown, void>({
    mutationFn: () => api.unenrollProject(project),
    onSuccess: () => {
      toast.success(t('projects.toast.unenrollSuccess', { project }));
      invalidateSyncQueries(queryClient);
    },
    onError: (err) => {
      toast.error(t('projects.toast.unenrollError', { project, error: extractErrorMessage(err) }));
    },
  });
}

export function useSyncProject(
  project: string,
): UseMutationResult<CloudSyncProjectResponse, unknown, void> {
  const queryClient = useQueryClient();
  const { t } = useTranslation();
  return useMutation<CloudSyncProjectResponse, unknown, void>({
    mutationFn: () => api.cloudSyncProject(project),
    onMutate: () => {
      toast.loading(t('projects.toast.syncStarted', { project }), { id: `sync-${project}` });
    },
    onSuccess: () => {
      toast.success(t('projects.toast.syncSuccess', { project }), { id: `sync-${project}` });
      invalidateSyncQueries(queryClient);
    },
    onError: (err) => {
      toast.error(t('projects.toast.syncError', { project, error: extractErrorMessage(err) }), {
        id: `sync-${project}`,
      });
    },
  });
}
