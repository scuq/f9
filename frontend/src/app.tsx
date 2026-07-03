import { useEffect, useRef, useState } from "preact/hooks";
import { TerminalView } from "./terminal";

const api = () => window.go.app.App;

const OPTION_KEYS: { key: string; label: string; hint: string }[] = [
  { key: "termType", label: "term type", hint: "xterm-256color" },
  { key: "keepaliveInterval", label: "keepalive", hint: "30s" },
  { key: "reconnect", label: "reconnect", hint: "off | prompt | auto" },
  { key: "theme", label: "theme", hint: "oled-black" },
  { key: "scrollbackLines", label: "scrollback lines", hint: "5000000" },
  { key: "auditScope", label: "audit scope", hint: "off | events | events+input | full-io" },
];

function uuid(): string {
  const c = (globalThis as any).crypto;
  return c?.randomUUID ? c.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

type Tab = { termId: string; sessionId: string; name: string };
type View = { kind: "empty" | "details" | "term"; id: string };
type IndKind = "output" | "prompt" | "match";
type IndFlags = { output?: boolean; prompt?: boolean; match?: boolean };
type TabCfg = { output: boolean; prompt: boolean; match: boolean; watch: string };

const DEFAULT_CFG = (d: { output: boolean; prompt: boolean; match: boolean }): TabCfg =>
  ({ output: d.output, prompt: d.prompt, match: d.match, watch: "" });

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
}) {
  const { node, depth, selected, selectedFolder, marked, onSelect, onSelectFolder, onToggleMark } = props;
  const [open, setOpen] = useState(depth < 2);
  return (
    <div>
      <div class={"row folder" + (node.id === selectedFolder ? " selected" : "")} style={{ paddingLeft: `${depth * 14}px` }}
        onClick={() => onSelectFolder(node)}>
        <span class="twist" onClick={(e) => { e.stopPropagation(); setOpen(!open); }}>{open ? "\u25be" : "\u25b8"}</span>
        <span class="fname">{node.name}</span>
      </div>
      {open && (node.sessions ?? []).map((s) => (
        <SessionRow key={s.id} s={s} indent={(depth + 1) * 14 + 8} selected={s.id === selected} marked={!!marked[s.id]}
          onSelect={() => onSelect(s)} onToggleMark={() => onToggleMark(s.id)} />
      ))}
      {open && (node.folders ?? []).map((c) => (
        <Folder key={c.id} node={c} depth={depth + 1} selected={selected} selectedFolder={selectedFolder}
          marked={marked} onSelect={onSelect} onSelectFolder={onSelectFolder} onToggleMark={onToggleMark} />
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

const STATE_LABEL: Record<string, string> = { dialing: "dialing…", connected: "connected", error: "error" };

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
  const [view, setView] = useState<View>({ kind: "empty", id: "" });
  const [pinned, setPinned] = useState<SessionNode[]>([]);
  const [pendingOpen, setPendingOpen] = useState<{ id: string; name: string }[]>([]);
  const [activity, setActivity] = useState<Record<string, IndFlags>>({});
  const [tabCfg, setTabCfg] = useState<Record<string, TabCfg>>({});
  const [defInd, setDefInd] = useState({ output: true, prompt: true, match: true });
  const [ctxMenu, setCtxMenu] = useState<{ termId: string; x: number; y: number } | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const debounce = useRef<number | undefined>(undefined);

  // refs so the event handler (registered once) reads current state
  const activeTermRef = useRef<string | null>(null);
  const tabCfgRef = useRef(tabCfg);
  const defIndRef = useRef(defInd);
  useEffect(() => { activeTermRef.current = view.kind === "term" ? view.id : null; }, [view]);
  useEffect(() => { tabCfgRef.current = tabCfg; }, [tabCfg]);
  useEffect(() => { defIndRef.current = defInd; }, [defInd]);

  const load = () => api().Tree().then((t) => { setTree(t); if (!selFolder) setSelFolder({ id: t.id, path: t.path }); }).catch((e) => setErr(String(e)));
  const refreshConns = () => api().ActiveConnections().then(setConns).catch(() => {});
  const refreshPinned = () => api().PinnedSessions().then((p) => setPinned(p ?? [])).catch(() => {});

  useEffect(() => {
    load();
    api().GetVersion().then(setVer).catch(() => {});
    refreshConns();
    refreshPinned();
    const offC = window.runtime.EventsOn("f9:conns", () => refreshConns());
    const offP = window.runtime.EventsOn("f9:prompt", (req: PromptRequest) => setPromptQ((qs) => [...qs, req]));
    const offT = window.runtime.EventsOn("f9:termclosed", (termId: string) => {
      setTabs((t) => t.filter((x) => x.termId !== termId));
      setView((v) => (v.kind === "term" && v.id === termId ? { kind: "empty", id: "" } : v));
      setActivity((a) => { if (!a[termId]) return a; const n = { ...a }; delete n[termId]; return n; });
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
    return () => { offC?.(); offP?.(); offT?.(); offA?.(); };
  }, []);

  const isConnected = (id: string) => conns.some((c) => c.sessionId === id && c.state === "connected");

  const activateTerm = (termId: string) => {
    setView({ kind: "term", id: termId });
    setActivity((a) => { if (!a[termId]) return a; const n = { ...a }; delete n[termId]; return n; });
  };
  const openTerminalFor = (sessionId: string, name: string) => {
    const termId = uuid();
    setTabs((t) => [...t, { termId, sessionId, name }]);
    setTabCfg((c) => ({ ...c, [termId]: DEFAULT_CFG(defIndRef.current) }));
    activateTerm(termId);
  };
  const connectAndOpen = (sessionId: string, name: string) => {
    if (isConnected(sessionId)) { openTerminalFor(sessionId, name); return; }
    api().ConnectSessions([sessionId]).catch((e) => setErr(String(e)));
    setPendingOpen((p) => (p.some((x) => x.id === sessionId) ? p : [...p, { id: sessionId, name }]));
  };

  useEffect(() => {
    if (pendingOpen.length === 0) return;
    const still: { id: string; name: string }[] = [];
    for (const p of pendingOpen) {
      if (conns.some((c) => c.sessionId === p.id && c.state === "connected")) openTerminalFor(p.id, p.name);
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

  const togglePin = (sessionId: string, currentlyPinned: boolean) => {
    const p = currentlyPinned ? api().UnpinSession(sessionId) : api().PinSession(sessionId);
    p.then(() => { refreshPinned(); load(); if (sel && sel.id === sessionId) api().SessionDetail(sessionId).then(setDetail).catch(() => {}); }).catch((e) => setErr(String(e)));
  };
  const closeTab = (termId: string) => {
    api().CloseTerminal(termId).catch(() => {});
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
  const selPinned = !!sel && pinned.some((p) => p.id === sel.id);
  const ctxCfg = ctxMenu ? (tabCfg[ctxMenu.termId] ?? DEFAULT_CFG(defInd)) : null;

  return (
    <div class="layout" onClick={() => { if (ctxMenu) setCtxMenu(null); if (settingsOpen) setSettingsOpen(false); }}>
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
              onSelect={select} onSelectFolder={(f) => setSelFolder({ id: f.id, path: f.path })} onToggleMark={toggleMark} />
          )}
        </div>
        {conns.length > 0 && (
          <div class="connpanel">
            <div class="connhead"><span>connections ({conns.length})</span><button onClick={() => api().DisconnectAll()}>disconnect all</button></div>
            {conns.map((c) => (
              <div class="connrow" key={c.sessionId} title={c.err}>
                <span class={"dot " + c.state} /><span class="cname">{c.name}</span>
                <span class="cstate">{STATE_LABEL[c.state] ?? c.state}</span>
                <button class="cx" onClick={() => api().Disconnect(c.sessionId)}>&#x2715;</button>
              </div>
            ))}
          </div>
        )}
        <div class="statusbar">f9 {ver}</div>
      </div>

      <div class="mainpane">
        {displayTabs.length > 0 && (
          <div class="tabstrip">
            {displayTabs.map((d) => d.type === "term" ? (
              <div key={d.tab.termId} class={"tab" + (view.kind === "term" && view.id === d.tab.termId ? " active" : "")}
                onClick={() => activateTerm(d.tab.termId)}
                onContextMenu={(e) => { e.preventDefault(); setCtxMenu({ termId: d.tab.termId, x: e.clientX, y: e.clientY }); }}>
                {(() => { const dc = dotClass(activity[d.tab.termId]); return dc ? <span class={"actdot " + dc} /> : null; })()}
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
            <span class="gear" title="indicator defaults" onClick={(e) => { e.stopPropagation(); setSettingsOpen((s) => !s); }}>{"\u2699"}</span>
            {settingsOpen && (
              <div class="settings-pop" onClick={(e) => e.stopPropagation()}>
                <div class="mhead">default indicators (new tabs)</div>
                <label class="mrow"><input type="checkbox" checked={defInd.output} onChange={(e) => setDefInd({ ...defInd, output: (e.target as HTMLInputElement).checked })} /> <span class="swatch output" /> output</label>
                <label class="mrow"><input type="checkbox" checked={defInd.prompt} onChange={(e) => setDefInd({ ...defInd, prompt: (e.target as HTMLInputElement).checked })} /> <span class="swatch prompt" /> command done</label>
                <label class="mrow"><input type="checkbox" checked={defInd.match} onChange={(e) => setDefInd({ ...defInd, match: (e.target as HTMLInputElement).checked })} /> <span class="swatch match" /> regex match</label>
              </div>
            )}
          </div>
        )}
        <div class="paneview">
          {tabs.map((t) => (
            <TerminalView key={t.termId} termId={t.termId} sessionId={t.sessionId} active={view.kind === "term" && view.id === t.termId} />
          ))}
          {view.kind === "term" ? null : sel && view.kind === "details" && detail ? (
            <div class="details">
              <h1>{detail.name}{selPinned && <span class="pinbadge big">{"\u2605"}</span>}</h1>
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
                <button class="danger" onClick={deleteSelected}>delete</button>
              </div>
            </div>
          ) : <div class="empty">select a session</div>}
        </div>
      </div>

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

      {modal === "session-new" && selFolder && <SessionModal folder={selFolder} detail={null} onClose={() => setModal("")} onSaved={afterMutation} />}
      {modal === "session-edit" && selFolder && detail && <SessionModal folder={selFolder} detail={detail} onClose={() => setModal("")} onSaved={afterMutation} />}
      {modal === "folder" && selFolder && <FolderModal parent={selFolder} onClose={() => setModal("")} onSaved={afterMutation} />}
      {promptQ.length > 0 && <PromptModal req={promptQ[0]} onResolve={resolvePrompt} />}
    </div>
  );
}
