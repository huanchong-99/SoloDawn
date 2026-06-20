import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { qualityPolicyApi, customRulesApi } from '@/lib/api';
import type {
  QualityGateConfig,
  CustomRuleInput,
  AuthorRuleRequest,
} from 'shared/types';

// ============================================================================
// Query Keys
// ============================================================================

export const qualityPolicyKeys = {
  all: ['qualityPolicy'] as const,
  default: ['qualityPolicy', 'default'] as const,
  metrics: ['qualityPolicy', 'metrics'] as const,
  project: (projectId: string) => ['qualityPolicy', 'project', projectId] as const,
  customRules: (projectId: string) =>
    ['qualityPolicy', 'customRules', projectId] as const,
  metricsLatest: (projectId: string) =>
    ['qualityPolicy', 'metricsLatest', projectId] as const,
  ruleValidations: (projectId: string, ruleId: string) =>
    ['qualityPolicy', 'ruleValidations', projectId, ruleId] as const,
};

// ============================================================================
// Hooks
// ============================================================================

/** Fetch the resolved quality policy for a specific project (DB-first). */
export function useProjectQualityPolicy(projectId: string | null) {
  return useQuery({
    queryKey: qualityPolicyKeys.project(projectId ?? ''),
    queryFn: () => qualityPolicyApi.getProject(projectId!),
    enabled: !!projectId,
    staleTime: 30_000,
  });
}

/** Fetch the default quality policy (parsed from BUNDLED_CENTRAL_POLICY). */
export function useDefaultQualityPolicy() {
  return useQuery({
    queryKey: qualityPolicyKeys.default,
    queryFn: () => qualityPolicyApi.getDefault(),
    staleTime: 5 * 60 * 1000,
  });
}

/** Fetch the closed-enum metric keys and supported operators for the editor picker. */
export function useQualityMetricKeys() {
  return useQuery({
    queryKey: qualityPolicyKeys.metrics,
    queryFn: () => qualityPolicyApi.getMetrics(),
    staleTime: 60 * 60 * 1000, // metric enum is static between deployments
  });
}

/** Save (PUT) a quality policy for a project; invalidates the project policy cache on success. */
export function useSaveQualityPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, config }: { projectId: string; config: QualityGateConfig }) =>
      qualityPolicyApi.putProject(projectId, config),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: qualityPolicyKeys.project(projectId) });
    },
  });
}

/** Reset (DELETE) the project quality policy; falls back to file/bundled chain on next load. */
export function useResetQualityPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (projectId: string) => qualityPolicyApi.deleteProject(projectId),
    onSuccess: (_data, projectId) => {
      qc.invalidateQueries({ queryKey: qualityPolicyKeys.project(projectId) });
    },
  });
}

// ============================================================================
// Custom Rules (AI-editable quality-gate rules)
// ============================================================================

/** List all custom rules for a project. */
export function useCustomRules(projectId: string | null) {
  return useQuery({
    queryKey: qualityPolicyKeys.customRules(projectId ?? ''),
    queryFn: () => customRulesApi.list(projectId!),
    enabled: !!projectId,
    staleTime: 30_000,
  });
}

/** Latest persisted-run metric snapshot for a project (D7; never recomputes). */
export function useProjectMetricsLatest(projectId: string | null) {
  return useQuery({
    queryKey: qualityPolicyKeys.metricsLatest(projectId ?? ''),
    queryFn: () => customRulesApi.metricsLatest(projectId!),
    enabled: !!projectId,
    staleTime: 30_000,
  });
}

/**
 * Validation artifacts for a single rule (newest first). Powers the G2 confirm
 * dialog's read-only evidence panel (empirical results + round-trip verdict).
 */
export function useCustomRuleValidations(
  projectId: string | null,
  ruleId: string | null
) {
  return useQuery({
    queryKey: qualityPolicyKeys.ruleValidations(projectId ?? '', ruleId ?? ''),
    queryFn: () => customRulesApi.getValidations(projectId!, ruleId!),
    enabled: !!projectId && !!ruleId,
    staleTime: 30_000,
  });
}

/** Create a custom rule (runs the admission gate server-side); invalidates the rule list. */
export function useCreateCustomRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, input }: { projectId: string; input: CustomRuleInput }) =>
      customRulesApi.create(projectId, input),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: qualityPolicyKeys.customRules(projectId) });
    },
  });
}

/** Update a custom rule (D8 edit policy); invalidates the rule list. */
export function useUpdateCustomRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      projectId,
      ruleId,
      input,
    }: {
      projectId: string;
      ruleId: string;
      input: CustomRuleInput;
    }) => customRulesApi.update(projectId, ruleId, input),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: qualityPolicyKeys.customRules(projectId) });
    },
  });
}

/** Delete a custom rule; invalidates the rule list. */
export function useDeleteCustomRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ projectId, ruleId }: { projectId: string; ruleId: string }) =>
      customRulesApi.remove(projectId, ruleId),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: qualityPolicyKeys.customRules(projectId) });
    },
  });
}

/** Promote/demote a rule's status (shadow→warn→enforce / disabled); invalidates the rule list. */
export function useSetCustomRuleStatus() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      projectId,
      ruleId,
      status,
    }: {
      projectId: string;
      ruleId: string;
      status: string;
    }) => customRulesApi.setStatus(projectId, ruleId, status),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: qualityPolicyKeys.customRules(projectId) });
    },
  });
}

/** Re-run the full AI validation pipeline (D8) for a rule; invalidates the rule list. */
export function useRevalidateRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      projectId,
      ruleId,
      req,
    }: {
      projectId: string;
      ruleId: string;
      req: AuthorRuleRequest;
    }) => customRulesApi.revalidate(projectId, ruleId, req),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: qualityPolicyKeys.customRules(projectId) });
    },
  });
}

/**
 * Run the multi-agent authoring pipeline to generate a candidate rule. Does NOT
 * invalidate: the result is a candidate awaiting the mandatory human confirm
 * (a subsequent create/update persists it).
 */
export function useGenerateRule() {
  return useMutation({
    mutationFn: ({ projectId, req }: { projectId: string; req: AuthorRuleRequest }) =>
      customRulesApi.author(projectId, req),
  });
}
