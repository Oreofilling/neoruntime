import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import {
  Check,
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Info,
  Settings2,
} from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import {
  useCapabilities,
  useParseModel,
  useRegisterModelV2,
  useUpdateModel,
  type ModelTypeDef,
  type ModelFieldDef,
} from '@/hooks/useModels';
import { useModels } from '@/hooks/useModels';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { Textarea } from '@/components/ui/textarea';
import FileUpload from '@/components/file-upload';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { filesApi } from '@/services/api';

/** Existing-model shape needed to prefill the update mode. */
interface UpdateTargetModel {
  model_id: string;
  model_type?: string;
  variant?: string;
  config?: Record<string, unknown>;
  [key: string]: unknown;
}

interface ImportModelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => unknown;
  /** update = edit an existing model (model_id fixed, file optional) */
  mode?: 'create' | 'update';
  /** required when mode="update" — the model being edited */
  model?: UpdateTargetModel | null;
}

interface ParseResult {
  file_hash: string;
  file_path: string;
  file_size: number;
  filename: string;
  network_name: string;
  vstream_info: string;
  suggested_type: string;
  format: string;
  input_width?: number;
  input_height?: number;
}

interface FormState {
  modelId: string;
  modelType: string;
  variant: string;
  config: Record<string, unknown>;
}

const initialFormState: FormState = {
  modelId: '',
  modelType: '',
  variant: '',
  config: {},
};

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function sanitizeModelId(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9_-]/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_|_$/g, '');
}

/** Plugin schema keys a custom `{…}` variant must carry — mirrors the
 *  backend guard so a broken blob is caught before the request leaves. */
const CUSTOM_VARIANT_KEYS = [
  'backend_function',
  'iou_threshold',
  'detection_threshold',
  'output_activation',
  'label_offset',
  'max_boxes',
  'labels',
];

/** Verified postprocess functions. Generic single-argument ones hardcode a
 *  0.4 threshold and COCO labels, so the UI steers users away from them. */
const SUPPORTED_BACKEND_FUNCTIONS = [
  'hailo_yolov8n',
  'hailo_yolov8s',
  'hailo_yolov8m',
];

type VariantIssue =
  | { kind: 'invalid-json' }
  | { kind: 'missing-keys'; keys: string[] }
  | { kind: 'unsupported-function'; fn: string };

/** Validate a custom variant blob client-side. Plain (non-JSON) variants
 *  keep their existing passthrough handling and always return null. */
function checkCustomVariant(variant: string): VariantIssue | null {
  const trimmed = variant.trim();
  if (!trimmed.startsWith('{')) return null;
  let parsed: Record<string, unknown>;
  try {
    parsed = JSON.parse(trimmed) as Record<string, unknown>;
  } catch {
    return { kind: 'invalid-json' };
  }
  const missing = CUSTOM_VARIANT_KEYS.filter(k => !(k in parsed));
  if (missing.length > 0) {
    return { kind: 'missing-keys', keys: missing };
  }
  const fn = parsed.backend_function;
  if (
    typeof fn !== 'string'
    || !SUPPORTED_BACKEND_FUNCTIONS.includes(fn)
  ) {
    return { kind: 'unsupported-function', fn: typeof fn === 'string' ? fn : String(fn) };
  }
  return null;
}

/** Postprocess profile basename → plugin backend function. */
function backendFunctionForProfile(profile: string): string {
  if (profile.includes('yolov8s')) return 'hailo_yolov8s';
  if (profile.includes('yolov8m')) return 'hailo_yolov8m';
  return 'hailo_yolov8n';
}

/** Map a variant check to user-facing text — shared by the live error display
 *  below the textarea and the submit-time validation. */
function variantIssueText(issue: VariantIssue, t: TFunction): string {
  switch (issue.kind) {
    case 'invalid-json':
      return t('sys.ai_models.form.variant_invalid_json', 'Variant is not valid JSON');
    case 'missing-keys':
      return t(
        'sys.ai_models.form.variant_missing_keys',
        'Variant JSON is missing required key(s): {{keys}}',
        { keys: issue.keys.join(', ') }
      );
    case 'unsupported-function':
      return t(
        'sys.ai_models.form.variant_function_unsupported',
        'backend_function must be one of: {{fns}}',
        { fns: SUPPORTED_BACKEND_FUNCTIONS.join(', ') }
      );
    default:
      return '';
  }
}

function fieldDefaultToState(fields: ModelFieldDef[]): Record<string, unknown> {
  const config: Record<string, unknown> = {};
  for (const f of fields) {
    if (f.default !== undefined) {
      config[f.key] = f.default;
    }
  }
  return config;
}

