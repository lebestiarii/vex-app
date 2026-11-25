export type CardLayoutType =
  | 'group'
  | 'link'
  | 'action-list'
  | 'facts'
  | 'segments'
  | 'steps'
  | 'convert'
  | 'table'
  | 'snippet';

export const ALL_CARD_LAYOUTS: readonly CardLayoutType[] = [
  'group',
  'link',
  'action-list',
  'facts',
  'segments',
  'steps',
  'convert',
  'table',
  'snippet',
] as const;

export type ChipTone = 'neutral' | 'blue' | 'amber' | 'green' | 'red' | 'mono';
export type StoplightDot = 'green' | 'amber' | 'red';

export interface GlanceChip {
  label: string;
  tone: ChipTone;
  dot?: StoplightDot;
}

export interface CardAction {
  id: string;
  label: string;
  shortcut?: string;
  type: 'open_url' | 'open_pwa' | 'launch_app' | 'copy' | 'http' | string;
  target: string;
  env?: Record<string, string>;
}

export interface CardNode<T = Record<string, unknown>> {
  id: string;
  layout: CardLayoutType;
  icon: string;
  title: string;
  subtitle?: string;
  description?: string;
  collection?: string;
  entityKey?: string;
  parentEntityKey?: string;
  moduleId?: string;
  chips: GlanceChip[];
  data: T;
  children?: CardNode[];
  actions?: CardAction[];
  defaultExpanded?: boolean;
  pinnable?: boolean;
}

// --- Strict Typed Payloads for the 9 Pure Generic Layouts ---

export interface GroupLayoutData {
  meta?: Record<string, string | number>;
}

export interface LinkLayoutData {
  url: string;
}

export interface RowAction {
  label: string;
  type: 'copy' | 'open_url';
  value: string;
  tone?: ChipTone;
  dot?: StoplightDot;
}

export interface ActionListRow {
  badge?: GlanceChip;
  primary: string;
  mono?: boolean;
  subtitle?: string;
  actions: RowAction[];
}

export interface ActionListLayoutData {
  rows: ActionListRow[];
}

export interface FactItem {
  label: string;
  value: string;
  mono?: boolean;
  copyable?: boolean;
}

export interface FactView {
  label: string;
  facts: FactItem[];
}

export interface FactsLayoutData {
  summary?: string;
  facts?: FactItem[];
  views?: FactView[];
}

export interface SegmentTile {
  value: string;
  role: string;
  meaning: string;
  tone?: 'blue' | 'amber' | 'green' | 'neutral';
}

export interface SegmentsLayoutData {
  segments: SegmentTile[];
  explanation: string;
}

export interface StepItem {
  label: string;
  caption?: string;
}

export interface StepsLayoutData {
  steps: StepItem[];
  activeStep?: number;
  summary: string;
  bullets?: string[];
}

export interface ConvertEquivalent {
  label: string;
  value: string;
}

export interface ConvertLayoutData {
  input: string;
  output: string;
  equivalents?: ConvertEquivalent[];
}

export interface TableLayoutData {
  columns: string[];
  rows: string[][];
  monoColumns?: number[];
  sourceUrl?: string;
}

export interface SnippetVar {
  key: string;
  label: string;
  defaultValue?: string;
}

export interface SnippetLayoutData {
  language?: string;
  body: string;
  vars?: SnippetVar[];
}

// --- Modules, Patterns, Diagnostics & Prefs ---

export interface ModuleStatus {
  id: string;
  name: string;
  category: string;
  description: string;
  icon?: string;
  version: string;
  enabled: boolean;
  defaultEnabled: boolean;
  localOnly?: boolean;
  cardCount: number;
  overlayCount: number;
  patternCount: number;
  filePath?: string;
}

export interface StarterModuleInfo {
  id: string;
  name: string;
  category: string;
  description: string;
  icon?: string;
  version: string;
  defaultEnabled: boolean;
  installed: boolean;
  cardCount: number;
  overlayCount: number;
  patternCount: number;
}

