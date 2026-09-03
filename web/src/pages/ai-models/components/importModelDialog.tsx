import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ArrowLeft, ArrowRight } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import {
  useCapabilities,
  useParseModel,
  useRegisterModelV2,
  useUpdateModel,
  useModels,
  type ModelTypeDef,
} from '@/hooks/useModels';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@/components/ui/dialog';
import { aiApi } from '@/services/api';
import SectionNav from '@/pages/apps/components/import/SectionNav';
import {
  backendFunctionForProfile,
  buildRegisterPreview,
  classifyOutputFormat,
  fieldDefaultToState,
  initialModelImportForm,
  mergeConfigOnTypeSwitch,
  modelFormIssueText,
  partitionFields,
  sanitizeModelId,
  validateModelForm,
  variantFormIssue,
  type ModelImportFormState,
  type ModelImportSectionId as SectionId,
  type ModelParseResult,
} from '../lib/modelImportFlow';
import SourceModelForm from './import/SourceModelForm';
import BasicInfoSection from './import/BasicInfoSection';
import OutputSection from './import/OutputSection';
import AdvancedVariantSection from './import/AdvancedVariantSection';
import FormJsonSwitch, { type ModelFormView } from './import/FormJsonSwitch';
import JsonPreviewPane from './import/JsonPreviewPane';

/** Existing-model shape needed to prefill the update mode. */
interface UpdateTargetModel {
  model_id: string;
  model_type?: string;
  output_mode?: string;
  variant?: string;
  vstream_info?: string;
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

type Screen = 'source' | 'configure';

/** Accept list when the capabilities schema has not loaded yet. */
const FALLBACK_ACCEPT = { 'application/octet-stream': ['.hef', '.bin'] };

/**
 * Model import wizard shell, patterned after apps' ImportAppDialog: a
 * wide two-screen dialog (source → configure) where the configure screen
 * paginates its sections through a left SectionNav and can flip to a
 * read-only JSON projection of the register payload. All parsing/register
 * side effects and their race handling live here; the pages themselves are
 * prop-driven components under ./import/.
 */
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

  const [screen, setScreen] = useState<Screen>('source');
  const [activeSection, setActiveSection] = useState<SectionId>('basic_info');
  const [view, setView] = useState<ModelFormView>('form');
  const [file, setFile] = useState<File | null>(null);
  const [parseResult, setParseResult] = useState<ModelParseResult | null>(null);
  const [form, setForm] = useState<ModelImportFormState>(
    initialModelImportForm
  );
  // Only blur/submit marks survive here — error text is derived from the
  // form via validateModelForm, so it clears itself as values become valid.
  const [touched, setTouched] = useState<Record<string, boolean>>({});
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

  // Cancel paths release the staged blob through the parse-scoped abandon
  // endpoint, whose server-side reference counting keeps any blob a
  // registered model already shares (identical uploads dedupe onto one CAS
  // blob). A generic file delete could destroy such a shared blob, so a
  // failed abandon only warns — never fall back to files.batchDelete.
  const abandonStagedFiles = async (
    staged: Array<ModelParseResult | null | undefined>
  ) => {
    const seen = new Set<string>();
    for (const result of staged) {
      if (!result?.file_hash || !result.file_path) continue;
      if (seen.has(result.file_hash)) continue;
      seen.add(result.file_hash);
      try {
        // Sequential by design: each abandon is refcounted server-side and a
        // failure must toast before the next blob is released.
        // eslint-disable-next-line no-await-in-loop
        await aiApi.abandonStaged(result.file_hash, result.file_path);
      } catch {
        toast({
          title: t(
            'sys.ai_models.wizard.abandon_failed',
            'Staged file left behind'
          ),
          description: t(
            'sys.ai_models.wizard.abandon_failed_hint',
            'Cleanup failed, but existing models are unaffected'
          ),
        });
      }
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
    if (!capabilities?.formats) return FALLBACK_ACCEPT;
    const map: Record<string, string[]> = {};
    for (const f of capabilities.formats) {
      if (!map[f.mime_type]) map[f.mime_type] = [];
      map[f.mime_type].push(f.extension);
    }
    // AMPK single-file packages (.bin = HEF + registration metadata) join
    // the .hef bucket; the server sniffs the package magic during parse.
    const octet = 'application/octet-stream';
    map[octet] = Array.from(new Set([...(map[octet] ?? []), '.bin']));
    return map;
  }, [capabilities]);

