export namespace app {
	
	export class FilterHit {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    proto: string;
	    detectedOs: string;
	    osPinned: boolean;
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
	    }
	}
	export class FolderNode {
	    id: string;
	    name: string;
	    path: string;
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

