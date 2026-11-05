import { render, type RenderContext } from 'fake-chart-ts-sdk';
import { configMapName } from './helpers.ts';

enum Kind {
  ConfigMap = 'ConfigMap',
}

await render(($: RenderContext) => ({
  apiVersion: 'v1',
  kind: Kind.ConfigMap,
  metadata: { name: configMapName() },
  data: { input: $.input },
}));
