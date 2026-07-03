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
          OpenTerminal(sessionId: string, cols: number, rows: number): Promise<void>;
          TermInput(sessionId: string, data: string): Promise<void>;
          TermResize(sessionId: string, cols: number, rows: number): Promise<void>;
          CloseTerminal(sessionId: string): Promise<void>;
        };
      };
    };
    runtime: {
      EventsOn(event: string, cb: (data: any) => void): () => void;
      EventsOff(event: string): void;
    };
  }

  interface SessionNode {
    id: string; name: string; host: string; port: number;
    user: string; proto: string; detectedOs: string; osPinned: boolean;
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
}