export interface CollectionStats {
  collection: string;
  cardCount: number;
  tags?: string[];
}

export interface LogDiagnostics {
  filePath: string;
  level: string;
  sizeBytes: number;
  metricsAddr?: string;
}

export interface PatternLinkRule {
  targetEntityKey?: string;
  targetCollection?: string;
  targetTag?: string;
  relationLabel?: string;
  autoExpand?: boolean;
  nestChildren?: boolean;
  copyChildrenOnly?: boolean;
  attachUnder?: string;
  maxDepth?: number;
  includeFields?: string[];
}

export interface PatternDataLookup {
  alias: string;
  targetEntityKey: string;
  extractFields?: Record<string, string>;
  fallbackValues?: Record<string, string>;
}

export interface PatternMCPLookup {
  serverId: string;
  toolName: string;
  arguments?: Record<string, string>;
}

export interface PatternLiveResultSpec {
  layout: 'facts' | 'table' | 'steps' | 'link';
  title?: string;
  icon?: string;
  defaultExpanded?: boolean;
  chips?: GlanceChip[];
  summary?: string;
  facts?: FactItem[];
  url?: string;
  columns?: string[];
  rowsPath?: string;
  rowFields?: string[];
  rows?: string[][];
  monoColumns?: number[];
  sourceUrl?: string;
  stepsPath?: string;
  stepLabel?: string;
  stepCaption?: string;
  steps?: StepItem[];
  activeStep?: number;
  activeStepPath?: string;
  bullets?: string[];
}

export interface PatternLiveHttpSpec {
  urlTemplate: string;
  trigger?: 'auto' | 'manual';
  timeoutMs?: number;
  cacheTtlSec?: number;
  auth?: string;
  headers?: Record<string, string>;
  resultCard: PatternLiveResultSpec;
}

export interface CLIAuthStatus {
  tool: 'gh' | 'az' | string;
  installed: boolean;
  authenticated: boolean;
  account?: string;
  hostOrTenant?: string;
  detail?: string;
}

export interface ExternalServerConfig {
  id: string;
  name: string;
  transport: string;
  command?: string;
  args?: string[];
  env?: string[];
  url?: string;
  headers?: Record<string, string>;
  enabled: boolean;
  localOnly?: boolean;
}

export interface ConnectedServerStatus {
  config: ExternalServerConfig;
  connected: boolean;
  toolCount: number;
  resourceCount: number;
  cardCount: number;
  lastSynced?: string;
  lastError?: string;
}

export interface PatternRule {
  id: string;
  name: string;
  category?: string;
  description?: string;
  example?: string;
  enabled?: boolean;
  priority?: number;
  normalize?: string[];
  regex: string;
  caseInsensitive?: boolean;
  checksum?: '' | 'iata-mod7' | 'iso6346' | 'luhn';
  lookups?: Record<string, Record<string, string>>;
  dataLookups?: PatternDataLookup[];
  titleTemplate: string;
  subtitleTemplate?: string;
  descriptionTemplate?: string;
  explanationTemplate?: string;
  segments?: SegmentTile[];
  collection?: string;
  layout?: CardLayoutType;
  icon?: string;
  defaultExpanded?: boolean;
  chips?: GlanceChip[];
  fields?: Record<string, string>;
  linkEntityKeys?: string[];
  linkRules?: PatternLinkRule[];
  actions?: CardAction[];
  liveHttp?: PatternLiveHttpSpec;
  liveMcp?: PatternMCPLookup;
  localOnly?: boolean;
}

export interface UserPrefs {
  onboardingCompleted: boolean;
  powerUserMode: boolean;
  launchAtLogin?: boolean;
  layoutMode?: string;
  selectedGoals?: string[];
  enabledModules?: string[];
  pinnedCategories?: string[];
}

export interface AppInfo {
  name: string;
  version: string;
}