export default function ImportModelDialog({
  open,
  onOpenChange,
  onSuccess,
  mode = 'create',
  model,
}: ImportModelDialogProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const navigate = useNavigate();

  const isUpdate = mode === 'update';

  const cancelRequestedRef = useRef(false);

  const [step, setStep] = useState(0);
  const [file, setFile] = useState<File | null>(null);
  const [parseResult, setParseResult] = useState<ParseResult | null>(null);
  const [form, setForm] = useState<FormState>(initialFormState);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [touched, setTouched] = useState<Record<string, boolean>>({});
  // Advanced section (custom variant JSON) — collapsed unless the model
  // already carries a JSON variant worth editing.
  const [advancedOpen, setAdvancedOpen] = useState(false);
  // Importing a detection HEF without the NMS output layer requires an
  // explicit acknowledgement, not just a dismissible warning.
  const [nmsAck, setNmsAck] = useState(false);
  // Update mode: profile as loaded, to hint when the selection changes.
  const initialProfileRef = useRef<string | null>(null);

  const { data: capabilities } = useCapabilities();
  const { data: existingModels = [], isSuccess: modelsReady } = useModels();
  const parseMutation = useParseModel();
  const registerMutation = useRegisterModelV2();
  const updateMutation = useUpdateModel();

  // 每次重新打开对话框时，重置取消标记，避免新一轮上传被当作“已取消”而丢弃 parseResult
  if (open && cancelRequestedRef.current) {
    cancelRequestedRef.current = false;
  }

  const cleanupUploadedFiles = async (
    paths: Array<string | undefined | null>
  ) => {
    const uniq = Array.from(new Set(paths.filter(Boolean))) as string[];
    if (uniq.length === 0) return;
    try {
      await filesApi.batchDelete(uniq);
    } catch {
      // best-effort cleanup
    }
  };

  const modelTypeOptions = useMemo(() => {
    if (!capabilities?.model_types) return [];
    return capabilities.model_types.map((mt: ModelTypeDef) => ({
      value: mt.id,
      label: t(`sys.ai_models.model_type.${mt.id}`, mt.label),
      fields: mt.fields,
    }));
  }, [capabilities, t]);

  const acceptFormats = useMemo(() => {
    if (!capabilities?.formats) return { 'application/octet-stream': ['.hef'] };
    const map: Record<string, string[]> = {};
    for (const f of capabilities.formats) {
      if (!map[f.mime_type]) map[f.mime_type] = [];
      map[f.mime_type].push(f.extension);
    }
    return map;
  }, [capabilities]);

  const formatHint = useMemo(() => {
    if (!capabilities?.formats) return t('sys.ai_models.form.file_hint', 'Only .hef format is supported');
    return capabilities.formats.map(f => f.extension).join(', ');
  }, [capabilities, t]);

  const existingModelIdSet = useMemo(
    () => new Set(
        (existingModels || [])
          .map((m: any) => (typeof m?.model_id === 'string'
              ? m.model_id.trim().toLowerCase()
              : ''))
          .filter(Boolean)
      ),
    [existingModels]
  );

  const isDuplicateModelId = (raw: string) => {
    if (!modelsReady) return false;
    const normalized = raw.trim().toLowerCase();
    return normalized !== '' && existingModelIdSet.has(normalized);
  };

  // Currently selected type's fields
  const currentFields = useMemo(() => {
    const opt = modelTypeOptions.find(o => o.value === form.modelType);
    return opt?.fields ?? [];
  }, [modelTypeOptions, form.modelType]);

  // Update mode: prefill the form from the existing model each time the
  // dialog opens (config values live at the model row's top level, e.g.
  // threshold / max_detections, falling back to schema defaults).
  useEffect(() => {
    if (!open || !isUpdate || !model) return;
    const typeOpt = modelTypeOptions.find(o => o.value === model.model_type);
    const config: Record<string, unknown> = {};
    for (const f of typeOpt?.fields ?? []) {
      const current = model[f.key];
      if (current !== undefined && current !== null && current !== '') {
        config[f.key] = current;
      } else if (f.default !== undefined) {
        config[f.key] = f.default;
      }
    }
    setStep(0);
    setFile(null);
    setParseResult(null);
    setErrors({});
    setTouched({});
    setNmsAck(false);
    const prefilledVariant = model.variant ?? '';
    setAdvancedOpen(prefilledVariant.trim().startsWith('{'));
    const initialProfile = typeof config.postprocess_profile === 'string'
      ? config.postprocess_profile
      : null;
    initialProfileRef.current = initialProfile;
    setForm({
      modelId: model.model_id,
      modelType: model.model_type ?? '',
      variant: prefilledVariant,
      config,
    });
  }, [open, isUpdate, model, modelTypeOptions]);

  const steps = useMemo(
    () => [
      { title: t('sys.ai_models.wizard.step_upload', 'Upload') },
      { title: t('sys.ai_models.wizard.step_confirm', 'Configure') },
    ],
    [t]
  );

  // Handlers
  const handleFileChange = (files: File[]) => {
    const next = files[0] || null;
    setFile(next);
    setParseResult(null);

    if (!next) return;

    const formData = new FormData();
    formData.append('model', next);

    parseMutation.mutate(formData, {
      onSuccess: (data: any) => {
        const result = data as ParseResult;
        if (cancelRequestedRef.current) {
          cleanupUploadedFiles([result?.file_path]);
          return;
        }
        setParseResult(result);
        setNmsAck(false);

        const suggestedType = result.suggested_type || '';
        const typeOpt = modelTypeOptions.find(o => o.value === suggestedType);
        const configDefaults = typeOpt
          ? fieldDefaultToState(typeOpt.fields)
          : {};

        setForm(prev => ({
          // Update mode: the model_id is fixed — a swapped file must not
          // rename the model being edited.
          modelId: isUpdate
            ? prev.modelId
            : sanitizeModelId(
                result.network_name
                  || result.filename?.replace(/\.[^.]+$/, '')
                  || ''
              ),
          modelType: suggestedType,
          variant: '',
          config: configDefaults,
        }));
      },
      onError: (error: any) => {
        toast({
          title: t(
            'sys.ai_models.wizard.parse_failed',
            'Failed to parse model'
          ),
          description: error?.response?.data?.message || error?.message,
          variant: 'destructive',
        });
      },
    });
  };

  const handleNext = () => {
    setTouched({});
    setErrors({});
    setStep(1);
  };

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {};
    if (!form.modelId.trim()) newErrors.modelId = t('sys.ai_models.form.required');
    // Update mode edits an existing id in place — the duplicate check
    // (which flags any existing id) does not apply.
    if (!newErrors.modelId && !isUpdate && isDuplicateModelId(form.modelId)) {
      newErrors.modelId = t(
        'sys.ai_models.form.model_id_exists',
        'Model ID already exists'
      );
    }
    if (!form.modelType) newErrors.modelType = t('sys.ai_models.form.required');

    // Custom variant JSON must clear the same bar the backend enforces.
    const variantIssue = checkCustomVariant(form.variant);
    if (variantIssue) newErrors.variant = variantIssueText(variantIssue, t);

    // Importing a detection HEF without the NMS output layer needs explicit
    // consent — the warning alone is too easy to scroll past.
    if (nmsIncompatible && !nmsAck) {
      newErrors.nmsAck = t(
        'sys.ai_models.wizard.nms_ack_required',
        'Please confirm the NMS compatibility warning to continue'
      );
    }

    // Validate dynamic fields
    for (const f of currentFields) {
      const value = form.config[f.key];
      if (f.required && (value === undefined || value === '')) {
        newErrors[`config_${f.key}`] = t('sys.ai_models.form.required');
        continue;
      }

      if (f.type === 'number' && value !== undefined && value !== '') {
        const n = typeof value === 'number' ? value : Number(value);
        if (!Number.isFinite(n)) {
          newErrors[`config_${f.key}`] = t(
            'sys.ai_models.form.invalid_number',
            'Please enter a valid number'
          );
          continue;
        }

        // Special validation for common detection thresholds
        if (f.key === 'threshold' && (n < 0 || n > 1)) {
          newErrors[`config_${f.key}`] = t(
            'sys.ai_models.form.threshold_range',
            'Threshold must be between 0 and 1'
          );
          continue;
        }
        if (f.key === 'nms_threshold' && (n < 0 || n > 1)) {
          newErrors[`config_${f.key}`] = t(
            'sys.ai_models.form.nms_threshold_range',
            'NMS threshold must be between 0 and 1'
          );
          continue;
        }

        // Generic min/max validation if provided by capability schema
        if (typeof f.min === 'number' && n < f.min) {
          newErrors[`config_${f.key}`] = t(
            'sys.ai_models.form.number_min',
            'Value must be ≥ {{min}}',
            { min: f.min }
          );
          continue;
        }
        if (typeof f.max === 'number' && n > f.max) {
          newErrors[`config_${f.key}`] = t(
            'sys.ai_models.form.number_max',
            'Value must be ≤ {{max}}',
            { max: f.max }
          );
        }
      }
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleModelTypeChange = (value: string) => {
    const typeOpt = modelTypeOptions.find(o => o.value === value);
    const configDefaults = typeOpt ? fieldDefaultToState(typeOpt.fields) : {};
    setForm(prev => ({
      ...prev,
      modelType: value,
      config: configDefaults,
    }));
    if (touched.modelType) {
      setErrors(prev => {
        const next = { ...prev };
        delete next.modelType;
        return next;
      });
    }
  };

  const updateConfig = (key: string, value: unknown) => {
    setForm(prev => ({
      ...prev,
      config: { ...prev.config, [key]: value },
    }));
    const errKey = `config_${key}`;
    if (touched[errKey]) {
      setErrors(prev => {
        const next = { ...prev };
        delete next[errKey];
        return next;
      });
    }
  };

  const toggleNmsAck = (checked: boolean) => {
    setNmsAck(checked);
    if (checked && touched.nmsAck) {
      setErrors(prev => {
        const next = { ...prev };
        delete next.nmsAck;
        return next;
      });
    }
  };

  // Seed the variant textarea with a schema-complete blob composed from the
  // visible form values, so the escape hatch starts from something the
  // postprocess plugin actually accepts.
  const insertVariantTemplate = () => {
    const profile = typeof form.config.postprocess_profile === 'string'
      ? form.config.postprocess_profile
      : 'hailo_yolov8n_384_640';
    const num = (v: unknown, fallback: number) => (
      typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : fallback
    );
    const rawLabels = typeof form.config.labels === 'string'
      ? form.config.labels
      : '';
    const labels = rawLabels
      .split(',')
      .map(s => s.trim())
      .filter(s => s !== '');
    const template = {
      backend_function: backendFunctionForProfile(profile),
      iou_threshold: num(form.config.nms_threshold, 0.45),
      detection_threshold: num(form.config.threshold, 0.25),
      output_activation: 'none',
      label_offset: 1,
      max_boxes: num(form.config.max_detections, 64),
      // Index 0 is a placeholder so labels[N] names class_id N.
      labels: ['unlabeled', ...labels],
    };
    setForm(prev => ({ ...prev, variant: JSON.stringify(template, null, 2) }));
    setAdvancedOpen(true);
  };

  const handleRegister = () => {
    const newTouched: Record<string, boolean> = {
      modelId: true,
      modelType: true,
      variant: true,
      ...(nmsIncompatible ? { nmsAck: true } : {}),
    };
    for (const f of currentFields) {
      newTouched[`config_${f.key}`] = true;
    }
    setTouched(newTouched);

    if (!validate()) return;

    if (isUpdate) {
      // No file uploaded → metadata-only update (file_hash omitted).
      updateMutation.mutate(
        {
          modelId: form.modelId.trim(),
          model_type: form.modelType,
          model_variant: form.variant.trim(),
          config: form.config,
          ...(parseResult
            ? {
                file_hash: parseResult.file_hash,
                file_size: parseResult.file_size,
                network_name: parseResult.network_name,
                vstream_info: parseResult.vstream_info,
                input_width: parseResult.input_width ?? 0,
                input_height: parseResult.input_height ?? 0,
              }
            : {}),
        },
        {
          onSuccess: async () => {
            toast({
              title: t(
                'sys.ai_models.message.update_success',
                'Model updated successfully'
              ),
            });
            handleReset();
            onOpenChange(false);
            await onSuccess?.();
          },
          onError: (error: any) => {
            toast({
              title: t('common.error', 'Error'),
              description: error?.response?.data?.message || error?.message,
              variant: 'destructive',
            });
          },
        }
      );
      return;
    }

    if (!parseResult) return;

    registerMutation.mutate(
      {
        file_hash: parseResult.file_hash,
        model_id: form.modelId.trim(),
        model_type: form.modelType,
        model_variant: form.variant.trim(),
        config: form.config,
        file_size: parseResult.file_size,
        network_name: parseResult.network_name,
        vstream_info: parseResult.vstream_info,
        input_width: parseResult.input_width ?? 0,
        input_height: parseResult.input_height ?? 0,
      },
      {
        onSuccess: async () => {
          toast({
            title: t(
              'sys.ai_models.message.import_success',
              'Model imported successfully'
            ),
          });
          handleReset();
          onOpenChange(false);
          await onSuccess?.();
        },
        onError: (error: any) => {
          toast({
            title: t('common.error', 'Error'),
            description: error?.response?.data?.message || error?.message,
            variant: 'destructive',
          });
        },
      }
    );
  };

  const handleReset = () => {
    setStep(0);
    setFile(null);
    setParseResult(null);
    setForm(initialFormState);
    setErrors({});
    setTouched({});
    setNmsAck(false);
    setAdvancedOpen(false);
    initialProfileRef.current = null;
  };

  const handleCancel = () => {
    cancelRequestedRef.current = true;
    cleanupUploadedFiles([parseResult?.file_path]);
    handleReset();
    onOpenChange(false);
    navigate('/models');
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      cancelRequestedRef.current = false;
      onOpenChange(true);
      return;
    }
    handleCancel();
  };

  const ph = t('sys.ai_models.form.placeholder', 'Please enter');
  const isLoading =    parseMutation.isPending
    || registerMutation.isPending
    || updateMutation.isPending;

  const nmsPresent = !!parseResult
    && (parseResult.vstream_info || '').includes('yolov8_nms_postprocess');

  // Detection HEFs whose output layers carry no yolov8_nms_postprocess
  // tensor cannot enter the Hailo yolov8 postprocess pipeline — importing
  // one anyway requires an explicit acknowledgement (nmsAck).
  const nmsIncompatible = !!parseResult
    && (form.modelType === 'detection'
      || parseResult.suggested_type === 'detection')
    && !nmsPresent;

  // Client mirror of the backend custom-variant guard, so a broken blob is
  // rejected before the request leaves the page.
  const variantErrorText = useMemo(() => {
    const issue = checkCustomVariant(form.variant);
    return issue ? variantIssueText(issue, t) : null;
  }, [form.variant, t]);

  const nmsWarning = nmsIncompatible ? (
    <div className="flex items-start gap-2 rounded-md border border-yellow-500/60 bg-yellow-500/10 p-3 text-sm text-yellow-700 dark:text-yellow-400">
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
      <span>
        {t(
          'sys.ai_models.wizard.nms_warning',
          'No yolov8_nms_postprocess output layer detected in this HEF; it may be incompatible with the Hailo yolov8 postprocess pipeline'
        )}
      </span>
    </div>
  ) : null;

  // The inverse cross-check: the HEF ships the NMS output layer but is not
  // classified as detection — almost certainly miscategorized.
  const typeMismatch = !!parseResult
    && parseResult.suggested_type !== 'detection'
    && form.modelType !== 'detection'
    && nmsPresent;

  // Update mode: hint that changing the postprocess profile reloads a
  // loaded model rather than silently keeping the old profile on the NPU.
  const profileChanged = isUpdate
    && initialProfileRef.current !== null
    && form.config.postprocess_profile !== undefined
    && form.config.postprocess_profile !== initialProfileRef.current;

  // Render a dynamic field based on its schema definition
  const renderField = (field: ModelFieldDef) => {
    const errKey = `config_${field.key}`;
    const hasError = touched[errKey] && errors[errKey];

    switch (field.type) {
      case 'number': {
        const val =          form.config[field.key] !== undefined
            ? String(form.config[field.key])
            : '';
        const effectiveMin =          field.key === 'threshold' || field.key === 'nms_threshold'
            ? 0
            : field.min;
        const effectiveMax =          field.key === 'threshold' || field.key === 'nms_threshold'
            ? 1
            : field.max;
        const effectiveStep =          field.key === 'threshold' || field.key === 'nms_threshold'
            ? (field.step ?? 0.01)
            : (field.step ?? 1);
        return (
          <div className="grid gap-2" key={field.key}>
            <Label htmlFor={field.key}>
              {t(`sys.ai_models.form.${field.key}`, field.key)}
            </Label>
            <Input
              id={field.key}
              type="number"
              step={effectiveStep}
              min={effectiveMin}
              max={effectiveMax}
              value={val}
              onChange={e => {
                const v =                  e.target.value === ''
                    ? undefined
                    : parseFloat(e.target.value);
                updateConfig(field.key, v);
              }}
              onBlur={() => {
                setTouched(prev => ({ ...prev, [errKey]: true }));
                const raw = form.config[field.key];
                if (raw === undefined || raw === '') return;
                const n = typeof raw === 'number' ? raw : Number(raw);
                if (!Number.isFinite(n)) {
                  setErrors(prev => ({
                    ...prev,
                    [errKey]: t(
                      'sys.ai_models.form.invalid_number',
                      'Please enter a valid number'
                    ),
                  }));
                  return;
                }
                if (field.key === 'threshold' && (n < 0 || n > 1)) {
                  setErrors(prev => ({
                    ...prev,
                    [errKey]: t(
                      'sys.ai_models.form.threshold_range',
                      'Threshold must be between 0 and 1'
                    ),
                  }));
                  return;
                }
                if (field.key === 'nms_threshold' && (n < 0 || n > 1)) {
                  setErrors(prev => ({
                    ...prev,
                    [errKey]: t(
                      'sys.ai_models.form.nms_threshold_range',
                      'NMS threshold must be between 0 and 1'
                    ),
                  }));
                  return;
                }
                if (typeof field.min === 'number' && n < field.min) {
                  setErrors(prev => ({
                    ...prev,
                    [errKey]: t(
                      'sys.ai_models.form.number_min',
                      'Value must be ≥ {{min}}',
                      { min: field.min }
                    ),
                  }));
                  return;
                }
                if (typeof field.max === 'number' && n > field.max) {
                  setErrors(prev => ({
                    ...prev,
                    [errKey]: t(
                      'sys.ai_models.form.number_max',
                      'Value must be ≤ {{max}}',
                      { max: field.max }
                    ),
                  }));
                }
              }}
              placeholder={ph}
              disabled={isLoading}
            />
            {hasError && (
              <p className="text-sm text-destructive">{errors[errKey]}</p>
            )}
          </div>
        );
      }
      case 'select': {
        const val = String(form.config[field.key] ?? '');
        return (
          <div className="grid gap-2" key={field.key}>
            <Label htmlFor={field.key}>
              {t(`sys.ai_models.form.${field.key}`, field.key)}
            </Label>
            <Select
              value={val}
              onValueChange={v => updateConfig(field.key, v)}
              disabled={isLoading}
            >
              <SelectTrigger id={field.key}>
                <SelectValue
                  placeholder={t(
                    'sys.ai_models.form.select_type',
                    'Please select'
                  )}
                />
              </SelectTrigger>
              <SelectContent>
                {(field.options ?? []).map(opt => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {t(
                      `sys.ai_models.form.${field.key}_${opt.value}`,
                      opt.label
                    )}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {t(`sys.ai_models.form.${field.key}_hint`, '') && (
              <p className="text-xs text-muted-foreground">
                {t(`sys.ai_models.form.${field.key}_hint`, '')}
              </p>
            )}
          </div>
        );
      }
      case 'boolean': {
        const checked = Boolean(form.config[field.key]);
        return (
          <div className="flex items-center gap-2" key={field.key}>
            <input
              type="checkbox"
              id={field.key}
              checked={checked}
              onChange={e => updateConfig(field.key, e.target.checked)}
              disabled={isLoading}
              className="h-4 w-4"
            />
            <Label htmlFor={field.key} className="font-normal">
              {t(`sys.ai_models.form.${field.key}`, field.key)}
            </Label>
          </div>
        );
      }
      case 'text': {
        const val = String(form.config[field.key] ?? '');
        const hint = t(`sys.ai_models.form.${field.key}_hint`, '');
        return (
          <div className="grid gap-2 sm:col-span-2" key={field.key}>
            <Label htmlFor={field.key}>
              {t(`sys.ai_models.form.${field.key}`, field.key)}
            </Label>
            <Input
              id={field.key}
              type="text"
              value={val}
              onChange={e => updateConfig(field.key, e.target.value)}
              placeholder={ph}
              disabled={isLoading}
            />
            {hint && (
              <p className="text-xs text-muted-foreground">{hint}</p>
            )}
          </div>
        );
      }
      default:
        return null;
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        onInteractOutside={e => e.preventDefault()}
        className="sm:max-w-xl"
      >
        <DialogHeader>
          <DialogTitle>
            {isUpdate
              ? t('sys.ai_models.dialog.update_title', 'Update Model')
              : step === 0
                ? t('sys.ai_models.action.import', 'Import Model')
                : t('sys.ai_models.wizard.step_confirm', 'Configure Model')}
          </DialogTitle>
        </DialogHeader>

        {/* Step indicator — scroll on narrow screens */}
        <div className="-mx-1 mb-4 mt-2 overflow-x-auto pb-1 sm:mx-0 sm:overflow-visible sm:pb-0">
          <div className="flex min-w-max items-center justify-center px-1 sm:min-w-0">
            {steps.map((s, i) => (
              <div key={i} className="flex items-center">
                <div className="flex flex-col items-center">
                  <div
                    className={`mb-1 flex h-7 w-7 shrink-0 items-center justify-center rounded-full border-2 text-xs transition-all sm:h-8 sm:w-8 sm:text-sm ${
                      i === step
                        ? 'border-primary bg-primary text-primary-foreground'
                        : i < step
                          ? 'border-green-500 bg-green-500 text-white'
                          : 'border-border bg-background text-muted-foreground'
                    }`}
                  >
                    {i < step ? (
                      <Check className="h-4 w-4 sm:h-5 sm:w-5" />
                    ) : (
                      i + 1
                    )}
                  </div>
                  <span
                    className={`max-w-17 truncate px-0.5 text-center text-[10px] sm:max-w-24 sm:text-xs ${i === step ? 'font-medium text-primary' : 'text-muted-foreground'}`}
                  >
                    {s.title}
                  </span>
                </div>
                {i < steps.length - 1 && (
                  <div
                    className={`-mt-4 mx-0.5 h-px w-10 shrink-0 sm:-mt-5 sm:mx-1 sm:w-14 ${i < step ? 'bg-green-500' : 'bg-border'}`}
                  />
                )}
              </div>
            ))}
          </div>
        </div>

        {step === 0 ? (
          /* Step 0: Upload + Parse (optional in update mode) */
          <div className="grid gap-4 py-2">
            {isUpdate && (
              <p className="text-sm text-muted-foreground">
                {t(
                  'sys.ai_models.wizard.update_file_optional',
                  'Optional: upload a new model file to replace the current one; skip to update configuration only'
                )}
              </p>
            )}
            <FileUpload
              single
              value={file ? [file] : []}
              onChange={handleFileChange}
              loading={parseMutation.isPending}
              disabled={isLoading}
              showFileList
              accept={acceptFormats}
              placeholder={t(
                'sys.ai_models.form.file_placeholder',
                'Drag and drop model file here'
              )}
              hint={formatHint}
            />

            {parseMutation.isPending && (
              <p className="text-sm text-muted-foreground animate-pulse">
                {t('sys.ai_models.wizard.parsing', 'Parsing model...')}
              </p>
            )}

            {parseResult && (
              <div className="rounded-lg border bg-muted/30 p-3 space-y-2">
                <div className="space-y-2 text-sm">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <Label className="shrink-0 text-muted-foreground">
                      {t('sys.ai_models.wizard.preview_title', 'Model Preview')}
                    </Label>
                    <Badge
                      variant="secondary"
                      className="max-w-full whitespace-normal break-words"
                    >
                      {t('sys.ai_models.wizard.suggested_type', 'Suggested')}:{' '}
                      {t(
                        `sys.ai_models.model_type.${parseResult.suggested_type}`,
                        parseResult.suggested_type
                      )}
                    </Badge>
                  </div>
                  <div className="flex items-start justify-between gap-4">
                    <Label className="shrink-0 text-muted-foreground">
                      {t('sys.ai_models.wizard.preview_network', 'Network')}
                    </Label>
                    <div className="min-w-0 flex-1 text-right break-all">
                      {parseResult.network_name || '—'}
                    </div>
                  </div>

                  <div className="flex items-start justify-between gap-4">
                    <Label className="shrink-0 text-muted-foreground">
                      {t('sys.ai_models.wizard.preview_file', 'File')}
                    </Label>
                    <div className="min-w-0 flex-1 text-right break-all">
                      {parseResult.filename} (
                      {formatFileSize(parseResult.file_size)})
                    </div>
                  </div>

                  <div className="flex items-start justify-between gap-4">
                    <Label className="shrink-0 text-muted-foreground">
                      {t('sys.ai_models.wizard.preview_input', 'Input')}
                    </Label>
                    <div className="min-w-0 flex-1 text-right break-all">
                      {parseResult.input_width && parseResult.input_height
                        ? `${parseResult.input_width}x${parseResult.input_height}`
                        : '—'}
                    </div>
                  </div>
                </div>
                {nmsWarning}
              </div>
            )}
          </div>
        ) : (
          /* Step 1: Configure + Register */
          <div className="grid gap-4 py-2">
            {/* Model ID */}
            <div className="grid gap-2">
              <Label htmlFor="model-id">
                {t('sys.ai_models.form.model_id', 'Model ID')} *
              </Label>
              <Input
                id="model-id"
                value={form.modelId}
                readOnly={isUpdate}
                title={
                  isUpdate
                    ? t(
                        'sys.ai_models.form.model_id_readonly',
                        'Model ID cannot be changed on update'
                      )
                    : undefined
                }
                onChange={e => {
                  setForm(prev => ({ ...prev, modelId: e.target.value }));
                  if (touched.modelId) {
                    setErrors(prev => {
                      const next = { ...prev };
                      delete next.modelId;
                      return next;
                    });
                  }
                }}
                onBlur={() => {
                  setTouched(prev => ({ ...prev, modelId: true }));
                  if (!form.modelId.trim()) {
                    setErrors(prev => ({
                      ...prev,
                      modelId: t('sys.ai_models.form.required'),
                    }));
                  } else if (isDuplicateModelId(form.modelId)) {
                    setErrors(prev => ({
                      ...prev,
                      modelId: t(
                        'sys.ai_models.form.model_id_exists',
                        'Model ID already exists'
                      ),
                    }));
                  }
                }}
                placeholder={ph}
                disabled={isLoading}
              />
              {touched.modelId && errors.modelId && (
                <p className="text-sm text-destructive">{errors.modelId}</p>
              )}
            </div>

            {/* Model Type */}
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="model-type">
                  {t('sys.ai_models.form.model_type', 'Model Type')} *
                </Label>
                <Select
                  value={form.modelType}
                  onValueChange={handleModelTypeChange}
                  disabled={isLoading}
                >
                  <SelectTrigger
                    id="model-type"
                    className={
                      touched.modelType && errors.modelType
                        ? 'border-destructive'
                        : ''
                    }
                  >
                    <SelectValue
                      placeholder={t(
                        'sys.ai_models.form.select_type',
                        'Please select'
                      )}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {modelTypeOptions.map(option => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {touched.modelType && errors.modelType && (
                  <p className="text-sm text-destructive">{errors.modelType}</p>
                )}
              </div>
            </div>

            {nmsWarning}
            {nmsIncompatible && (
              <div className="grid gap-2">
                <div className="flex items-start gap-2">
                  <input
                    type="checkbox"
                    id="nms-ack"
                    checked={nmsAck}
                    onChange={e => toggleNmsAck(e.target.checked)}
                    disabled={isLoading}
                    className="mt-0.5 h-4 w-4 shrink-0"
                  />
                  <Label htmlFor="nms-ack" className="font-normal">
                    {t(
                      'sys.ai_models.wizard.nms_confirm',
                      'I understand this HEF may not work with the Hailo yolov8 postprocess pipeline; import it anyway'
                    )}
                  </Label>
                </div>
                {touched.nmsAck && errors.nmsAck && (
                  <p className="text-sm text-destructive">{errors.nmsAck}</p>
                )}
              </div>
            )}

            {typeMismatch && (
              <div className="flex items-start justify-between gap-2 rounded-md border border-blue-500/60 bg-blue-500/10 p-3 text-sm text-blue-700 dark:text-blue-400">
                <div className="flex items-start gap-2">
                  <Info className="mt-0.5 h-4 w-4 shrink-0" />
                  <span>
                    {t(
                      'sys.ai_models.wizard.type_mismatch',
                      'This HEF carries the yolov8_nms_postprocess output layer but the type is not detection — it is likely miscategorized'
                    )}
                  </span>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => handleModelTypeChange('detection')}
                  disabled={isLoading}
                  className="shrink-0"
                >
                  {t(
                    'sys.ai_models.wizard.type_mismatch_switch',
                    'Switch to detection'
                  )}
                </Button>
              </div>
            )}

            {profileChanged && (
              <div className="flex items-start gap-2 rounded-md border border-blue-500/40 bg-blue-500/5 p-3 text-xs text-muted-foreground">
                <Info className="mt-0.5 h-4 w-4 shrink-0" />
                <span>
                  {t(
                    'sys.ai_models.wizard.profile_change_hint',
                    'Changing the postprocess profile reloads the model if it is currently loaded'
                  )}
                </span>
              </div>
            )}

            {/* Dynamic fields from schema */}
            {currentFields.length > 0 && (
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                {currentFields.map(field => renderField(field))}
              </div>
            )}

            {/* Advanced: custom variant JSON (escape hatch into the
                postprocess plugin's schema) */}
            <div className="rounded-md border">
              <button
                type="button"
                onClick={() => setAdvancedOpen(prev => !prev)}
                className="flex w-full items-center gap-2 p-3 text-sm font-medium text-muted-foreground hover:text-foreground"
              >
                {advancedOpen
                  ? <ChevronDown className="h-4 w-4" />
                  : <ChevronRight className="h-4 w-4" />}
                <Settings2 className="h-4 w-4" />
                {t('sys.ai_models.form.advanced', 'Advanced — custom variant JSON')}
              </button>
              {advancedOpen && (
                <div className="grid gap-2 border-t p-3">
                  <Label htmlFor="variant">
                    {t('sys.ai_models.form.variant', 'Variant')}
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    {t(
                      'sys.ai_models.form.variant_hint',
                      'Overrides the composed postprocess config. A {…} blob must carry the full plugin schema; leave empty to use the profile selection above'
                    )}
                  </p>
                  <Textarea
                    id="variant"
                    rows={5}
                    value={form.variant}
                    onChange={e => {
                      const { value } = e.target;
                      setForm(prev => ({ ...prev, variant: value }));
                      if (touched.variant) {
                        setErrors(prev => {
                          const next = { ...prev };
                          delete next.variant;
                          return next;
                        });
                      }
                    }}
                    placeholder={t(
                      'sys.ai_models.form.variant_placeholder',
                      'Empty for defaults, or paste a full custom JSON blob'
                    )}
                    className="font-mono text-xs"
                    disabled={isLoading}
                  />
                  {touched.variant && errors.variant && (
                    <p className="text-sm text-destructive">{errors.variant}</p>
                  )}
                  {variantErrorText && (
                    <p className="text-sm text-destructive">{variantErrorText}</p>
                  )}
                  <div>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={insertVariantTemplate}
                      disabled={isLoading}
                    >
                      {t(
                        'sys.ai_models.form.variant_template',
                        'Insert template from current values'
                      )}
                    </Button>
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        <DialogFooter>
          {step === 0 ? (
            <>
              <Button
                variant="outline"
                onClick={handleCancel}
                disabled={isLoading}
              >
                {t('common.cancel', 'Cancel')}
              </Button>
              <Button
                variant="carbon"
                onClick={handleNext}
                disabled={(!parseResult && !isUpdate) || isLoading}
              >
                {t('common.next', 'Next')}
              </Button>
            </>
          ) : (
            <>
              <Button
                variant="outline"
                onClick={() => setStep(0)}
                disabled={isLoading}
              >
                {t('common.back', 'Back')}
              </Button>
              <Button
                variant="carbon"
                onClick={handleRegister}
                disabled={isLoading || (nmsIncompatible && !nmsAck)}
              >
                {isUpdate && updateMutation.isPending
                  ? t('common.loading', 'Loading...')
                  : isUpdate
                    ? t('sys.ai_models.wizard.confirm_update', 'Update')
                    : registerMutation.isPending
                      ? t('common.loading', 'Loading...')
                      : t('sys.ai_models.wizard.confirm_register', 'Register')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
