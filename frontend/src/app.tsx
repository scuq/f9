import { useEffect, useRef, useState } from "preact/hooks";
import { TerminalView } from "./terminal";
import { setTheme as applyThemeColors, setSettings as applyOverrides } from "./theme";
import { onFindRequested, onPickerRequested, setPickerEnabled } from "./termsearch";

const api = () => window.go.app.App;
const MS_CONFIRM_THRESHOLD = 10;

const BAR_TEMPLATE = `rows:
  - buttons:
      - label: save config
        color: "#2ea043"
        action:
          kind: send
          text: "{{ save_cmd }}"
      - label: write-mem
        os: nxos
        action:
          kind: send
          text: copy run start
      - label: docs
        action:
          kind: url
          text: https://example.com
`;

const OPTION_KEYS: { key: string; label: string; hint: string }[] = [
  { key: "termType", label: "term type", hint: "xterm-256color" },
  { key: "keepaliveInterval", label: "keepalive", hint: "30s" },
  { key: "reconnect", label: "reconnect", hint: "off | prompt | auto" },
  { key: "theme", label: "theme", hint: "oled-black" },
  { key: "scrollbackLines", label: "scrollback lines", hint: "5000000" },
  { key: "auditScope", label: "audit scope", hint: "off | events | events+input | full-io" },
  { key: "keyFile", label: "SSH key file", hint: "~/.ssh/id_ed25519 (overrides global)" },
  { key: "useAgent", label: "use SSH agent", hint: "true | false (empty = inherit)" },
  { key: "socksPort", label: "SOCKS port", hint: "local ssh -D dynamic-forward port, e.g. 22013 (empty = off)" },
  { key: "socksOnly", label: "SOCKS only", hint: "true | false — connect for SOCKS with no terminal" },
];

const UI_FONTS = ["Inter", "system-ui", "Segoe UI", "Roboto", "Helvetica Neue", "Arial", "Ubuntu", "Cantarell", "Noto Sans"];
const MONO_FONTS = ["JetBrains Mono", "Fira Code", "Cascadia Code", "Cascadia Mono", "Source Code Pro", "IBM Plex Mono", "Hack", "Menlo", "Monaco", "Consolas", "Ubuntu Mono", "DejaVu Sans Mono", "Liberation Mono"];

