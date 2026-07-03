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
    folders: FolderNode[] | null;
    sessions: SessionNode[] | null;
  }
}
