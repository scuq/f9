export {};

declare global {
  interface Window {
    go: {
      app: {
        App: {
          Tree(): Promise<FolderNode>;
          GetVersion(): Promise<string>;
          Filter(query: string): Promise<FilterHit[]>;
          SessionDetail(id: string): Promise<SessionDetail>;
          SaveSession(input: SessionInput): Promise<string>;
          SaveFolder(input: FolderInput): Promise<string>;
          DeleteSession(id: string): Promise<void>;
          DeleteFolder(id: string): Promise<void>;
          ConnectSessions(ids: string[]): Promise<void>;
          ConnectFolder(id: string): Promise<void>;
          ActiveConnections(): Promise<Conn[]>;
          Disconnect(id: string): Promise<void>;
          DisconnectAll(): Promise<void>;
          ResolvePrompt(id: string, reply: PromptReply): Promise<void>;
          OpenTerminal(termId: string, sessionId: string, cols: number, rows: number): Promise<void>;
          TermInput(termId: string, data: string): Promise<void>;
          TermResize(termId: string, cols: number, rows: number): Promise<void>;
          CloseTerminal(termId: string): Promise<void>;
          SetTerminalWatch(termId: string, pattern: string): Promise<void>;
          Themes(): Promise<string[]>;
          Theme(name: string): Promise<ThemeData>;
          CurrentTheme(): Promise<string>;
          Settings(): Promise<UISettings>;
          SaveSettings(s: UISettings): Promise<void>;
          ImportITermTheme(): Promise<string>;
          GrepTerminal(termId: string, pattern: string, opts: GrepOptsInput): Promise<GrepResult>;
          TerminalStats(termId: string): Promise<number>;
          TerminalPeek(termId: string, lineNo0: number, context: number): Promise<PeekResult>;
          PinSession(id: string): Promise<void>;
          UnpinSession(id: string): Promise<void>;
          PinnedSessions(): Promise<SessionNode[]>;
          VarsList(scope: VarsScope, family: string): Promise<Record<string, string>>;
          VarsPut(scope: VarsScope, key: string, value: string, os: string): Promise<void>;
          VarsDelete(scope: VarsScope, key: string, os: string): Promise<void>;
          TemplateUnresolved(sessionId: string, body: string): Promise<string[] | null>;
          RenderTemplate(sessionId: string, body: string, extra: Record<string, string>): Promise<string>;
          SendToTerminal(termId: string, text: string, lineDelayMs: number, bracketed: boolean): Promise<void>;
          SendTemplate(termId: string, body: string, extra: Record<string, string>, lineDelayMs: number, bracketed: boolean): Promise<void>;
          BarForSession(sessionId: string): Promise<Bar>;
          BarResolved(folderId: string): Promise<Bar>;
          BarRaw(folderId: string): Promise<Bar | null>;
          BarSave(folderId: string, bar: Bar): Promise<void>;
          BarDelete(folderId: string): Promise<void>;
          BarExport(folderId: string): Promise<string>;
          BarImport(folderId: string, yamlText: string): Promise<void>;
          LaunchApp(args: string[]): Promise<void>;
          OpenURL(url: string): Promise<void>;
        };
      };
    };
    runtime: {
      EventsOn(event: string, cb: (data: any) => void): () => void;
      EventsOff(event: string): void;
      WindowMinimise?: () => void;
      WindowToggleMaximise?: () => void;
      Quit?: () => void;
    };
  }

  interface SessionNode {
    id: string; name: string; host: string; port: number;
    user: string; proto: string; detectedOs: string; osPinned: boolean; pinned: boolean;
  }
  interface FolderNode {
    id: string; name: string; path: string;
    folders: FolderNode[] | null; sessions: SessionNode[] | null;
  }
  interface FilterHit extends SessionNode { path: string; score: number; }
  interface OptionField { value: string; effective: string; source: string; }
  interface JumpHop { host: string; port: number; user: string; mode: string; userOverride: string; }
  interface SessionDetail {
    id: string; name: string; folderId: string; folderPath: string;
    host: string; port: number; user: string; proto: string;
    options: Record<string, OptionField>; jumpChain: JumpHop[] | null; jumpSource: string;
  }
  interface SessionInput {
    id: string; folderId: string; name: string; host: string;
    port: number; user: string; proto: string; options: Record<string, string>;
  }
  interface FolderInput { id: string; parentId: string; name: string; }
  interface Conn {
    sessionId: string; name: string; host: string;
    state: "dialing" | "connected" | "error"; err: string; since: string;
  }
  interface PromptRequest {
    id: string; kind: "password" | "passphrase" | "hostkey" | "kbi";
    user: string; host: string; keyPath: string; fingerprint: string;
    prompt: string; echo: boolean;
  }
  interface PromptReply { value: string; useForAll: boolean; accept: boolean; cancel: boolean; }
  interface UISettings {
    theme: string; zoom: number;
    fontUI: string; fontMono: string; fontUISize: number; fontTermSize: number;
  }
  interface GrepOptsInput { invert: boolean; ignoreCase: boolean; before: number; after: number; maxMatches: number; }
  interface GrepMatch { lineNo: number; line: string; before: string[] | null; after: string[] | null; }
  interface GrepResult { matches: GrepMatch[] | null; count: number; truncated: boolean; lines: number; }
  interface PeekResult { start: number; lines: string[] | null; }
  interface VarsScope { folderId: string; sessionId: string; }
  interface BarAction { kind: string; text?: string; snippetId?: string; args?: string[] | null; delayMs?: number; bracketed?: boolean; }
  interface BarButton { icon?: string; label: string; color?: string; action: BarAction; }
  interface BarRow { buttons: BarButton[] | null; }
  interface Bar { rows: BarRow[] | null; }
  interface ThemeData {
    name: string;
    ui: { bg: string; bgRaised: string; fg: string; accent: string; border: string; folderFg: string; selectedBg: string; danger: string };
    font: { ui: string; mono: string; size: number };
    terminal: {
      background: string; foreground: string; cursor: string; cursorAccent: string; selection: string;
      ansi: {
        black: string; red: string; green: string; yellow: string; blue: string; magenta: string; cyan: string; white: string;
        brightBlack: string; brightRed: string; brightGreen: string; brightYellow: string; brightBlue: string; brightMagenta: string; brightCyan: string; brightWhite: string;
      };
    };
  }
}
