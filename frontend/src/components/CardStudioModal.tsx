import { useState, type ChangeEvent } from 'react';
import {
  Activity,
  BookOpen,
  ChevronDown,
  ChevronRight,
  FileSpreadsheet,
  Globe,
  Loader2,
  Plus,
  Sparkles,
  Terminal,
  Upload,
  X,
} from 'lucide-react';
import { bridge } from '../lib/bridge';
import type {
  CardActionType,
  CardTemplateType,
  ETLMappingPreview,
  ETLMappingRule,
  RawDataSample,
  SearchCard,
} from '../types/models';

interface CardStudioModalProps {
  open: boolean;
  powerUserMode?: boolean;
  aiEnabled?: boolean;
  onClose: () => void;
  onSaved: () => void;
}

type IntentPreset = 'link' | 'note' | 'command' | 'info';

const TEMPLATES: CardTemplateType[] = ['shortcut', 'knowledge', 'command', 'kv-grid', 'metric-card'];
const ACTION_TYPES: CardActionType[] = ['open_url', 'open_pwa', 'launch_app', 'copy', 'http', 'ai_prompt'];

export function CardStudioModal({
  open,
  powerUserMode = false,
  aiEnabled = false,
  onClose,
  onSaved,
}: CardStudioModalProps) {
  const [tab, setTab] = useState<'builder' | 'import'>('builder');
  const [intent, setIntent] = useState<IntentPreset>('link');
  const [showAdvanced, setShowAdvanced] = useState(false);

  // --- Tab 1: Add Item state ---
  const [title, setTitle] = useState('');
  const [subtitle, setSubtitle] = useState('');
  const [collection, setCollection] = useState('shortcuts');
  const [template, setTemplate] = useState<CardTemplateType>('shortcut');
  const [description, setDescription] = useState('');
  const [tagsText, setTagsText] = useState('');
  const [modifiersText, setModifiersText] = useState('');
  const [actionType, setActionType] = useState<CardActionType>('open_url');
  const [actionLabel, setActionLabel] = useState('Open');
  const [actionTarget, setActionTarget] = useState('');
  const [dataJson, setDataJson] = useState('{}');
  const [saving, setSaving] = useState(false);
  const [builderMsg, setBuilderMsg] = useState<string | null>(null);

  // --- Tab 2: Spreadsheet / File Import state ---
  const [rawContent, setRawContent] = useState('');
  const [filenameHint, setFilenameHint] = useState('import.csv');
  const [userHint, setUserHint] = useState('');
  const [sample, setSample] = useState<RawDataSample | null>(null);
  const [preview, setPreview] = useState<ETLMappingPreview | null>(null);
  const [rule, setRule] = useState<ETLMappingRule | null>(null);
  const [wizardBusy, setWizardBusy] = useState(false);
  const [wizardMsg, setWizardMsg] = useState<string | null>(null);

  if (!open) return null;

  const selectIntent = (next: IntentPreset) => {
    setIntent(next);
    if (next === 'link') {
      setTemplate('shortcut');
      setCollection('shortcuts');
      setActionType('open_url');
      setActionLabel('Open Website');
    } else if (next === 'note') {
      setTemplate('knowledge');
      setCollection('notes');
      setActionType('copy');
      setActionLabel('Copy Note');
    } else if (next === 'command') {
      setTemplate('command');
      setCollection('tools');
      setActionType('copy');
      setActionLabel('Copy Command');
    } else if (next === 'info') {
      setTemplate('kv-grid');
      setCollection('reference');
      setActionType('copy');
      setActionLabel('Copy Value');
    }
  };

  const resetAndClose = () => {
    setTitle('');
    setSubtitle('');
    setDescription('');
    setActionTarget('');
    setTagsText('');
    setModifiersText('');
    setDataJson('{}');
    setBuilderMsg(null);
    onClose();
  };

  const handleSaveCard = async () => {
    if (!title.trim()) {
      setBuilderMsg('Please enter a name for this item.');
      return;
    }
    let parsedData: Record<string, unknown> = {};
    if (dataJson.trim()) {
      try {
        parsedData = JSON.parse(dataJson);
      } catch {
        setBuilderMsg('Structured Data JSON is not valid JSON.');
        return;
      }
    }

    // For a simple Website/App card, also store URL in data.url for clean display
    const effectiveTarget =
      intent === 'note' && !actionTarget.trim() && description.trim()
        ? description.trim()
        : actionTarget.trim();

    if (intent === 'link' && effectiveTarget && !parsedData.url) {
      parsedData = { ...parsedData, url: effectiveTarget };
    }

    setSaving(true);
    setBuilderMsg(null);
    try {
      const card: SearchCard = {
        id: '',
        title: title.trim(),
        subtitle: subtitle.trim() || undefined,
        description: description.trim() || undefined,
        collection: collection.trim() || 'default',
        template,
        tags: tagsText
          .split(',')
          .map((t) => t.trim())
          .filter(Boolean),
        modifiers: modifiersText
          .split(',')
          .map((m) => m.trim())
          .filter(Boolean),
        data: Object.keys(parsedData).length > 0 ? parsedData : undefined,
        actions: effectiveTarget
          ? [
              {
                id: 'primary',
                label: actionLabel.trim() || 'Open',
                shortcut: 'Enter',
                type: actionType,
                target: effectiveTarget,
              },
            ]
          : undefined,
      };
      await bridge.upsertCard(card);
      onSaved();
      resetAndClose();
    } catch (e) {
      setBuilderMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const handleFileUpload = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setFilenameHint(file.name);
    const reader = new FileReader();
    reader.onload = () => {
      const text = typeof reader.result === 'string' ? reader.result : '';
      setRawContent(text);
      setSample(null);
      setPreview(null);
      setRule(null);
    };
    reader.readAsText(file);
  };

  const handlePreviewSample = async () => {
    if (!rawContent.trim()) return;
    setWizardBusy(true);
    setWizardMsg(null);
    try {
      const s = await bridge.sampleRawData(rawContent, filenameHint);
      setSample(s);
      const urlField = s.fields.find((f) => /url|link|href|website/i.test(f));
      setRule({
        name: `Import ${filenameHint}`,
        sourceFormat: s.format,
        rootPath: s.detectedRootPath,
        targetCollection: 'imported',
        defaultTemplate: urlField ? 'shortcut' : 'kv-grid',
        titleField: s.fields[0] ? `{{${s.fields[0]}}}` : '',
        subtitleField: s.fields[1] ? `{{${s.fields[1]}}}` : '',
        actionType: urlField ? 'open_url' : '',
        actionLabel: urlField ? 'Open Link' : '',
        actionTarget: urlField ? `{{${urlField}}}` : '',
      });
    } catch (e) {
      setWizardMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setWizardBusy(false);
    }
  };

  const handleAutoMapAI = async () => {
    if (!rawContent.trim()) return;
    setWizardBusy(true);
    setWizardMsg(null);
    try {
      const p = await bridge.generateETLMappingWithPreview(rawContent, filenameHint, userHint);
      setSample(p.sample);
      setPreview(p);
      setRule(p.rule);
      setWizardMsg(`Ready to import ${p.totalCards} items into "${p.rule.targetCollection}".`);
    } catch (e) {
      setWizardMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setWizardBusy(false);
    }
  };

  const handleCommitImport = async () => {
    if (!rawContent.trim() || !rule) return;
    setWizardBusy(true);
    setWizardMsg(null);
    try {
      const res = await bridge.importWithRule(rawContent, rule);
      setWizardMsg(`Imported ${res.importedCount} items!`);
      onSaved();
      onClose();
    } catch (e) {
      setWizardMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setWizardBusy(false);
    }
  };

  const intentChoices: { id: IntentPreset; label: string; desc: string; icon: typeof Globe; powerOnly?: boolean }[] = [
    { id: 'link', label: 'Website or App', desc: 'Save a link to open in one click', icon: Globe },
    { id: 'note', label: 'Note or Info', desc: 'Save text, snippets, or reminders', icon: BookOpen },
    { id: 'command', label: 'Command', desc: 'Save a copyable CLI snippet', icon: Terminal, powerOnly: true },
    { id: 'info', label: 'Info / Metric', desc: 'Structured key-value or status card', icon: Activity, powerOnly: true },
  ];

  const visibleIntents = powerUserMode
    ? intentChoices
    : intentChoices.filter((c) => !c.powerOnly);

  return (
    <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
      <div className="w-full max-w-2xl max-h-[88vh] bg-zinc-950 border border-zinc-800 rounded-2xl shadow-2xl flex flex-col overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-zinc-800">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setTab('builder')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                tab === 'builder' ? 'bg-indigo-600 text-white' : 'text-zinc-400 hover:bg-zinc-800'
              }`}
            >
              <Plus className="w-3.5 h-3.5" />
              <span>Add Item</span>
            </button>
            <button
              type="button"
              onClick={() => setTab('import')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                tab === 'import' ? 'bg-indigo-600 text-white' : 'text-zinc-400 hover:bg-zinc-800'
              }`}
            >
              <FileSpreadsheet className="w-3.5 h-3.5" />
              <span>Import Spreadsheet or File</span>
            </button>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="p-1 text-zinc-400 hover:text-zinc-100 rounded hover:bg-zinc-800"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-4 space-y-3.5 text-xs">
          {tab === 'builder' ? (
            <>
              {/* Visual Intent Picker */}
              <div className={`grid gap-2 ${visibleIntents.length > 2 ? 'grid-cols-4' : 'grid-cols-2'}`}>
                {visibleIntents.map((item) => {
                  const Icon = item.icon;
                  const active = intent === item.id;
                  return (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => selectIntent(item.id)}
                      className={`text-left p-3 rounded-xl border transition flex items-start gap-2.5 ${
                        active
                          ? 'bg-indigo-600/15 border-indigo-500/60 text-zinc-100'
                          : 'bg-zinc-900/50 border-zinc-800 text-zinc-300 hover:border-zinc-700'
                      }`}
                    >
                      <Icon className={`w-4 h-4 shrink-0 mt-0.5 ${active ? 'text-indigo-300' : 'text-zinc-400'}`} />
                      <div className="min-w-0">
                        <div className="font-medium truncate">{item.label}</div>
                        <div className="text-[10px] text-zinc-400 mt-0.5 leading-snug">{item.desc}</div>
                      </div>
                    </button>
                  );
                })}
              </div>

              {/* Essential Fields */}
              <label className="block space-y-1">
                <span className="text-zinc-300 font-medium">Name *</span>
                <input
                  autoFocus
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder={
                    intent === 'link'
                      ? 'e.g. Company Payroll, Gmail, or Project Board'
                      : intent === 'note'
                        ? 'e.g. Office Wi-Fi Password or Meeting Checklist'
                        : 'e.g. Restart Local Server'
                  }
                  className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-3 py-2 text-zinc-100 focus:outline-none focus:border-indigo-500"
                />
              </label>

              {intent === 'link' && (
                <label className="block space-y-1">
                  <span className="text-zinc-300 font-medium">Website Address (URL) *</span>
                  <input
                    value={actionTarget}
                    onChange={(e) => setActionTarget(e.target.value)}
                    placeholder="https://..."
                    className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-3 py-2 text-zinc-100 font-mono focus:outline-none focus:border-indigo-500"
                  />
                </label>
              )}

              {intent === 'command' && (
                <label className="block space-y-1">
                  <span className="text-zinc-300 font-medium">Shell Command *</span>
                  <input
                    value={actionTarget}
                    onChange={(e) => setActionTarget(e.target.value)}
                    placeholder="e.g. git status"
                    className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-3 py-2 text-zinc-100 font-mono focus:outline-none focus:border-indigo-500"
                  />
                </label>
              )}

              <label className="block space-y-1">
                <span className="text-zinc-300 font-medium">
                  {intent === 'note' ? 'Note Content' : 'Short Note (optional)'}
                </span>
                <textarea
                  rows={intent === 'note' ? 4 : 2}
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder={
                    intent === 'note'
                      ? 'Write anything you want to find quickly later...'
                      : 'Add a quick reminder or keywords to help you find it...'
                  }
                  className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-3 py-2 text-zinc-100 focus:outline-none focus:border-indigo-500"
                />
              </label>

              {/* Collapsible Advanced Options */}
              <div className="border border-zinc-800/80 rounded-xl overflow-hidden">
                <button
                  type="button"
                  onClick={() => setShowAdvanced((v) => !v)}
                  className="w-full flex items-center justify-between px-3 py-2 bg-zinc-900/50 text-zinc-400 hover:text-zinc-200 transition"
                >
                  <span className="font-medium">Advanced options (folder, tags, shortcuts, JSON)</span>
                  {showAdvanced ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
                </button>

                {showAdvanced && (
                  <div className="p-3 space-y-3 bg-zinc-950/60 border-t border-zinc-800/80">
                    <div className="grid grid-cols-2 gap-3">
                      <label className="space-y-1">
                        <span className="text-zinc-400">Collection / Folder</span>
                        <input
                          value={collection}
                          onChange={(e) => setCollection(e.target.value)}
                          placeholder="shortcuts"
                          className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100"
                        />
                      </label>
                      <label className="space-y-1">
                        <span className="text-zinc-400">Card Template</span>
                        <select
                          value={template}
                          onChange={(e) => setTemplate(e.target.value as CardTemplateType)}
                          className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100"
                        >
                          {TEMPLATES.map((t) => (
                            <option key={t} value={t}>
                              {t}
                            </option>
                          ))}
                        </select>
                      </label>
                    </div>

                    <div className="grid grid-cols-2 gap-3">
                      <label className="space-y-1">
                        <span className="text-zinc-400">Subtitle</span>
                        <input
                          value={subtitle}
                          onChange={(e) => setSubtitle(e.target.value)}
                          placeholder="Optional subtitle"
                          className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100"
                        />
                      </label>
                      <label className="space-y-1">
                        <span className="text-zinc-400">Tags (comma-separated)</span>
                        <input
                          value={tagsText}
                          onChange={(e) => setTagsText(e.target.value)}
                          placeholder="work, finance"
                          className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100"
                        />
                      </label>
                    </div>

                    <div className="grid grid-cols-3 gap-2">
                      <label className="space-y-1">
                        <span className="text-zinc-400">Action Type</span>
                        <select
                          value={actionType}
                          onChange={(e) => setActionType(e.target.value as CardActionType)}
                          className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2 py-1.5 text-zinc-100"
                        >
                          {ACTION_TYPES.map((t) => (
                            <option key={t} value={t}>
                              {t}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label className="space-y-1">
                        <span className="text-zinc-400">Button Label</span>
                        <input
                          value={actionLabel}
                          onChange={(e) => setActionLabel(e.target.value)}
                          className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2 py-1.5 text-zinc-100"
                        />
                      </label>
                      <label className="space-y-1">
                        <span className="text-zinc-400">Quick Aliases (/mod)</span>
                        <input
                          value={modifiersText}
                          onChange={(e) => setModifiersText(e.target.value)}
                          placeholder="e.g. mail, hr"
                          className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2 py-1.5 text-zinc-100"
                        />
                      </label>
                    </div>

                    {intent !== 'link' && intent !== 'command' && (
                      <label className="block space-y-1">
                        <span className="text-zinc-400">Action Target (URL, command, or text to copy)</span>
                        <input
                          value={actionTarget}
                          onChange={(e) => setActionTarget(e.target.value)}
                          placeholder="Optional target..."
                          className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100 font-mono"
                        />
                      </label>
                    )}

                    <label className="block space-y-1">
                      <span className="text-zinc-400">Structured Data JSON (optional)</span>
                      <textarea
                        rows={2}
                        value={dataJson}
                        onChange={(e) => setDataJson(e.target.value)}
                        className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100 font-mono"
                      />
                    </label>
                  </div>
                )}
              </div>

              {builderMsg && <div className="text-red-400">{builderMsg}</div>}

              <div className="flex justify-end gap-2 pt-1">
                <button
                  type="button"
                  onClick={onClose}
                  className="px-3.5 py-2 rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  disabled={saving}
                  onClick={handleSaveCard}
                  className="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium disabled:opacity-50"
                >
                  {saving ? 'Saving...' : 'Save Item'}
                </button>
              </div>
            </>
          ) : (
            <>
              <p className="text-zinc-400">
                Bring in a list of links, contacts, or notes from a spreadsheet (<code className="text-zinc-200">.csv</code>) or <code className="text-zinc-200">.json</code> file.
              </p>

              <div className="flex items-center justify-between gap-2">
                <label className="flex items-center gap-2 px-3 py-2 rounded-lg bg-zinc-900 border border-zinc-700 hover:border-zinc-500 cursor-pointer text-zinc-200">
                  <Upload className="w-3.5 h-3.5" />
                  <span>Choose .CSV or .JSON File</span>
                  <input type="file" accept=".csv,.tsv,.json,.jsonl,.txt" onChange={handleFileUpload} className="hidden" />
                </label>
                <span className="text-zinc-500 font-mono">{filenameHint}</span>
              </div>

              <textarea
                rows={4}
                value={rawContent}
                onChange={(e) => setRawContent(e.target.value)}
                placeholder="Or paste rows from Excel, Google Sheets (CSV), or JSON here..."
                className="w-full bg-zinc-900 border border-zinc-800 rounded-lg p-2.5 text-zinc-100 font-mono text-[11px]"
              />

              <div className="flex items-center gap-2">
                {(aiEnabled || powerUserMode) && (
                  <input
                    value={userHint}
                    onChange={(e) => setUserHint(e.target.value)}
                    placeholder="Optional hint for AI (e.g. 'Turn these into clickable bookmarks')"
                    className="flex-1 bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100"
                  />
                )}
                <button
                  type="button"
                  disabled={wizardBusy || !rawContent.trim()}
                  onClick={handlePreviewSample}
                  className="px-3 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-zinc-200 disabled:opacity-50"
                >
                  Preview Columns
                </button>
                {(aiEnabled || powerUserMode) && (
                  <button
                    type="button"
                    disabled={wizardBusy || !rawContent.trim()}
                    onClick={handleAutoMapAI}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-purple-600 hover:bg-purple-500 text-white font-medium disabled:opacity-50"
                  >
                    {wizardBusy ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Sparkles className="w-3.5 h-3.5" />}
                    <span>Auto-Organize with AI</span>
                  </button>
                )}
              </div>

              {wizardMsg && (
                <div className="text-indigo-200 bg-indigo-500/10 border border-indigo-500/25 rounded-lg px-3 py-2">
                  {wizardMsg}
                </div>
              )}

              {sample && (
                <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-2.5">
                  <div className="text-zinc-400">
                    Found <strong className="text-zinc-200">{sample.totalRows}</strong> rows ({sample.format.toUpperCase()}) • Columns:{' '}
                    <span className="font-mono text-indigo-300">{sample.fields.join(', ')}</span>
                  </div>
                </div>
              )}

              {rule && (
                <div className="grid grid-cols-2 gap-2 rounded-lg border border-zinc-800 bg-zinc-900/40 p-3">
                  <label className="space-y-1">
                    <span className="text-zinc-400">Folder / Collection Name</span>
                    <input
                      value={rule.targetCollection}
                      onChange={(e) => setRule({ ...rule, targetCollection: e.target.value })}
                      className="w-full bg-zinc-950 border border-zinc-800 rounded px-2 py-1 text-zinc-100"
                    />
                  </label>
                  <label className="space-y-1">
                    <span className="text-zinc-400">Card Style</span>
                    <select
                      value={rule.defaultTemplate}
                      onChange={(e) => setRule({ ...rule, defaultTemplate: e.target.value as CardTemplateType })}
                      className="w-full bg-zinc-950 border border-zinc-800 rounded px-2 py-1 text-zinc-100"
                    >
                      {TEMPLATES.map((t) => (
                        <option key={t} value={t}>
                          {t}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="space-y-1">
                    <span className="text-zinc-400">Title Column</span>
                    <input
                      value={rule.titleField}
                      onChange={(e) => setRule({ ...rule, titleField: e.target.value })}
                      className="w-full bg-zinc-950 border border-zinc-800 rounded px-2 py-1 text-zinc-100 font-mono"
                    />
                  </label>
                  <label className="space-y-1">
                    <span className="text-zinc-400">Link / Action Column</span>
                    <input
                      value={rule.actionTarget ?? ''}
                      onChange={(e) => setRule({ ...rule, actionTarget: e.target.value })}
                      className="w-full bg-zinc-950 border border-zinc-800 rounded px-2 py-1 text-zinc-100 font-mono"
                    />
                  </label>
                </div>
              )}

              {preview && preview.previewCards.length > 0 && (
                <div className="space-y-1.5">
                  <div className="text-[10px] uppercase tracking-wider text-zinc-500">
                    Preview ({preview.previewCards.length} of {preview.totalCards} items)
                  </div>
                  <div className="space-y-1 max-h-32 overflow-y-auto">
                    {preview.previewCards.map((c, idx) => (
                      <div key={idx} className="flex items-center justify-between px-2.5 py-1.5 rounded bg-zinc-900 border border-zinc-800">
                        <span className="font-medium text-zinc-100 truncate">{c.title}</span>
                        <span className="text-[10px] text-zinc-400 font-mono">{c.subtitle || c.template}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <div className="flex justify-end gap-2 pt-1">
                <button
                  type="button"
                  onClick={onClose}
                  className="px-3.5 py-2 rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  disabled={wizardBusy || !rule}
                  onClick={handleCommitImport}
                  className="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium disabled:opacity-50"
                >
                  Import Items
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
