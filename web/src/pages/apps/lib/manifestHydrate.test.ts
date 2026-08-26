import { describe, expect, it } from 'vitest';

import type { AppManifestDTO, WizardConfig } from '@/services/types';
import {
  canonicalize,
  changedPatchFields,
  isDirty,
  manifestToWizardConfig,
  wizardConfigToPatchFields,
} from './manifestHydrate';

const manifest: AppManifestDTO = {
  apiVersion: 'v1',
  kind: 'Application',
  metadata: {
    id: 'demo-app',
    name: 'Demo',
    version: '1.2.3',
    description: 'a demo',
  },
  spec: {
    image: 'docker.io/library/alpine:latest',
    permissions: {
      video: ['camera_front'],
      inference: { models: ['clip_vit_b_32'], max_qps: 5 },
      device: { light: true },
      network: { mode: 'host', inbound: [8080] },
    },
    resources: { cpu: '50%', memory: '256Mi' },
    env: [{ name: 'A', value: '1' }],
    volumes: [{ host: '/data', container: '/data' }],
    autostart: true,
    restart_policy: 'on-failure',
    security: { no_new_privileges: true, readonly_rootfs: false },
    // present on the DTO but not wizard-expressible
    models: { clip: { id: 'clip_vit_b_32', required: true } },
  },
};

describe('canonicalize', () => {
  it('ignores key insertion order', () => {
    expect(canonicalize({ a: 1, b: 2 })).toBe(canonicalize({ b: 2, a: 1 }));
  });

  it('treats array order as significant', () => {
    expect(canonicalize([1, 2])).not.toBe(canonicalize([2, 1]));
  });

  it('drops undefined keys so sparse spread updates stay equal', () => {
    expect(canonicalize({ a: 1, b: undefined })).toBe(canonicalize({ a: 1 }));
  });
});

describe('manifestToWizardConfig', () => {
  it('maps every wizard field and deep-copies arrays', () => {
    const config = manifestToWizardConfig(manifest);
    expect(config.metadata.id).toBe('demo-app');
    expect(config.permissions?.inference?.models).toEqual(['clip_vit_b_32']);
    expect(config.security).toEqual({ no_new_privileges: true, readonly_rootfs: false });
    expect(config.env).toEqual([{ name: 'A', value: '1' }]);

    // Independent copy: mutating the config must not touch the manifest.
    config.permissions!.inference!.models!.push('x');
    expect(manifest.spec.permissions?.inference?.models).toEqual([
      'clip_vit_b_32',
    ]);
  });

  it('keeps manifest-omitted fields undefined instead of defaulting', () => {
    const config = manifestToWizardConfig({
      metadata: { id: 'a', name: 'A', version: '1' },
      spec: {},
    });
    expect(config.resources?.cpu).toBeUndefined();
    expect(config.autostart).toBeUndefined();
    expect(config.security).toBeUndefined();
    expect(config.permissions?.video).toBeUndefined();
  });
});

describe('wizardConfigToPatchFields', () => {
  it('never emits id, image or spec.models', () => {
    const fields = wizardConfigToPatchFields(manifestToWizardConfig(manifest));
    expect(Object.keys(fields)).not.toContain('metadata.id');
    expect(Object.keys(fields)).not.toContain('spec.image');
    expect(Object.keys(fields)).not.toContain('spec.models');
  });

  it('omits empty descriptions so absent fields stay absent', () => {
    const fields = wizardConfigToPatchFields({
      metadata: { id: 'a', name: 'A', version: '1', description: '' },
      image: '',
    });
    expect(fields).not.toHaveProperty('metadata.description');
  });
});

describe('dirty tracking', () => {
  const hydrated = manifestToWizardConfig(manifest);

  it('reports clean on the untouched snapshot', () => {
    expect(isDirty(hydrated, hydrated)).toBe(false);
    expect(Object.keys(changedPatchFields(hydrated, hydrated))).toHaveLength(0);
  });

  it('stays clean when only key order changed via spread updates', () => {
    // A spread-style setConfig that rebuilds permissions before metadata
    // produces a different key order but the same content.
    const reordered: WizardConfig = {
      permissions: { ...hydrated.permissions },
      metadata: { ...hydrated.metadata },
      image: hydrated.image,
      resources: { ...hydrated.resources },
      env: hydrated.env?.map(e => ({ ...e })),
      volumes: hydrated.volumes?.map(v => ({ ...v })),
      autostart: hydrated.autostart,
      restart_policy: hydrated.restart_policy,
      security: { ...hydrated.security },
    };
    expect(isDirty(reordered, hydrated)).toBe(false);
  });

  it('reports exactly the edited field', () => {
    const edited: WizardConfig = {
      ...hydrated,
      metadata: { ...hydrated.metadata, name: 'Renamed' },
    };
    const changed = changedPatchFields(edited, hydrated);
    expect(Object.keys(changed)).toEqual(['metadata.name']);
    expect(changed['metadata.name']).toBe('Renamed');
  });

  it('detects a reverted-then-changed array', () => {
    const edited: WizardConfig = {
      ...hydrated,
      permissions: {
        ...hydrated.permissions,
        inference: {
          ...hydrated.permissions!.inference,
          models: ['clip_vit_b_32', 'yolo_v5'],
        },
      },
    };
    const changed = changedPatchFields(edited, hydrated);
    expect(changed['spec.permissions.inference.models']).toEqual([
      'clip_vit_b_32',
      'yolo_v5',
    ]);
  });

  it('is clean against a null snapshot (non-package sources)', () => {
    expect(isDirty(hydrated, null)).toBe(false);
    expect(changedPatchFields(hydrated, null)).toEqual({});
  });
});
