import { useTranslation } from 'react-i18next';

export interface JsonPreviewPaneProps {
  /** buildRegisterPreview(form) output — the exact register payload. */
  preview: Record<string, unknown>;
}

/**
 * Read-only projection of the register payload, refreshed live from the
 * form. One-way by design: schema keys are validated on input, so a free
 * JSON editor would mostly produce payloads the backend rejects — JSON-level
 * customization rides the advanced section's variant textarea instead.
 */
export default function JsonPreviewPane({ preview }: JsonPreviewPaneProps) {
  const { t } = useTranslation();

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">
        {t(
          'sys.ai_models.wizard.json_preview_hint',
          'Read-only preview of the payload that will be submitted. For JSON-level customization, edit the variant in the Advanced section.'
        )}
      </p>
      <pre className="overflow-x-auto rounded-lg border border-border bg-muted/40 p-4 font-mono text-xs leading-relaxed text-foreground">
        {JSON.stringify(preview, null, 2)}
      </pre>
    </div>
  );
}
