import { useTranslation } from 'react-i18next';
import { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { useModelInfo, useExportModel } from '@/hooks/useModels';
import { useToast } from '@/hooks/use-toast';
import {
  HardDrive,
  Clock,
  Tag,
  FolderOpen,
  ExternalLink,
  Hash,
  Settings2,
  AppWindow,
  Download,
  Power,
  PowerOff,
  Loader2,
  type LucideIcon,
} from 'lucide-react';
import { getModelTypeLabel, getModelTypeDescription } from '../utils';
import { getModelIcon } from '../modelIcons';

interface ModelData {
  model_id: string;
  name?: string;
  model_path?: string;
  version?: string;
  load_timestamp?: number;
  status?: string;
  estimated_memory?: number;
  estimated_tops?: number;
  inputs?: unknown;
  outputs?: unknown;
  // Enriched fields from database
  model_type?: string;
  // Output delivery mode: 'platform' (plugin-decoded) | 'raw' (bare tensors)
  output_mode?: string;
  variant?: string;
  threshold?: number;
  max_detections?: number;
  file_size?: number;
  file_hash?: string;
  network_name?: string;
  used_by_apps?: string[];
  // Input dimensions from HEF
  input_width?: number;
  input_height?: number;
  // Schema-driven config; the API may send a parsed object or a JSON string
  config?: unknown;
}

interface ModelDetailDialogProps {
  model: ModelData | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** runtime actions surfaced in the dialog footer (card/list pass theirs) */
  onLoad?: (modelId: string) => void;
  onUnload?: (modelId: string, modelName: string) => void;
  loadingAction?: string | null;
}

/** Chips shown before the "+N more" fold kicks in. */
const LABELS_FOLD_MAX = 8;

// Format timestamp to readable time
const formatTimestamp = (timestamp: number | undefined): string => {
  if (!timestamp) return '-';
  const date = new Date(timestamp * 1000);
  return date.toLocaleString();
};

// Format file size
const formatFileSize = (bytes: number | undefined): string => {
  if (!bytes) return '-';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
};

const formatIoSummary = (io: unknown): string => {
  if (!io) return '-';
  if (Array.isArray(io)) return `${io.length}`;
  if (typeof io === 'object') {
    return `${Object.keys(io as Record<string, unknown>).length}`;
  }
  return '-';
};

const hasNonEmptyString = (value: unknown): value is string => typeof value === 'string' && value.trim().length > 0;

const hasNumber = (value: unknown): value is number => typeof value === 'number' && !Number.isNaN(value);

/** Config arrives as a parsed object or a JSON string; normalize to a map. */
const parseModelConfig = (config: unknown): Record<string, unknown> | null => {
  if (!config) return null;
  if (typeof config === 'object' && !Array.isArray(config)) {
    return config as Record<string, unknown>;
  }
  if (typeof config === 'string' && config.trim().startsWith('{')) {
    try {
      const parsed: unknown = JSON.parse(config);
      if (
        typeof parsed === 'object'
        && parsed !== null
        && !Array.isArray(parsed)
      ) {
        return parsed as Record<string, unknown>;
      }
    } catch {
      return null;
    }
  }
  return null;
};

/** Labels live in config.labels as an array or comma-separated string. */
const extractLabels = (config: Record<string, unknown> | null): string[] => {
  const raw: unknown = config?.labels;
  if (Array.isArray(raw)) {
    return raw.map(item => String(item).trim()).filter(Boolean);
  }
  if (typeof raw === 'string') {
    return raw
      .split(/[,，\n]/)
      .map(item => item.trim())
      .filter(Boolean);
  }
  return [];
};

// Get model type info using shared utilities
const getModelTypeInfo = (
  modelType: string | undefined,
  modelId: string,
  t: any
): { type: string; description: string } => ({
  type: getModelTypeLabel(modelType, modelId, t),
  description: getModelTypeDescription(modelType, modelId, t),
});

export default function ModelDetailDialog({
  model,
  open,
  onOpenChange,
  onLoad,
  onUnload,
  loadingAction,
}: ModelDetailDialogProps) {
  const { t } = useTranslation();
  const [labelsExpanded, setLabelsExpanded] = useState(false);
  const [unloadConfirmOpen, setUnloadConfirmOpen] = useState(false);

  const modelId = open ? (model?.model_id ?? '') : '';
  const { data: modelDetail } = useModelInfo(modelId);
  const exportMutation = useExportModel();
  const { toast } = useToast();

  if (!model) return null;
  const mergedModel: ModelData =    modelDetail && typeof modelDetail === 'object'
      ? { ...model, ...(modelDetail as Partial<ModelData>) }
      : model;

  const typeInfo = getModelTypeInfo(
    mergedModel.model_type,
    mergedModel.model_id,
    t
  );
  const inputSize =    hasNumber(mergedModel.input_width) && hasNumber(mergedModel.input_height)
      ? `${mergedModel.input_width} × ${mergedModel.input_height}`
      : null;
  const appsCount = mergedModel.used_by_apps?.length || 0;
  const isLoaded = mergedModel.status === 'loaded';
  const isActing = loadingAction === mergedModel.model_id;

  // Only DB-backed device models (with a content hash) can be exported as
  // AMPK packages; runtime-only fallback rows have nothing to package.
  const canExport = hasNonEmptyString(mergedModel.file_hash);
  const isRawMode = mergedModel.output_mode === 'raw';

  const config = parseModelConfig(mergedModel.config);
  const labels = extractLabels(config);
  const nmsThreshold = config?.nms_threshold;
  const hasNmsThreshold = hasNumber(nmsThreshold);

  const hasPostprocessSection =    hasNonEmptyString(mergedModel.output_mode)
    || hasNumber(mergedModel.threshold)
    || hasNumber(mergedModel.max_detections)
    || hasNmsThreshold
    || labels.length > 0;

  const visibleLabels = labelsExpanded
    ? labels
    : labels.slice(0, LABELS_FOLD_MAX);
  const hiddenLabelsCount = labels.length - visibleLabels.length;

  const handleExport = () => {
    exportMutation.mutate(mergedModel.model_id, {
      onSuccess: () => {
        toast({
          title: t('sys.ai_models.detail.export_started', '导出已开始'),
          description: `${mergedModel.model_id}.bin`,
        });
      },
      onError: (error: any) => {
        toast({
          title: t('sys.ai_models.detail.export_failed', '导出失败'),
          description: error?.response?.data?.message || error?.message,
          variant: 'destructive',
        });
      },
    });
  };

  // One labelled KV cell of a section grid.
  const item = (
    icon: LucideIcon,
    label: string,
    node: React.ReactNode,
    wide = false
  ) => {
    const Icon = icon;
    return (
      <div className={`min-w-0 space-y-1 ${wide ? 'col-span-2' : ''}`}>
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Icon className="w-3.5 h-3.5 shrink-0" />
          {label}
        </div>
        <div className="text-sm font-medium break-words">{node}</div>
      </div>
    );
  };

  // Grouped section wrapper — title row + two-column grid body.
  const section = (title: string, children: React.ReactNode) => (
    <section className="space-y-3">
      <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground border-b border-border pb-2">
        {title}
      </h4>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 min-w-0">
        {children}
      </div>
    </section>
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] flex flex-col overflow-hidden">
        <DialogHeader className="min-w-0 shrink-0">
          <DialogTitle className="flex items-start gap-2 min-w-0">
            <div className="w-8 h-8 shrink-0 rounded-lg flex items-center justify-center bg-primary/10 text-primary">
              {getModelIcon(
                mergedModel.model_type,
                mergedModel.model_id,
                'w-4 h-4'
              )}
            </div>
            <span className="min-w-0 flex-1 break-all leading-snug">
              {mergedModel.name || mergedModel.model_id}
            </span>
          </DialogTitle>
          <DialogDescription className="sr-only">
            {t('sys.ai_models.detail.description', '模型详情')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5 py-4 flex-1 min-h-0 overflow-y-auto">
          {/* Badges + type description */}
          <div className="space-y-2">
            <div className="flex items-center gap-2 flex-wrap">
              <Badge variant="secondary" className="rounded-full">
                {typeInfo.type}
              </Badge>
              {mergedModel.variant && (
                <Badge
                  variant="outline"
                  className="max-w-full rounded-full text-xs break-words whitespace-normal"
                >
                  {mergedModel.variant}
                </Badge>
              )}
              {mergedModel.output_mode && (
                <Badge
                  variant={isRawMode ? 'outline' : 'secondary'}
                  className="rounded-full text-xs"
                >
                  {isRawMode
                    ? t('sys.ai_models.detail.output_mode_raw', '裸张量')
                    : t(
                        'sys.ai_models.detail.output_mode_platform',
                        '平台解码'
                      )}
                </Badge>
              )}
            </div>
            <p className="text-sm text-muted-foreground break-words">
              {typeInfo.description}
            </p>
          </div>

          {/* 基本信息 */}
          {section(
            t('sys.ai_models.detail.section_basic', '基本信息'),
            <>
              {item(
                Tag,
                t('sys.ai_models.detail.model_id', '模型 ID'),
                <span className="font-mono break-all">
                  {mergedModel.model_id}
                </span>,
                true
              )}
              {hasNonEmptyString(mergedModel.model_type)
                && item(
                  Tag,
                  t('sys.ai_models.detail.model_type', '模型类型'),
                  mergedModel.model_type
                )}
              {hasNonEmptyString(mergedModel.variant)
                && item(
                  Tag,
                  t('sys.ai_models.detail.variant', '变体'),
                  mergedModel.variant
                )}
              {hasNonEmptyString(mergedModel.version)
                && item(
                  Tag,
                  t('sys.ai_models.detail.version', '版本'),
                  mergedModel.version
                )}
              {inputSize
                && item(
                  ExternalLink,
                  t('sys.ai_models.detail.input_size', '输入尺寸'),
                  inputSize
                )}
              {hasNonEmptyString(mergedModel.network_name)
                && item(
                  Tag,
                  t('sys.ai_models.detail.network_name', '网络名称'),
                  mergedModel.network_name
                )}
            </>
          )}

          {/* 运行状态 */}
          {section(
            t('sys.ai_models.detail.section_runtime', '运行状态'),
            <>
              {item(
                Clock,
                t('sys.ai_models.detail.status', '状态'),
                <Badge
                  variant={isLoaded ? 'default' : 'secondary'}
                  className={`text-xs ${
                    isLoaded ? 'bg-emerald-600 hover:bg-emerald-700' : ''
                  }`}
                >
                  {isLoaded
                    ? t('sys.ai_models.status.loaded', '已加载')
                    : t('sys.ai_models.status.uploaded', '未加载')}
                </Badge>
              )}
              {hasNumber(mergedModel.load_timestamp)
                && item(
                  Clock,
                  t('sys.ai_models.detail.load_time', '加载时间'),
                  formatTimestamp(mergedModel.load_timestamp)
                )}
              {hasNumber(mergedModel.estimated_tops)
                && item(
                  HardDrive,
                  t('sys.ai_models.detail.estimated_tops', '预估算力'),
                  `${mergedModel.estimated_tops}`
                )}
              {hasNumber(mergedModel.estimated_memory)
                && item(
                  HardDrive,
                  t('sys.ai_models.detail.estimated_memory', '预估内存'),
                  `${mergedModel.estimated_memory}`
                )}
              {mergedModel.inputs !== null
                && mergedModel.inputs !== undefined
                && item(
                  ExternalLink,
                  t('sys.ai_models.detail.inputs', '输入'),
                  formatIoSummary(mergedModel.inputs)
                )}
              {mergedModel.outputs !== null
                && mergedModel.outputs !== undefined
                && item(
                  ExternalLink,
                  t('sys.ai_models.detail.outputs', '输出'),
                  formatIoSummary(mergedModel.outputs)
                )}
            </>
          )}

          {/* 后处理参数 */}
          {hasPostprocessSection
            && section(
              t('sys.ai_models.detail.section_postprocess', '后处理参数'),
              <>
                {hasNonEmptyString(mergedModel.output_mode)
                  && item(
                    Settings2,
                    t('sys.ai_models.form.output_mode', '输出模式'),
                    isRawMode
                      ? t('sys.ai_models.detail.output_mode_raw', '裸张量')
                      : t(
                          'sys.ai_models.detail.output_mode_platform',
                          '平台解码'
                        )
                  )}
                {hasNumber(mergedModel.threshold)
                  && item(
                    Settings2,
                    t('sys.ai_models.detail.threshold', '置信阈值'),
                    `${(mergedModel.threshold * 100).toFixed(0)}%`
                  )}
                {hasNumber(mergedModel.max_detections)
                  && item(
                    Settings2,
                    t('sys.ai_models.detail.max_detections', '最大检测数'),
                    `${mergedModel.max_detections}`
                  )}
                {hasNmsThreshold
                  && item(
                    Settings2,
                    t('sys.ai_models.detail.nms_threshold', 'NMS 阈值'),
                    `${nmsThreshold}`
                  )}
                {labels.length > 0
                  && item(
                    Tag,
                    t('sys.ai_models.detail.labels', '类别标签'),
                    <div className="flex flex-wrap items-center gap-1.5">
                      {visibleLabels.map(label => (
                        <Badge
                          key={label}
                          variant="secondary"
                          className="text-xs font-mono"
                        >
                          {label}
                        </Badge>
                      ))}
                      {(hiddenLabelsCount > 0 || labelsExpanded) && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-6 px-2 text-xs text-muted-foreground"
                          onClick={() => setLabelsExpanded(prev => !prev)}
                        >
                          {labelsExpanded
                            ? t('sys.ai_models.detail.labels_collapse', '收起')
                            : t(
                                'sys.ai_models.detail.labels_more',
                                '还有 {{count}} 个',
                                { count: hiddenLabelsCount }
                              )}
                        </Button>
                      )}
                    </div>,
                    true
                  )}
              </>
            )}

          {/* 关联与文件 */}
          {section(
            t('sys.ai_models.detail.section_files', '关联与文件'),
            <>
              {appsCount > 0
                && item(
                  AppWindow,
                  `${t('sys.ai_models.detail.used_by_apps', '关联应用')} (${appsCount})`,
                  <div className="flex flex-wrap gap-1.5">
                    {mergedModel.used_by_apps?.map((appId: string) => (
                      <Badge
                        key={appId}
                        variant="secondary"
                        className="text-xs"
                      >
                        {appId}
                      </Badge>
                    ))}
                  </div>,
                  true
                )}
              {hasNumber(mergedModel.file_size)
                && item(
                  HardDrive,
                  t('sys.ai_models.detail.file_size', '文件大小'),
                  formatFileSize(mergedModel.file_size)
                )}
              {hasNonEmptyString(mergedModel.model_path)
                && item(
                  FolderOpen,
                  t('sys.ai_models.detail.model_path', '模型路径'),
                  <code className="block w-full rounded-lg bg-muted/50 px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap">
                    {mergedModel.model_path}
                  </code>,
                  true
                )}
              {hasNonEmptyString(mergedModel.file_hash)
                && item(
                  Hash,
                  t('sys.ai_models.detail.file_hash', '文件哈希'),
                  <code className="block w-full rounded-lg bg-muted/50 px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap">
                    {mergedModel.file_hash}
                  </code>,
                  true
                )}
            </>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 shrink-0">
          {onLoad
            && onUnload
            && (isLoaded ? (
              <Button
                variant="outline"
                disabled={isActing}
                onClick={() => setUnloadConfirmOpen(true)}
              >
                {isActing ? (
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                ) : (
                  <PowerOff className="w-4 h-4 mr-2" />
                )}
                {t('sys.ai_models.action.unload', '卸载')}
              </Button>
            ) : (
              <Button
                disabled={isActing}
                onClick={() => onLoad(mergedModel.model_id)}
              >
                {isActing ? (
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                ) : (
                  <Power className="w-4 h-4 mr-2" />
                )}
                {t('sys.ai_models.action.load', '加载')}
              </Button>
            ))}
          {canExport && (
            <Button
              variant="outline"
              onClick={handleExport}
              disabled={exportMutation.isPending}
            >
              <Download className="w-4 h-4 mr-2" />
              {exportMutation.isPending
                ? t('sys.ai_models.detail.exporting', '导出中...')
                : t('sys.ai_models.detail.export_bin', '导出 .bin')}
            </Button>
          )}
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.close', '关闭')}
          </Button>
        </div>
      </DialogContent>

      {/* Unload confirmation — mirrors the card/list dialogs */}
      <AlertDialog open={unloadConfirmOpen} onOpenChange={setUnloadConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('sys.ai_models.confirm.unload_title', '确认卸载')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'sys.ai_models.confirm.unload',
                '确认将模型 "{{name}}" 从 NPU 卸载？',
                { name: mergedModel.name || mergedModel.model_id }
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel', '取消')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                onUnload?.(
                  mergedModel.model_id,
                  mergedModel.name || mergedModel.model_id
                );
                setUnloadConfirmOpen(false);
              }}
            >
              {t('sys.ai_models.action.unload', '卸载')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Dialog>
  );
}
