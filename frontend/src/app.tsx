import { useEffect, useRef, useState } from "preact/hooks";

// Phase 01b: filtering runs in Go (internal/filter, <5ms/10k). While the
// query is non-empty the tree is replaced by the ranked flat list; clearing
// it restores the tree. CRUD dialogs use the inheritance view from
// SessionDetail — empty option inputs mean "inherit".

const api = () => window.go.app.App;

const OPTION_KEYS: { key: string; label: string; hint: string }[] = [
  { key: "termType", label: "term type", hint: "xterm-256color" },
  { key: "keepaliveInterval", label: "keepalive", hint: "30s" },
  { key: "reconnect", label: "reconnect", hint: "off | prompt | auto" },
  { key: "theme", label: "theme", hint: "oled-black" },
  { key: "scrollbackLines", label: "scrollback lines", hint: "5000000" },
  { key: "auditScope", label: "audit scope", hint: "off | events | events+input | full-io" },
];

function Folder(props: {
  node: FolderNode;
  depth: number;
  selected: string;
  selectedFolder: string;
  onSelect: (s: SessionNode) => void;
  onSelectFolder: (f: FolderNode) => void;
}) {
  const { node, depth, selected, selectedFolder, onSelect, onSelectFolder } = props;
  const [open, setOpen] = useState(depth < 2);
  const pad = { paddingLeft: `${depth * 14}px` };
  return (
    <div>
      <div
        class={"row folder" + (node.id === selectedFolder ? " selected" : "")}
        style={pad}
        onClick={() => onSelectFolder(node)}
      >
        <span class="twist" onClick={(e) => { e.stopPropagation(); setOpen(!open); }}>
          {open ? "\u25be" : "\u25b8"}
        </span>
        <span class="fname">{node.name}</span>
      </div>
      {open &&
        (node.sessions ?? []).map((s) => (
          <div
            key={s.id}
            class={"row session" + (s.id === selected ? " selected" : "")}
            style={{ paddingLeft: `${(depth + 1) * 14 + 12}px` }}
            onClick={() => onSelect(s)}
          >
            <span class="sname">{s.name}</span>
            {s.detectedOs && <span class="ostag">{s.detectedOs}</span>}
          </div>
        ))}
      {open &&
        (node.folders ?? []).map((c) => (
          <Folder
            key={c.id}
            node={c}
            depth={depth + 1}
            selected={selected}
            selectedFolder={selectedFolder}
            onSelect={onSelect}
            onSelectFolder={onSelectFolder}
          />
        ))}
    </div>
  );
}

