export namespace engine {
	
	export class AddRequest {
	    url: string;
	    dir: string;
	    fileName: string;
	    connections: number;
	    speedLimit: number;
	    headers: Record<string, string>;
	    pageUrl: string;
	    startPaused: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AddRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.dir = source["dir"];
	        this.fileName = source["fileName"];
	        this.connections = source["connections"];
	        this.speedLimit = source["speedLimit"];
	        this.headers = source["headers"];
	        this.pageUrl = source["pageUrl"];
	        this.startPaused = source["startPaused"];
	    }
	}
	export class Config {
	    downloadDir: string;
	    maxActive: number;
	    connections: number;
	    speedLimit: number;
	    proxy: string;
	    userAgent: string;
	    categorize: boolean;
	    watchClipboard: boolean;
	    confirmCaptured: boolean;
	    maxRetries: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.downloadDir = source["downloadDir"];
	        this.maxActive = source["maxActive"];
	        this.connections = source["connections"];
	        this.speedLimit = source["speedLimit"];
	        this.proxy = source["proxy"];
	        this.userAgent = source["userAgent"];
	        this.categorize = source["categorize"];
	        this.watchClipboard = source["watchClipboard"];
	        this.confirmCaptured = source["confirmCaptured"];
	        this.maxRetries = source["maxRetries"];
	    }
	}
	export class SegmentInfo {
	    start: number;
	    end: number;
	    done: number;
	
	    static createFrom(source: any = {}) {
	        return new SegmentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = source["start"];
	        this.end = source["end"];
	        this.done = source["done"];
	    }
	}
	export class Info {
	    id: string;
	    url: string;
	    fileName: string;
	    dir: string;
	    path: string;
	    category: string;
	    size: number;
	    downloaded: number;
	    speed: number;
	    eta: number;
	    status: string;
	    error: string;
	    resumable: boolean;
	    connections: number;
	    speedLimit: number;
	    pageUrl: string;
	    order: number;
	    segments: SegmentInfo[];
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    completedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.fileName = source["fileName"];
	        this.dir = source["dir"];
	        this.path = source["path"];
	        this.category = source["category"];
	        this.size = source["size"];
	        this.downloaded = source["downloaded"];
	        this.speed = source["speed"];
	        this.eta = source["eta"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.resumable = source["resumable"];
	        this.connections = source["connections"];
	        this.speedLimit = source["speedLimit"];
	        this.pageUrl = source["pageUrl"];
	        this.order = source["order"];
	        this.segments = this.convertValues(source["segments"], SegmentInfo);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.completedAt = this.convertValues(source["completedAt"], null);
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
	export class ProbeResult {
	    finalUrl: string;
	    fileName: string;
	    size: number;
	    resumable: boolean;
	    etag: string;
	    lastModified: string;
	    contentType: string;
	
	    static createFrom(source: any = {}) {
	        return new ProbeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.finalUrl = source["finalUrl"];
	        this.fileName = source["fileName"];
	        this.size = source["size"];
	        this.resumable = source["resumable"];
	        this.etag = source["etag"];
	        this.lastModified = source["lastModified"];
	        this.contentType = source["contentType"];
	    }
	}

}

export namespace main {
	
	export class ExternalAdd {
	    url: string;
	    fileName: string;
	    headers: Record<string, string>;
	    pageUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new ExternalAdd(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.fileName = source["fileName"];
	        this.headers = source["headers"];
	        this.pageUrl = source["pageUrl"];
	    }
	}
	export class Integration {
	    hostPath: string;
	    hostFound: boolean;
	    extensionId: string;
	    extensionDir: string;
	    extensionFound: boolean;
	    firefoxExtensionId: string;
	    firefoxExtensionDir: string;
	    firefoxExtensionFound: boolean;
	    browsers: nativehost.BrowserStatus[];
	
	    static createFrom(source: any = {}) {
	        return new Integration(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostPath = source["hostPath"];
	        this.hostFound = source["hostFound"];
	        this.extensionId = source["extensionId"];
	        this.extensionDir = source["extensionDir"];
	        this.extensionFound = source["extensionFound"];
	        this.firefoxExtensionId = source["firefoxExtensionId"];
	        this.firefoxExtensionDir = source["firefoxExtensionDir"];
	        this.firefoxExtensionFound = source["firefoxExtensionFound"];
	        this.browsers = this.convertValues(source["browsers"], nativehost.BrowserStatus);
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

export namespace nativehost {
	
	export class BrowserStatus {
	    name: string;
	    installed: boolean;
	    current: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BrowserStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.installed = source["installed"];
	        this.current = source["current"];
	    }
	}

}

