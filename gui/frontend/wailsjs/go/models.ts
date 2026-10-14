export namespace main {
	
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
	
	    static createFrom(source: any = {}) {
	        return new StartConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.nats_url = source["nats_url"];
	        this.payload_url = source["payload_url"];
	        this.concurrency = source["concurrency"];
	    }
	}
	export class SystemInfo {
	    num_cpu: number;
	    default_threads: number;
	    os: string;
	    worker_id: string;
	    ip: string;
	
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
	    }
	}

}

