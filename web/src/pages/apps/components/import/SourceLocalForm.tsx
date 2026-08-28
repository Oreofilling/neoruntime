import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import FileUpload from '@/components/file-upload';
import { Label } from '@/components/ui/label';
import { CheckCircle2, Package, X } from 'lucide-react';
import type { LocalMode } from '@/pages/apps/lib/importFlow';
import ImageUpload from '../ImageUpload';

export interface SourceLocalFormProps {
  manifestPath: string;
  manifestMeta: { id: string; name?: string; version?: string } | null;
  imageTarPath: string;
  imageTarName: string;
  isUploadingManifest: boolean;
  existingAppIds: Set<string>;
  /** Which local mode the current uploads resolve to (null = nothing yet). */
  localMode: LocalMode | null;
  onManifestUpload: (file: File) => void;
  /** Also resets hydrate snapshot / multi-container flag in the shell. */
  onManifestClear: () => void;
  onTarUploadSuccess: (path: string, name: string, size: number) => void;
  onTarUploadingChange: (uploading: boolean) => void;
  onTarClear: (path?: string) => void;
}

/**
 * The expanded panel of the 本地上传 source card: one optional app.yaml
 * slot + one optional image.tar slot. Both upload flows are migrated
 * verbatim from the old upload/package source steps — all upload, cleanup
 * and manifest-hydrate side effects live in the parent via callbacks.
 */
export default function SourceLocalForm({
  manifestPath,
  manifestMeta,
  imageTarPath,
  imageTarName,
  isUploadingManifest,
  existingAppIds,
  localMode,
  onManifestUpload,
  onManifestClear,
  onTarUploadSuccess,
  onTarUploadingChange,
  onTarClear,
}: SourceLocalFormProps) {
  const { t } = useTranslation();

  return (
    <div className="space-y-6">
      {localMode && (
        <div className="rounded-lg border border-border bg-muted/40 p-3 text-sm text-muted-foreground">
          {localMode === 'manifest'
            ? t(
                'sys.apps.import.local_mode_manifest_hint',
                '已上传 app.yaml：以配置文件为准，可在后续表单中微调'
              )
            : t(
                'sys.apps.import.local_mode_image_hint',
                '未上传 app.yaml：由后续表单生成配置文件'
              )}
        </div>
      )}

      {/* Manifest upload (optional) */}
      <div>
        <Label className="text-base font-semibold mb-2 block">
          {t('sys.apps.import.manifest_file', 'App Manifest (app.yaml)')}
          <span className="text-xs text-muted-foreground ml-2 font-normal">
            {t('sys.apps.import.local_manifest_optional', '可选')}
          </span>
        </Label>
        {manifestPath ? (
          <>
            <div className="flex items-center justify-between gap-2 text-sm border rounded-lg p-3">
              <div className="flex items-center gap-2 min-w-0">
                <CheckCircle2 className="w-5 h-5 text-green-500 shrink-0" />
                <span className="font-medium text-foreground truncate">
                  {manifestMeta?.id || 'app.yaml'}
                </span>
                <span className="text-muted-foreground truncate">
                  {manifestMeta?.name
                    && `(${manifestMeta.name} v${manifestMeta?.version || '1.0.0'})`}
                </span>
              </div>
              <Button variant="ghost" size="sm" onClick={onManifestClear}>
                <X className="w-4 h-4" />
              </Button>
            </div>
            {manifestMeta?.id && existingAppIds.has(manifestMeta.id) && (
              <p className="mt-1 text-sm text-red-500">
                {t(
                  'sys.apps.import.duplicate_id_warning',
                  '此应用ID已存在，安装时将覆盖已有应用'
                )}
              </p>
            )}
          </>
        ) : (
          <FileUpload
            single
            loading={isUploadingManifest}
            accept={{
              'application/x-yaml': ['.yaml', '.yml'],
              'text/yaml': ['.yaml', '.yml'],
              'text/plain': ['.yaml', '.yml'],
            }}
            placeholder={t(
              'sys.apps.import.click_upload_manifest',
              'Click to upload app.yaml'
            )}
            onUpload={async files => {
              const file = files[0];
              if (file) onManifestUpload(file);
            }}
          />
        )}
      </div>

      {/* Image upload (optional — app.yaml may reference registry image) */}
      <div>
        <Label className="text-base font-semibold mb-2 block">
          {t('sys.apps.import.package_image', 'Container Image')}
          <span className="text-xs text-muted-foreground ml-2 font-normal">
            {t(
              'sys.apps.import.package_image_hint',
              'Optional if app.yaml uses registry image'
            )}
          </span>
        </Label>
        {imageTarPath ? (
          <div className="flex items-center gap-2 text-sm border rounded-lg p-3">
            <Package className="w-5 h-5 text-muted-foreground" />
            <span className="font-medium text-foreground">
              {imageTarName || 'image.tar'}
            </span>
            <Button variant="ghost" size="sm" onClick={() => onTarClear()}>
              <X className="w-4 h-4" />
            </Button>
          </div>
        ) : (
          <ImageUpload
            onUploadSuccess={(path, imageName, size) => onTarUploadSuccess(path, imageName, size)}
            onUploadingChange={onTarUploadingChange}
            onClear={path => onTarClear(path)}
          />
        )}
      </div>
    </div>
  );
}
