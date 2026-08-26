import type { AppManifestDTO, WizardConfig } from '@/services/types';

/**
 * mode-3 (upload app.yaml) hydration: the server returns the full parsed
 * manifest, these pure functions map it onto the wizard form state and back
 * into PATCH-able field edits. Kept pure so it can be tested without React.
 */

/**
 * Deterministic JSON-ish string: objects get keys sorted recursively, arrays
 * keep order (order changes in arrays are real changes). Two configs that
 * differ only in key insertion order canonicalize identically, so spread-style
 * setConfig updates never report a false dirty.
 */
export function canonicalize(value: unknown): string {
  if (Array.isArray(value)) {
    return `[${value.map(canonicalize).join(',')}]`;
  }
  if (value !== null && typeof value === 'object') {
    const record = value as Record<string, unknown>;
    const body = Object.keys(record)
      .filter(key => record[key] !== undefined)
      .sort()
      .map(key => `${JSON.stringify(key)}:${canonicalize(record[key])}`)
      .join(',');
    return `{${body}}`;
  }
  return JSON.stringify(value) ?? 'null';
}

/** True when the current wizard config differs from the hydrated snapshot. */
export function isDirty(
  current: WizardConfig,
  hydrated: WizardConfig | null
): boolean {
  if (!hydrated) return false;
  return canonicalize(current) !== canonicalize(hydrated);
}

/**
 * Map a parsed manifest onto the wizard form. Fields the manifest omits stay
 * undefined (not defaulted) so an untouched walk-through of the wizard is
 * byte-identical to the uploaded file — defaults would fake edits.
 */
export function manifestToWizardConfig(m: AppManifestDTO): WizardConfig {
  const spec = m.spec ?? ({} as AppManifestDTO['spec']);
  const perms = spec.permissions ?? {};
  const inf = perms.inference ?? {};
  const events = perms.events ?? {};
  const device = perms.device ?? {};
  const network = perms.network ?? {};
  const meta = m.metadata;

  return {
    metadata: {
      id: meta?.id ?? '',
      name: meta?.name ?? '',
      version: meta?.version ?? '',
      description: meta?.description ?? '',
    },
    image: spec.image ?? '',
    resources: {
      ...(spec.resources?.cpu !== undefined && { cpu: spec.resources.cpu }),
      ...(spec.resources?.memory !== undefined && {
        memory: spec.resources.memory,
      }),
    },
    permissions: {
      ...(perms.video !== undefined && { video: [...perms.video] }),
      inference: {
        ...(inf.models !== undefined && { models: [...inf.models] }),
        ...(inf.max_qps !== undefined && { max_qps: inf.max_qps }),
        ...(inf.max_concurrent !== undefined && {
          max_concurrent: inf.max_concurrent,
        }),
        ...(inf.allow_register_model !== undefined && {
          allow_register_model: inf.allow_register_model,
        }),
      },
      events: {
        ...(events.publish !== undefined && { publish: [...events.publish] }),
        ...(events.subscribe !== undefined && {
          subscribe: [...events.subscribe],
        }),
      },
      device: {
        ...(device.light !== undefined && { light: device.light }),
        ...(device.ir_cut !== undefined && { ir_cut: device.ir_cut }),
        ...(device.ptz !== undefined && { ptz: device.ptz }),
        ...(device.lens !== undefined && { lens: device.lens }),
      },
      network: {
        ...(network.mode !== undefined && { mode: network.mode }),
        ...(network.inbound !== undefined && { inbound: [...network.inbound] }),
      },
    },
    ...(spec.env !== undefined && { env: spec.env.map(e => ({ ...e })) }),
    ...(spec.volumes !== undefined && {
      volumes: spec.volumes.map(v => ({ ...v })),
    }),
    ...(spec.autostart !== undefined && { autostart: spec.autostart }),
    ...(spec.restart_policy !== undefined && {
      restart_policy: spec.restart_policy,
    }),
    ...(spec.security !== undefined && {
      security: {
        ...(typeof spec.security.no_new_privileges === 'boolean' && {
          no_new_privileges: spec.security.no_new_privileges,
        }),
        ...(typeof spec.security.readonly_rootfs === 'boolean' && {
          readonly_rootfs: spec.security.readonly_rootfs,
        }),
      },
    }),
  };
}

/**
 * Build PATCH /apps/manifest field edits from the wizard config. Only
 * server-whitelisted paths are emitted; metadata.id (directory binding) and
 * image (tar RepoTag reconciliation) are excluded by construction. Undefined
 * fields are omitted so an untouched config produces an empty object — the
 * caller installs the original file without patching.
 */
export function wizardConfigToPatchFields(
  config: WizardConfig
): Record<string, unknown> {
  const fields: Record<string, unknown> = {};
  const set = (path: string, value: unknown) => {
    if (value !== undefined) fields[path] = value;
  };

  set('metadata.name', config.metadata?.name);
  set('metadata.version', config.metadata?.version);
  // Empty descriptions are omitted rather than written as "" so a manifest
  // without a description does not grow an empty field.
  if (config.metadata?.description) {
    set('metadata.description', config.metadata.description);
  }

  set('spec.resources.cpu', config.resources?.cpu);
  set('spec.resources.memory', config.resources?.memory);
  set('spec.autostart', config.autostart);
  set('spec.restart_policy', config.restart_policy);

  set('spec.permissions.video', config.permissions?.video);
  set(
    'spec.permissions.inference.models',
    config.permissions?.inference?.models
  );
  set(
    'spec.permissions.inference.max_qps',
    config.permissions?.inference?.max_qps
  );
  set(
    'spec.permissions.inference.max_concurrent',
    config.permissions?.inference?.max_concurrent
  );
  set(
    'spec.permissions.inference.allow_register_model',
    config.permissions?.inference?.allow_register_model
  );
  set('spec.permissions.events.publish', config.permissions?.events?.publish);
  set(
    'spec.permissions.events.subscribe',
    config.permissions?.events?.subscribe
  );
  set('spec.permissions.device.light', config.permissions?.device?.light);
  set('spec.permissions.device.ir_cut', config.permissions?.device?.ir_cut);
  set('spec.permissions.device.ptz', config.permissions?.device?.ptz);
  set('spec.permissions.device.lens', config.permissions?.device?.lens);
  set('spec.permissions.network.mode', config.permissions?.network?.mode);
  set('spec.permissions.network.inbound', config.permissions?.network?.inbound);

  set('spec.env', config.env);
  set('spec.volumes', config.volumes);

  set('spec.security.no_new_privileges', config.security?.no_new_privileges);
  set('spec.security.readonly_rootfs', config.security?.readonly_rootfs);

  return fields;
}

/**
 * The effective edit set: fields whose value actually changed relative to the
 * hydrated snapshot. Drives both the "N edits" review hint and the PATCH body
 * (patching only what changed keeps the diff on disk minimal).
 */
export function changedPatchFields(
  current: WizardConfig,
  hydrated: WizardConfig | null
): Record<string, unknown> {
  if (!hydrated) return {};
  const all = wizardConfigToPatchFields(current);
  const before = wizardConfigToPatchFields(hydrated);
  const changed: Record<string, unknown> = {};
  for (const [path, value] of Object.entries(all)) {
    if (canonicalize(before[path]) !== canonicalize(value)) {
      changed[path] = value;
    }
  }
  return changed;
}
