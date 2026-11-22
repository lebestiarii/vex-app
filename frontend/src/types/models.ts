export type CardTemplateType =
  | 'shortcut'
  | 'knowledge'
  | 'command'
  | 'kv-grid'
  | 'metric-card';

export type CardActionType =
  | 'open_url'
  | 'open_pwa'
  | 'launch_app'
  | 'copy'
  | 'http'
  | 'ai_prompt';

export interface CardAction {
  id: string;
  label: string;
  shortcut?: string;
  type: CardActionType | string;
  target: string;
  env?: Record<string, string>;
}

export interface Provenance {
  sourceName?: string;
  sourceUrl?: string;
  verifiedAt?: string;
  authority?: 'official' | 'internal-curated' | 'automated-sync' | string;
  localOnly?: boolean;
}

export interface CardSection {
  moduleId: string;
  moduleName: string;
  title: string;
  badge?: string;
  notes?: string;
  fields?: Record<string, unknown>;
  actions?: CardAction[];
  provenance?: Provenance;
  localOnly?: boolean;
}

export interface SearchCard {
  id: string;
  entityKey?: string;
  moduleId?: string;
  title: string;
  subtitle?: string;
  description?: string;
  collection: string;
  template: CardTemplateType;
  icon?: string;
  tags?: string[];
  modifiers?: string[];
  glancePills?: string[];
  data?: Record<string, unknown>;
  sections?: CardSection[];
  relatedKeys?: string[];
  actions?: CardAction[];
  provenance?: Provenance;
  localOnly?: boolean;
  updatedAt?: string;
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
  builtIn: boolean;
  localOnly?: boolean;
  cardCount: number;
  overlayCount: number;
}

export interface ETLMappingRule {
  name: string;
  sourceFormat: 'csv' | 'tsv' | 'json' | 'jsonl' | string;
  rootPath?: string;
  targetCollection: string;
  defaultTemplate: CardTemplateType;
  titleField: string;
  subtitleField?: string;
  descriptionField?: string;
  tagsField?: string;
  staticTags?: string[];
  modifiers?: string[];
  iconField?: string;
  defaultIcon?: string;
  actionType?: string;
  actionLabel?: string;
  actionTarget?: string;
  dataFields?: string[];
}

export interface RawDataSample {
  format: string;
  fields: string[];
  sampleRows: Record<string, unknown>[];
  totalRows: number;
  detectedRootPath?: string;
}

export type QueryMode = 'search' | 'ai';

export interface ParsedQuery {
  raw: string;
  mode: QueryMode;
  modifier?: string;
  collection?: string;
  tags?: string[];
  template?: CardTemplateType;
  params?: Record<string, string>;
  args?: string[];
  text: string;
}

export interface SearchResult {
  card: SearchCard;
  score: number;
  matchSource: string;
}

export interface SearchResponse {
  parsed: ParsedQuery;
  results: SearchResult[];
  total: number;
}

export interface ActionResult {
  success: boolean;
  actionType: string;
  resolvedTarget: string;
  output?: string;
  error?: string;
  data?: Record<string, unknown>;
  durationMs: number;
  openAiMode?: boolean;
}

export type AIProviderType = 'ollama' | 'openai' | 'anthropic' | 'gemini';

export interface AIConfig {
  provider: AIProviderType;
  baseUrl: string;
  apiKey?: string;
  chatModel: string;
  embeddingModel?: string;
  temperature?: number;
  maxContextCards?: number;
}

export interface RAGRequest {
  query: string;
  collection?: string;
  tags?: string[];
  limit?: number;
}

export interface RAGSource {
  index: number;
  cardId: string;
  title: string;
  subtitle?: string;
  collection: string;
  template: CardTemplateType;
  score: number;
  matchSource: string;
}

export interface RAGResponse {
  answer: string;
  sources: RAGSource[];
  model: string;
  provider: string;
  durationMs: number;
}

export interface ETLMappingPreview {
  rule: ETLMappingRule;
  sample: RawDataSample;
  previewCards: SearchCard[];
  totalCards: number;
}

export interface CollectionStats {
  collection: string;
  cardCount: number;
  tags: string[];
}

export interface AppInfo {
  name: string;
  edition: string;
  version: string;
}

export interface UpdateCheckResult {
  currentVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
  releasedAt?: string;
  notes?: string;
  platformKey: string;
  downloadUrl?: string;
  downloadSha256?: string;
  source: string;
}

export interface LogEntry {
  time: string;
  level: string;
  msg: string;
  service?: string;
  component?: string;
  attrs?: Record<string, unknown>;
}

export interface LogDiagnostics {
  filePath: string;
  level: string;
  sizeBytes: number;
  metricsAddr?: string;
}

export interface WeatherInfo {
  tempC: number;
  tempF: number;
  condition: string;
  cachedAt: string;
}

export type LayoutMode = 'compact' | 'split';
export type AnchorDirection = 'down' | 'up';

export interface UserPrefs {
  onboardingCompleted: boolean;
  powerUserMode: boolean;
  aiEnabled: boolean;
  launchAtLogin?: boolean;
  layoutMode?: LayoutMode;
  selectedGoals?: string[];
  enabledModules?: string[];
  pinnedCategories?: string[];
}

export interface ConnectedServerStatus {
  config: {
    id: string;
    name: string;
    transport: string;
    command?: string;
    url?: string;
    enabled: boolean;
  };
  connected: boolean;
  toolCount: number;
  resourceCount: number;
  cardCount: number;
  lastSynced?: string;
  lastError?: string;
}
