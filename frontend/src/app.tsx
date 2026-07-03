import { useEffect, useState } from "preact/hooks";

// Phase 01a: scaffold + live tree. The filter below is a plain client-side
// substring match — the real fuzzy scorer runs in Go (phase 01b, <5ms/10k).
// Virtualized rendering also lands in 01b; plain recursion is fine until
// the tree grows past a few thousand visible rows.

function matches(s: SessionNode, q: string): boolean {
  return (
    s.name.toLowerCase().includes(q) ||
    s.host.toLowerCase().includes(q) ||
    s.user.toLowerCase().includes(q)
  );
}

function folderMatches(f: FolderNode, q: string): boolean {
  if (f.name.toLowerCase().includes(q)) return true;
  if ((f.sessions ?? []).some((s) => matches(s, q))) return true;
  return (f.folders ?? []).some((c) => folderMatches(c, q));
}

function Folder(props: {
  node: FolderNode;
  q: string;
  depth: number;
  selected: string;
  onSelect: (s: SessionNode) => void;
}) {
  const { node, q, depth, selected, onSelect } = props;
  const [open, setOpen] = useState(depth < 2);
  if (q !== "" && !folderMatches(node, q)) return null;

  const pad = { paddingLeft: `${depth * 14}px` };
  return (
    <div>
      <div class="row folder" style={pad} onClick={() => setOpen(!open)}>
        <span class="twist">{open ? "\u25be" : "\u25b8"}</span>
        <span class="fname">{node.name}</span>
      </div>
      {open &&
        (node.sessions ?? [])
          .filter((s) => q === "" || matches(s, q) || node.name.toLowerCase().includes(q))
          .map((s) => (
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
          <Folder key={c.id} node={c} q={q} depth={depth + 1} selected={selected} onSelect={onSelect} />
        ))}
    </div>
  );
}

export function App() {
  const [tree, setTree] = useState<FolderNode | null>(null);
  const [err, setErr] = useState("");
  const [q, setQ] = useState("");
  const [sel, setSel] = useState<SessionNode | null>(null);
  const [ver, setVer] = useState("");

  const load = () => {
    window.go.app.App.Tree().then(setTree).catch((e) => setErr(String(e)));
  };

  useEffect(() => {
    load();
    window.go.app.App.GetVersion().then(setVer).catch(() => {});
  }, []);

  return (
    <div class="layout">
      <div class="sidebar">
        <div class="filterbar">
          <input
            type="text"
            placeholder="filter sessions..."
            value={q}
            onInput={(e) => setQ((e.target as HTMLInputElement).value.toLowerCase())}
          />
          <button title="reload store" onClick={load}>&#x21bb;</button>
        </div>
        <div class="tree">
          {err && <div class="error">{err}</div>}
          {tree && (
            <Folder node={tree} q={q} depth={0} selected={sel?.id ?? ""} onSelect={setSel} />
          )}
        </div>
        <div class="statusbar">f9 {ver}</div>
      </div>
      <div class="mainpane">
        {sel ? (
          <div class="details">
            <h1>{sel.name}</h1>
            <table>
              <tr><td>host</td><td>{sel.host}{sel.port && sel.port !== 22 ? ":" + sel.port : ""}</td></tr>
              <tr><td>user</td><td>{sel.user || "\u2014"}</td></tr>
              <tr><td>proto</td><td>{sel.proto}</td></tr>
              <tr><td>os</td><td>{sel.detectedOs ? sel.detectedOs + (sel.osPinned ? " (pinned)" : "") : "not detected yet"}</td></tr>
            </table>
            <div class="hint">terminal attach arrives in phase 02 — until then: <code>f9 connect {sel.name}</code></div>
          </div>
        ) : (
          <div class="empty">select a session</div>
        )}
      </div>
    </div>
  );
}
