export type CardTemplateType =
  | 'location'
  | 'server-matrix'
  | 'website-link'
  | 'pattern-breakdown'
  | 'conversion'
  | 'branch-gateway'
  | 'env-bookmarks'
  | 'reference-decoder'
  | 'container-blueprint'
  | 'branch-contact'
  | 'dictionary-guide'
  | 'kv-grid'
  | 'shortcut'
  | 'knowledge'
  | 'command'
  | 'metric-card';

export interface GlanceChip {
  label: string;
  tone: 'neutral' | 'blue' | 'amber' | 'green' | 'red' | 'mono';
  dot?: 'green' | 'amber' | 'red';
}

export interface CardAction {
  id: string;
  label: string;
  shortcut?: string;
  type: 'open_url' | 'open_pwa' | 'launch_app' | 'copy' | 'http' | 'ai_prompt' | string;
  target: string;
}

export interface CardNode<T = Record<string, unknown>> {
  id: string;
  template: CardTemplateType;
  icon: string;
  title: string;
  subtitle?: string;
  description?: string;
  collection?: string;
  entityKey?: string;
  moduleId?: string;
  chips: GlanceChip[];
  data: T;
  children?: CardNode[];
  actions?: CardAction[];
  defaultExpanded?: boolean;
  pinnable?: boolean;
}

export interface ServerRow {
  env?: string;
  badge?: string;
  tone?: 'green' | 'amber' | 'red' | 'neutral';
  host?: string;
  value?: string;
  port?: string;
  role?: string;
  subtitle?: string;
  sshCommand?: string;
  url?: string;
  copyValue?: string;
}

export interface ServerMatrixData {
  contract?: string;
  servers?: ServerRow[];
  rows?: ServerRow[];
}

export interface EnvLinkButton {
  label: string;
  url: string;
  tone?: 'green' | 'amber' | 'red' | 'blue' | 'neutral';
  dot?: 'green' | 'amber' | 'red';
}

export interface BookmarkRow {
  name: string;
  subtitle?: string;
  links?: EnvLinkButton[];
  qaLabel?: string;
  prodLabel?: string;
  qaUrl?: string;
  prodUrl?: string;
}

export interface WebsiteLinkData {
  contract?: string;
  url?: string; // Single-button mode (1 link -> whole card is clickable with top-right ↗, no chevron)
  bookmarks?: BookmarkRow[];
}

export type EnvBookmarksData = WebsiteLinkData;

export interface PatternSegment {
  value: string;
  role: string;
  meaning: string;
  tone?: 'blue' | 'amber' | 'green' | 'neutral';
}

export interface PatternBreakdownData {
  patternId?: string;
  matchedInput?: string;
  segments?: PatternSegment[];
  explanation?: string;
  [key: string]: unknown;
}

export interface ConversionEquivalent {
  label: string;
  value: string;
}

export interface ConversionData {
  input?: string;
  output?: string;
  result?: string;
  category?: string;
  equivalents?: ConversionEquivalent[];
  [key: string]: unknown;
}

export interface DimensionTriple {
  length: string;
  width: string;
  height: string;
}

export interface DoorDims {
  width: string;
  height: string;
}

export interface ContainerBlueprintData {
  mode?: 'summary' | 'blueprint' | 'capacity';
  isoCode?: string;
  shortCode?: string;
  aspect?: '20gp' | '40gp' | '40hc';
  category?: string;
  volumeCbm?: string;
  volumeCuFt?: string;
  maxPayload?: string;
  tareWeight?: string;
  maxGross?: string;
  tempRange?: string;
  intMetric?: DimensionTriple;
  doorMetric?: DoorDims;
  intImperial?: DimensionTriple;
  doorImperial?: DoorDims;
  euroPallets?: number;
  euroNote?: string;
  usPallets?: number;
  usNote?: string;
}

export interface ContactRow {
  label: string;
  value: string;
  mono?: boolean;
  url?: string;
  actionLabel?: string;
}

export interface BranchContactData {
  phone?: string;
  email?: string;
  district?: string;
  address?: string;
  hours?: string;
  mapUrl?: string;
  note?: string;
  rows?: ContactRow[];
}

export interface DictionaryStep {
  index: number;
  label: string;
  caption?: string;
}

export interface DictionaryGuideData {
  activeStep?: number;
  timelineLabel?: string;
  steps?: DictionaryStep[];
  summary: string;
  bullets?: string[];
}

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
}

export interface PatternDataLookup {
  alias: string;
  targetEntityKey: string;
  extractFields?: Record<string, string>;
  fallbackValues?: Record<string, string>;
}

export interface PatternRule {
  id: string;
  name: string;
  category?: string;
  description?: string;
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
  segments?: PatternSegment[];
  collection?: string;
  template?: CardTemplateType;
  icon?: string;
  defaultExpanded?: boolean;
  chips?: GlanceChip[];
  fields?: Record<string, string>;
  linkEntityKeys?: string[];
  linkRules?: PatternLinkRule[];
  localOnly?: boolean;
}

export interface UserPrefs {
  onboardingCompleted: boolean;
  powerUserMode: boolean;
  aiEnabled: boolean;
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