function SessionModal(props: {
  folder: { id: string; path: string };
  detail: SessionDetail | null; // null = create
  onClose: () => void;
  onSaved: (id: string) => void;
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

  const save = () => {
    const input: SessionInput = {
      id: detail?.id ?? "",
      folderId: detail?.folderId ?? folder.id,
      name,
      host,
      port: port.trim() === "" ? 0 : parseInt(port, 10) || 0,
      user,
      proto: detail?.proto ?? "ssh",
      options: opts,
    };
    api().SaveSession(input).then(onSaved).catch((e) => setErr(String(e)));
  };

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
        <div class="formrow"><label>name</label>
          <input value={name} onInput={(e) => setName((e.target as HTMLInputElement).value)} /></div>
        <div class="formrow"><label>host</label>
          <input value={host} onInput={(e) => setHost((e.target as HTMLInputElement).value)} /></div>
        <div class="formrow"><label>port</label>
          <input value={port} placeholder="22" onInput={(e) => setPort((e.target as HTMLInputElement).value)} /></div>
        <div class="formrow"><label>user</label>
          <input value={user} onInput={(e) => setUser((e.target as HTMLInputElement).value)} /></div>

        <div class="opthead">options — empty = inherit</div>
        {OPTION_KEYS.map(({ key, label }) => (
          <div class="formrow" key={key}>
            <label>{label}</label>
            <input
              value={opts[key]}
              placeholder={placeholderFor(key)}
              onInput={(e) => setOpts({ ...opts, [key]: (e.target as HTMLInputElement).value })}
            />
          </div>
        ))}

        {detail && detail.jumpChain && detail.jumpChain.length > 0 && (
          <div class="jumpinfo">
            jump chain ({detail.jumpSource}):{" "}
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

function FolderModal(props: {
  parent: { id: string; path: string };
  onClose: () => void;
  onSaved: () => void;
}) {
  const { parent, onClose, onSaved } = props;
  const [name, setName] = useState("");
  const [err, setErr] = useState("");
  const save = () => {
    api()
      .SaveFolder({ id: "", parentId: parent.id, name })
      .then(() => onSaved())
      .catch((e) => setErr(String(e)));
  };
  return (
    <div class="modal-overlay" onClick={onClose}>
      <div class="modal" onClick={(e) => e.stopPropagation()}>
        <h2>new folder in {parent.path}</h2>
        {err && <div class="error">{err}</div>}
        <div class="formrow"><label>name</label>
          <input value={name} onInput={(e) => setName((e.target as HTMLInputElement).value)} /></div>
        <div class="modal-actions">
          <button onClick={onClose}>cancel</button>
          <button class="primary" onClick={save}>save</button>
        </div>
      </div>
    </div>
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
  const [ver, setVer] = useState("");
  const [modal, setModal] = useState<"" | "session-new" | "session-edit" | "folder">("");
  const debounce = useRef<number | undefined>(undefined);

  const load = () => {
    api().Tree().then((t) => {
      setTree(t);
      if (!selFolder) setSelFolder({ id: t.id, path: t.path });
    }).catch((e) => setErr(String(e)));
  };

  useEffect(() => {
    load();
    api().GetVersion().then(setVer).catch(() => {});
  }, []);

  const onQuery = (raw: string) => {
    setQ(raw);
    window.clearTimeout(debounce.current);
    if (raw.trim() === "") {
      setHits(null);
      return;
    }
    debounce.current = window.setTimeout(() => {
      api().Filter(raw).then(setHits).catch((e) => setErr(String(e)));
    }, 80);
  };

  const select = (s: SessionNode) => {
    setSel(s);
    api().SessionDetail(s.id).then(setDetail).catch((e) => setErr(String(e)));
  };

  const afterMutation = () => {
    setModal("");
    load();
    if (sel) api().SessionDetail(sel.id).then(setDetail).catch(() => { setSel(null); setDetail(null); });
    if (q.trim() !== "") api().Filter(q).then(setHits).catch(() => {});
  };

  const deleteSelected = () => {
    if (!sel) return;
    if (!confirm(`delete session ${sel.name}?`)) return;
    api().DeleteSession(sel.id).then(() => {
      setSel(null);
      setDetail(null);
      afterMutation();
    }).catch((e) => setErr(String(e)));
  };

  return (
    <div class="layout">
      <div class="sidebar">
        <div class="toolbar">
          <button onClick={() => setModal("session-new")} disabled={!selFolder}>+ session</button>
          <button onClick={() => setModal("folder")} disabled={!selFolder}>+ folder</button>
          <span class="tbpath">{selFolder?.path ?? ""}</span>
        </div>
        <div class="filterbar">
          <input
            type="text"
            placeholder="filter sessions..."
            value={q}
            onInput={(e) => onQuery((e.target as HTMLInputElement).value)}
          />
          <button title="reload store" onClick={load}>&#x21bb;</button>
        </div>
        <div class="tree">
          {err && <div class="error" onClick={() => setErr("")}>{err}</div>}
          {hits !== null ? (
            hits.length === 0 ? (
              <div class="nohits">no matches</div>
            ) : (
              hits.map((h) => (
                <div
                  key={h.id}
                  class={"row session" + (h.id === sel?.id ? " selected" : "")}
                  style={{ paddingLeft: "10px" }}
                  onClick={() => select(h)}
                >
                  <span class="hitpath">{h.path}/</span>
                  <span class="sname">{h.name}</span>
                  {h.detectedOs && <span class="ostag">{h.detectedOs}</span>}
                </div>
              ))
            )
          ) : (
            tree && (
              <Folder
                node={tree}
                depth={0}
                selected={sel?.id ?? ""}
                selectedFolder={selFolder?.id ?? ""}
                onSelect={select}
                onSelectFolder={(f) => setSelFolder({ id: f.id, path: f.path })}
              />
            )
          )}
        </div>
        <div class="statusbar">f9 {ver}</div>
      </div>
      <div class="mainpane">
        {sel && detail ? (
          <div class="details">
            <h1>{detail.name}</h1>
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
                return (
                  <tr key={key}>
                    <td>{label}</td>
                    <td>{f.effective} <span class="badge">{f.source}</span></td>
                  </tr>
                );
              })}
              {detail.jumpChain && detail.jumpChain.length > 0 && (
                <tr>
                  <td>jump chain</td>
                  <td>
                    {detail.jumpChain.map((j) => `${j.user ? j.user + "@" : ""}${j.host} [${j.mode}]`).join(" \u2192 ")}{" "}
                    <span class="badge">{detail.jumpSource}</span>
                  </td>
                </tr>
              )}
            </table>
            <div class="detail-actions">
              <button onClick={() => setModal("session-edit")}>edit</button>
              <button class="danger" onClick={deleteSelected}>delete</button>
            </div>
            <div class="hint">terminal attach arrives in phase 02 — until then: <code>f9 connect {detail.name}</code></div>
          </div>
        ) : (
          <div class="empty">select a session</div>
        )}
      </div>

      {modal === "session-new" && selFolder && (
        <SessionModal folder={selFolder} detail={null} onClose={() => setModal("")} onSaved={afterMutation} />
      )}
      {modal === "session-edit" && selFolder && detail && (
        <SessionModal folder={selFolder} detail={detail} onClose={() => setModal("")} onSaved={afterMutation} />
      )}
      {modal === "folder" && selFolder && (
        <FolderModal parent={selFolder} onClose={() => setModal("")} onSaved={afterMutation} />
      )}
    </div>
  );
}
