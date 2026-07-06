export namespace app {
	
	export class AgentStatus {
	    endpoints: sshx.AgentEndpoint[];
	
	    static createFrom(source: any = {}) {
	        return new AgentStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpoints = this.convertValues(source["endpoints"], sshx.AgentEndpoint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CredState {
	    initialized: boolean;
	    locked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CredState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.initialized = source["initialized"];
	        this.locked = source["locked"];
	    }
	}
	export class FilterHit {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    proto: string;
	    detectedOs: string;
	    osPinned: boolean;
	    pinned: boolean;
	    generated: boolean;
	    path: string;
	    score: number;
	
	    static createFrom(source: any = {}) {
	        return new FilterHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.proto = source["proto"];
	        this.detectedOs = source["detectedOs"];
	        this.osPinned = source["osPinned"];
	        this.pinned = source["pinned"];
	        this.generated = source["generated"];
	        this.path = source["path"];
	        this.score = source["score"];
	    }
	}
	export class FolderInput {
	    id: string;
	    parentId: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new FolderInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.parentId = source["parentId"];
	        this.name = source["name"];
	    }
	}
	export class SessionNode {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    proto: string;
	    detectedOs: string;
	    osPinned: boolean;
	    pinned: boolean;
	    generated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SessionNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.proto = source["proto"];
	        this.detectedOs = source["detectedOs"];
	        this.osPinned = source["osPinned"];
	        this.pinned = source["pinned"];
	        this.generated = source["generated"];
	    }
	}
	export class FolderNode {
	    id: string;
	    name: string;
	    path: string;
	    hasSource: boolean;
	    folders: FolderNode[];
	    sessions: SessionNode[];
	
	    static createFrom(source: any = {}) {
	        return new FolderNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.hasSource = source["hasSource"];
	        this.folders = this.convertValues(source["folders"], FolderNode);
	        this.sessions = this.convertValues(source["sessions"], SessionNode);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GrepMatchDTO {
	    lineNo: number;
	    line: string;
	    before: string[];
	    after: string[];
	
	    static createFrom(source: any = {}) {
	        return new GrepMatchDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lineNo = source["lineNo"];
	        this.line = source["line"];
	        this.before = source["before"];
	        this.after = source["after"];
	    }
	}
	export class GrepOptsDTO {
	    invert: boolean;
	    ignoreCase: boolean;
	    before: number;
	    after: number;
	    maxMatches: number;
	
	    static createFrom(source: any = {}) {
	        return new GrepOptsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.invert = source["invert"];
	        this.ignoreCase = source["ignoreCase"];
	        this.before = source["before"];
	        this.after = source["after"];
	        this.maxMatches = source["maxMatches"];
	    }
	}
	export class GrepResultDTO {
	    matches: GrepMatchDTO[];
	    count: number;
	    truncated: boolean;
	    lines: number;
	
	    static createFrom(source: any = {}) {
	        return new GrepResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.matches = this.convertValues(source["matches"], GrepMatchDTO);
	        this.count = source["count"];
	        this.truncated = source["truncated"];
	        this.lines = source["lines"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class JumpHopDTO {
	    host: string;
	    port: number;
	    user: string;
	    mode: string;
	    userOverride: string;
	
	    static createFrom(source: any = {}) {
	        return new JumpHopDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.mode = source["mode"];
	        this.userOverride = source["userOverride"];
	    }
	}
	export class MSPreview {
	    termId: string;
	    sessionId: string;
	    name: string;
	    host: string;
	    osFamily: string;
	    line: string;
	    unresolved: string[];
	    err: string;
	
	    static createFrom(source: any = {}) {
	        return new MSPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.termId = source["termId"];
	        this.sessionId = source["sessionId"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.osFamily = source["osFamily"];
	        this.line = source["line"];
	        this.unresolved = source["unresolved"];
	        this.err = source["err"];
	    }
	}
	export class OptionField {
	    value: string;
	    effective: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new OptionField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.effective = source["effective"];
	        this.source = source["source"];
	    }
	}
	export class PeekDTO {
	    start: number;
	    lines: string[];
	
	    static createFrom(source: any = {}) {
	        return new PeekDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = source["start"];
	        this.lines = source["lines"];
	    }
	}
	export class PromptReply {
	    value: string;
	    useForAll: boolean;
	    accept: boolean;
	    cancel: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PromptReply(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.useForAll = source["useForAll"];
	        this.accept = source["accept"];
	        this.cancel = source["cancel"];
	    }
	}
	export class RefreshResult {
	    added: number;
	    updated: number;
	    removed: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new RefreshResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.updated = source["updated"];
	        this.removed = source["removed"];
	        this.error = source["error"];
	    }
	}
	export class SessionDetail {
	    id: string;
	    name: string;
	    folderId: string;
	    folderPath: string;
	    host: string;
	    port: number;
	    user: string;
	    proto: string;
	    options: Record<string, OptionField>;
	    jumpChain: JumpHopDTO[];
	    jumpSource: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.folderId = source["folderId"];
	        this.folderPath = source["folderPath"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.proto = source["proto"];
	        this.options = this.convertValues(source["options"], OptionField, true);
	        this.jumpChain = this.convertValues(source["jumpChain"], JumpHopDTO);
	        this.jumpSource = source["jumpSource"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionInput {
	    id: string;
	    folderId: string;
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    proto: string;
	    options: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new SessionInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.folderId = source["folderId"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.proto = source["proto"];
	        this.options = source["options"];
	    }
	}
	
	export class SourceDTO {
	    url: string;
	    format: string;
	    auth: string;
	    header: string;
	    reconcileBy: string;
	    insecure: boolean;
	    fieldMap: Record<string, string>;
	    filter?: store.FilterGroup;
	    hasSecret: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SourceDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.format = source["format"];
	        this.auth = source["auth"];
	        this.header = source["header"];
	        this.reconcileBy = source["reconcileBy"];
	        this.insecure = source["insecure"];
	        this.fieldMap = source["fieldMap"];
	        this.filter = this.convertValues(source["filter"], store.FilterGroup);
	        this.hasSecret = source["hasSecret"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TestResult {
	    ok: boolean;
	    count: number;
	    sample: string[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.count = source["count"];
	        this.sample = source["sample"];
	        this.error = source["error"];
	    }
	}
	export class UISettings {
	    theme: string;
	    zoom: number;
	    fontUI: string;
	    fontMono: string;
	    fontUISize: number;
	    fontTermSize: number;
	    showGlobalBar: boolean;
	    showFolderBar: boolean;
	    showTemplates: boolean;
	    showSnippets: boolean;
	    barVertical: boolean;
	    barUnpinned: boolean;
	    showMultiSend: boolean;
	    keyFiles: string[];
	    disableAgent: boolean;
	    agentSockets: string[];
	
	    static createFrom(source: any = {}) {
	        return new UISettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.zoom = source["zoom"];
	        this.fontUI = source["fontUI"];
	        this.fontMono = source["fontMono"];
	        this.fontUISize = source["fontUISize"];
	        this.fontTermSize = source["fontTermSize"];
	        this.showGlobalBar = source["showGlobalBar"];
	        this.showFolderBar = source["showFolderBar"];
	        this.showTemplates = source["showTemplates"];
	        this.showSnippets = source["showSnippets"];
	        this.barVertical = source["barVertical"];
	        this.barUnpinned = source["barUnpinned"];
	        this.showMultiSend = source["showMultiSend"];
	        this.keyFiles = source["keyFiles"];
	        this.disableAgent = source["disableAgent"];
	        this.agentSockets = source["agentSockets"];
	    }
	}
	export class VarsScopeDTO {
	    folderId: string;
	    sessionId: string;
	
	    static createFrom(source: any = {}) {
	        return new VarsScopeDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folderId = source["folderId"];
	        this.sessionId = source["sessionId"];
	    }
	}

}

export namespace buttonbar {
	
	export class Action {
	    kind: string;
	    text: string;
	    snippetId: string;
	    args: string[];
	    delayMs: number;
	    bracketed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Action(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.snippetId = source["snippetId"];
	        this.args = source["args"];
	        this.delayMs = source["delayMs"];
	        this.bracketed = source["bracketed"];
	    }
	}
	export class Button {
	    icon: string;
	    label: string;
	    color: string;
	    os: string;
	    action: Action;
	
	    static createFrom(source: any = {}) {
	        return new Button(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.icon = source["icon"];
	        this.label = source["label"];
	        this.color = source["color"];
	        this.os = source["os"];
	        this.action = this.convertValues(source["action"], Action);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Row {
	    buttons: Button[];
	
	    static createFrom(source: any = {}) {
	        return new Row(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.buttons = this.convertValues(source["buttons"], Button);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Bar {
	    rows: Row[];
	
	    static createFrom(source: any = {}) {
	        return new Bar(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = this.convertValues(source["rows"], Row);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace connmgr {
	
	export class Conn {
	    sessionId: string;
	    name: string;
	    host: string;
	    state: string;
	    err: string;
	    since: string;
	
	    static createFrom(source: any = {}) {
	        return new Conn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.state = source["state"];
	        this.err = source["err"];
	        this.since = source["since"];
	    }
	}

}

export namespace snippets {
	
	export class Folder {
	    id: string;
	    parentId: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Folder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.parentId = source["parentId"];
	        this.name = source["name"];
	    }
	}
	export class Snippet {
	    id: string;
	    folderId: string;
	    name: string;
	    body: string;
	    os: string;
	    delayMs: number;
	    bracketed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Snippet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.folderId = source["folderId"];
	        this.name = source["name"];
	        this.body = source["body"];
	        this.os = source["os"];
	        this.delayMs = source["delayMs"];
	        this.bracketed = source["bracketed"];
	    }
	}

}

export namespace sshx {
	
	export class AgentKey {
	    comment: string;
	    format: string;
	    fingerprint: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.comment = source["comment"];
	        this.format = source["format"];
	        this.fingerprint = source["fingerprint"];
	    }
	}
	export class AgentEndpoint {
	    socket: string;
	    available: boolean;
	    keys: AgentKey[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentEndpoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.socket = source["socket"];
	        this.available = source["available"];
	        this.keys = this.convertValues(source["keys"], AgentKey);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace store {
	
	export class FilterRule {
	    field: string;
	    kind: string;
	    value: string;
	    negate: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FilterRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.field = source["field"];
	        this.kind = source["kind"];
	        this.value = source["value"];
	        this.negate = source["negate"];
	    }
	}
	export class FilterGroup {
	    op: string;
	    rules: FilterRule[];
	    groups: FilterGroup[];
	
	    static createFrom(source: any = {}) {
	        return new FilterGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.op = source["op"];
	        this.rules = this.convertValues(source["rules"], FilterRule);
	        this.groups = this.convertValues(source["groups"], FilterGroup);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace theme {
	
	export class ANSI {
	    black: string;
	    red: string;
	    green: string;
	    yellow: string;
	    blue: string;
	    magenta: string;
	    cyan: string;
	    white: string;
	    brightBlack: string;
	    brightRed: string;
	    brightGreen: string;
	    brightYellow: string;
	    brightBlue: string;
	    brightMagenta: string;
	    brightCyan: string;
	    brightWhite: string;
	
	    static createFrom(source: any = {}) {
	        return new ANSI(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.black = source["black"];
	        this.red = source["red"];
	        this.green = source["green"];
	        this.yellow = source["yellow"];
	        this.blue = source["blue"];
	        this.magenta = source["magenta"];
	        this.cyan = source["cyan"];
	        this.white = source["white"];
	        this.brightBlack = source["brightBlack"];
	        this.brightRed = source["brightRed"];
	        this.brightGreen = source["brightGreen"];
	        this.brightYellow = source["brightYellow"];
	        this.brightBlue = source["brightBlue"];
	        this.brightMagenta = source["brightMagenta"];
	        this.brightCyan = source["brightCyan"];
	        this.brightWhite = source["brightWhite"];
	    }
	}
	export class Font {
	    ui: string;
	    mono: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new Font(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ui = source["ui"];
	        this.mono = source["mono"];
	        this.size = source["size"];
	    }
	}
	export class Terminal {
	    background: string;
	    foreground: string;
	    cursor: string;
	    cursorAccent: string;
	    selection: string;
	    ansi: ANSI;
	
	    static createFrom(source: any = {}) {
	        return new Terminal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.background = source["background"];
	        this.foreground = source["foreground"];
	        this.cursor = source["cursor"];
	        this.cursorAccent = source["cursorAccent"];
	        this.selection = source["selection"];
	        this.ansi = this.convertValues(source["ansi"], ANSI);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UI {
	    bg: string;
	    bgRaised: string;
	    fg: string;
	    accent: string;
	    border: string;
	    folderFg: string;
	    selectedBg: string;
	    danger: string;
	
	    static createFrom(source: any = {}) {
	        return new UI(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bg = source["bg"];
	        this.bgRaised = source["bgRaised"];
	        this.fg = source["fg"];
	        this.accent = source["accent"];
	        this.border = source["border"];
	        this.folderFg = source["folderFg"];
	        this.selectedBg = source["selectedBg"];
	        this.danger = source["danger"];
	    }
	}
	export class Theme {
	    name: string;
	    ui: UI;
	    font: Font;
	    terminal: Terminal;
	
	    static createFrom(source: any = {}) {
	        return new Theme(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ui = this.convertValues(source["ui"], UI);
	        this.font = this.convertValues(source["font"], Font);
	        this.terminal = this.convertValues(source["terminal"], Terminal);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace updater {
	
	export class Info {
	    current: string;
	    latest: string;
	    newer: boolean;
	    url: string;
	    notes: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.newer = source["newer"];
	        this.url = source["url"];
	        this.notes = source["notes"];
	        this.error = source["error"];
	    }
	}

}