function uuid(): string {
  const c = (globalThis as any).crypto;
  return c?.randomUUID ? c.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

type Tab = { termId: string; sessionId: string; name: string };
type View = { kind: "empty" | "details" | "term"; id: string };
type IndKind = "output" | "prompt" | "match";
type IndFlags = { output?: boolean; prompt?: boolean; match?: boolean };
type TabCfg = { output: boolean; prompt: boolean; match: boolean; watch: string };
type DefInd = { output: boolean; prompt: boolean; match: boolean };

const DEFAULT_CFG = (d: DefInd): TabCfg => ({ output: d.output, prompt: d.prompt, match: d.match, watch: "" });

function dotClass(fl?: IndFlags): string | null {
  if (!fl) return null;
  if (fl.match) return "match";
  if (fl.prompt) return "prompt";
  if (fl.output) return "output";
  return null;
}

function SessionRow(props: {
  s: SessionNode; pathPrefix?: string; indent: number; selected: boolean; marked: boolean;
  onSelect: () => void; onToggleMark: () => void;
}) {
  const { s, pathPrefix, indent, selected, marked, onSelect, onToggleMark } = props;
  return (
    <div class={"row session" + (selected ? " selected" : "")} style={{ paddingLeft: `${indent}px` }} onClick={onSelect}>
      <input type="checkbox" checked={marked} onClick={(e) => { e.stopPropagation(); onToggleMark(); }} />
      {pathPrefix && <span class="hitpath">{pathPrefix}</span>}
      <span class="sname">{s.name}</span>
      {s.pinned && <span class="pinbadge">{"\u2605"}</span>}
      {s.detectedOs && <span class="ostag">{s.detectedOs}</span>}
    </div>
  );
}

function Folder(props: {
  node: FolderNode; depth: number; selected: string; selectedFolder: string; marked: Record<string, true>;
  onSelect: (s: SessionNode) => void; onSelectFolder: (f: FolderNode) => void; onToggleMark: (id: string) => void;
  onFolderCtx: (node: FolderNode, x: number, y: number) => void;
}) {
  const { node, depth, selected, selectedFolder, marked, onSelect, onSelectFolder, onToggleMark, onFolderCtx } = props;
  const [open, setOpen] = useState(depth < 2);
  return (
    <div>
      <div class={"row folder" + (node.id === selectedFolder ? " selected" : "")} style={{ paddingLeft: `${depth * 14}px` }}
        onClick={() => onSelectFolder(node)}
        onContextMenu={(e) => { e.preventDefault(); e.stopPropagation(); onFolderCtx(node, e.clientX, e.clientY); }}>
        <span class="twist" onClick={(e) => { e.stopPropagation(); setOpen(!open); }}>{open ? "\u25be" : "\u25b8"}</span>
        <span class="fname">{node.name}</span>
        {node.hasSource && <span class="genmark" title="import source configured on this folder">src</span>}
      </div>
      {open && (node.sessions ?? []).map((s) => (
        <SessionRow key={s.id} s={s} indent={(depth + 1) * 14 + 8} selected={s.id === selected} marked={!!marked[s.id]}
          onSelect={() => onSelect(s)} onToggleMark={() => onToggleMark(s.id)} />
      ))}
      {open && (node.folders ?? []).map((c) => (
        <Folder key={c.id} node={c} depth={depth + 1} selected={selected} selectedFolder={selectedFolder}
          marked={marked} onSelect={onSelect} onSelectFolder={onSelectFolder} onToggleMark={onToggleMark} onFolderCtx={onFolderCtx} />
      ))}
    </div>
  );
}

function PromptModal(props: { req: PromptRequest; onResolve: (r: PromptReply) => void }) {
  const { req, onResolve } = props;
  const [value, setValue] = useState("");
  const [useForAll, setUseForAll] = useState(false);
  const reply = (patch: Partial<PromptReply>): PromptReply => ({ value: "", useForAll: false, accept: false, cancel: false, ...patch });
  return (
    <div class="modal-overlay">
      <div class="modal">
        <h2>{req.prompt}</h2>
        {req.kind === "hostkey" ? (
          <>
            <div class="fpline">fingerprint<br /><code>{req.fingerprint}</code></div>
            <div class="modal-actions">
              <button onClick={() => onResolve(reply({ cancel: true }))}>cancel</button>
              <button class="danger" onClick={() => onResolve(reply({ accept: false }))}>reject</button>
              <button class="primary" onClick={() => onResolve(reply({ accept: true }))}>accept &amp; save</button>
            </div>
          </>
        ) : (
          <>
            <div class="formrow">
              <input type={req.echo ? "text" : "password"} autoFocus value={value}
                onInput={(e) => setValue((e.target as HTMLInputElement).value)}
                onKeyDown={(e) => { if (e.key === "Enter") onResolve(reply({ value, useForAll, accept: true })); }} />
            </div>
            {req.kind === "password" && (
              <label class="checkrow">
                <input type="checkbox" checked={useForAll} onChange={(e) => setUseForAll((e.target as HTMLInputElement).checked)} />
                use this password for all sessions in this batch
              </label>
            )}
            <div class="modal-actions">
              <button onClick={() => onResolve(reply({ cancel: true }))}>cancel</button>
              <button class="primary" onClick={() => onResolve(reply({ value, useForAll, accept: true }))}>ok</button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function SessionModal(props: {
  folder: { id: string; path: string }; detail: SessionDetail | null; onClose: () => void; onSaved: (id: string) => void;
}) {
  const { folder, detail, onClose, onSaved } = props;
  const [name, setName] = useState(detail?.name ?? "");
  const [host, setHost] = useState(detail?.host ?? "");
  const [port, setPort] = useState(detail?.port ? String(detail.port) : "");
  const [user, setUser] = useState(detail?.user ?? "");
  const [opts, setOpts] = useState<Record<string, string>>(() => {
    const o: Record<string, string> = {};
    for (const { key } of OPTION_KEYS) o[key] = detail?.options[key]?.value ?? "";
    return o;
  });
  const [err, setErr] = useState("");
  const save = () => api().SaveSession({
    id: detail?.id ?? "", folderId: detail?.folderId ?? folder.id, name, host,
    port: port.trim() === "" ? 0 : parseInt(port, 10) || 0, user, proto: detail?.proto ?? "ssh", options: opts,
  }).then(onSaved).catch((e) => setErr(String(e)));
  const placeholderFor = (key: string) => {
    const f = detail?.options[key];
    if (f && f.effective !== "") return `${f.effective}  (${f.source})`;
    return OPTION_KEYS.find((o) => o.key === key)?.hint ?? "";
  };
  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="modal" onClick={(e) => e.stopPropagation()}>
        <h2>{detail ? `edit ${detail.name}` : `new session in ${folder.path}`}</h2>
        {err && <div class="error">{err}</div>}
        <div class="formrow"><label>name</label><input value={name} onInput={(e) => setName((e.target as HTMLInputElement).value)} /></div>
        <div class="formrow"><label>host</label><input value={host} onInput={(e) => setHost((e.target as HTMLInputElement).value)} /></div>
        <div class="formrow"><label>port</label><input value={port} placeholder="22" onInput={(e) => setPort((e.target as HTMLInputElement).value)} /></div>
        <div class="formrow"><label>user</label><input value={user} onInput={(e) => setUser((e.target as HTMLInputElement).value)} /></div>
        <div class="opthead">options — empty = inherit</div>
        {OPTION_KEYS.map(({ key, label }) => (
          <div class="formrow" key={key}>
            <label>{label}</label>
            <input value={opts[key]} placeholder={placeholderFor(key)}
              onInput={(e) => setOpts({ ...opts, [key]: (e.target as HTMLInputElement).value })} />
          </div>
        ))}
        {detail && detail.jumpChain && detail.jumpChain.length > 0 && (
          <div class="jumpinfo">jump chain ({detail.jumpSource}):{" "}
            {detail.jumpChain.map((j) => `${j.user ? j.user + "@" : ""}${j.host} [${j.mode}]`).join(" \u2192 ")}
            <div class="hintsmall">jump-chain editor arrives with the tree drag/drop work</div>
          </div>
        )}
        <div class="modal-actions">
          <button onClick={onClose}>cancel</button>
          <button class="primary" onClick={save}>save</button>
        </div>
      </div>
    </div>
  );
}

function FolderModal(props: { parent: { id: string; path: string }; onClose: () => void; onSaved: () => void }) {
  const { parent, onClose, onSaved } = props;
  const [name, setName] = useState("");
  const [err, setErr] = useState("");
  const save = () => api().SaveFolder({ id: "", parentId: parent.id, name }).then(() => onSaved()).catch((e) => setErr(String(e)));
  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="modal" onClick={(e) => e.stopPropagation()}>
        <h2>new folder in {parent.path}</h2>
        {err && <div class="error">{err}</div>}
        <div class="formrow"><label>name</label><input value={name} onInput={(e) => setName((e.target as HTMLInputElement).value)} /></div>
        <div class="modal-actions">
          <button onClick={onClose}>cancel</button>
          <button class="primary" onClick={save}>save</button>
        </div>
      </div>
    </div>
  );
}

function SettingsModal(props: {
  settings: UISettings; themeList: string[]; defInd: DefInd;
  onChangeTheme: (n: string) => void; onImport: () => void; onSave: (patch: Partial<UISettings>) => void;
  onDefInd: (d: DefInd) => void; onClose: () => void;
}) {
  const { settings, themeList, defInd, onChangeTheme, onImport, onSave, onDefInd, onClose } = props;
  const numOrZero = (v: string) => parseInt(v, 10) || 0;
  const [agent, setAgent] = useState<AgentStatus | null>(null);
  const refreshAgent = () => api().SSHAgentStatus().then(setAgent).catch(() => setAgent(null));
  useEffect(() => { refreshAgent(); }, []);
  const keyFiles = settings.keyFiles ?? [];
  const setKeyFiles = (next: string[]) => onSave({ keyFiles: next });
  const agentSockets = settings.agentSockets ?? [];
  const setAgentSockets = (next: string[]) => onSave({ agentSockets: next });
  const [altDraft, setAltDraft] = useState<AltUser[]>(settings.altUsers ?? []);
  const commitAlt = (next: AltUser[]) => { setAltDraft(next); onSave({ altUsers: next }); };
  const [mapScripts, setMapScripts] = useState<MapScript[]>([]);
  const [mapSel, setMapSel] = useState("");
  const [mapName, setMapName] = useState("");
  const [mapCode, setMapCode] = useState("");
  const [mapErr, setMapErr] = useState("");
  const loadMapScripts = () => api().MapScriptList().then((l) => setMapScripts(l ?? [])).catch(() => {});
  useEffect(() => { loadMapScripts(); }, []);
  const selectMapScript = (name: string) => {
    setMapSel(name); setMapErr("");
    if (name === "") { setMapName(""); setMapCode(""); return; }
    const s = mapScripts.find((x) => x.name === name);
    setMapName(name); setMapCode(s ? s.code : "");
  };
  const saveMapScript = () => {
    api().MapScriptPut(mapName, mapCode).then(() => { setMapErr(""); setMapSel(mapName); loadMapScripts(); }).catch((e) => setMapErr(String(e)));
  };
  const deleteMapScript = () => {
    api().MapScriptDelete(mapSel).then(() => { setMapErr(""); selectMapScript(""); loadMapScripts(); }).catch((e) => setMapErr(String(e)));
  };
  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="modal settings-modal" onClick={(e) => e.stopPropagation()}>
        <h2>settings</h2>

        <div class="opthead">theme</div>
        <div class="formrow">
          <label>theme</label>
          <select class="themesel" value={settings.theme} onChange={(e) => onChangeTheme((e.target as HTMLSelectElement).value)}>
            {themeList.map((n) => <option value={n} key={n}>{n}</option>)}
          </select>
        </div>
        <div class="formrow"><label></label><button class="importbtn" onClick={onImport}>import iTerm2 theme…</button></div>

        <div class="opthead">fonts (empty = theme default)</div>
        <div class="formrow"><label>UI font</label>
          <input list="f9-ui-fonts" value={settings.fontUI} placeholder="Inter" onInput={(e) => onSave({ fontUI: (e.target as HTMLInputElement).value })} /></div>
        <div class="formrow"><label>UI size</label>
          <input type="number" min="8" max="40" value={settings.fontUISize || ""} placeholder="theme"
            onInput={(e) => onSave({ fontUISize: numOrZero((e.target as HTMLInputElement).value) })} /></div>
        <div class="formrow"><label>terminal font</label>
          <input list="f9-mono-fonts" value={settings.fontMono} placeholder="JetBrains Mono" onInput={(e) => onSave({ fontMono: (e.target as HTMLInputElement).value })} /></div>
        <div class="formrow"><label>terminal size</label>
          <input type="number" min="8" max="40" value={settings.fontTermSize || ""} placeholder="theme"
            onInput={(e) => onSave({ fontTermSize: numOrZero((e.target as HTMLInputElement).value) })} /></div>
        <datalist id="f9-ui-fonts">{UI_FONTS.map((f) => <option value={f} key={f} />)}</datalist>
        <datalist id="f9-mono-fonts">{MONO_FONTS.map((f) => <option value={f} key={f} />)}</datalist>

        <div class="opthead">zoom</div>
        <div class="formrow">
          <label>{Math.round((settings.zoom || 1) * 100)}%</label>
          <input type="range" min="0.6" max="2" step="0.05" value={String(settings.zoom || 1)}
            onInput={(e) => onSave({ zoom: parseFloat((e.target as HTMLInputElement).value) })} />
        </div>

        <div class="opthead">tab indicators (new tabs)</div>
        <label class="checkrow"><input type="checkbox" checked={defInd.output} onChange={(e) => onDefInd({ ...defInd, output: (e.target as HTMLInputElement).checked })} /> <span class="swatch output" /> output</label>
        <label class="checkrow"><input type="checkbox" checked={defInd.prompt} onChange={(e) => onDefInd({ ...defInd, prompt: (e.target as HTMLInputElement).checked })} /> <span class="swatch prompt" /> command done</label>
        <label class="checkrow"><input type="checkbox" checked={defInd.match} onChange={(e) => onDefInd({ ...defInd, match: (e.target as HTMLInputElement).checked })} /> <span class="swatch match" /> regex match</label>

        <div class="opthead">feature bars (off by default)</div>
        <label class="checkrow"><input type="checkbox" checked={settings.showGlobalBar} onChange={(e) => onSave({ showGlobalBar: (e.target as HTMLInputElement).checked })} /> G-Bar (global)</label>
        <label class="checkrow"><input type="checkbox" checked={settings.showFolderBar} onChange={(e) => onSave({ showFolderBar: (e.target as HTMLInputElement).checked })} /> C-Bar (context: folder / OS)</label>
        <label class="checkrow"><input type="checkbox" checked={settings.showTemplates} onChange={(e) => onSave({ showTemplates: (e.target as HTMLInputElement).checked })} /> template composer</label>
        <label class="checkrow"><input type="checkbox" checked={settings.showSnippets} onChange={(e) => onSave({ showSnippets: (e.target as HTMLInputElement).checked })} /> snippet library (Ctrl+P)</label>
        <label class="checkrow"><input type="checkbox" checked={settings.showMultiSend} onChange={(e) => onSave({ showMultiSend: (e.target as HTMLInputElement).checked })} /> multi-send (broadcast to marked tabs)</label>

        <div class="opthead">button bar layout</div>
        <div class="formrow"><label>layout</label>
          <select value={settings.barVertical ? "vertical" : "horizontal"} onChange={(e) => onSave({ barVertical: (e.target as HTMLSelectElement).value === "vertical" })}>
            <option value="horizontal">horizontal (bottom)</option>
            <option value="vertical">vertical (right)</option>
          </select>
        </div>
        <label class="checkrow"><input type="checkbox" checked={!settings.barUnpinned} onChange={(e) => onSave({ barUnpinned: !(e.target as HTMLInputElement).checked })} /> pin vertical bar (uncheck = auto-collapse)</label>

        <div class="opthead">SSH agent sockets</div>
        <div class="ssh-note">GUI apps don't inherit a shell's SSH_AUTH_SOCK (macOS hands them the launchd agent). Add socket paths to point f9 at your agent(s) — e.g. gpg-agent or Secretive. Empty = use SSH_AUTH_SOCK.</div>
        {agentSockets.map((sk, i) => (
          <div class="formrow" key={i}>
            <label></label>
            <input value={sk} placeholder="/path/to/agent.sock" onInput={(e) => { const next = agentSockets.slice(); next[i] = (e.target as HTMLInputElement).value; setAgentSockets(next); }} />
            <button class="ssh-del" onClick={() => setAgentSockets(agentSockets.filter((_, j) => j !== i))}>remove</button>
          </div>
        ))}
        <div class="formrow"><label></label><button class="importbtn" onClick={() => setAgentSockets([...agentSockets, ""])}>add agent socket…</button></div>
        <div class="ssh-block">
          {agent === null
            ? <div class="ssh-note">checking…</div>
            : (agent.endpoints ?? []).length === 0
              ? <div class="ssh-note">no agent configured (no SSH_AUTH_SOCK and no sockets above)</div>
              : (agent.endpoints ?? []).map((ep, i) => (
                  <div class="ssh-ep" key={i}>
                    <div class="ssh-note">{ep.available ? "connected" : "not connected"} · <span class="ssh-mono">{ep.socket}</span>{ep.error ? " — " + ep.error : ""}</div>
                    {ep.available && ((ep.keys ?? []).length === 0
                      ? <div class="ssh-note">no keys loaded</div>
                      : (<ul class="ssh-keys">
                          {(ep.keys ?? []).map((k, j) => (
                            <li key={j}><span class="ssh-mono">{k.format}</span> {k.comment || "(no comment)"} <span class="ssh-fp">{k.fingerprint}</span></li>
                          ))}
                        </ul>))}
                  </div>
                ))}
          <button class="importbtn" onClick={refreshAgent}>refresh agent</button>
        </div>
        <label class="checkrow"><input type="checkbox" checked={!settings.disableAgent} onChange={(e) => onSave({ disableAgent: !(e.target as HTMLInputElement).checked })} /> use SSH agent when available</label>

        <div class="opthead">SSH key files (used when no agent, or alongside it)</div>
        <div class="ssh-note">empty = auto-discover ~/.ssh/id_ed25519, id_ecdsa, id_rsa. Encrypted keys prompt for a passphrase on connect.</div>
        {keyFiles.map((kf, i) => (
          <div class="formrow" key={i}>
            <label></label>
            <input value={kf} placeholder="~/.ssh/id_ed25519" onInput={(e) => { const next = keyFiles.slice(); next[i] = (e.target as HTMLInputElement).value; setKeyFiles(next); }} />
            <button class="ssh-del" onClick={() => setKeyFiles(keyFiles.filter((_, j) => j !== i))}>remove</button>
          </div>
        ))}
        <div class="formrow"><label></label><button class="importbtn" onClick={() => setKeyFiles([...keyFiles, ""])}>add key file…</button></div>

        <div class="opthead">alternative usernames</div>
        <div class="ssh-note">named logins for different target kinds (e.g. jumphost → jdoe, linux → u1234567, windows → john.doe). Available in map scripts as f9.alt_user("label").</div>
        {altDraft.map((au, i) => (
          <div class="formrow" key={i}>
            <label></label>
            <input class="alt-label" placeholder="label" value={au.label} onInput={(e) => setAltDraft(altDraft.map((x, j) => j === i ? { ...x, label: (e.target as HTMLInputElement).value } : x))} onBlur={() => onSave({ altUsers: altDraft })} />
            <input placeholder="username" value={au.user} onInput={(e) => setAltDraft(altDraft.map((x, j) => j === i ? { ...x, user: (e.target as HTMLInputElement).value } : x))} onBlur={() => onSave({ altUsers: altDraft })} />
            <button class="ssh-del" onClick={() => commitAlt(altDraft.filter((_, j) => j !== i))}>remove</button>
          </div>
        ))}
        <div class="formrow"><label></label><button class="importbtn" onClick={() => commitAlt([...altDraft, { label: "", user: "" }])}>add username…</button></div>

        <div class="opthead">import map scripts (Lua)</div>
        <div class="ssh-note">map(r) runs per imported record after the filter: return r to keep it (fields: name, host, port, user, proto, folder, tags, attrs, raw), or nil to drop it.</div>
        <div class="formrow"><label>script</label>
          <select value={mapSel} onChange={(e) => selectMapScript((e.target as HTMLSelectElement).value)}>
            <option value="">new script…</option>
            {mapScripts.map((s) => <option key={s.name} value={s.name}>{s.name}</option>)}
          </select>
        </div>
        <div class="formrow"><label>name</label><input value={mapName} onInput={(e) => setMapName((e.target as HTMLInputElement).value)} placeholder="my-script" /></div>
        <textarea class="lua-editor" rows={10} spellcheck={false} value={mapCode} onInput={(e) => setMapCode((e.target as HTMLTextAreaElement).value)} placeholder={"function map(r)\n  return r\nend"} />
        {mapErr && <div class="imp-warn">{mapErr}</div>}
        <div class="filt-actions">
          <button class="importbtn" disabled={mapName === ""} onClick={saveMapScript}>save script</button>
          {mapSel !== "" && <button class="ssh-del" onClick={deleteMapScript}>delete script</button>}
        </div>

        <div class="modal-actions"><button class="primary" onClick={onClose}>done</button></div>
      </div>
    </div>
  );
}

function SearchPanel(props: {
  stats: number | null; res: GrepResult | null; busy: boolean;
  q: string; ic: boolean; inv: boolean; ctx: number;
  peek: Record<number, PeekResult | null>;
  onQ: (v: string) => void; onIC: (v: boolean) => void; onInv: (v: boolean) => void; onCtx: (v: number) => void;
  onRun: () => void; onClose: () => void; onRow: (i: number, lineNo: number) => void;
}) {
  const { stats, res, busy, q, ic, inv, ctx, peek, onQ, onIC, onInv, onCtx, onRun, onClose, onRow } = props;
  const matches = res?.matches ?? [];
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => { inputRef.current?.focus(); inputRef.current?.select(); }, []);
  return (
    <div class="searchpanel">
      <div class="searchbar">
        <input ref={inputRef} class="searchinput" autoFocus placeholder="regex over full scrollback…" value={q}
          onInput={(e) => onQ((e.target as HTMLInputElement).value)}
          onKeyDown={(e) => { if (e.key === "Enter") onRun(); if (e.key === "Escape") onClose(); }} />
        <label class="sopt" title="ignore case"><input type="checkbox" checked={ic} onChange={(e) => onIC((e.target as HTMLInputElement).checked)} /> i</label>
        <label class="sopt" title="invert match"><input type="checkbox" checked={inv} onChange={(e) => onInv((e.target as HTMLInputElement).checked)} /> v</label>
        <label class="sopt" title="context lines">ctx <input type="number" min="0" max="10" value={ctx} onInput={(e) => onCtx(parseInt((e.target as HTMLInputElement).value, 10) || 0)} /></label>
        <button class="searchgo" onClick={onRun} disabled={busy}>{busy ? "…" : "search"}</button>
        <button class="searchx" title="close (Esc)" onClick={onClose}>{"\u2715"}</button>
      </div>
      <div class="searchmeta">
        {stats != null && <span>{stats.toLocaleString()} lines in scrollback</span>}
        {res && <span>{res.count.toLocaleString()} match{res.count === 1 ? "" : "es"}{res.truncated ? " (truncated)" : ""}</span>}
      </div>
      <div class="searchresults">
        {res && res.count === 0 && <div class="snohits">no matches</div>}
        {matches.map((m, i) => (
          <div class="sresult" key={i}>
            {(m.before ?? []).map((b, j) => <div class="sline sctx" key={"b" + j}>{b}</div>)}
            <div class="sline shit clickable" title="click for context" onClick={() => onRow(i, m.lineNo)}>
              <span class="sno">{m.lineNo}</span>{m.line}
            </div>
            {(m.after ?? []).map((af, j) => <div class="sline sctx" key={"a" + j}>{af}</div>)}
            {peek[i] && (
              <div class="speek">
                {(peek[i]!.lines ?? []).map((ln, j) => {
                  const no = peek[i]!.start + j;
                  return <div class={"sline " + (no === m.lineNo ? "shit" : "sctx")} key={j}><span class="sno">{no}</span>{ln}</div>;
                })}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

const STATE_LABEL: Record<string, string> = { dialing: "dialing…", connected: "connected", error: "error" };
const EMPTY_SETTINGS: UISettings = { theme: "", zoom: 1, fontUI: "", fontMono: "", fontUISize: 0, fontTermSize: 0, showGlobalBar: false, showFolderBar: false, showTemplates: false, showSnippets: false, barVertical: false, barUnpinned: false, showMultiSend: false };

function UnresolvedModal(props: {
  names: string[];
  onSubmit: (vals: Record<string, string>, remember: boolean) => void;
  onCancel: () => void;
}) {
  const { names, onSubmit, onCancel } = props;
  const [vals, setVals] = useState<Record<string, string>>(() => Object.fromEntries(names.map((n) => [n, ""])));
  const [remember, setRemember] = useState(true);
  const set = (k: string, v: string) => setVals((p) => ({ ...p, [k]: v }));
  const allFilled = names.every((n) => (vals[n] ?? "").trim() !== "");
  return (
    <div class="modal-overlay">
      <div class="modal">
        <h2>fill in template variables</h2>
        {names.map((n, i) => (
          <div class="formrow" key={n}>
            <label>{n}</label>
            <input autoFocus={i === 0} value={vals[n] ?? ""}
              onInput={(e) => set(n, (e.target as HTMLInputElement).value)}
              onKeyDown={(e) => { if (e.key === "Enter" && allFilled) onSubmit(vals, remember); }} />
          </div>
        ))}
        <label class="checkrow">
          <input type="checkbox" checked={remember} onChange={(e) => setRemember((e.target as HTMLInputElement).checked)} />
          remember for this session
        </label>
        <div class="modal-actions">
          <button onClick={onCancel}>cancel</button>
          <button class="primary" disabled={!allFilled} onClick={() => onSubmit(vals, remember)}>send</button>
        </div>
      </div>
    </div>
  );
}

function SendPanel(props: {
  body: string; delay: number; bracketed: boolean;
  onBody: (v: string) => void; onDelay: (v: number) => void; onBracketed: (v: boolean) => void;
  onSend: () => void; onClose: () => void;
}) {
  const { body, delay, bracketed, onBody, onDelay, onBracketed, onSend, onClose } = props;
  const taRef = useRef<HTMLTextAreaElement>(null);
  useEffect(() => { taRef.current?.focus(); }, []);
  return (
    <div class="sendpanel">
      <div class="sendhead">
        <span class="sendtitle">send template</span>
        <label class="sopt" title="bracketed paste"><input type="checkbox" checked={bracketed} onChange={(e) => onBracketed((e.target as HTMLInputElement).checked)} /> bracketed</label>
        <label class="sopt" title="per-line delay (ms)">delay <input type="number" min="0" max="5000" step="10" value={delay} onInput={(e) => onDelay(parseInt((e.target as HTMLInputElement).value, 10) || 0)} /></label>
        <button class="searchx" title="close (Esc)" onClick={onClose}>{"\u2715"}</button>
      </div>
      <textarea ref={taRef} class="sendinput" value={body}
        placeholder="pongo2 template \u2014 {{ vlan_id }}, {% if %} \u2026 \u2014 Ctrl+Enter to send"
        onInput={(e) => onBody((e.target as HTMLTextAreaElement).value)}
        onKeyDown={(e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); onSend(); } if (e.key === "Escape") onClose(); }} />
      <div class="sendfoot">
        <span class="sendhint">vars resolve by session \u00b7 OS-aware \u00b7 Ctrl+Enter sends</span>
        <button class="searchgo" onClick={onSend}>send</button>
      </div>
    </div>
  );
}

function ButtonBar(props: { bar: Bar; onAction: (a: BarAction) => void }) {
  const rows = props.bar.rows ?? [];
  return (
    <div class="buttonbar">
      {rows.map((r, ri) => (
        <div class="bbrow" key={ri}>
          {(r.buttons ?? []).map((b, bi) => (
            <button class="bbbtn" key={bi} title={b.action.kind}
              style={b.color ? { borderColor: b.color, color: b.color } : undefined}
              onClick={() => props.onAction(b.action)}>
              {b.icon ? <span class="bbicon">{b.icon}</span> : null}{b.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  );
}

function BarStrip(props: {
  global: Bar | null; folder: Bar | null;
  showGlobal: boolean; showFolder: boolean; folderActive: boolean;
  onAction: (a: BarAction) => void; onEditGlobal: () => void; onEditFolder: () => void;
}) {
  const showG = props.showGlobal;
  const showF = props.showFolder && props.folderActive;
  if (!showG && !showF) return null;
  return (
    <div class="barstrip">
      {showG && (
        <div class="barstrip-row">
          <span class="barstrip-label">G-Bar</span>
          <ButtonBar bar={props.global ?? { rows: [] }} onAction={props.onAction} />
          <button class="barcog" title="configure G-Bar (global)" onClick={props.onEditGlobal}>{"\u2699"}</button>
        </div>
      )}
      {showF && (
        <div class="barstrip-row">
          <span class="barstrip-label">C-Bar</span>
          <ButtonBar bar={props.folder ?? { rows: [] }} onAction={props.onAction} />
          <button class="barcog" title="configure C-Bar (context)" onClick={props.onEditFolder}>{"\u2699"}</button>
        </div>
      )}
    </div>
  );
}

function BarEditorModal(props: {
  scope: "folder" | "global"; yaml: string; err: string; canDelete: boolean;
  onYaml: (v: string) => void; onSave: () => void; onDelete: () => void; onClose: () => void;
}) {
  const { scope, yaml, err, canDelete, onYaml, onSave, onDelete, onClose } = props;
  return (
    <div class="modal-overlay">
      <div class="modal bar-editor">
        <h2>{scope === "global" ? "G-Bar (global button bar)" : "C-Bar (context button bar)"}</h2>
        <div class="bareditor-hint">
          {scope === "global"
            ? "G-Bar - shown on every session (each button OS-filtered by its os: field)."
            : "C-Bar - context bar for this folder/OS. add os: <family> (e.g. nxos, ios) to a button to show it only on that detected OS; omit os for all."}
        </div>
        <textarea class="bareditor-yaml" spellcheck={false} value={yaml}
          onInput={(e) => onYaml((e.target as HTMLTextAreaElement).value)} />
        {err && <div class="bareditor-err">{err}</div>}
        <div class="modal-actions">
          <button onClick={onClose}>close</button>
          {canDelete && <button class="danger" onClick={onDelete}>delete</button>}
          <button class="primary" onClick={onSave}>save</button>
        </div>
      </div>
    </div>
  );
}

function SnippetLibraryModal(props: {
  folders: SnippetFolder[]; snippets: Snippet[]; err: string;
  onSaveFolder: (name: string) => void; onDeleteFolder: (id: string) => void;
  onSaveSnippet: (s: Snippet) => Promise<Snippet | null>; onDeleteSnippet: (id: string) => void;
  onClose: () => void;
}) {
  const { folders, snippets, err, onSaveFolder, onDeleteFolder, onSaveSnippet, onDeleteSnippet, onClose } = props;
  const blank = (): Snippet => ({ id: "", folderId: "", name: "", body: "", os: "", delayMs: 0, bracketed: false });
  const [sel, setSel] = useState<Snippet | null>(null);
  const [form, setForm] = useState<Snippet>(blank());
  const [newFolder, setNewFolder] = useState("");
  const folderName = (id?: string) => folders.find((f) => f.id === id)?.name ?? "(no folder)";
  const OS_OPTS = ["", "all", "unknown", "linux", "openbsd", "ios", "nxos", "panos", "junos", "windows"];
  const doSave = () => { onSaveSnippet(form).then((saved) => { if (saved) { setSel(saved); setForm({ ...saved }); } }); };
  return (
    <div class="modal-overlay">
      <div class="modal snippet-lib">
        <h2>snippet library</h2>
        {err && <div class="bareditor-err">{err}</div>}
        <div class="snlib-body">
          <div class="snlib-list">
            <div class="snlib-listhead"><span>snippets</span><button onClick={() => { setSel(null); setForm(blank()); }}>+ new</button></div>
            {snippets.length === 0 && <div class="snlib-empty">no snippets yet</div>}
            {snippets.map((s) => (
              <div class={"snlib-item" + (sel?.id === s.id ? " active" : "")} key={s.id} onClick={() => { setSel(s); setForm({ ...s }); }}>
                <span class="snlib-name">{s.name}</span>
                <span class="snlib-meta">{folderName(s.folderId)}{s.os ? " \u00b7 " + s.os : ""}</span>
              </div>
            ))}
            <div class="snlib-listhead"><span>folders</span></div>
            {folders.map((f) => (
              <div class="snlib-folder" key={f.id}><span>{f.name}</span>
                <button class="snlib-del" title="delete folder" onClick={() => onDeleteFolder(f.id)}>{"\u2715"}</button>
              </div>
            ))}
            <div class="snlib-newfolder">
              <input placeholder="new folder" value={newFolder} onInput={(e) => setNewFolder((e.target as HTMLInputElement).value)} />
              <button onClick={() => { if (newFolder.trim()) { onSaveFolder(newFolder.trim()); setNewFolder(""); } }}>add</button>
            </div>
          </div>
          <div class="snlib-form">
            <div class="formrow"><label>name</label><input value={form.name} onInput={(e) => setForm({ ...form, name: (e.target as HTMLInputElement).value })} /></div>
            <div class="formrow"><label>folder</label>
              <select value={form.folderId} onChange={(e) => setForm({ ...form, folderId: (e.target as HTMLSelectElement).value })}>
                <option value="">(no folder)</option>
                {folders.map((f) => <option value={f.id} key={f.id}>{f.name}</option>)}
              </select>
            </div>
            <div class="formrow"><label>OS</label>
              <select value={form.os} onChange={(e) => setForm({ ...form, os: (e.target as HTMLSelectElement).value })}>
                {OS_OPTS.map((o) => <option value={o} key={o}>{o || "(any)"}</option>)}
              </select>
            </div>
            <div class="formrow"><label>delay (ms)</label>
              <input type="number" min="0" max="5000" value={form.delayMs || ""} onInput={(e) => setForm({ ...form, delayMs: parseInt((e.target as HTMLInputElement).value, 10) || 0 })} /></div>
            <label class="checkrow"><input type="checkbox" checked={!!form.bracketed} onChange={(e) => setForm({ ...form, bracketed: (e.target as HTMLInputElement).checked })} /> bracketed paste</label>
            <textarea class="snlib-body-input" spellcheck={false} placeholder="pongo2 template body - {{ vlan_id }}, {% if %} ..." value={form.body}
              onInput={(e) => setForm({ ...form, body: (e.target as HTMLTextAreaElement).value })} />
            <div class="snlib-formactions">
              {sel && <button class="danger" onClick={() => { onDeleteSnippet(sel.id); setSel(null); setForm(blank()); }}>delete</button>}
              <button class="primary" disabled={form.name.trim() === ""} onClick={doSave}>{sel ? "save" : "create"}</button>
            </div>
          </div>
        </div>
        <div class="modal-actions"><button onClick={onClose}>close</button></div>
      </div>
    </div>
  );
}

function fuzzyScore(q: string, text: string): number {
  if (q === "") return 1;
  const ql = q.toLowerCase(), tl = text.toLowerCase();
  let ti = 0, score = 0, streak = 0;
  for (let qi = 0; qi < ql.length; qi++) {
    let found = -1;
    for (let j = ti; j < tl.length; j++) { if (tl[j] === ql[qi]) { found = j; break; } }
    if (found === -1) return 0;
    streak = found === ti ? streak + 1 : 0;
    score += 1 + streak;
    ti = found + 1;
  }
  return score;
}

function SnippetPicker(props: {
  snippets: Snippet[]; folderName: (id?: string) => string;
  onRun: (s: Snippet) => void; onClose: () => void; onEdit: () => void;
}) {
  const { snippets, folderName, onRun, onClose, onEdit } = props;
  const [q, setQ] = useState("");
  const [idx, setIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  useEffect(() => { inputRef.current?.focus(); }, []);
  const ranked = snippets
    .map((s) => ({ s, score: fuzzyScore(q, s.name + " " + folderName(s.folderId)) }))
    .filter((r) => r.score > 0)
    .sort((a, b) => b.score - a.score || a.s.name.localeCompare(b.s.name));
  const clamp = Math.min(idx, Math.max(0, ranked.length - 1));
  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="picker" onClick={(e) => e.stopPropagation()}>
        <input ref={inputRef} class="picker-input" placeholder="run snippet..." value={q}
          onInput={(e) => { setQ((e.target as HTMLInputElement).value); setIdx(0); }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") { e.preventDefault(); setIdx((i) => Math.min(i + 1, ranked.length - 1)); }
            else if (e.key === "ArrowUp") { e.preventDefault(); setIdx((i) => Math.max(i - 1, 0)); }
            else if (e.key === "Enter") { e.preventDefault(); if (ranked[clamp]) onRun(ranked[clamp].s); }
            else if (e.key === "Escape") { e.preventDefault(); onClose(); }
          }} />
        <div class="picker-list">
          {ranked.length === 0 && (
            <div class="picker-empty">no snippets \u2014 <span class="picker-editlink" onClick={onEdit}>manage library</span></div>
          )}
          {ranked.map((r, i) => (
            <div class={"picker-item" + (i === clamp ? " active" : "")} key={r.s.id}
              onMouseEnter={() => setIdx(i)} onClick={() => onRun(r.s)}>
              <span class="picker-name">{r.s.name}</span>
              <span class="picker-meta">{folderName(r.s.folderId)}{r.s.os ? " \u00b7 " + r.s.os : ""}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function BarRail(props: {
  global: Bar | null; folder: Bar | null;
  showGlobal: boolean; showFolder: boolean; folderActive: boolean; pinned: boolean;
  onAction: (a: BarAction) => void; onEditGlobal: () => void; onEditFolder: () => void;
}) {
  const [hover, setHover] = useState(false);
  const showG = props.showGlobal;
  const showF = props.showFolder && props.folderActive;
  if (!showG && !showF) return null;
  const expanded = props.pinned || hover;
  const act = (a: BarAction) => { props.onAction(a); if (!props.pinned) setHover(false); };
  const buttons = (bar: Bar | null) => (bar?.rows ?? []).flatMap((r, ri) =>
    (r.buttons ?? []).map((b, bi) => (
      <button class="bbbtn barrail-btn" key={ri + "-" + bi} title={b.action.kind}
        style={b.color ? { borderColor: b.color, color: b.color } : undefined}
        onClick={() => act(b.action)}>{b.icon ? <span class="bbicon">{b.icon}</span> : null}{b.label}</button>
    )));
  return (
    <div class={"barrail " + (expanded ? "expanded" : "collapsed")}
      onMouseEnter={() => { if (!props.pinned) setHover(true); }}
      onMouseLeave={() => { if (!props.pinned) setHover(false); }}>
      {expanded ? (
        <div class="barrail-inner">
          {showG && (
            <div class="barrail-section">
              <div class="barrail-head"><span>G-Bar</span><button class="barcog" title="configure G-Bar" onClick={props.onEditGlobal}>{"\u2699"}</button></div>
              {buttons(props.global)}
            </div>
          )}
          {showF && (
            <div class="barrail-section">
              <div class="barrail-head"><span>C-Bar</span><button class="barcog" title="configure C-Bar" onClick={props.onEditFolder}>{"\u2699"}</button></div>
              {buttons(props.folder)}
            </div>
          )}
        </div>
      ) : (
        <div class="barrail-collapsed">
          {showG && <button class="barcog" title="G-Bar" onClick={props.onEditGlobal}>{"\u2699"}</button>}
          {showF && <button class="barcog" title="C-Bar" onClick={props.onEditFolder}>{"\u2699"}</button>}
          <span class="barrail-handle" title="hover to expand">{"\u25c2"}</span>
        </div>
      )}
    </div>
  );
}

function MultiSendModal(props: {
  targets: { termId: string; name: string }[];
  line: string; seq: boolean; timeout: number;
  preview: MSPreview[] | null; results: Record<string, MSResult>; running: boolean;
  onLine: (v: string) => void; onSeq: (v: boolean) => void; onTimeout: (v: number) => void;
  onMarkAll: () => void; onClear: () => void; onUnmark: (id: string) => void;
  onDryRun: () => void; onSend: () => void; onCancel: () => void; onClose: () => void;
  confirm: string | null; onConfirmSend: () => void; onConfirmCancel: () => void; onJump: (id: string) => void;
}) {
  const { targets, line, seq, timeout, preview, results, running, onLine, onSeq, onTimeout, onMarkAll, onClear, onUnmark, onDryRun, onSend, onCancel, onClose, confirm, onConfirmSend, onConfirmCancel, onJump } = props;
  const tail1 = (r?: MSResult) => (r?.errText || (r?.tail ?? "").split("\n").filter(Boolean).slice(-1)[0] || "").slice(0, 70);
  return (
    <div class="modal-overlay">
      <div class="modal multisend">
        <h2>multi-send {"\u2014"} {targets.length} target{targets.length === 1 ? "" : "s"}</h2>
        <div class="ms-markctl"><button onClick={onMarkAll}>mark all open</button><button onClick={onClear}>clear</button></div>
        {targets.length === 0 && <div class="ms-empty">mark terminals with the checkbox in the tab strip.</div>}
        <div class="ms-targets">
          {targets.map((t) => (
            <span class="ms-chip" key={t.termId}>{t.name}<span class="ms-chipx" onClick={() => onUnmark(t.termId)}>{"\u2715"}</span></span>
          ))}
        </div>
        <textarea class="ms-input" placeholder="command or template - {{ vlan_id }}, {% if %} ..." value={line}
          onInput={(e) => onLine((e.target as HTMLTextAreaElement).value)} />
        <div class="ms-opts">
          <label class="sopt"><input type="checkbox" checked={seq} onChange={(e) => onSeq((e.target as HTMLInputElement).checked)} /> sequential</label>
          <label class="sopt">timeout <input type="number" min="1000" max="120000" step="500" value={timeout} onInput={(e) => onTimeout(parseInt((e.target as HTMLInputElement).value, 10) || 15000)} /> ms</label>
          <button onClick={onDryRun} disabled={targets.length === 0}>dry-run</button>
          {running
            ? <button class="danger" onClick={onCancel}>cancel</button>
            : confirm
              ? null
              : <button class="primary" onClick={onSend} disabled={targets.length === 0 || line.trim() === ""}>send</button>}
        </div>
        {confirm && !running && (
          <div class="ms-confirm"><span class="ms-warn">heads up: {confirm}</span><button class="danger" onClick={onConfirmSend}>send anyway</button><button onClick={onConfirmCancel}>cancel</button></div>
        )}
        {preview && (
          <div class="ms-grid preview">
            <div class="ms-gridhead"><span>session</span><span>os</span><span>rendered</span></div>
            {preview.map((p) => (
              <div class="ms-row clickable" key={p.termId} onClick={() => onJump(p.termId)}>
                <span>{p.name || p.termId}</span>
                <span class="ms-os">{p.osFamily || "?"}</span>
                <span class="ms-line">{p.err ? <span class="ms-err">{p.err}</span> : p.line}{(p.unresolved ?? []).length > 0 ? <span class="ms-warn"> \u00b7 needs: {(p.unresolved ?? []).join(", ")}</span> : null}</span>
              </div>
            ))}
          </div>
        )}
        {Object.keys(results).length > 0 && (
          <div class="ms-grid results">
            <div class="ms-gridhead"><span>session</span><span>state</span><span>ms</span><span>tail</span></div>
            {targets.map((t) => {
              const r = results[t.termId];
              return (
                <div class="ms-row clickable" key={t.termId} onClick={() => onJump(t.termId)}>
                  <span>{t.name}</span>
                  <span class={"msstate ms-" + (r?.state ?? "pending")}>{r?.state ?? "-"}</span>
                  <span class="ms-ms">{r?.millis ?? ""}</span>
                  <span class="ms-tail" title={r?.tail ?? ""}>{tail1(r)}</span>
                </div>
              );
            })}
          </div>
        )}
        <div class="modal-actions"><button onClick={onClose}>close</button></div>
      </div>
    </div>
  );
}

type ImportState = { folderId: string; dto: SourceDTO; secret: string; test: TestResult | null; testing: boolean; err: string };

function FolderCtxMenu(props: {
  x: number; y: number; hasSource: boolean;
  onImport: () => void; onRefresh: () => void; onClear: () => void;
}) {
  return (
    <div class="ctxmenu" style={{ left: `${props.x + 2}px`, top: `${props.y + 6}px` }} onClick={(e) => e.stopPropagation()}>
      <div class="mitem" onClick={props.onImport}>{props.hasSource ? "edit import source\u2026" : "import source\u2026"}</div>
      {props.hasSource && <div class="mitem" onClick={props.onRefresh}>refresh source</div>}
      {props.hasSource && <div class="mitem danger" onClick={props.onClear}>clear source</div>}
    </div>
  );
}

function splitSourceUrl(u: string): { base: string; path: string } {
  if (!u) return { base: "", path: "/api/dcim/devices/" };
  try {
    const p = new URL(u);
    return { base: p.origin, path: (p.pathname || "/") + p.search };
  } catch {
    return { base: "", path: u };
  }
}

function combineSourceUrl(base: string, path: string): string {
  let b = base;
  while (b.endsWith("/")) b = b.slice(0, -1);
  if (!b) return path;
  return b + (path.startsWith("/") ? path : "/" + path);
}

function isEmptyFilter(g: FilterGroup): boolean {
  return (g.rules ?? []).length === 0 && (g.groups ?? []).length === 0;
}

function FilterGroupEditor(props: { group: FilterGroup; depth: number; onChange: (g: FilterGroup) => void; onRemove?: () => void }) {
  const { group: g, depth, onChange, onRemove } = props;
  const rules = g.rules ?? [];
  const groups = g.groups ?? [];
  const setRule = (i: number, patch: Partial<FilterRule>) => onChange({ ...g, rules: rules.map((r, j) => (j === i ? { ...r, ...patch } : r)) });
  return (
    <div class="filt-group">
      <div class="filt-head">
        <select value={g.op || "and"} onChange={(e) => onChange({ ...g, op: (e.target as HTMLSelectElement).value })}>
          <option value="and">match ALL (AND)</option>
          <option value="or">match ANY (OR)</option>
        </select>
        {onRemove && <button class="filt-del" onClick={onRemove}>remove group</button>}
      </div>
      {rules.map((r, i) => (
        <div class="filt-rule" key={i}>
          <select value={(r.field || "").startsWith("cf:") ? "__cf" : r.field} onChange={(e) => { const v = (e.target as HTMLSelectElement).value; setRule(i, { field: v === "__cf" ? "cf:" : v }); }}>
            {["status", "role", "hostname", "manufacturer", "model", "tenant", "site"].map((f) => <option key={f} value={f}>{f}</option>)}
            <option value="__cf">custom field\u2026</option>
          </select>
          {(r.field || "").startsWith("cf:") && <input class="filt-cfkey" placeholder="cf key (e.g. cmdbSupportTeam)" value={(r.field || "").slice(3)} onInput={(e) => setRule(i, { field: "cf:" + (e.target as HTMLInputElement).value })} />}
          <select value={r.kind} onChange={(e) => setRule(i, { kind: (e.target as HTMLSelectElement).value })}>
            <option value="eq">is</option>
            <option value="contains">contains</option>
            <option value="regex">regex</option>
          </select>
          <input value={r.value} placeholder="value" onInput={(e) => setRule(i, { value: (e.target as HTMLInputElement).value })} />
          <label class="filt-neg"><input type="checkbox" checked={r.negate} onChange={(e) => setRule(i, { negate: (e.target as HTMLInputElement).checked })} /> not</label>
          <button class="filt-del" onClick={() => onChange({ ...g, rules: rules.filter((_, j) => j !== i) })}>{"\u00d7"}</button>
        </div>
      ))}
      {groups.map((sub, i) => (
        <FilterGroupEditor key={i} group={sub} depth={depth + 1} onChange={(x) => onChange({ ...g, groups: groups.map((y, j) => (j === i ? x : y)) })} onRemove={() => onChange({ ...g, groups: groups.filter((_, j) => j !== i) })} />
      ))}
      <div class="filt-actions">
        <button class="importbtn" onClick={() => onChange({ ...g, rules: [...rules, { field: "role", kind: "eq", value: "", negate: false }] })}>+ rule</button>
        {depth < 3 && <button class="importbtn" onClick={() => onChange({ ...g, groups: [...groups, { op: "and", rules: [], groups: [] }] })}>+ group</button>}
      </div>
    </div>
  );
}

function JumpChainModal(props: { initial: JumpHop[]; onSave: (hops: JumpHop[]) => Promise<void>; onClose: () => void; onSaved: () => void }) {
  const { initial, onSave, onClose, onSaved } = props;
  const [hops, setHops] = useState<JumpHop[]>(initial.map((h) => ({ ...h })));
  const [err, setErr] = useState("");
  const setHop = (i: number, patch: Partial<JumpHop>) => setHops(hops.map((h, j) => (j === i ? { ...h, ...patch } : h)));
  const move = (i: number, d: number) => {
    const j = i + d;
    if (j < 0 || j >= hops.length) return;
    const next = hops.slice();
    const tmp = next[i]; next[i] = next[j]; next[j] = tmp;
    setHops(next);
  };
  const save = () => {
    onSave(hops.map((h) => ({ host: h.host, port: h.port || 0, user: h.user || "", mode: h.mode || "proxyjump", userOverride: h.userOverride || "" })))
      .then(() => { onSaved(); onClose(); })
      .catch((e) => setErr(String(e)));
  };
  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="modal jump-modal" onClick={(e) => e.stopPropagation()}>
        <h2>jump chain</h2>
        <div class="ssh-note">hops apply in order, first to last, before the target. proxyjump tunnels through the hop; shell-hop runs ssh on the hop's shell (last hop may set a user override).</div>
        {hops.map((h, i) => (
          <div class="jump-row" key={i}>
            <input class="jump-host" placeholder="host" value={h.host} onInput={(e) => setHop(i, { host: (e.target as HTMLInputElement).value })} />
            <input class="jump-port" placeholder="22" value={h.port ? String(h.port) : ""} onInput={(e) => setHop(i, { port: parseInt((e.target as HTMLInputElement).value, 10) || 0 })} />
            <input class="jump-user" placeholder="user" value={h.user} onInput={(e) => setHop(i, { user: (e.target as HTMLInputElement).value })} />
            <select value={h.mode || "proxyjump"} onChange={(e) => setHop(i, { mode: (e.target as HTMLSelectElement).value })}>
              <option value="proxyjump">proxyjump</option>
              <option value="shell-hop">shell-hop</option>
            </select>
            {h.mode === "shell-hop" && <input class="jump-user" placeholder="user override" value={h.userOverride} onInput={(e) => setHop(i, { userOverride: (e.target as HTMLInputElement).value })} />}
            <button class="importbtn" disabled={i === 0} onClick={() => move(i, -1)}>{"\u2191"}</button>
            <button class="importbtn" disabled={i === hops.length - 1} onClick={() => move(i, 1)}>{"\u2193"}</button>
            <button class="ssh-del" onClick={() => setHops(hops.filter((_, j) => j !== i))}>{"\u00d7"}</button>
          </div>
        ))}
        <div class="filt-actions">
          <button class="importbtn" onClick={() => setHops([...hops, { host: "", port: 0, user: "", mode: "proxyjump", userOverride: "" }])}>+ hop</button>
        </div>
        {err && <div class="imp-warn">{err}</div>}
        <div class="modal-actions">
          <button onClick={onClose}>cancel</button>
          <button class="primary" onClick={save}>save</button>
        </div>
      </div>
    </div>
  );
}

function ImportSourceModal(props: {
  st: ImportState;
  onChange: (patch: Partial<ImportState>) => void;
  onDTO: (patch: Partial<SourceDTO>) => void;
  onTest: () => void; onSave: () => void; onClose: () => void; onEditJump: () => void;
}) {
  const { st, onChange, onDTO, onTest, onSave, onClose, onEditJump } = props;
  const dto = st.dto;
  const [scripts, setScripts] = useState<MapScript[]>([]);
  useEffect(() => { api().MapScriptList().then((l) => setScripts(l ?? [])).catch(() => {}); }, []);
  const initURL = splitSourceUrl(dto.url);
  const [base, setBase] = useState(initURL.base);
  const [path, setPath] = useState(initURL.path);
  const applyURL = (b: string, p: string) => { setBase(b); setPath(p); onDTO({ url: combineSourceUrl(b, p) }); };
  const httpsOk = /^https:\/\/.+/.test(dto.url);
  const fm = dto.fieldMap ?? {};
  const mappedOk = dto.format !== "mapped" || Object.keys(fm).length > 0;
  const secretOk = dto.auth === "none" || st.secret !== "" || dto.hasSecret;
  const canSave = httpsOk && mappedOk && secretOk && !!(st.test && st.test.ok);
  const secretLabel = dto.auth === "basic" ? "user:password" : dto.auth === "mtls" ? "cert+key PEM" : "token";
  return (
    <div class="modal-overlay">
      <div class="modal import-src">
        <h2>import source</h2>
        <div class="formrow"><label>base URL</label><input placeholder="https://netbox.example" value={base} onInput={(e) => applyURL((e.target as HTMLInputElement).value, path)} /></div>
        {base !== "" && !base.startsWith("https://") && <div class="imp-warn">base URL must start with https://</div>}
        <div class="formrow"><label>path</label><input placeholder="/api/dcim/devices/" value={path} onInput={(e) => applyURL(base, (e.target as HTMLInputElement).value)} /></div>
        <div class="formrow"><label>format</label>
          <select value={dto.format} onChange={(e) => onDTO({ format: (e.target as HTMLSelectElement).value })}>
            <option value="f9-native">f9-native</option>
            <option value="netbox">NetBox</option>
            <option value="mapped">mapped JSON</option>
          </select>
        </div>
        <div class="formrow"><label>reconcile by</label>
          <select value={dto.reconcileBy} onChange={(e) => onDTO({ reconcileBy: (e.target as HTMLSelectElement).value })}>
            <option value="hostname">hostname</option>
            <option value="externalId">external id</option>
          </select>
        </div>
        <div class="formrow"><label>map script</label>
          <select value={dto.mapScript || ""} onChange={(e) => onDTO({ mapScript: (e.target as HTMLSelectElement).value })}>
            <option value="">none</option>
            {scripts.map((s) => <option key={s.name} value={s.name}>{s.name}</option>)}
          </select>
        </div>
        <div class="formrow"><label>auth</label>
          <select value={dto.auth} onChange={(e) => onDTO({ auth: (e.target as HTMLSelectElement).value })}>
            <option value="none">none</option>
            <option value="bearer">bearer / token header</option>
            <option value="basic">HTTP basic</option>
            <option value="mtls">mTLS client cert</option>
          </select>
        </div>
        {dto.auth === "bearer" && (
          <div class="formrow"><label>header</label><input placeholder="Authorization (blank = Bearer <token>)" value={dto.header} onInput={(e) => onDTO({ header: (e.target as HTMLInputElement).value })} /></div>
        )}
        {dto.auth !== "none" && (dto.auth === "mtls"
          ? <div class="formrow"><label>{secretLabel}</label><textarea class="imp-pem" placeholder={dto.hasSecret ? "(stored, leave blank to keep)" : "-----BEGIN CERTIFICATE-----\n...\n-----BEGIN EC PRIVATE KEY-----\n..."} value={st.secret} onInput={(e) => onChange({ secret: (e.target as HTMLTextAreaElement).value })} /></div>
          : <div class="formrow"><label>{secretLabel}</label><input type="password" placeholder={dto.hasSecret ? "(stored, leave blank to keep)" : ""} value={st.secret} onInput={(e) => onChange({ secret: (e.target as HTMLInputElement).value })} /></div>
        )}
        {dto.format === "mapped" && (
          <div class="imp-map">
            <div class="imp-maphead">field map (f9 field {"\u2190"} source key)</div>
            {["name", "host", "port", "user", "proto", "externalId"].map((f) => (
              <div class="formrow" key={f}><label>{f}</label><input placeholder={"source key for " + f} value={fm[f] ?? ""}
                onInput={(e) => { const next = { ...fm }; const v = (e.target as HTMLInputElement).value; if (v) next[f] = v; else delete next[f]; onDTO({ fieldMap: next }); }} /></div>
            ))}
          </div>
        )}
        <label class="checkrow"><input type="checkbox" checked={dto.insecure} onChange={(e) => onDTO({ insecure: (e.target as HTMLInputElement).checked })} /> skip TLS verification (lab / self-signed only)</label>
        {dto.format === "netbox" && (
          <div class="filt-block">
            <div class="imp-maphead">filter (optional — empty imports all; test shows the filtered count)</div>
            <FilterGroupEditor group={dto.filter ?? { op: "and", rules: [], groups: [] }} depth={0} onChange={(g) => onDTO({ filter: isEmptyFilter(g) ? null : g })} />
          </div>
        )}
        <div class="formrow"><label>jump chain</label>
          <button class="importbtn" onClick={onEditJump}>edit jump chain (applied to all sessions from this source)</button>
        </div>
        <div class="imp-actions">
          <button onClick={onTest} disabled={!httpsOk || st.testing}>{st.testing ? "testing\u2026" : "test connection"}</button>
          {st.test && (st.test.ok
            ? <span class="imp-ok">ok {"\u2014"} {st.test.count} session{st.test.count === 1 ? "" : "s"}{(st.test.sample ?? []).length > 0 ? ": " + (st.test.sample ?? []).join(", ") : ""}</span>
            : <span class="imp-err">{st.test.error}</span>)}
        </div>
        {st.err && <div class="imp-err">{st.err}</div>}
        <div class="modal-actions">
          {httpsOk && mappedOk && secretOk && !(st.test && st.test.ok) && <span class="import-hint">test the connection before saving</span>}
          <button onClick={onClose}>close</button>
          <button class="primary" onClick={onSave} disabled={!canSave}>save</button>
        </div>
      </div>
    </div>
  );
}

function CredPromptModal(props: { mode: "create" | "unlock"; err: string; onSubmit: (pass: string) => void; onCancel: () => void }) {
  const [pass, setPass] = useState("");
  return (
    <div class="modal-overlay">
      <div class="modal">
        <h2>{props.mode === "create" ? "set credential passphrase" : "unlock credentials"}</h2>
        <div class="imp-note">{props.mode === "create"
          ? "encrypts stored source credentials at rest; it is never saved anywhere."
          : "unlocks stored source credentials for this session."}</div>
        <div class="formrow"><input type="password" autoFocus value={pass}
          onInput={(e) => setPass((e.target as HTMLInputElement).value)}
          onKeyDown={(e) => { if (e.key === "Enter" && pass) props.onSubmit(pass); }} /></div>
        {props.err && <div class="imp-err">{props.err}</div>}
        <div class="modal-actions">
          <button onClick={props.onCancel}>cancel</button>
          <button class="primary" disabled={pass === ""} onClick={() => props.onSubmit(pass)}>{props.mode === "create" ? "set" : "unlock"}</button>
        </div>
      </div>
    </div>
  );
}

function Logo() {
  return (
    <svg class="tb-logo" viewBox="0 0 1024 1024" width="18" height="18" aria-hidden="true">
      <rect x="0" y="0" width="1024" height="1024" rx="224" fill="#0D1218" />
      <rect x="64" y="64" width="896" height="896" rx="184" fill="#1A2330" stroke="#2E3B4B" stroke-width="8" />
      <path d="M 260 152 Q 512 128 764 152" fill="none" stroke="#38485C" stroke-width="10" stroke-linecap="round" />
      <g fill="#3DDE7C">
        <rect x="264" y="300" width="92" height="424" />
        <rect x="264" y="300" width="230" height="92" />
        <rect x="264" y="478" width="190" height="92" />
      </g>
      <circle cx="614" cy="446" r="100" fill="none" stroke="#3DDE7C" stroke-width="92" />
      <rect x="668" y="446" width="92" height="278" fill="#3DDE7C" />
    </svg>
  );
}

export function App() {
  const [tree, setTree] = useState<FolderNode | null>(null);
  const [err, setErr] = useState("");
  const [q, setQ] = useState("");
  const [hits, setHits] = useState<FilterHit[] | null>(null);
  const [sel, setSel] = useState<SessionNode | null>(null);
  const [detail, setDetail] = useState<SessionDetail | null>(null);
  const [selFolder, setSelFolder] = useState<{ id: string; path: string } | null>(null);
  const [marked, setMarked] = useState<Record<string, true>>({});
  const [conns, setConns] = useState<Conn[]>([]);
  const [promptQ, setPromptQ] = useState<PromptRequest[]>([]);
  const [ver, setVer] = useState("");
  const [modal, setModal] = useState<"" | "session-new" | "session-edit" | "folder">("");
  const [tabs, setTabs] = useState<Tab[]>([]);
  const [dead, setDead] = useState<Set<string>>(new Set());
  const [view, setView] = useState<View>({ kind: "empty", id: "" });
  const [pinned, setPinned] = useState<SessionNode[]>([]);
  const [pendingOpen, setPendingOpen] = useState<{ id: string; name: string }[]>([]);
  const [activity, setActivity] = useState<Record<string, IndFlags>>({});
  const [tabCfg, setTabCfg] = useState<Record<string, TabCfg>>({});
  const [defInd, setDefInd] = useState<DefInd>({ output: true, prompt: true, match: true });
  const [ctxMenu, setCtxMenu] = useState<{ termId: string; x: number; y: number } | null>(null);
  const [folderCtx, setFolderCtx] = useState<{ node: FolderNode; x: number; y: number } | null>(null);
  const [imp, setImp] = useState<ImportState | null>(null);
  const [credPrompt, setCredPrompt] = useState<{ mode: "create" | "unlock"; run: () => void } | null>(null);
  const [credErr, setCredErr] = useState("");
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
  const [themeList, setThemeList] = useState<string[]>([]);
  const [settings, setSettings] = useState<UISettings>(EMPTY_SETTINGS);
  const [settingsModal, setSettingsModal] = useState(false);
  const [jumpEdit, setJumpEdit] = useState<{ sessionId: string; initial: JumpHop[] } | null>(null);
  const [folderJump, setFolderJump] = useState<{ folderId: string; initial: JumpHop[] } | null>(null);
  const [refreshing, setRefreshing] = useState<Set<string>>(new Set());
  const [snLib, setSnLib] = useState(false);
  const [snFolders, setSnFolders] = useState<SnippetFolder[]>([]);
  const [snList, setSnList] = useState<Snippet[]>([]);
  const [snErr, setSnErr] = useState("");
  const [pickerOpen, setPickerOpen] = useState(false);
  const [marks, setMarks] = useState<Set<string>>(new Set());
  const [msOpen, setMsOpen] = useState(false);
  const [msLine, setMsLine] = useState("");
  const [msSeq, setMsSeq] = useState(false);
  const [msTimeout, setMsTimeout] = useState(15000);
  const [msPreview, setMsPreview] = useState<MSPreview[] | null>(null);
  const [msResults, setMsResults] = useState<Record<string, MSResult>>({});
  const [msRunning, setMsRunning] = useState(false);
  const [msConfirm, setMsConfirm] = useState<string | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQ, setSearchQ] = useState("");
  const [searchIC, setSearchIC] = useState(false);
  const [searchInv, setSearchInv] = useState(false);
  const [searchCtx, setSearchCtx] = useState(0);
  const [searchRes, setSearchRes] = useState<GrepResult | null>(null);
  const [searchStats, setSearchStats] = useState<number | null>(null);
  const [searchBusy, setSearchBusy] = useState(false);
  const [peek, setPeek] = useState<Record<number, PeekResult | null>>({});
  const [sendOpen, setSendOpen] = useState(false);
  const [sendBody, setSendBody] = useState("");
  const [sendDelay, setSendDelay] = useState(0);
  const [sendBracketed, setSendBracketed] = useState(false);
  const [unresolved, setUnresolved] = useState<string[] | null>(null);
  const [sendMemo, setSendMemo] = useState<Record<string, Record<string, string>>>({});
  const [pendingSend, setPendingSend] = useState<{ body: string; delay: number; bracketed: boolean } | null>(null);
  const [bar, setBar] = useState<Bar | null>(null);
  const [gbar, setGbar] = useState<Bar | null>(null);
  const [barEditor, setBarEditor] = useState(false);
  const [barScope, setBarScope] = useState<"folder" | "global">("folder");
  const [barFolderId, setBarFolderId] = useState("");
  const [barYaml, setBarYaml] = useState("");
  const [barErr, setBarErr] = useState("");
  const [barHasOwn, setBarHasOwn] = useState(false);
  const debounce = useRef<number | undefined>(undefined);

  const activeTermRef = useRef<string | null>(null);
  const tabCfgRef = useRef(tabCfg);
  const defIndRef = useRef(defInd);
  useEffect(() => { activeTermRef.current = view.kind === "term" ? view.id : null; }, [view]);
  useEffect(() => { tabCfgRef.current = tabCfg; }, [tabCfg]);
  useEffect(() => { defIndRef.current = defInd; }, [defInd]);

  const load = () => api().Tree().then((t) => { setTree(t); if (!selFolder) setSelFolder({ id: t.id, path: t.path }); }).catch((e) => setErr(String(e)));

  const openFolderCtx = (node: FolderNode, x: number, y: number) => setFolderCtx({ node, x, y });
  const blankDTO = (): SourceDTO => ({ url: "", format: "f9-native", auth: "none", header: "", reconcileBy: "hostname", insecure: false, fieldMap: {}, filter: null, mapScript: "", hasSecret: false });
  const openImport = (folderId: string) => {
    setFolderCtx(null);
    api().FolderSourceGet(folderId).then((got) => {
      const dto = got ?? blankDTO();
      setImp({ folderId, dto: { ...dto, fieldMap: dto.fieldMap ?? {} }, secret: "", test: null, testing: false, err: "" });
    }).catch((e) => setErr(String(e)));
  };
  const testImport = () => {
    const s = imp;
    if (!s) return;
    const run = () => {
      setImp((cur) => cur ? { ...cur, testing: true, test: null, err: "" } : cur);
      api().FolderSourceTest(s.folderId, s.dto, s.secret)
        .then((tr) => setImp((cur) => cur ? { ...cur, testing: false, test: tr } : cur))
        .catch((e) => setImp((cur) => cur ? { ...cur, testing: false, err: String(e) } : cur));
    };
    // Relying on a stored secret (editing without re-entering it) needs the
    // cred store unlocked; surface the unlock prompt instead of a raw error.
    if (s.dto.auth !== "none" && s.secret === "" && s.dto.hasSecret) {
      api().CredStatus().then((cs) => {
        if (cs.locked) { setCredErr(""); setCredPrompt({ mode: "unlock", run }); return; }
        run();
      }).catch(() => run());
    } else {
      run();
    }
  };
  const doSaveImport = () => {
    const s = imp;
    if (!s) return;
    const finish = () => api().FolderSourceSet(s.folderId, s.dto, s.secret)
      .then(() => { setImp(null); load(); })
      .catch((e) => setImp((cur) => cur ? { ...cur, err: String(e) } : cur));
    if (s.dto.auth !== "none" && s.secret !== "") {
      api().CredStatus().then((cs) => {
        if (!cs.initialized) { setCredErr(""); setCredPrompt({ mode: "create", run: finish }); return; }
        if (cs.locked) { setCredErr(""); setCredPrompt({ mode: "unlock", run: finish }); return; }
        finish();
      }).catch((e) => setImp((cur) => cur ? { ...cur, err: String(e) } : cur));
    } else {
      finish();
    }
  };
  const refreshFolder = (folderId: string) => {
    setFolderCtx(null);
    const mark = (on: boolean) => setRefreshing((r) => { const n = new Set(r); if (on) n.add(folderId); else n.delete(folderId); return n; });
    mark(true);
    const finish = () => api().FolderSourceRefresh(folderId).then((rr) => {
      mark(false);
      if (rr.error) {
        if (rr.error.toLowerCase().includes("unlock")) { setCredErr(""); setCredPrompt({ mode: "unlock", run: finish }); return; }
        setErr(rr.error); return;
      }
      if (rr.skipped > 0) setErr("import: added " + rr.added + ", skipped " + rr.skipped + " duplicate name(s)");
      load();
    }).catch((e) => { mark(false); setErr(String(e)); });
    finish();
  };
  const clearImport = (folderId: string) => {
    setFolderCtx(null);
    api().FolderSourceClear(folderId).then(() => load()).catch((e) => setErr(String(e)));
  };
  const submitCredPrompt = (pass: string) => {
    const cp = credPrompt;
    if (!cp) return;
    const p = cp.mode === "create" ? api().CredSetPassphrase(pass) : api().CredUnlock(pass);
    p.then(() => { setCredPrompt(null); setCredErr(""); cp.run(); }).catch((e) => setCredErr(String(e)));
  };
  const refreshConns = () => api().ActiveConnections().then(setConns).catch(() => {});
  const refreshPinned = () => api().PinnedSessions().then((p) => setPinned(p ?? [])).catch(() => {});

  useEffect(() => {
    load();
    api().GetVersion().then(setVer).catch(() => {});
    refreshConns();
    refreshPinned();
    api().Themes().then((ts) => setThemeList(ts ?? [])).catch(() => {});
    api().Settings().then((s) => {
      setSettings(s);
      applyOverrides({ zoom: s.zoom, fontUI: s.fontUI, fontMono: s.fontMono, fontUISize: s.fontUISize, fontTermSize: s.fontTermSize });
      return api().Theme(s.theme);
    }).then((t) => applyThemeColors(t)).catch(() => {});

    const offC = window.runtime.EventsOn("f9:conns", () => refreshConns());
    const offP = window.runtime.EventsOn("f9:prompt", (req: PromptRequest) => setPromptQ((qs) => [...qs, req]));
    const offT = window.runtime.EventsOn("f9:termclosed", (ev: { termId: string; died: boolean }) => {
      const termId = ev.termId;
      if (ev.died) {
        // Unexpected disconnect: keep the tab and its scrollback, just mark it
        // disconnected (red bar). The user closes it explicitly when done.
        setDead((d) => { const n = new Set(d); n.add(termId); return n; });
        return;
      }
      setTabs((t) => t.filter((x) => x.termId !== termId));
      setView((v) => (v.kind === "term" && v.id === termId ? { kind: "empty", id: "" } : v));
      setActivity((a) => { if (!a[termId]) return a; const n = { ...a }; delete n[termId]; return n; });
      setMarks((m) => { if (!m.has(termId)) return m; const n = new Set(m); n.delete(termId); return n; });
    });
    const offA = window.runtime.EventsOn("f9:termactivity", (ev: { termId: string; kind: IndKind }) => {
      if (activeTermRef.current === ev.termId) return;
      const cfg = tabCfgRef.current[ev.termId] ?? DEFAULT_CFG(defIndRef.current);
      if (!cfg[ev.kind]) return;
      setActivity((prev) => {
        const cur = prev[ev.termId] ?? {};
        if (cur[ev.kind]) return prev;
        return { ...prev, [ev.termId]: { ...cur, [ev.kind]: true } };
      });
    });
    const offTh = window.runtime.EventsOn("f9:themes", () => {
      api().Themes().then((ts) => setThemeList(ts ?? [])).catch(() => {});
      api().Settings().then((s) => api().Theme(s.theme)).then((t) => applyThemeColors(t)).catch(() => {});
    });
    const offMS = window.runtime.EventsOn("f9:multisend", (r: MSResult) => setMsResults((prev) => ({ ...prev, [r.id]: r })));
    const offMSD = window.runtime.EventsOn("f9:multisenddone", () => setMsRunning(false));
    return () => { offC?.(); offP?.(); offT?.(); offA?.(); offTh?.(); offMS?.(); offMSD?.(); };
  }, []);

  // Suppress the default WebKit context menu everywhere; custom menus (set via
  // onContextMenu handlers) still open because they run before this bubbles.
  useEffect(() => {
    const suppress = (e: MouseEvent) => e.preventDefault();
    window.addEventListener("contextmenu", suppress);
    return () => window.removeEventListener("contextmenu", suppress);
  }, []);

  useEffect(() => {
    api().CheckForUpdate().then((u) => { if (u && u.newer) setUpdateInfo(u); }).catch(() => {});
  }, []);

  const isConnected = (id: string) => conns.some((c) => c.sessionId === id && c.state === "connected");

  useEffect(() => { setSearchRes(null); setSearchStats(null); setPeek({}); }, [view.kind === "term" ? view.id : ""]);
  const openSearch = () => {
    if (view.kind !== "term") return;
    setSearchOpen(true);
    api().TerminalStats(view.id).then(setSearchStats).catch(() => setSearchStats(null));
  };
  const runSearch = () => {
    if (view.kind !== "term" || searchQ.trim() === "") return;
    setSearchBusy(true);
    setPeek({});
    api().GrepTerminal(view.id, searchQ, { invert: searchInv, ignoreCase: searchIC, before: searchCtx, after: searchCtx, maxMatches: 0 })
      .then((r) => { setSearchRes(r); setSearchStats(r.lines); })
      .catch((e) => setErr(String(e)))
      .finally(() => setSearchBusy(false));
  };
  const togglePeek = (i: number, lineNo: number) => {
    if (view.kind !== "term") return;
    if (peek[i]) { setPeek((p) => { const n = { ...p }; delete n[i]; return n; }); return; }
    api().TerminalPeek(view.id, lineNo - 1, 6).then((r) => setPeek((p) => ({ ...p, [i]: r }))).catch((e) => setErr(String(e)));
  };

  useEffect(() => {
    const off = onFindRequested((termId) => {
      if (view.kind === "term" && view.id === termId) openSearch();
    });
    return off;
  }, [view]);

  const activateTerm = (termId: string) => {
    setView({ kind: "term", id: termId });
    setActivity((a) => { if (!a[termId]) return a; const n = { ...a }; delete n[termId]; return n; });
  };
  const isSocksOnly = (sessionId: string) => conns.some((c) => c.sessionId === sessionId && c.socksOnly);
  const openTerminalFor = (sessionId: string, name: string) => {
    const termId = uuid();
    setTabs((t) => [...t, { termId, sessionId, name }]);
    setTabCfg((c) => ({ ...c, [termId]: DEFAULT_CFG(defIndRef.current) }));
    activateTerm(termId);
  };
  const connectAndOpen = (sessionId: string, name: string) => {
    if (isConnected(sessionId)) { if (!isSocksOnly(sessionId)) openTerminalFor(sessionId, name); return; }
    api().ConnectSessions([sessionId]).catch((e) => setErr(String(e)));
    setPendingOpen((p) => (p.some((x) => x.id === sessionId) ? p : [...p, { id: sessionId, name }]));
  };

  useEffect(() => {
    if (pendingOpen.length === 0) return;
    const still: { id: string; name: string }[] = [];
    for (const p of pendingOpen) {
      if (conns.some((c) => c.sessionId === p.id && c.state === "connected")) { if (!isSocksOnly(p.id)) openTerminalFor(p.id, p.name); }
      else if (conns.some((c) => c.sessionId === p.id && c.state === "error")) { /* drop */ }
      else still.push(p);
    }
    if (still.length !== pendingOpen.length) setPendingOpen(still);
  }, [conns]);

  const onQuery = (raw: string) => {
    setQ(raw);
    window.clearTimeout(debounce.current);
    if (raw.trim() === "") { setHits(null); return; }
    debounce.current = window.setTimeout(() => api().Filter(raw).then(setHits).catch((e) => setErr(String(e))), 80);
  };
  const select = (s: SessionNode) => {
    setSel(s); setView({ kind: "details", id: s.id });
    api().SessionDetail(s.id).then(setDetail).catch((e) => setErr(String(e)));
  };
  const toggleMark = (id: string) => setMarked((m) => { const n = { ...m }; if (n[id]) delete n[id]; else n[id] = true; return n; });
  const markedIds = Object.keys(marked);
  const connectMarked = () => { if (markedIds.length) api().ConnectSessions(markedIds).catch((e) => setErr(String(e))); };
  const connectFolder = () => { if (selFolder) api().ConnectFolder(selFolder.id).catch((e) => setErr(String(e))); };
  const cycleSessionTabs = (sessionId: string) => {
    const sessTabs = tabs.filter((t) => t.sessionId === sessionId);
    if (sessTabs.length === 0) {
      const conn = conns.find((c) => c.sessionId === sessionId);
      openTerminalFor(sessionId, conn?.name ?? sessionId);
      return;
    }
    let idx = 0;
    if (view.kind === "term") {
      const cur = sessTabs.findIndex((t) => t.termId === view.id);
      if (cur >= 0) idx = (cur + 1) % sessTabs.length;
    }
    activateTerm(sessTabs[idx].termId);
  };

  const saveSettings = (patch: Partial<UISettings>) => {
    const next = { ...settings, ...patch };
    setSettings(next);
    applyOverrides({ zoom: next.zoom, fontUI: next.fontUI, fontMono: next.fontMono, fontUISize: next.fontUISize, fontTermSize: next.fontTermSize });
    api().SaveSettings(next).catch((e) => setErr(String(e)));
  };
  const changeTheme = (name: string) => {
    saveSettings({ theme: name });
    api().Theme(name).then((t) => applyThemeColors(t)).catch((e) => setErr(String(e)));
  };
  const importIterm = () => {
    api().ImportITermTheme().then((name) => {
      if (name) { api().Themes().then((ts) => setThemeList(ts ?? [])).catch(() => {}); changeTheme(name); }
    }).catch((e) => setErr(String(e)));
  };

  const togglePin = (sessionId: string, currentlyPinned: boolean) => {
    const p = currentlyPinned ? api().UnpinSession(sessionId) : api().PinSession(sessionId);
    p.then(() => { refreshPinned(); load(); if (sel && sel.id === sessionId) api().SessionDetail(sessionId).then(setDetail).catch(() => {}); }).catch((e) => setErr(String(e)));
  };
  const closeTab = (termId: string) => {
    api().CloseTerminal(termId).catch(() => {});
    setDead((d) => { if (!d.has(termId)) return d; const n = new Set(d); n.delete(termId); return n; });
    setTabs((t) => t.filter((x) => x.termId !== termId));
    setView((v) => (v.kind === "term" && v.id === termId ? { kind: "empty", id: "" } : v));
    setActivity((a) => { if (!a[termId]) return a; const n = { ...a }; delete n[termId]; return n; });
    setTabCfg((c) => { const n = { ...c }; delete n[termId]; return n; });
  };
  const setCfg = (termId: string, patch: Partial<TabCfg>) => {
    setTabCfg((c) => {
      const cur = c[termId] ?? DEFAULT_CFG(defIndRef.current);
      const next = { ...cur, ...patch };
      if ("watch" in patch) api().SetTerminalWatch(termId, next.watch).catch((e) => setErr(String(e)));
      return { ...c, [termId]: next };
    });
  };

  const afterMutation = () => {
    setModal(""); load(); refreshPinned();
    if (sel) api().SessionDetail(sel.id).then(setDetail).catch(() => { setSel(null); setDetail(null); });
    if (q.trim() !== "") api().Filter(q).then(setHits).catch(() => {});
  };
  const deleteSelected = () => {
    if (!sel || !confirm(`delete session ${sel.name}?`)) return;
    api().DeleteSession(sel.id).then(() => { setSel(null); setDetail(null); afterMutation(); }).catch((e) => setErr(String(e)));
  };
  const duplicateSelected = () => {
    if (!detail) return;
    api().SessionDuplicate(detail.id).then((newId) => {
      load();
      api().SessionDetail(newId).then((d) => {
        setDetail(d);
        setSel({ id: d.id, name: d.name, host: d.host, port: d.port, user: d.user, proto: d.proto, detectedOs: "", osPinned: false, pinned: false, generated: false });
        setView({ kind: "details", id: newId });
      }).catch(() => {});
    }).catch((e) => setErr(String(e)));
  };
  const resolvePrompt = (r: PromptReply) => {
    const cur = promptQ[0];
    if (cur) api().ResolvePrompt(cur.id, r);
    setPromptQ((qs) => qs.slice(1));
  };

  const pinnedIds = new Set(pinned.map((p) => p.id));
  const displayTabs: ({ type: "term"; tab: Tab; pinned: boolean } | { type: "pin"; sessionId: string; name: string })[] = [];
  for (const p of pinned) {
    const openForP = tabs.filter((t) => t.sessionId === p.id);
    if (openForP.length) openForP.forEach((t) => displayTabs.push({ type: "term", tab: t, pinned: true }));
    else displayTabs.push({ type: "pin", sessionId: p.id, name: p.name });
  }
  for (const t of tabs) if (!pinnedIds.has(t.sessionId)) displayTabs.push({ type: "term", tab: t, pinned: false });

  const activeTab = view.kind === "term" ? tabs.find((t) => t.termId === view.id) : undefined;

  const sendResolved = (p: { body: string; delay: number; bracketed: boolean }, extra: Record<string, string>) => {
    if (!activeTab) return;
    api().SendTemplate(activeTab.termId, p.body, extra, p.delay, p.bracketed)
      .then(() => { setUnresolved(null); setPendingSend(null); })
      .catch((e) => setErr(String(e)));
  };
  const runSend = (body: string, delay: number, bracketed: boolean) => {
    if (view.kind !== "term" || !activeTab || body.trim() === "") return;
    const sid = activeTab.sessionId;
    const p = { body, delay, bracketed };
    setPendingSend(p);
    api().TemplateUnresolved(sid, body).then((names) => {
      const memo = sendMemo[sid] ?? {};
      const need = (names ?? []).filter((n) => !(n in memo));
      if (need.length === 0) sendResolved(p, memo);
      else setUnresolved(need);
    }).catch((e) => setErr(String(e)));
  };
  const doSend = () => runSend(sendBody, sendDelay, sendBracketed);
  const submitUnresolved = (values: Record<string, string>, remember: boolean) => {
    if (!activeTab || !pendingSend) return;
    const sid = activeTab.sessionId;
    const memo = sendMemo[sid] ?? {};
    if (remember) setSendMemo({ ...sendMemo, [sid]: { ...memo, ...values } });
    sendResolved(pendingSend, { ...memo, ...values });
  };

  const runSnippetObj = (s: Snippet) => {
    if (view.kind === "term" && activeTab) runSend(s.body, s.delayMs ?? 0, !!s.bracketed);
  };
  const runSnippetById = (id: string) => {
    if (!id) return;
    api().SnippetGet(id).then((sn) => { if (sn) runSnippetObj(sn); else setErr("snippet not found"); }).catch((e) => setErr(String(e)));
  };
  const openPicker = () => { refreshSnips(); setPickerOpen(true); };
  const toggleMsMark = (termId: string) => setMarks((m) => { const n = new Set(m); n.has(termId) ? n.delete(termId) : n.add(termId); return n; });
  const markAll = () => setMarks(new Set(tabs.map((t) => t.termId)));
  const clearMarks = () => setMarks(new Set());
  const markedTargets = () => tabs.filter((t) => marks.has(t.termId)).map((t) => ({ termId: t.termId, name: t.name }));
  const doDryRun = () => {
    const ids = markedTargets().map((t) => t.termId);
    if (ids.length === 0) return;
    api().MultiSendPreview(ids, msLine).then((p) => setMsPreview(p ?? [])).catch((e) => setErr(String(e)));
  };
  const reallyMultiSend = (ids: string[]) => {
    setMsConfirm(null); setMsResults({}); setMsRunning(true);
    api().MultiSendStart(ids, msLine, {}, msSeq, msTimeout).catch((e) => { setErr(String(e)); setMsRunning(false); });
  };
  const doMultiSend = () => {
    const ids = markedTargets().map((t) => t.termId);
    if (ids.length === 0 || msLine.trim() === "") return;
    api().MultiSendPreview(ids, msLine).then((preview) => {
      const p = preview ?? [];
      setMsPreview(p);
      const fams = Array.from(new Set(p.map((x) => x.osFamily).filter(Boolean)));
      const reasons: string[] = [];
      if (ids.length > MS_CONFIRM_THRESHOLD) reasons.push(ids.length + " targets");
      if (fams.length > 1) reasons.push("mixed OS families: " + fams.join(", "));
      if (reasons.length > 0) setMsConfirm(reasons.join("; "));
      else reallyMultiSend(ids);
    }).catch((e) => setErr(String(e)));
  };
  const confirmMultiSend = () => reallyMultiSend(markedTargets().map((t) => t.termId));
  const jumpToTerm = (termId: string) => { setMsOpen(false); activateTerm(termId); };
  const doMultiCancel = () => { api().MultiSendCancel().catch(() => {}); setMsRunning(false); };
  useEffect(() => { setPickerEnabled(settings.showSnippets); }, [settings.showSnippets]);
  useEffect(() => {
    const off = onPickerRequested((termId) => { if (activeTermRef.current === termId) openPicker(); });
    return off;
  }, []);

  const runAction = (a: BarAction) => {
    switch (a.kind) {
      case "send": runSend(a.text ?? "", a.delayMs ?? 0, a.bracketed ?? false); break;
      case "launch": api().LaunchApp(a.args ?? []).catch((e) => setErr(String(e))); break;
      case "url": api().OpenURL(a.text ?? "").catch((e) => setErr(String(e))); break;
      case "internal":
        if (a.text === "grep-overlay") setSearchOpen(true);
        else if (a.text === "send-composer") setSendOpen(true);
        else setErr("unknown internal command: " + (a.text ?? ""));
        break;
      case "snippet": runSnippetById(a.snippetId ?? ""); break;
      default: setErr("unknown action kind: " + a.kind);
    }
  };

  const refreshBar = () => {
    const sid = view.kind === "term" && activeTab ? activeTab.sessionId : "";
    api().GlobalBar(sid).then(setGbar).catch(() => setGbar(null));
    if (sid) api().BarForSession(sid).then(setBar).catch(() => setBar(null));
    else setBar(null);
  };
  useEffect(() => {
    const sid = view.kind === "term" && activeTab ? activeTab.sessionId : "";
    api().GlobalBar(sid).then(setGbar).catch(() => setGbar(null));
    if (sid) api().BarForSession(sid).then(setBar).catch(() => setBar(null));
    else setBar(null);
  }, [view.kind === "term" && activeTab ? activeTab.sessionId : ""]);

  const loadBarYaml = (fid: string) => {
    api().BarExport(fid).then((y) => { setBarYaml(y); setBarHasOwn(true); })
      .catch(() => { setBarYaml(BAR_TEMPLATE); setBarHasOwn(false); });
  };
  const openGlobalBar = () => {
    setBarErr("");
    setBarScope("global");
    setBarFolderId("");
    loadBarYaml("");
    setBarEditor(true);
  };
  const openFolderBar = () => {
    if (!activeTab) return;
    setBarErr("");
    api().SessionDetail(activeTab.sessionId).then((d) => {
      setBarFolderId(d.folderId);
      setBarScope("folder");
      loadBarYaml(d.folderId);
      setBarEditor(true);
    }).catch((e) => setErr(String(e)));
  };
  const barScopeId = () => (barScope === "global" ? "" : barFolderId);
  const saveBar = () => {
    api().BarImport(barScopeId(), barYaml).then(() => { setBarEditor(false); setBarErr(""); refreshBar(); })
      .catch((e) => setBarErr(String(e)));
  };
  const deleteBar = () => {
    api().BarDelete(barScopeId()).then(() => { setBarEditor(false); refreshBar(); })
      .catch((e) => setBarErr(String(e)));
  };

  const refreshSnips = () => {
    api().SnippetFolders().then((f) => setSnFolders(f ?? [])).catch(() => setSnFolders([]));
    api().SnippetList().then((l) => setSnList(l ?? [])).catch(() => setSnList([]));
  };
  const openSnippetLib = () => { setSnErr(""); refreshSnips(); setSnLib(true); };
  const saveSnFolder = (name: string) => {
    api().SnippetSaveFolder({ id: "", name }).then(() => { setSnErr(""); refreshSnips(); }).catch((e) => setSnErr(String(e)));
  };
  const deleteSnFolder = (id: string) => {
    api().SnippetDeleteFolder(id).then(() => { setSnErr(""); refreshSnips(); }).catch((e) => setSnErr(String(e)));
  };
  const saveSnippet = (s: Snippet): Promise<Snippet | null> => {
    return api().SnippetSave(s).then((saved) => { setSnErr(""); refreshSnips(); return saved; }).catch((e) => { setSnErr(String(e)); return null; });
  };
  const deleteSnippet = (id: string) => {
    api().SnippetDelete(id).then(() => { setSnErr(""); refreshSnips(); }).catch((e) => setSnErr(String(e)));
  };
  const selPinned = !!sel && pinned.some((p) => p.id === sel.id);
  const ctxCfg = ctxMenu ? (tabCfg[ctxMenu.termId] ?? DEFAULT_CFG(defInd)) : null;

  return (
    <div class="approot">
      <div class="titlebar" style={{ "--wails-draggable": "drag" } as any}
        onDblClick={() => window.runtime.WindowToggleMaximise?.()}>
        <span class="tb-brand"><Logo /></span>
        <div class="tb-controls" style={{ "--wails-draggable": "no-drag" } as any}>
          <button class="tb-btn" title="minimise" onClick={() => window.runtime.WindowMinimise?.()}>{"\u2013"}</button>
          <button class="tb-btn tb-close" title="close" onClick={() => window.runtime.Quit?.()}>{"\u2715"}</button>
        </div>
      </div>
      <div class="layout" onClick={() => { if (ctxMenu) setCtxMenu(null); if (folderCtx) setFolderCtx(null); }}>
      <div class="sidebar">
        <div class="toolbar">
          <button onClick={() => setModal("session-new")} disabled={!selFolder}>+ session</button>
          <button onClick={() => setModal("folder")} disabled={!selFolder}>+ folder</button>
        </div>
        <div class="toolbar">
          <button onClick={connectMarked} disabled={markedIds.length === 0}>connect marked ({markedIds.length})</button>
          <button onClick={connectFolder} disabled={!selFolder}>connect folder</button>
        </div>
        <div class="filterbar">
          <input type="text" placeholder="filter sessions..." value={q} onInput={(e) => onQuery((e.target as HTMLInputElement).value)} />
          <button title="reload store" onClick={load}>&#x21bb;</button>
          {refreshing.size > 0 && <span class="refresh-spin" title="refreshing import source\u2026">&#x21bb;</span>}
        </div>
        <div class="tree">
          {err && <div class="error" onClick={() => setErr("")}>{err}</div>}
          {hits !== null ? (
            hits.length === 0 ? <div class="nohits">no matches</div> :
              hits.map((h) => (
                <SessionRow key={h.id} s={h} pathPrefix={h.path + "/"} indent={10} selected={h.id === sel?.id}
                  marked={!!marked[h.id]} onSelect={() => select(h)} onToggleMark={() => toggleMark(h.id)} />
              ))
          ) : (
            tree && <Folder node={tree} depth={0} selected={sel?.id ?? ""} selectedFolder={selFolder?.id ?? ""} marked={marked}
              onSelect={select} onSelectFolder={(f) => setSelFolder({ id: f.id, path: f.path })} onToggleMark={toggleMark} onFolderCtx={openFolderCtx} />
          )}
        </div>
        {conns.length > 0 && (
          <div class="connpanel">
            <div class="connhead"><span>connections ({conns.length})</span><button onClick={() => api().DisconnectAll()}>disconnect all</button></div>
            {conns.map((c) => {
              const tc = tabs.filter((t) => t.sessionId === c.sessionId).length;
              return (
                <div class="connrow" key={c.sessionId} title={tc > 1 ? `${tc} terminals — click to cycle` : c.err}
                  onClick={() => cycleSessionTabs(c.sessionId)}>
                  <span class={"dot " + c.state} /><span class="cname">{c.name}</span>
                  {c.socksPort > 0 && <span class={"socksbadge " + (c.socksActive ? "on" : "off")} title={c.socksActive ? ("SOCKS proxy on 127.0.0.1:" + c.socksPort) : ("SOCKS :" + c.socksPort + " failed to bind (port in use?)")}>{"SOCKS:" + c.socksPort}</span>}
                  {tc > 1 && <span class="conncount">{"\u00d7" + tc}</span>}
                  <span class="cstate">{STATE_LABEL[c.state] ?? c.state}</span>
                  <button class="cx" title="disconnect" onClick={(e) => { e.stopPropagation(); api().Disconnect(c.sessionId); }}>&#x2715;</button>
                </div>
              );
            })}
          </div>
        )}
        <div class="statusbar">
          <span>f9 {ver}</span>
          {settings.showSnippets && <span class="gear" title="snippet library" onClick={openSnippetLib}>{"\u2261"}</span>}
          {settings.showMultiSend && <span class={"gear" + (msRunning ? " busy" : "")} title="multi-send" onClick={() => setMsOpen(true)}>{"\u21c9"}</span>}
          <span class="gear" title="settings" onClick={() => setSettingsModal(true)}>{"\u2699"}</span>
        </div>
      </div>

      <div class="mainpane">
        {displayTabs.length > 0 && (
          <div class="tabstrip">
            {displayTabs.map((d) => d.type === "term" ? (
              <div key={d.tab.termId} class={"tab" + (view.kind === "term" && view.id === d.tab.termId ? " active" : "") + (dead.has(d.tab.termId) ? " down" : "")}
                onClick={() => activateTerm(d.tab.termId)}
                onContextMenu={(e) => { e.preventDefault(); setCtxMenu({ termId: d.tab.termId, x: e.clientX, y: e.clientY }); }}>
                <span class={"tabconn " + (dead.has(d.tab.termId) ? "down" : "up")} title={dead.has(d.tab.termId) ? "disconnected" : "connected"} />
                {(() => { const dc = dotClass(activity[d.tab.termId]); return dc ? <span class={"actdot " + dc} /> : null; })()}
                {settings.showMultiSend && (
                  <span class={"tabmark" + (marks.has(d.tab.termId) ? " on" : "")} title="mark for multi-send"
                    onClick={(e) => { e.stopPropagation(); toggleMsMark(d.tab.termId); }}>{marks.has(d.tab.termId) ? "\u2611" : "\u2610"}</span>
                )}
                <span class={"pin" + (d.pinned ? " filled" : "")} title={d.pinned ? "unpin" : "pin"}
                  onClick={(e) => { e.stopPropagation(); togglePin(d.tab.sessionId, d.pinned); }}>{d.pinned ? "\u2605" : "\u2606"}</span>
                <span class="tabname">{d.tab.name}</span>
                <span class="tabx" title="close" onClick={(e) => { e.stopPropagation(); closeTab(d.tab.termId); }}>{"\u2715"}</span>
              </div>
            ) : (
              <div key={"pin:" + d.sessionId} class="tab pinned-empty" onClick={() => connectAndOpen(d.sessionId, d.name)}>
                <span class="pin filled" title="unpin" onClick={(e) => { e.stopPropagation(); togglePin(d.sessionId, true); }}>{"\u2605"}</span>
                <span class="tabname">{d.name}</span>
              </div>
            ))}
            {activeTab && (
              <span class="tabnew" title="new terminal for this session" onClick={() => openTerminalFor(activeTab.sessionId, activeTab.name)}>+</span>
            )}
            {activeTab && settings.showTemplates && (
              <span class="tabsend" title="send template" onClick={() => setSendOpen(true)}>{"\u27a4"}</span>
            )}

          </div>
        )}
        <div class="paneview">
          {tabs.map((t) => (
            <TerminalView key={t.termId} termId={t.termId} sessionId={t.sessionId} active={view.kind === "term" && view.id === t.termId} />
          ))}
          {searchOpen && view.kind === "term" && (
            <SearchPanel
              stats={searchStats} res={searchRes} busy={searchBusy} peek={peek}
              q={searchQ} ic={searchIC} inv={searchInv} ctx={searchCtx}
              onQ={setSearchQ} onIC={setSearchIC} onInv={setSearchInv} onCtx={setSearchCtx}
              onRun={runSearch} onClose={() => setSearchOpen(false)} onRow={togglePeek}
            />
          )}
          {sendOpen && view.kind === "term" && settings.showTemplates && (
            <SendPanel body={sendBody} delay={sendDelay} bracketed={sendBracketed}
              onBody={setSendBody} onDelay={setSendDelay} onBracketed={setSendBracketed}
              onSend={doSend} onClose={() => setSendOpen(false)} />
          )}
          {view.kind === "term" ? null : sel && view.kind === "details" && detail ? (
            <div class="details">
              <h1>{detail.name}{selPinned && <span class="pinbadge big">{"\u2605"}</span>}{sel.generated && <span class="genbadge">import source</span>}</h1>
              <table>
                <tr><td>folder</td><td>{detail.folderPath}</td></tr>
                <tr><td>host</td><td>{detail.host}{detail.port && detail.port !== 22 ? ":" + detail.port : ""}</td></tr>
                <tr><td>user</td><td>{detail.user || "\u2014"}</td></tr>
                <tr><td>proto</td><td>{detail.proto}</td></tr>
                <tr><td>os</td><td>{sel.detectedOs ? sel.detectedOs + (sel.osPinned ? " (pinned)" : "") : "not detected yet"}</td></tr>
              </table>
              <div class="opthead">options</div>
              <table>
                {OPTION_KEYS.map(({ key, label }) => {
                  const f = detail.options[key];
                  if (!f || f.effective === "") return null;
                  return <tr key={key}><td>{label}</td><td>{f.effective} <span class="badge">{f.source}</span></td></tr>;
                })}
                {detail.jumpChain && detail.jumpChain.length > 0 && (
                  <tr><td>jump chain</td><td>
                    {detail.jumpChain.map((j) => `${j.user ? j.user + "@" : ""}${j.host} [${j.mode}]`).join(" \u2192 ")}{" "}
                    <span class="badge">{detail.jumpSource}</span>
                  </td></tr>
                )}
              </table>
              <div class="detail-actions">
                <button onClick={() => connectAndOpen(detail.id, detail.name)}>{isConnected(detail.id) ? "open terminal" : "connect"}</button>
                {isConnected(detail.id) && <button onClick={() => openTerminalFor(detail.id, detail.name)}>new terminal</button>}
                <button onClick={() => togglePin(detail.id, selPinned)}>{selPinned ? "unpin" : "pin"}</button>
                <button onClick={() => setModal("session-edit")}>edit</button>
                <button onClick={() => setJumpEdit({ sessionId: detail.id, initial: detail.jumpChain ?? [] })}>jump chain</button>
                <button onClick={duplicateSelected}>duplicate</button>
                {sel.generated
                  ? <span class="gen-note" title="managed by the folder's import source">local edits revert on next refresh</span>
                  : <button class="danger" onClick={deleteSelected}>delete</button>}
              </div>
            </div>
          ) : <div class="empty">select a session</div>}
        </div>
        {!settings.barVertical && (
          <BarStrip global={gbar} folder={bar}
            showGlobal={settings.showGlobalBar} showFolder={settings.showFolderBar}
            folderActive={view.kind === "term" && !!activeTab}
            onAction={runAction} onEditGlobal={openGlobalBar} onEditFolder={openFolderBar} />
        )}
      </div>
      {settings.barVertical && (
        <BarRail global={gbar} folder={bar}
          showGlobal={settings.showGlobalBar} showFolder={settings.showFolderBar}
          folderActive={view.kind === "term" && !!activeTab} pinned={!settings.barUnpinned}
          onAction={runAction} onEditGlobal={openGlobalBar} onEditFolder={openFolderBar} />
      )}

      {ctxMenu && ctxCfg && (
        <div class="ctxmenu" style={{ left: `${ctxMenu.x}px`, top: `${ctxMenu.y}px` }} onClick={(e) => e.stopPropagation()}>
          <div class="mhead">tab indicators</div>
          <label class="mrow"><input type="checkbox" checked={ctxCfg.output} onChange={(e) => setCfg(ctxMenu.termId, { output: (e.target as HTMLInputElement).checked })} /> <span class="swatch output" /> output</label>
          <label class="mrow"><input type="checkbox" checked={ctxCfg.prompt} onChange={(e) => setCfg(ctxMenu.termId, { prompt: (e.target as HTMLInputElement).checked })} /> <span class="swatch prompt" /> command done</label>
          <label class="mrow"><input type="checkbox" checked={ctxCfg.match} onChange={(e) => setCfg(ctxMenu.termId, { match: (e.target as HTMLInputElement).checked })} /> <span class="swatch match" /> regex match</label>
          <div class="mhead">watch regex</div>
          <div class="mrow">
            <input type="text" placeholder="e.g. ERROR|down" value={ctxCfg.watch}
              onInput={(e) => setCfg(ctxMenu.termId, { watch: (e.target as HTMLInputElement).value })} />
          </div>
        </div>
      )}
      {folderCtx && (
        <FolderCtxMenu x={folderCtx.x} y={folderCtx.y} hasSource={folderCtx.node.hasSource}
          onImport={() => openImport(folderCtx.node.id)}
          onRefresh={() => refreshFolder(folderCtx.node.id)}
          onClear={() => clearImport(folderCtx.node.id)} />
      )}
      {imp && (
        <ImportSourceModal st={imp}
          onChange={(patch) => setImp((c) => c ? { ...c, ...patch, test: null } : c)}
          onDTO={(patch) => setImp((c) => c ? { ...c, dto: { ...c.dto, ...patch }, test: null } : c)}
          onTest={testImport} onSave={doSaveImport} onClose={() => setImp(null)}
          onEditJump={() => api().FolderJumpChain(imp.folderId).then((h) => setFolderJump({ folderId: imp.folderId, initial: h ?? [] })).catch(() => setFolderJump({ folderId: imp.folderId, initial: [] }))} />
      )}
      {credPrompt && (
        <CredPromptModal mode={credPrompt.mode} err={credErr}
          onSubmit={submitCredPrompt} onCancel={() => { setCredPrompt(null); setCredErr(""); }} />
      )}

      {modal === "session-new" && selFolder && <SessionModal folder={selFolder} detail={null} onClose={() => setModal("")} onSaved={afterMutation} />}
      {modal === "session-edit" && selFolder && detail && <SessionModal folder={selFolder} detail={detail} onClose={() => setModal("")} onSaved={afterMutation} />}
      {jumpEdit && <JumpChainModal initial={jumpEdit.initial} onSave={(hops) => api().SessionSetJumpChain(jumpEdit.sessionId, hops)} onClose={() => setJumpEdit(null)} onSaved={() => { const id = jumpEdit.sessionId; api().SessionDetail(id).then(setDetail).catch(() => {}); }} />}
      {folderJump && <JumpChainModal initial={folderJump.initial} onSave={(hops) => api().FolderSetJumpChain(folderJump.folderId, hops)} onClose={() => setFolderJump(null)} onSaved={() => {}} />}
      {modal === "folder" && selFolder && <FolderModal parent={selFolder} onClose={() => setModal("")} onSaved={afterMutation} />}
      {settingsModal && (
        <SettingsModal settings={settings} themeList={themeList} defInd={defInd}
          onChangeTheme={changeTheme} onImport={importIterm} onSave={saveSettings} onDefInd={setDefInd}
          onClose={() => setSettingsModal(false)} />
      )}
      {promptQ.length > 0 && <PromptModal req={promptQ[0]} onResolve={resolvePrompt} />}
      {unresolved && view.kind === "term" && activeTab && (
        <UnresolvedModal names={unresolved} onSubmit={submitUnresolved} onCancel={() => setUnresolved(null)} />
      )}
      {barEditor && (
        <BarEditorModal scope={barScope} yaml={barYaml} err={barErr} canDelete={barHasOwn}
          onYaml={setBarYaml} onSave={saveBar} onDelete={deleteBar}
          onClose={() => { setBarEditor(false); setBarErr(""); }} />
      )}
      {snLib && (
        <SnippetLibraryModal folders={snFolders} snippets={snList} err={snErr}
          onSaveFolder={saveSnFolder} onDeleteFolder={deleteSnFolder}
          onSaveSnippet={saveSnippet} onDeleteSnippet={deleteSnippet}
          onClose={() => setSnLib(false)} />
      )}
      {pickerOpen && settings.showSnippets && view.kind === "term" && activeTab && (
        <SnippetPicker snippets={snList}
          folderName={(id) => snFolders.find((f) => f.id === id)?.name ?? "(no folder)"}
          onRun={(s) => { setPickerOpen(false); runSnippetObj(s); }}
          onClose={() => setPickerOpen(false)}
          onEdit={() => { setPickerOpen(false); openSnippetLib(); }} />
      )}
      {msOpen && (
        <MultiSendModal targets={markedTargets()} line={msLine} seq={msSeq} timeout={msTimeout}
          preview={msPreview} results={msResults} running={msRunning}
          onLine={setMsLine} onSeq={setMsSeq} onTimeout={setMsTimeout}
          onMarkAll={markAll} onClear={clearMarks} onUnmark={toggleMsMark}
          onDryRun={doDryRun} onSend={doMultiSend} onCancel={doMultiCancel} onClose={() => setMsOpen(false)}
          confirm={msConfirm} onConfirmSend={confirmMultiSend} onConfirmCancel={() => setMsConfirm(null)} onJump={jumpToTerm} />
      )}
      {updateInfo && (
        <div class="update-toast">
          <span class="update-msg">f9 {updateInfo.latest} is available</span>
          <button class="primary" onClick={() => api().OpenURL(updateInfo.url)}>download</button>
          <button onClick={() => setUpdateInfo(null)}>dismiss</button>
        </div>
      )}
      </div>
    </div>
  );
}
