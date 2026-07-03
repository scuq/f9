// Wails runtime bridge. Using window.go directly keeps the frontend
// independent of the generated wailsjs/ modules and their generation order.
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
        };
      };
    };
  }

  interface SessionNode {
    id: string;
    name: string;
    host: string;
    port: number;
    user: string;
    proto: string;
    detectedOs: string;
    osPinned: boolean;
  }

  interface FolderNode {
    id: string;
    name: string;
    path: string;
    folders: FolderNode[] | null;
    sessions: SessionNode[] | null;
  }

  interface FilterHit extends SessionNode {
    path: string;
    score: number;
  }

  interface OptionField {
    value: string;
    effective: string;
    source: string;
  }

  interface JumpHop {
    host: string;
    port: number;
    user: string;
    mode: string;
    userOverride: string;
  }

  interface SessionDetail {
    id: string;
    name: string;
    folderId: string;
    folderPath: string;
    host: string;
    port: number;
    user: string;
    proto: string;
    options: Record<string, OptionField>;
    jumpChain: JumpHop[] | null;
    jumpSource: string;
  }

  interface SessionInput {
    id: string;
    folderId: string;
    name: string;
    host: string;
    port: number;
    user: string;
    proto: string;
    options: Record<string, string>;
  }

  interface FolderInput {
    id: string;
    parentId: string;
    name: string;
  }
}
