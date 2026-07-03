export namespace app {
	
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
	    folders: FolderNode[];
	    sessions: SessionNode[];
	
	    static createFrom(source: any = {}) {
	        return new FolderNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
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

}

