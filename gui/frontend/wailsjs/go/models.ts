export namespace main {
	
	export class EngineCapabilities {
	    platform: string;
	    heavy_mode_supported: boolean;
	    engine_name: string;
	    is_installed: boolean;
	    binary_path: string;
	    version: string;
	    can_auto_install: boolean;
	    gpu_available: boolean;
	    gpu_vendor: string;
	    status_message: string;
	
	    static createFrom(source: any = {}) {
	        return new EngineCapabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.platform = source["platform"];
	        this.heavy_mode_supported = source["heavy_mode_supported"];
	        this.engine_name = source["engine_name"];
	        this.is_installed = source["is_installed"];
	        this.binary_path = source["binary_path"];
	        this.version = source["version"];
	        this.can_auto_install = source["can_auto_install"];
	        this.gpu_available = source["gpu_available"];
	        this.gpu_vendor = source["gpu_vendor"];
	        this.status_message = source["status_message"];
	    }
	}
	export class ProjectMetadata {
	    input_url: string;
	    project_id: string;
	    nats_ws_url: string;
	    payload_url: string;
	    master_host: string;
	    reachable: boolean;
	    status_text: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectMetadata(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.input_url = source["input_url"];
	        this.project_id = source["project_id"];
	        this.nats_ws_url = source["nats_ws_url"];
	        this.payload_url = source["payload_url"];
	        this.master_host = source["master_host"];
	        this.reachable = source["reachable"];
	        this.status_text = source["status_text"];
	    }
	}
	export class StartConfig {
	    project_id: string;
	    nats_url: string;
	    payload_url: string;
	    concurrency: number;
	    compute_mode: string;
	    enable_gpu: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StartConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.nats_url = source["nats_url"];
	        this.payload_url = source["payload_url"];
	        this.concurrency = source["concurrency"];
	        this.compute_mode = source["compute_mode"];
	        this.enable_gpu = source["enable_gpu"];
	    }
	}
	export class SystemInfo {
	    num_cpu: number;
	    default_threads: number;
	    os: string;
	    worker_id: string;
	    ip: string;
	    engine_caps: EngineCapabilities;
	
	    static createFrom(source: any = {}) {
	        return new SystemInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.num_cpu = source["num_cpu"];
	        this.default_threads = source["default_threads"];
	        this.os = source["os"];
	        this.worker_id = source["worker_id"];
	        this.ip = source["ip"];
	        this.engine_caps = this.convertValues(source["engine_caps"], EngineCapabilities);
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

