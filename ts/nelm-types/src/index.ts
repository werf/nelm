export interface RenderContext {
  Values: Record<string, any>;
  Release: Release;
  Chart: ChartMetadata;
  Capabilities: Capabilities;
  Runtime: Record<string, any>;
  Files: Record<string, Uint8Array>;
}

export interface Release {
  Name: string;
  Namespace: string;
  Revision: number;
  IsInstall: boolean;
  IsUpgrade: boolean;
  Service: string;
}

export interface ChartMetadata {
  Name: string;
  Version: string;
  AppVersion: string;
  Description: string;
  Home: string;
  Icon: string;
  APIVersion: string;
  Condition: string;
  Tags: string;
  Type: string;
  Keywords: string[];
  Sources: string[];
  Maintainers: Maintainer[];
  Annotations: Record<string, string>;
}

export interface Maintainer {
  Name: string;
  Email: string;
  URL: string;
}

export interface Capabilities {
  APIVersions: string[];
  KubeVersion: KubeVersion;
  HelmVersion: HelmVersion;
}

export interface KubeVersion {
  Version: string;
  Major: string;
  Minor: string;
}

export interface HelmVersion {
  Version: string;
  GitCommit: string;
  GitTreeState: string;
  GoVersion: string;
}

export interface RenderResult {
  manifests: object[] | null;
}