  const formatHint = useMemo(() => {
    if (!capabilities?.formats) {
      return t(
        'sys.ai_models.form.file_hint',
        'Only .hef and .bin formats are supported'
      );
    }
    const exts = capabilities.formats.map(f => f.extension);
    if (!exts.includes('.bin')) exts.push('.bin');
    return exts.join(', ');
  }, [capabilities, t]);

  const existingModelIdSet = useMemo(() => {
    const ids = new Set<string>();
    for (const m of existingModels || []) {
      if (typeof m?.model_id === 'string') {
        ids.add(m.model_id.trim().toLowerCase());
      }
    }
    return ids;
  }, [existingModels]);

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
    setScreen('source');
    setActiveSection('basic_info');
    setView('form');
    setFile(null);
    setParseResult(null);
    setTouched({});
    const rawProfile = config.postprocess_profile;
    const initialProfile = typeof rawProfile === 'string' ? rawProfile : null;
    initialProfileRef.current = initialProfile;
    setForm({
      modelId: model.model_id,
      modelType: model.model_type ?? '',
      outputMode: model.output_mode === 'raw' ? 'raw' : 'platform',
      variant: model.variant ?? '',
      config,
    });
  }, [open, isUpdate, model, modelTypeOptions]);

  const isLoading =    parseMutation.isPending
    || registerMutation.isPending
    || updateMutation.isPending;

  // Output format from the parse result (server-classified from the HEF's
  // output vstream names); fall back to the client mirror for update mode,
  // where the vstream info lives on the model row.
  const outputFormat = useMemo(() => {
    if (parseResult?.output_format) return parseResult.output_format;
    if (parseResult) return classifyOutputFormat(parseResult.vstream_info);
    if (isUpdate && typeof model?.vstream_info === 'string') {
      return classifyOutputFormat(model.vstream_info);
    }
    return '';
  }, [parseResult, isUpdate, model]);

  // Platform decode requires the NMS output layer: a feature-map detection
  // HEF cannot enter the plugin pipeline at all, so the platform card is
  // disabled with the reason shown on the card itself.
  const platformModeDisabled =    form.modelType === 'detection' && outputFormat === 'feature_map';

  // When platform decode becomes impossible, fall through to raw instead of
  // sitting on an unregisterable selection.
  useEffect(() => {
    if (platformModeDisabled && form.outputMode === 'platform') {
      setForm(prev => ({ ...prev, outputMode: 'raw' }));
    }
  }, [platformModeDisabled, form.outputMode]);

  // One validator drives everything: submit gates on issues[0] (toast +
  // jump to its section) and inline text is the same issue, gated on blur.
  const issues = useMemo(
    () => validateModelForm(form, {
        isUpdate,
        platformModeDisabled,
        existingModelIds: modelsReady ? existingModelIdSet : null,
        fields: currentFields,
      }),
    [
      form,
      isUpdate,
      platformModeDisabled,
      modelsReady,
      existingModelIdSet,
      currentFields,
    ]
  );

  const errorFor = (field: string): string | undefined => {
    if (!touched[field]) return undefined;
    const issue = issues.find(i => i.field === field);
    return issue ? modelFormIssueText(issue, t) : undefined;
  };

  // Live client mirror of the backend custom-variant guard, so a broken
  // blob is rejected before the request leaves the page (not blur-gated —
  // matches the old behavior under the textarea).
  const variantLiveText = useMemo(() => {
    const issue = variantFormIssue(form.variant);
    return issue ? modelFormIssueText(issue, t) : null;
  }, [form.variant, t]);

  // The inverse cross-check: the HEF ships the NMS output layer but is not
  // classified as detection — almost certainly miscategorized.
  const typeMismatch =    !!parseResult
    && parseResult.suggested_type !== 'detection'
    && form.modelType !== 'detection'
    && outputFormat === 'nms';

  // Update mode: hint that changing the postprocess profile reloads a
  // loaded model rather than silently keeping the old profile on the NPU.
  const profileChanged =    isUpdate
    && initialProfileRef.current !== null
    && form.config.postprocess_profile !== undefined
    && form.config.postprocess_profile !== initialProfileRef.current;

  const { basic: basicFields, postprocess: postprocessFields } = useMemo(
    () => partitionFields(currentFields),
    [currentFields]
  );

  const preview = useMemo(() => buildRegisterPreview(form), [form]);

  const sections = useMemo(
    () => [
      {
        id: 'basic_info',
        label: t('sys.ai_models.wizard.nav_basic_info', 'Basic Info'),
      },
      {
        id: 'output',
        label: t('sys.ai_models.wizard.nav_output', 'Output & Postprocess'),
      },
      {
        id: 'advanced',
        label: t('sys.ai_models.wizard.nav_advanced', 'Advanced'),
      },
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
        const result = data as ModelParseResult;
        if (cancelRequestedRef.current) {
          abandonStagedFiles([result]);
          return;
        }
        setParseResult(result);

        // AMPK packages carry their registration metadata; the prefill lets
        // the user confirm/adjust instead of re-entering everything.
        const pkg = result.package;
        const suggestedType = pkg?.model_type || result.suggested_type || '';
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
                pkg?.model_id
                  || result.network_name
                  || result.filename?.replace(/\.[^.]+$/, '')
                  || ''
              ),
          modelType: suggestedType,
          // Package imports keep their delivery mode; plain HEFs start at
          // platform decode (the auto-switch below corrects feature-map
          // detection HEFs to raw).
          outputMode: pkg?.output_mode === 'raw' ? 'raw' : 'platform',
          variant: '',
          config: { ...configDefaults, ...(pkg?.config ?? {}) },
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

  const handleContinue = () => {
    setTouched({});
    setActiveSection('basic_info');
    setView('form');
    setScreen('configure');
  };

  const handleBackToSource = () => {
    setScreen('source');
  };

  const handleClearFile = () => {
    setFile(null);
    setParseResult(null);
  };

  const handleSectionChange = (id: string) => {
    setActiveSection(id as SectionId);
  };

  const handleModelIdChange = (value: string) => {
    setForm(prev => ({ ...prev, modelId: value }));
  };

  const handleOutputModeChange = (value: string) => {
    setForm(prev => ({ ...prev, outputMode: value }));
  };

  const handleVariantChange = (value: string) => {
    setForm(prev => ({ ...prev, variant: value }));
  };

  const markModelIdTouched = () => {
    setTouched(prev => ({ ...prev, modelId: true }));
  };

  const markVariantTouched = () => {
    setTouched(prev => ({ ...prev, variant: true }));
  };

  const markFieldTouched = (key: string) => {
    setTouched(prev => ({ ...prev, [`config_${key}`]: true }));
  };

  const handleSwitchToDetection = () => handleModelTypeChange('detection');

  const handleModelTypeChange = (value: string) => {
    const typeOpt = modelTypeOptions.find(o => o.value === value);
    setForm(prev => ({
      ...prev,
      modelType: value,
      // New type's defaults overlaid with previously-entered values for keys
      // the new type also understands — keeps a tuned threshold alive across
      // a detection↔pose switch instead of silently wiping it.
      config: mergeConfigOnTypeSwitch(prev.config, typeOpt?.fields ?? []),
    }));
  };

  const updateConfig = (key: string, value: unknown) => {
    setForm(prev => ({
      ...prev,
      config: { ...prev.config, [key]: value },
    }));
  };

  // Seed the variant textarea with a schema-complete blob composed from the
  // visible form values, so the escape hatch starts from something the
  // postprocess plugin actually accepts.
  const insertVariantTemplate = () => {
    const rawProfile = form.config.postprocess_profile;
    const fallbackProfile = 'hailo_yolov8n_384_640';
    const profile =      typeof rawProfile === 'string' ? rawProfile : fallbackProfile;
    const num = (v: unknown, fallback: number) => {
      if (typeof v !== 'number' || !Number.isFinite(v) || v <= 0) {
        return fallback;
      }
      return v;
    };
    const configLabels = form.config.labels;
    const rawLabels = typeof configLabels === 'string' ? configLabels : '';
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
  };

  const handleRegister = () => {
    const newTouched: Record<string, boolean> = {
      modelId: true,
      modelType: true,
      variant: true,
      outputMode: true,
    };
    for (const f of currentFields) {
      newTouched[`config_${f.key}`] = true;
    }
    setTouched(newTouched);

    if (issues.length > 0) {
      toast({
        title: modelFormIssueText(issues[0], t),
        variant: 'destructive',
      });
      setActiveSection(issues[0].section);
      return;
    }

    if (isUpdate) {
      // No file uploaded → metadata-only update (file_hash omitted).
      updateMutation.mutate(
        {
          modelId: form.modelId.trim(),
          model_type: form.modelType,
          output_mode: form.outputMode,
          model_variant: form.variant.trim(),
          config: form.config,
          ...(parseResult
            ? {
                file_hash: parseResult.file_hash,
                file_size: parseResult.file_size,
                network_name: parseResult.network_name,
                vstream_info: parseResult.vstream_info,
                // UpdateModel's pointer semantics treat an explicit 0 as
                // "clear" — send a dimension only when the parser extracted
                // one, so an unknown width never wipes a real value.
                ...(parseResult.input_width != null && {
                  input_width: parseResult.input_width,
                }),
                ...(parseResult.input_height != null && {
                  input_height: parseResult.input_height,
                }),
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
        output_mode: form.outputMode,
        model_variant: form.variant.trim(),
        config: form.config,
        file_size: parseResult.file_size,
        network_name: parseResult.network_name,
        vstream_info: parseResult.vstream_info,
        ...(parseResult.input_width != null && {
          input_width: parseResult.input_width,
        }),
        ...(parseResult.input_height != null && {
          input_height: parseResult.input_height,
        }),
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
    setScreen('source');
    setActiveSection('basic_info');
    setView('form');
    setFile(null);
    setParseResult(null);
    setForm(initialModelImportForm);
    setTouched({});
    initialProfileRef.current = null;
  };

  const handleCancel = () => {
    cancelRequestedRef.current = true;
    abandonStagedFiles([parseResult]);
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

  const updating = isUpdate && updateMutation.isPending;
  const submitLabel = updating
    ? t('common.loading', 'Loading...')
    : isUpdate
      ? t('sys.ai_models.wizard.confirm_update', 'Update')
      : registerMutation.isPending
        ? t('common.loading', 'Loading...')
        : t('sys.ai_models.wizard.confirm_register', 'Register');

  // Mini-card above the section nav: what is being imported. A picked file
  // names itself; update mode without a new file falls back to the model id.
  const navHeader = (
    <div className="rounded-lg border border-border bg-muted/40 px-3 py-2">
      <div className="flex items-center gap-1.5">
        <span className="truncate text-sm font-medium text-foreground">
          {parseResult?.filename ?? (isUpdate ? (model?.model_id ?? '') : '')}
        </span>
        {parseResult?.package && (
          <Badge variant="outline" className="shrink-0 text-xs">
            AMPK
          </Badge>
        )}
      </div>
      <p className="mt-0.5 truncate text-xs text-muted-foreground">
        {formatHint}
      </p>
    </div>
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        onInteractOutside={e => e.preventDefault()}
        className={`flex max-h-[90vh] w-full max-w-[calc(100%-1rem)] flex-col overflow-hidden rounded-2xl border-none p-0 shadow-2xl max-sm:fixed max-sm:inset-0 max-sm:left-0 max-sm:top-0 max-sm:h-dvh max-sm:max-h-dvh max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-none sm:max-w-[1050px] ${
          screen === 'configure' ? 'sm:h-[90vh]' : ''
        }`}
      >
        <div className="p-4 pb-2 sm:p-6 sm:pb-2">
          <DialogTitle className="pr-10 text-lg sm:text-xl">
            {isUpdate
              ? t('sys.ai_models.dialog.update_title', 'Update Model')
              : t('sys.ai_models.action.import', 'Import Model')}
          </DialogTitle>
          <DialogDescription className="sr-only">
            {t(
              'sys.ai_models.wizard.source_desc',
              'Upload a model file and configure its registration'
            )}
          </DialogDescription>
        </div>

        {screen === 'source' ? (
          <div className="flex min-h-0 flex-1 flex-col">
            <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5 sm:px-6 lg:px-8">
              <h3 className="mb-1 text-base font-semibold text-foreground">
                {t('sys.ai_models.wizard.source_title', 'Choose a Model File')}
              </h3>
              <p className="mb-4 text-sm text-muted-foreground">
                {t(
                  'sys.ai_models.wizard.source_desc',
                  'Upload a bare .hef to configure by hand, or an AMPK .bin package whose metadata pre-fills the form'
                )}
              </p>
              <SourceModelForm
                file={file}
                onFileChange={handleFileChange}
                onClear={handleClearFile}
                isParsing={parseMutation.isPending}
                disabled={isLoading}
                parseResult={parseResult}
                outputFormat={outputFormat}
                acceptFormats={acceptFormats}
                formatHint={formatHint}
                isUpdate={isUpdate}
              />
            </div>
          </div>
        ) : (
          <div className="flex min-h-0 flex-1 flex-col sm:flex-row">
            <SectionNav
              sections={sections}
              activeId={activeSection}
              onActiveChange={handleSectionChange}
              header={navHeader}
            />
            <div className="flex min-h-0 flex-1 flex-col">
              <div className="flex items-center justify-between gap-2 border-b border-border px-4 py-2 sm:px-6">
                <FormJsonSwitch view={view} onChange={setView} />
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5 sm:px-6 lg:px-8">
                {view === 'json' ? (
                  <JsonPreviewPane preview={preview} />
                ) : activeSection === 'basic_info' ? (
                  <BasicInfoSection
                    form={form}
                    onModelIdChange={handleModelIdChange}
                    onModelTypeChange={handleModelTypeChange}
                    onBlurModelId={markModelIdTouched}
                    modelTypeOptions={modelTypeOptions}
                    basicFields={basicFields}
                    isUpdate={isUpdate}
                    disabled={isLoading}
                    errorFor={errorFor}
                    onBlurField={markFieldTouched}
                    onConfigChange={updateConfig}
                  />
                ) : activeSection === 'output' ? (
                  <OutputSection
                    outputMode={form.outputMode}
                    onOutputModeChange={handleOutputModeChange}
                    platformModeDisabled={platformModeDisabled}
                    postprocessFields={postprocessFields}
                    config={form.config}
                    typeMismatch={typeMismatch}
                    onSwitchToDetection={handleSwitchToDetection}
                    profileChanged={profileChanged}
                    disabled={isLoading}
                    errorFor={errorFor}
                    onBlurField={markFieldTouched}
                    onConfigChange={updateConfig}
                  />
                ) : (
                  <AdvancedVariantSection
                    variant={form.variant}
                    onChange={handleVariantChange}
                    onBlur={markVariantTouched}
                    liveErrorText={variantLiveText}
                    onInsertTemplate={insertVariantTemplate}
                    isRawMode={form.outputMode === 'raw'}
                    disabled={isLoading}
                  />
                )}
              </div>
            </div>
          </div>
        )}

        <div className="flex flex-row items-center gap-2 border-t border-border bg-muted/20 px-4 py-3 sm:justify-between sm:px-6 sm:py-4">
          {screen === 'source' ? (
            <>
              <Button
                variant="outline"
                onClick={handleCancel}
                disabled={isLoading}
                className="flex-1 sm:flex-none"
              >
                {t('common.cancel', 'Cancel')}
              </Button>
              <Button
                variant="carbon"
                onClick={handleContinue}
                disabled={(!parseResult && !isUpdate) || isLoading}
                className="flex-1 sm:flex-none"
              >
                {t('common.next', 'Next')}
                <ArrowRight className="ml-2 h-4 w-4" />
              </Button>
            </>
          ) : (
            <>
              <Button
                variant="outline"
                onClick={handleBackToSource}
                disabled={isLoading}
                className="flex-1 sm:flex-none"
              >
                <ArrowLeft className="mr-2 h-4 w-4" />
                {t('sys.ai_models.wizard.back_to_source', 'Back to Upload')}
              </Button>
              <Button
                variant="ghost"
                onClick={handleCancel}
                disabled={isLoading}
                className="hidden flex-1 sm:flex-none sm:inline-flex"
              >
                {t('common.cancel', 'Cancel')}
              </Button>
              <Button
                variant="carbon"
                onClick={handleRegister}
                disabled={isLoading}
                className="flex-1 sm:flex-none"
              >
                {submitLabel}
              </Button>
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
