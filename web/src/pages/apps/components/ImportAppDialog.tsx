import {
  Suspense,
  lazy,
  useCallback,
  useMemo,
  useRef,
  useState,
  useEffect,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@/components/ui/dialog';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Progress } from '@/components/ui/progress';
import {
  Globe,
  UploadCloud,
  CheckCircle2,
  ArrowRight,
  ArrowLeft,
  Loader2,
  AlertCircle,
} from 'lucide-react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { aiApi, streamsApi, appsApi, filesApi } from '@/services/api';
import { useWizardInstall, useInstallProgress } from '@/hooks';
import type { WizardConfig } from '@/services/types';
import {
  translateInstallError,
  resolveInstallApiError,
} from '../lib/installErrorMessage';
import {
  translateInstallProgress,
  translateInstallPhase,
} from '../lib/installProgressMessage';
import {
  manifestToWizardConfig,
  changedPatchFields,
  isDirty,
} from '../lib/manifestHydrate';
import { resolveYamlViewMode, wizardConfigToYaml } from '../lib/wizardYaml';
import {
  resolveLocalMode,
  isValidContainerImageRef,
  collectInstallErrors,
  type ImportSectionId,
} from '../lib/importFlow';
import BasicInfoSection from './import/BasicInfoSection';
import ResourcesSection from './import/ResourcesSection';
import ModelsSection from './import/ModelsSection';
import PermissionsSection from './import/PermissionsSection';
import AdvancedSection from './import/AdvancedSection';
import SourceLocalForm from './import/SourceLocalForm';
import SectionNav from './import/SectionNav';
import EditViewSwitch, { type EditView } from './import/EditViewSwitch';
import {
  useManifestYaml,
  type ManifestUploadResult,
} from './import/useManifestYaml';

// Monaco is heavy and CDN-loaded — keep it out of the dialog's critical
// chunk; it only loads the first time the YAML view is opened.
const YamlEditorPane = lazy(() => import('./import/YamlEditorPane'));

export interface ImportAppDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const defaultConfig: WizardConfig = {
  metadata: { id: '', name: '', version: '1.0.0', description: '' },
  image: '',
  image_path: '',
  resources: { cpu: '50%', memory: '256Mi' },
  permissions: {
    video: [],
    inference: {
      models: [],
      max_qps: 10,
      max_concurrent: 0,
      allow_register_model: false,
    },
    events: { publish: [], subscribe: [] },
    device: { light: false, ir_cut: false, ptz: false, lens: false },
    network: { mode: 'isolated' },
  },
  env: [],
  volumes: [],
  autostart: false,
  restart_policy: 'on-failure',
};

/** Two frames — enough for a view-switch setState to reach the DOM before
 * scroll math runs in the same async handler. */
const nextPaint = () => new Promise<void>(resolve => {
    requestAnimationFrame(() => {
      requestAnimationFrame(() => resolve());
    });
  });

/**
 * Import dialog shell: a source screen (镜像仓库 / 本地上传) followed by a
 * single-page form with a left anchor nav. The local source merges the old
 * upload (tar only → form generates the manifest) and package (app.yaml is
 * the source of truth, edits PATCH back) flows — which one applies is
 * derived from what was uploaded, never chosen by the user.
 */
export default function ImportAppDialog({
  open,
  onOpenChange,
}: ImportAppDialogProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  // The old 6-step stepper collapses into three screens.
  const [page, setPage] = useState<'source' | 'form' | 'progress'>('source');
  const [sourceType, setSourceType] = useState<'registry' | 'local'>(
    'registry'
  );
  const [config, setConfig] = useState<WizardConfig>({ ...defaultConfig });
  const [isUploadingImage, setIsUploadingImage] = useState(false);
  const [taskId, setTaskId] = useState<string | null>(null);
  const [imageAddressError, setImageAddressError] = useState<string | null>(
    null
  );

  // Local source state: one yaml slot + one unified tar slot (the old
  // packageImagePath / uploadedImageRef dual track is gone).
  const [manifestPath, setManifestPath] = useState('');
  const [manifestMeta, setManifestMeta] = useState<{
    id: string;
    name: string;
    version: string;
    description: string;
  } | null>(null);
  const [imageTarPath, setImageTarPath] = useState('');
  const [imageTarName, setImageTarName] = useState('');
  const [isUploadingManifest, setIsUploadingManifest] = useState(false);
  // the uploaded manifest is multi-container (spec.containers) — the wizard
  // cannot express it, so edits are never written back.
  const [isMultiContainer, setIsMultiContainer] = useState(false);

  const [activeSection, setActiveSection] =    useState<ImportSectionId>('basic_info');

  // Form/YAML dual view on the form screen. Leaving the YAML view always
  // flushes pending text (re-upload) first — so yamlDirty can only ever be
  // true while ON the yaml view.
  const [editView, setEditView] = useState<EditView>('form');
  // Mount the (lazy) YAML pane on first visit and keep it alive so Monaco's
  // undo history survives view toggles; visibility is a class toggle.
  const [yamlMounted, setYamlMounted] = useState(false);

  const cancelRequestedRef = useRef(false);
  // snapshot of the config hydrated from the uploaded manifest; dirty
  // checks and the PATCH body diff against it.
  const hydratedConfigRef = useRef<WizardConfig | null>(null);
  // Snapshot written synchronously by each YAML flush — lets the install
  // handler branch on post-flush truth instead of awaiting re-renders.
  const lastFlushedRef = useRef<{
    path: string;
    config: WizardConfig;
    hydrated: WizardConfig;
    multiContainer: boolean;
  } | null>(null);

  const hasManifest = !!manifestPath;
  const localMode = resolveLocalMode({ manifestPath, imageTarPath });
  const isIdReadOnly = sourceType === 'local' && hasManifest;

  const isSourceReady =    sourceType === 'registry'
      ? isValidContainerImageRef(config.image)
      : !!(manifestPath || imageTarPath);

  const installMutation = useWizardInstall();
  const { data: progress } = useInstallProgress(taskId);

  // ---- YAML view state (text-side; flush = re-upload via the server) ----

  // Stable: setState-only + refs, so flushYaml's onApplied never goes stale.
  const applyManifestResponse = useCallback(
    (data: ManifestUploadResult, ctx: { source: 'upload' | 'flush' }) => {
      if (!data?.path) return;
      setManifestPath(data.path);
      setManifestMeta(
        data.metadata
          ? {
              id: data.metadata.id,
              name: data.metadata.name ?? '',
              version: data.metadata.version ?? '',
              description: data.metadata.description ?? '',
            }
          : null
      );
      if (!data.manifest) {
        hydratedConfigRef.current = null;
        setIsMultiContainer(false);
        return;
      }
      // With live sync the flushed text already carries the form state, so
      // flush and upload are the same operation: replace the form outright.
      // config and the dirty-check snapshot are independent mappings.
      const next = manifestToWizardConfig(data.manifest);
      const formState = manifestToWizardConfig(data.manifest);
      lastFlushedRef.current =        ctx.source === 'flush'
          ? {
              path: data.path,
              config: formState,
              hydrated: next,
              multiContainer: !!data.multi_container,
            }
          : null;
      setConfig(formState);
      hydratedConfigRef.current = next;
      setIsMultiContainer(!!data.multi_container);
    },
    []
  );

  // Live YAML → form hydration: the edited text parsed cleanly and maps onto
  // this config. It becomes BOTH the form state and the dirty-check snapshot,
  // so text-side edits stay text-dirty (yamlDirty) instead of form-dirty and
  // install via the view-leave flush.
  const handleLiveParse = useCallback((next: WizardConfig) => {
    setConfig(next);
    hydratedConfigRef.current = next;
  }, []);

  const yaml = useManifestYaml({
    onApplied: applyManifestResponse,
    onLiveParse: handleLiveParse,
  });

  // Form → text projection: every config change is AST-written onto the
  // editor text. The hook no-ops it while the editor is focused, while
  // flushing, or when the text does not parse (the user's text wins).
  useEffect(() => {
    yaml.applyConfig(config);
  }, [config, yaml.applyConfig]);

  // dirty tracking against the uploaded file: how many whitelisted fields
  // diverge. 0 → install the original bytes untouched.
  const editedFieldCount =    sourceType === 'local' && hasManifest
      ? Object.keys(changedPatchFields(config, hydratedConfigRef.current))
          .length
      : 0;
  const manifestDirty =    sourceType === 'local'
    && hasManifest
    && isDirty(config, hydratedConfigRef.current);

  const yamlViewMode = resolveYamlViewMode({ sourceType, hasManifest });
  const previewYaml = useMemo(
    () => (yamlViewMode === 'preview' ? wizardConfigToYaml(config) : ''),
    [yamlViewMode, config]
  );

  const cleanupPaths = useMemo(() => {
    const paths = [
      manifestPath,
      imageTarPath,
      ...yaml.uploadedPathsRef.current,
    ].filter(Boolean) as string[];
    return Array.from(new Set(paths));
    // uploadedPathsRef content is read at recompute time; every flush also
    // changes manifestPath, so the deps cover ref mutations.
  }, [manifestPath, imageTarPath]);

  const resetWizardState = () => {
    setPage('source');
    setSourceType('registry');
    setConfig({ ...defaultConfig });
    setIsUploadingImage(false);
    setTaskId(null);
    setImageAddressError(null);

    setManifestPath('');
    setManifestMeta(null);
    setImageTarPath('');
    setImageTarName('');
    setIsUploadingManifest(false);
    setIsMultiContainer(false);
    hydratedConfigRef.current = null;
    setActiveSection('basic_info');

    setEditView('form');
    setYamlMounted(false);
    lastFlushedRef.current = null;
    yaml.reset();

    installMutation.reset();
  };

  const cleanupUploadedFiles = async (paths: string[]) => {
    const uniq = Array.from(new Set(paths.filter(Boolean)));
    if (uniq.length === 0) return;
    try {
      await filesApi.batchDelete(uniq);
    } catch {
      // best-effort cleanup
    }
  };

  const handleCancel = () => {
    cancelRequestedRef.current = true;

    // Stop async polling immediately (best effort; backend task may still run)
    setTaskId(null);

    // Cleanup uploaded artifacts (best effort)
    cleanupUploadedFiles(cleanupPaths);

    // Reset local wizard state and close
    resetWizardState();
    onOpenChange(false);

    // Return to apps main page
    navigate('/apps');
  };

  // Fetch existing apps for duplicate check
  const { data: existingAppsData } = useQuery({
    queryKey: ['apps'],
    queryFn: () => appsApi.list().then(res => res.data || {}),
    enabled: open,
  });
  const existingAppIds: Set<string> = new Set(
    (existingAppsData?.apps || []).map((a: any) => a.id || a.app_id)
  );

  // Fetch available models
  const { data: modelsData, isSuccess: modelsLoaded } = useQuery({
    queryKey: ['models'],
    queryFn: () => aiApi.list().then(res => res.data || {}),
    enabled: open,
  });
  const availableModels: Array<{ model_id: string; name?: string }> =    modelsData?.models || [];

  // Fetch available video streams
  const { data: streamsData } = useQuery({
    queryKey: ['streams'],
    queryFn: () => streamsApi.list().then(res => res.data || {}),
    enabled: open,
    retry: false,
    initialData: { streams: [] },
  });
  const availableStreams: Array<{
    stream_id: string;
    width?: number;
    height?: number;
    fps?: number;
    status?: string;
  }> = streamsData?.streams || [];

  // Reset on close
  useEffect(() => {
    if (open) {
      cancelRequestedRef.current = false;
      return;
    }

    setPage('source');
    setSourceType('registry');
    setConfig({ ...defaultConfig });
    setTaskId(null);
    setImageAddressError(null);
    setManifestPath('');
    setManifestMeta(null);
    setImageTarPath('');
    setImageTarName('');
    setIsUploadingManifest(false);
    setIsMultiContainer(false);
    hydratedConfigRef.current = null;
    setActiveSection('basic_info');
    setEditView('form');
    setYamlMounted(false);
    lastFlushedRef.current = null;
    yaml.reset();
    installMutation.reset();
  }, [open, yaml.reset]);

  // ---- Local source handlers (upload side effects stay in the shell) ----

  const handleManifestUpload = async (file: File) => {
    setIsUploadingManifest(true);
    try {
      const res = await appsApi.uploadManifest(file);
      const data = res?.data;
      if (data?.path) {
        // every uploaded version must be tracked for cancel-time cleanup
        yaml.recordPath(data.path);
        // A fresh file replaces the form outright (upload semantics — no
        // form-edit merge), and its text becomes the YAML editor baseline.
        applyManifestResponse(data, { source: 'upload' });
        yaml.attachUpload(file);
      }
    } catch (err: unknown) {
      toast.error(
        resolveInstallApiError(err, t)
          || t('sys.apps.import.manifest_upload_failed', 'Manifest upload failed')
      );
    } finally {
      setIsUploadingManifest(false);
    }
  };

  const handleManifestClear = () => {
    setManifestPath('');
    setManifestMeta(null);
    hydratedConfigRef.current = null;
    setIsMultiContainer(false);
    lastFlushedRef.current = null;
    setEditView('form');
    yaml.reset();
  };

  const handleTarUploadSuccess = (path: string, imageName: string) => {
    if (cancelRequestedRef.current) {
      cleanupUploadedFiles([path]);
      return;
    }
    setImageTarPath(path);
    setImageTarName(imageName);
  };

  const handleTarClear = (path?: string) => {
    const pathToDelete = path || imageTarPath;
    if (pathToDelete) cleanupUploadedFiles([pathToDelete]);
    setImageTarPath('');
    setImageTarName('');
  };

  // ---- Navigation ----

  const handleActiveChange = useCallback((id: string) => {
    setActiveSection(id as ImportSectionId);
  }, []);

  // Pagination: jumping to a page is a plain state switch — the pane
  // renders one section at a time and never scrolls between them.
  const scrollToSection = (id: ImportSectionId) => {
    setActiveSection(id);
  };

  // ---- Form/YAML dual view ----

  /**
   * Flush pending YAML text and return to the form view. Returns false (and
   * stays on the YAML view) when the server rejects the text; on success the
   * returned snapshot mirrors what was just applied — async callers must not
   * trust the `manifestPath`/`config` closure values past this point.
   */
  const leaveYamlView = async (): Promise<
    { ok: false } | { ok: true; flushed?: boolean }
  > => {
    if (editView !== 'yaml') return { ok: true };
    const res = await yaml.flushYaml();
    if (!res.ok) return { ok: false };
    setEditView('form');
    const flushed = !!res.path;
    if (flushed) await nextPaint();
    return { ok: true, flushed };
  };

  const handleViewChange = async (next: EditView) => {
    if (next === editView) return;
    if (next === 'yaml') {
      setYamlMounted(true);
      setEditView('yaml');
      return;
    }
    await leaveYamlView();
  };

  /** Anchor click while in the YAML view: flush first, scroll only on
   * success (a rejected file keeps the user editing the YAML). */
  const handleBeforeNavigate = async () => {
    const res = await leaveYamlView();
    return res.ok;
  };

  const handleContinue = () => {
    if (sourceType === 'registry') {
      const v = config.image.trim();
      if (!v) {
        setImageAddressError(
          t('sys.apps.import.image_required', 'Image address is required')
        );
        return;
      }
      if (!isValidContainerImageRef(v)) {
        setImageAddressError(
          t(
            'sys.apps.import.invalid_image_ref',
            'Invalid image address. Use a valid registry path, e.g. docker.io/library/nginx:latest'
          )
        );
        return;
      }
      setImageAddressError(null);
      setPage('form');
      setEditView('form');
      return;
    }

    if (isUploadingImage || isUploadingManifest) {
      toast.error(
        t(
          'sys.apps.import.image_uploading',
          'Image upload is still in progress, please wait'
        )
      );
      return;
    }
    if (!manifestPath && !imageTarPath) {
      toast.error(
        t(
          'sys.apps.import.local_source_required',
          '请先上传 app.yaml 或镜像 tar 文件'
        )
      );
      return;
    }
    setPage('form');
    setActiveSection('basic_info');
    setEditView('form');
  };

  // ---- Install ----

  const handleInstall = async () => {
    // Flush pending text before deciding the install path — both direct
    // yaml-view edits and form edits live-synced onto the text. A rejected
    // file surfaces its error and stops the install.
    let flushed: {
      path: string;
      config: WizardConfig;
      hydrated: WizardConfig;
      multiContainer: boolean;
    } | null = null;
    if (editView === 'yaml') {
      const res = await leaveYamlView();
      if (!res.ok) return;
      // Only trust the snapshot when this run actually uploaded a new file;
      // a clean no-op flush must fall back to the render-time values.
      if (res.flushed) flushed = lastFlushedRef.current;
    } else if (yaml.yamlDirty) {
      // Form view: the text carries unflushed form edits — apply them so the
      // manifest on the server matches what the user sees, then install it
      // as-is (the PATCH fallback below no longer fires for form edits).
      const res = await yaml.flushYaml();
      if (!res.ok) {
        // The persisted error lives on the YAML pane — bring the user there.
        setYamlMounted(true);
        setEditView('yaml');
        return;
      }
      if (res.path) {
        await nextPaint();
        flushed = lastFlushedRef.current;
      }
    }

    // Post-flush truth: when this run just flushed, the closure values of
    // manifestPath/config/isMultiContainer are one flush behind — read the
    // snapshot the flush wrote instead.
    const effectiveManifestPath = flushed?.path ?? manifestPath;
    const effectiveConfig = flushed?.config ?? config;
    const effectiveHydrated = flushed?.hydrated ?? hydratedConfigRef.current;
    const effectiveMulti = flushed?.multiContainer ?? isMultiContainer;

    // One-page form: run the full validation at once and jump to the
    // first offending section (the old wizard gated step by step).
    const issues = collectInstallErrors(effectiveConfig, {
      sourceType,
      sourceReady: isSourceReady,
      // undefined while loading → availability check skipped (backend still
      // fast-fails); once loaded, an empty list is a real empty device.
      availableModelIds: modelsLoaded
        ? availableModels.map(m => m.model_id)
        : undefined,
    });
    if (issues.length > 0) {
      toast.error(t(`sys.apps.import.${issues[0].reason}`));
      scrollToSection(issues[0].section);
      return;
    }

    if (sourceType === 'local' && effectiveManifestPath) {
      // Manifest mode: patch the uploaded manifest with the wizard's
      // edits (when possible), then install from it.
      const startPackageInstall = () => {
        appsApi
          .installPackage({
            manifest_path: effectiveManifestPath,
            image_path: imageTarPath || undefined,
          })
          .then((res: any) => {
            const tid = res?.data?.task_id;
            if (tid) {
              setTaskId(tid);
              setPage('progress');
            } else {
              toast.success(
                t('sys.apps.toast.installSuccess', 'App installed')
              );
              queryClient.invalidateQueries({ queryKey: ['apps'] });
              onOpenChange(false);
            }
          })
          .catch((error: unknown) => {
            toast.error(resolveInstallApiError(error, t));
          });
      };

      const dirty = isDirty(effectiveConfig, effectiveHydrated);

      // Untouched walkthrough → install the uploaded bytes as-is (byte-faithful).
      if (!dirty) {
        startPackageInstall();
        return;
      }
      // Multi-container manifests are not wizard-expressible; the server
      // rejects patching them, so install the original file unchanged.
      if (effectiveMulti) {
        toast.info(t('sys.apps.import.multi_container_hint'));
        startPackageInstall();
        return;
      }

      const fields = changedPatchFields(effectiveConfig, effectiveHydrated);
      appsApi
        .patchManifest({ manifest_path: effectiveManifestPath, fields })
        .then(startPackageInstall)
        .catch((error: unknown) => {
          toast.error(
            resolveInstallApiError(error, t)
              || t(
                'sys.apps.import.patch_failed',
                'Failed to apply manifest edits'
              )
          );
        });
      return;
    }

    // Registry / tar-only local: wizard install. The tar path is only
    // merged into the payload here — config itself never stores it, so
    // re-entering the source screen can't leak stale state.
    const installConfig: WizardConfig =      sourceType === 'local'
        ? {
            ...effectiveConfig,
            image_path: imageTarPath,
            image: imageTarName || effectiveConfig.image,
          }
        : effectiveConfig;
    installMutation.mutate(installConfig, {
      onSuccess: (data: any) => {
        const tid = data?.task_id;
        if (tid) {
          setTaskId(tid);
          setPage('progress');
        } else {
          // Fallback: no task_id returned, treat as sync success
          toast.success(t('sys.apps.toast.installSuccess', 'App installed'));
          queryClient.invalidateQueries({ queryKey: ['store', 'apps'] });
          onOpenChange(false);
        }
      },
      onError: (error: unknown) => {
        toast.error(resolveInstallApiError(error, t));
      },
    });
  };

  // Watch progress for completion/error
  useEffect(() => {
    if (!progress || !taskId) return;
    if (progress.phase === 'complete') {
      toast.success(t('sys.apps.toast.installSuccess', 'App installed'));
      queryClient.invalidateQueries({ queryKey: ['apps'] });
      queryClient.invalidateQueries({ queryKey: ['store', 'apps'] });
      queryClient.invalidateQueries({ queryKey: ['containers'] });
      onOpenChange(false);
    }
    if (progress.phase === 'error') {
      queryClient.invalidateQueries({ queryKey: ['apps'] });
      queryClient.invalidateQueries({ queryKey: ['store', 'apps'] });
    }
  }, [progress?.phase]);

  // ---- Derived render data ----

  const sections: { id: ImportSectionId; label: string }[] = [
    { id: 'basic_info', label: t('sys.apps.import.basic_info') },
    { id: 'resources', label: t('sys.apps.import.resources') },
    { id: 'models', label: t('sys.apps.import.models_section', '模型配置') },
    { id: 'permissions', label: t('sys.apps.import.permissions') },
    { id: 'advanced', label: t('sys.apps.import.advanced', 'Advanced Config') },
  ];

  const installDisabled =    installMutation.isPending
    || isUploadingImage
    || isUploadingManifest
    || yaml.isFlushing;

  const navHeader = (
    <div className="space-y-3">
      <div className="rounded-lg border border-border bg-muted/40 px-3 py-2">
        <p className="text-sm font-medium text-foreground">
          {sourceType === 'registry'
            ? t('sys.apps.import.registry_title')
            : t('sys.apps.import.local_title', '本地上传')}
        </p>
        {sourceType === 'local' && (
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {localMode === 'manifest'
              ? 'app.yaml'
              : imageTarName || 'image.tar'}
          </p>
        )}
      </div>
      {sourceType === 'local' && hasManifest && (
        <div
          className={`rounded-lg border px-3 py-2 text-xs ${
            manifestDirty
              ? 'border-amber-500/50 bg-amber-500/10 text-amber-600 dark:text-amber-400'
              : 'border-border bg-muted/40 text-muted-foreground'
          }`}
        >
          {manifestDirty
            ? isMultiContainer
              ? t('sys.apps.import.multi_container_edited', {
                  n: editedFieldCount,
                })
              : t('sys.apps.import.manifest_edited', {
                  n: editedFieldCount,
                })
            : t('sys.apps.import.manifest_untouched')}
        </div>
      )}
      <Button
        variant="carbon"
        className="w-full"
        onClick={handleInstall}
        disabled={installDisabled}
      >
        {installMutation.isPending
          ? t('sys.apps.import.installing', 'Installing…')
          : t('common.install', 'Install')}
      </Button>
    </div>
  );

  // ---- Screens ----

  const sourceScreen = (
    <div className="px-4 py-4 sm:px-6 sm:py-5 md:px-10 lg:px-12">
      <h2 className="mb-2 text-center text-xl font-bold text-foreground sm:text-2xl">
        {t('sys.apps.import.source_title')}
      </h2>
      <p className="mb-6 text-center text-muted-foreground sm:mb-8">
        {t('sys.apps.import.source_desc')}
      </p>

      <div className="mb-6 grid grid-cols-1 gap-4 sm:mb-8 sm:grid-cols-2 sm:gap-4">
        {/* Registry card */}
        <div
          className={`relative flex cursor-pointer flex-col items-center rounded-xl border-2 p-4 transition-all sm:p-6 ${
            sourceType === 'registry'
              ? 'border-primary bg-primary/5'
              : 'border-border hover:border-primary/50'
          }`}
          onClick={() => {
            if (sourceType === 'registry') return;
            setSourceType('registry');
            setImageAddressError(null);
          }}
        >
          {sourceType === 'registry' && (
            <div className="absolute top-2 right-2 text-primary">
              <CheckCircle2
                className="w-5 h-5"
                fill="currentColor"
                stroke="white"
              />
            </div>
          )}
          <div className="w-12 h-12 bg-primary rounded-xl flex items-center justify-center text-primary-foreground mb-4">
            <Globe className="w-6 h-6" />
          </div>
          <h3 className="font-semibold text-foreground mb-1">
            {t('sys.apps.import.registry_title')}
          </h3>
          <p className="text-sm text-muted-foreground text-center">
            {t('sys.apps.import.registry_desc')}
          </p>
        </div>

        {/* Local upload card (merges the old upload + package sources) */}
        <div
          className={`relative flex cursor-pointer flex-col items-center rounded-xl border-2 p-4 transition-all sm:p-6 ${
            sourceType === 'local'
              ? 'border-primary bg-primary/5'
              : 'border-border hover:border-primary/50'
          }`}
          onClick={() => {
            if (sourceType === 'local') return;
            setSourceType('local');
            setImageAddressError(null);
          }}
        >
          {sourceType === 'local' && (
            <div className="absolute top-2 right-2 text-primary">
              <CheckCircle2
                className="w-5 h-5"
                fill="currentColor"
                stroke="white"
              />
            </div>
          )}
          <div className="w-12 h-12 bg-primary rounded-xl flex items-center justify-center text-primary-foreground mb-4">
            <UploadCloud className="w-6 h-6" />
          </div>
          <h3 className="font-semibold text-foreground mb-1">
            {t('sys.apps.import.local_title', '本地上传')}
          </h3>
          <p className="text-sm text-muted-foreground text-center">
            {t(
              'sys.apps.import.local_desc',
              '上传 app.yaml 和/或镜像 tar 文件'
            )}
          </p>
        </div>
      </div>

      {sourceType === 'registry' ? (
        <div>
          <Label>
            {t('sys.apps.import.image_address')}
            <span className="text-red-500 ml-1">*</span>
          </Label>
          <Input
            placeholder="docker.io/library/nginx:latest"
            value={config.image}
            onChange={e => {
              setConfig({ ...config, image: e.target.value });
              setImageAddressError(null);
            }}
            className={`mt-2 ${imageAddressError ? 'border-red-500 focus-visible:ring-red-500' : ''}`}
          />
          {imageAddressError && (
            <div className="mt-2 text-sm text-red-500">{imageAddressError}</div>
          )}
        </div>
      ) : (
        <SourceLocalForm
          manifestPath={manifestPath}
          manifestMeta={manifestMeta}
          imageTarPath={imageTarPath}
          imageTarName={imageTarName}
          isUploadingManifest={isUploadingManifest}
          existingAppIds={existingAppIds}
          localMode={localMode}
          onManifestUpload={handleManifestUpload}
          onManifestClear={handleManifestClear}
          onTarUploadSuccess={handleTarUploadSuccess}
          onTarUploadingChange={setIsUploadingImage}
          onTarClear={handleTarClear}
        />
      )}
    </div>
  );

  const sectionHeading = (label: string) => (
    <h3 className="mb-4 text-base font-semibold text-foreground">{label}</h3>
  );

  const formScreen = (
    <div className="flex min-h-0 flex-1 flex-col sm:flex-row">
      <SectionNav
        sections={sections}
        activeId={activeSection}
        onActiveChange={handleActiveChange}
        onBeforeNavigate={handleBeforeNavigate}
        header={navHeader}
      />
      <div className="flex min-h-0 flex-1 flex-col">
        <div className="flex items-center justify-between gap-2 border-b border-border px-4 py-2 sm:px-6">
          <EditViewSwitch
            view={editView}
            yamlDirty={yaml.yamlDirty}
            onChange={handleViewChange}
          />
        </div>

        {/* The form pane and the YAML pane stay mounted (visibility is a
         * class toggle) so Monaco's undo history survives view switches;
         * the YAML pane only mounts on first visit (lazy). Each nav entry
         * is one page — only the active section renders, so the pane never
         * scrolls between sections (long lists scroll inside their own
         * ScrollArea). */}
        <div className="flex min-h-0 flex-1 flex-col">
          <div
            className={`min-h-0 flex-1 overflow-y-auto px-4 py-5 sm:px-6 lg:px-8 ${
              editView === 'yaml' ? 'hidden' : ''
            }`}
          >
            {activeSection === 'basic_info' && (
              <>
                {sectionHeading(t('sys.apps.import.basic_info'))}
                <BasicInfoSection
                  config={config}
                  onChange={setConfig}
                  isIdReadOnly={isIdReadOnly}
                  existingAppIds={existingAppIds}
                />
              </>
            )}

            {activeSection === 'resources' && (
              <>
                {sectionHeading(t('sys.apps.import.resources'))}
                <ResourcesSection config={config} onChange={setConfig} />
              </>
            )}

            {activeSection === 'models' && (
              <>
                {sectionHeading(
                  t('sys.apps.import.models_section', '模型配置')
                )}
                <ModelsSection
                  config={config}
                  onChange={setConfig}
                  availableModels={availableModels}
                />
              </>
            )}

            {activeSection === 'permissions' && (
              <>
                {sectionHeading(t('sys.apps.import.permissions'))}
                <PermissionsSection
                  config={config}
                  onChange={setConfig}
                  availableStreams={availableStreams}
                />
              </>
            )}

            {activeSection === 'advanced' && (
              <>
                {sectionHeading(
                  t('sys.apps.import.advanced', 'Advanced Config')
                )}
                <AdvancedSection config={config} onChange={setConfig} />
              </>
            )}
          </div>

          {yamlMounted && (
            <div
              className={`min-h-0 flex-1 flex-col ${
                editView === 'yaml' ? 'flex' : 'hidden'
              }`}
            >
              <Suspense
                fallback={(
                  <div className="flex min-h-0 flex-1 items-center justify-center bg-muted/30 text-sm text-muted-foreground">
                    {t('sys.apps.import.yaml_loading', '加载编辑器…')}
                  </div>
                )}
              >
                <YamlEditorPane
                  mode={yamlViewMode}
                  value={
                    yamlViewMode === 'editable'
                      ? yaml.manifestText
                      : previewYaml
                  }
                  originalValue={yaml.originalText}
                  dirty={yaml.yamlDirty}
                  isFlushing={yaml.isFlushing}
                  yamlError={yaml.yamlError}
                  // Editable only: the editor's text is the manifest's.
                  // Preview shows form-generated YAML — wiring its onChange
                  // to setManifestText would let the controlled editor push
                  // the generated text into the (empty) manifest state,
                  // marking it dirty and making every view switch try to
                  // "flush" a file that was never uploaded.
                  onChange={
                    yamlViewMode === 'editable'
                      ? yaml.setManifestText
                      : undefined
                  }
                  onApply={() => {
                    yaml.flushYaml();
                  }}
                  onFocusChange={yaml.setFocused}
                />
              </Suspense>
            </div>
          )}
        </div>
      </div>
    </div>
  );

  const progressScreen = (
    <div className="flex flex-col items-center px-4 py-8 sm:px-8 sm:py-10 md:px-12 lg:px-12">
      {progress?.phase === 'error' ? (
        <>
          <div className="w-16 h-16 bg-destructive/10 rounded-full flex items-center justify-center mb-4">
            <AlertCircle className="w-8 h-8 text-destructive" />
          </div>
          <h2 className="text-xl font-bold text-foreground mb-2">
            {t('sys.apps.import.install_failed', 'Install Failed')}
          </h2>
          <p className="text-sm text-muted-foreground mb-6 max-w-md text-center">
            {translateInstallError(progress?.error || progress?.message, t)}
          </p>
          <Button
            variant="carbon"
            onClick={() => {
              setTaskId(null);
              setPage('source');
              installMutation.reset();
            }}
          >
            {t('common.retry', 'Retry')}
          </Button>
        </>
      ) : (
        <>
          <div className="w-16 h-16 bg-primary/10 rounded-full flex items-center justify-center mb-4">
            <Loader2 className="w-8 h-8 text-primary animate-spin" />
          </div>
          <h2 className="text-xl font-bold text-foreground mb-2">
            {t('sys.apps.import.installing_title', 'Installing app...')}
          </h2>
          <p className="text-sm text-muted-foreground mb-6">
            {translateInstallProgress(progress, t)}
          </p>
          <div className="w-full max-w-md">
            <Progress value={progress?.percent ?? 0} className="h-2" />
            <div className="flex justify-between mt-2 text-xs text-muted-foreground">
              <span>{translateInstallPhase(progress?.phase, t)}</span>
              <span>{Math.round(progress?.percent ?? 0)}%</span>
            </div>
          </div>
        </>
      )}
    </div>
  );

  return (
    <Dialog
      open={open}
      onOpenChange={nextOpen => {
        if (nextOpen) {
          cancelRequestedRef.current = false;
          onOpenChange(true);
          return;
        }
        handleCancel();
      }}
    >
      <DialogContent
        className={`flex max-h-[90vh] w-full max-w-[calc(100%-1rem)] flex-col overflow-hidden rounded-2xl border-none p-0 shadow-2xl max-sm:fixed max-sm:inset-0 max-sm:left-0 max-sm:top-0 max-sm:h-dvh max-sm:max-h-dvh max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-none sm:max-w-[1050px] ${
          // Fixed height on the form screen: the YAML pane is a flex child
          // with no intrinsic height, so a content-sized dialog would
          // collapse the moment the form column hides.
          page === 'form' ? 'sm:h-[90vh]' : ''
        }`}
        onInteractOutside={e => e.preventDefault()}
      >
        <div className="p-4 pb-2 sm:p-6 sm:pb-2">
          <DialogTitle className="pr-10 text-lg font-bold text-foreground sm:text-xl">
            {t('sys.apps.import.wizard_title', 'Application Setup Wizard')}
          </DialogTitle>
          <DialogDescription className="hidden">
            {t('sys.apps.import.wizard_description', 'Setup Wizard')}
          </DialogDescription>
        </div>

        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
          {page === 'source' && (
            <div className="min-h-0 flex-1 overflow-y-auto">{sourceScreen}</div>
          )}
          {page === 'form' && formScreen}
          {page === 'progress' && (
            <div className="min-h-0 flex-1 overflow-y-auto">
              {progressScreen}
            </div>
          )}
        </div>

        {page === 'source' && (
          <div className="flex flex-row items-center gap-2 border-t border-border bg-muted/20 px-4 py-3 sm:justify-between sm:px-6 sm:py-4">
            <Button
              variant="outline"
              className="flex-1 text-muted-foreground hover:text-foreground sm:hidden"
              onClick={handleCancel}
              disabled={installMutation.isPending}
            >
              {t('common.cancel')}
            </Button>

            <div className="flex flex-1 items-center justify-end gap-2 sm:flex-none sm:gap-4">
              <Button
                variant="outline"
                className="hidden text-muted-foreground hover:text-foreground sm:inline-flex"
                onClick={handleCancel}
                disabled={installMutation.isPending}
              >
                {t('common.cancel')}
              </Button>
              <Button
                variant="carbon"
                className="flex-1 sm:flex-none"
                onClick={handleContinue}
                disabled={
                  !isSourceReady
                  || isUploadingImage
                  || isUploadingManifest
                  || installMutation.isPending
                }
              >
                {t('sys.apps.import.continue')}
                <ArrowRight className="ml-2 h-4 w-4" />
              </Button>
            </div>
          </div>
        )}

        {page === 'form' && (
          <div className="flex flex-row items-center gap-2 border-t border-border bg-muted/20 px-4 py-3 sm:justify-between sm:px-6 sm:py-4">
            <Button
              variant="outline"
              className="flex-1 text-muted-foreground hover:text-foreground sm:flex-none"
              onClick={() => setPage('source')}
              disabled={installMutation.isPending}
            >
              <ArrowLeft className="mr-2 h-4 w-4" />
              {t('sys.apps.import.back_to_source', '返回来源')}
            </Button>

            <Button
              variant="outline"
              className="hidden text-muted-foreground hover:text-foreground sm:inline-flex"
              onClick={handleCancel}
              disabled={installMutation.isPending}
            >
              {t('common.cancel')}
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
