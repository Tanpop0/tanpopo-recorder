export namespace config {

	export class TelegramConfig {
	    enabled: boolean;
	    bot_token: string;
	    chat_id: string;
	    notify_on_start: boolean;
	    notify_on_finish: boolean;
	    notify_on_error: boolean;

	    static createFrom(source: any = {}) {
	        return new TelegramConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.bot_token = source["bot_token"];
	        this.chat_id = source["chat_id"];
	        this.notify_on_start = source["notify_on_start"];
	        this.notify_on_finish = source["notify_on_finish"];
	        this.notify_on_error = source["notify_on_error"];
	    }
	}
	export class NotificationsConfig {
	    telegram: TelegramConfig;

	    static createFrom(source: any = {}) {
	        return new NotificationsConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.telegram = this.convertValues(source["telegram"], TelegramConfig);
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
	export class ProxyConfig {
	    enabled: boolean;
	    url: string;

	    static createFrom(source: any = {}) {
	        return new ProxyConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.url = source["url"];
	    }
	}
	export class RecordingConfig {
	    quality_mode: string;
	    container_mode: string;
	    save_info_text: boolean;
	    save_comments_text: boolean;
	    save_comments_text_file: boolean;
	    comment_text_template: string;
	    min_duration_seconds: number;
	    min_file_size_mb: number;
	    startup_stagger_seconds: number;
	    ffmpeg_path: string;
	    ffprobe_path: string;
	    worker_enabled: boolean;
	    worker_path: string;
	    worker_check_interval_seconds: number;
	    worker_max_restarts: number;

	    static createFrom(source: any = {}) {
	        return new RecordingConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.quality_mode = source["quality_mode"];
	        this.container_mode = source["container_mode"];
	        this.save_info_text = source["save_info_text"];
	        this.save_comments_text = source["save_comments_text"];
	        this.save_comments_text_file = source["save_comments_text_file"];
	        this.comment_text_template = source["comment_text_template"];
	        this.min_duration_seconds = source["min_duration_seconds"];
	        this.min_file_size_mb = source["min_file_size_mb"];
	        this.startup_stagger_seconds = source["startup_stagger_seconds"];
	        this.ffmpeg_path = source["ffmpeg_path"];
	        this.ffprobe_path = source["ffprobe_path"];
	        this.worker_enabled = source["worker_enabled"];
	        this.worker_path = source["worker_path"];
	        this.worker_check_interval_seconds = source["worker_check_interval_seconds"];
	        this.worker_max_restarts = source["worker_max_restarts"];
	    }
	}
	export class CookieConfig {
	    enabled: boolean;
	    file_path: string;

	    static createFrom(source: any = {}) {
	        return new CookieConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.file_path = source["file_path"];
	    }
	}
	export class OAuthConfig {
	    client_id: string;
	    client_secret: string;
	    redirect_uri: string;
	    access_token: string;
	    token_type?: string;
	    scope?: string;

	    static createFrom(source: any = {}) {
	        return new OAuthConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.client_id = source["client_id"];
	        this.client_secret = source["client_secret"];
	        this.redirect_uri = source["redirect_uri"];
	        this.access_token = source["access_token"];
	        this.token_type = source["token_type"];
	        this.scope = source["scope"];
	    }
	}
	export class StreamerConfig {
	    screen_id: string;
	    schedule: string;
	    disabled?: boolean;
	    nickname?: string;
	    avatar?: string;
	    // Go type: time
	    metadata_updated_at?: any;
	    quality_mode?: string;
	    container_mode?: string;
	    auth_mode?: string;
	    telegram_enabled?: boolean;

	    static createFrom(source: any = {}) {
	        return new StreamerConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.screen_id = source["screen_id"];
	        this.schedule = source["schedule"];
	        this.disabled = source["disabled"];
	        this.nickname = source["nickname"];
	        this.avatar = source["avatar"];
	        this.metadata_updated_at = this.convertValues(source["metadata_updated_at"], null);
	        this.quality_mode = source["quality_mode"];
	        this.container_mode = source["container_mode"];
	        this.auth_mode = source["auth_mode"];
	        this.telegram_enabled = source["telegram_enabled"];
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
	export class Config {
	    Streamers: StreamerConfig[];
	    output_directory: string;
	    check_interval_seconds?: number;
	    auth_mode: string;
	    oauth: OAuthConfig;
	    cookies: CookieConfig;
	    recording: RecordingConfig;
	    proxy: ProxyConfig;
	    notifications: NotificationsConfig;

	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Streamers = this.convertValues(source["Streamers"], StreamerConfig);
	        this.output_directory = source["output_directory"];
	        this.check_interval_seconds = source["check_interval_seconds"];
	        this.auth_mode = source["auth_mode"];
	        this.oauth = this.convertValues(source["oauth"], OAuthConfig);
	        this.cookies = this.convertValues(source["cookies"], CookieConfig);
	        this.recording = this.convertValues(source["recording"], RecordingConfig);
	        this.proxy = this.convertValues(source["proxy"], ProxyConfig);
	        this.notifications = this.convertValues(source["notifications"], NotificationsConfig);
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

export namespace history {

	export class RecordingRecord {
	    id: string;
	    streamer_id: string;
	    title: string;
	    nickname?: string;
	    avatar?: string;
	    file_path: string;
	    file_size: number;
	    duration: string;
	    // Go type: time
	    start_time: any;
	    // Go type: time
	    end_time: any;
	    status: string;
	    error_code?: string;
	    error_summary?: string;
	    error_detail?: string;
	    // Go type: time
	    error_at?: any;
	    media_bitrate?: number;
	    video_bitrate?: number;
	    audio_bitrate?: number;
	    width?: number;
	    height?: number;
	    frame_rate?: number;
	    video_codec?: string;
	    audio_codec?: string;
	    comment_text_path?: string;
	    comment_jsonl_path?: string;
	    comment_text_exists?: boolean;
	    comment_jsonl_exists?: boolean;

	    static createFrom(source: any = {}) {
	        return new RecordingRecord(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.streamer_id = source["streamer_id"];
	        this.title = source["title"];
	        this.nickname = source["nickname"];
	        this.avatar = source["avatar"];
	        this.file_path = source["file_path"];
	        this.file_size = source["file_size"];
	        this.duration = source["duration"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.end_time = this.convertValues(source["end_time"], null);
	        this.status = source["status"];
	        this.error_code = source["error_code"];
	        this.error_summary = source["error_summary"];
	        this.error_detail = source["error_detail"];
	        this.error_at = this.convertValues(source["error_at"], null);
	        this.media_bitrate = source["media_bitrate"];
	        this.video_bitrate = source["video_bitrate"];
	        this.audio_bitrate = source["audio_bitrate"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.frame_rate = source["frame_rate"];
	        this.video_codec = source["video_codec"];
	        this.audio_codec = source["audio_codec"];
	        this.comment_text_path = source["comment_text_path"];
	        this.comment_jsonl_path = source["comment_jsonl_path"];
	        this.comment_text_exists = source["comment_text_exists"];
	        this.comment_jsonl_exists = source["comment_jsonl_exists"];
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

export namespace main {

	export class HealthCheckItem {
	    name: string;
	    status: string;
	    message: string;

	    static createFrom(source: any = {}) {
	        return new HealthCheckItem(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.message = source["message"];
	    }
	}
	export class HealthCheckReport {
	    ok: boolean;
	    items: HealthCheckItem[];

	    static createFrom(source: any = {}) {
	        return new HealthCheckReport(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.items = this.convertValues(source["items"], HealthCheckItem);
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
	export class RuntimeDiagnostics {
	    config_path: string;
	    history_path: string;
	    logs_dir: string;
	    output_directory: string;
	    streamer_count: number;
	    worker_enabled: boolean;
	    proxy_enabled: boolean;
	    oauth_configured: boolean;
	    cookie_enabled: boolean;
	    cookie_path: string;

	    static createFrom(source: any = {}) {
	        return new RuntimeDiagnostics(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config_path = source["config_path"];
	        this.history_path = source["history_path"];
	        this.logs_dir = source["logs_dir"];
	        this.output_directory = source["output_directory"];
	        this.streamer_count = source["streamer_count"];
	        this.worker_enabled = source["worker_enabled"];
	        this.proxy_enabled = source["proxy_enabled"];
	        this.oauth_configured = source["oauth_configured"];
	        this.cookie_enabled = source["cookie_enabled"];
	        this.cookie_path = source["cookie_path"];
	    }
	}
	export class SettingsPayload {
	    output_directory: string;
	    auth_mode: string;
	    oauth: config.OAuthConfig;
	    cookies: config.CookieConfig;
	    recording: config.RecordingConfig;
	    proxy: config.ProxyConfig;
	    notifications: config.NotificationsConfig;

	    static createFrom(source: any = {}) {
	        return new SettingsPayload(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.output_directory = source["output_directory"];
	        this.auth_mode = source["auth_mode"];
	        this.oauth = this.convertValues(source["oauth"], config.OAuthConfig);
	        this.cookies = this.convertValues(source["cookies"], config.CookieConfig);
	        this.recording = this.convertValues(source["recording"], config.RecordingConfig);
	        this.proxy = this.convertValues(source["proxy"], config.ProxyConfig);
	        this.notifications = this.convertValues(source["notifications"], config.NotificationsConfig);
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
	export class StreamerDiagnostics {
	    screen_id: string;
	    last_error: string;
	    last_file_path: string;
	    recent_logs: string[];

	    static createFrom(source: any = {}) {
	        return new StreamerDiagnostics(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.screen_id = source["screen_id"];
	        this.last_error = source["last_error"];
	        this.last_file_path = source["last_file_path"];
	        this.recent_logs = source["recent_logs"];
	    }
	}
	export class StreamerStatus {
	    screen_id: string;
	    schedule: string;
	    is_monitoring: boolean;
	    nickname: string;
	    avatar: string;
	    quality_mode: string;
	    container_mode: string;
	    auth_mode: string;
	    telegram_enabled: boolean;
	    last_error: string;
	    last_file_path: string;
	    recent_logs: string[];
	    current_status: string;
	    last_message: string;

	    static createFrom(source: any = {}) {
	        return new StreamerStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.screen_id = source["screen_id"];
	        this.schedule = source["schedule"];
	        this.is_monitoring = source["is_monitoring"];
	        this.nickname = source["nickname"];
	        this.avatar = source["avatar"];
	        this.quality_mode = source["quality_mode"];
	        this.container_mode = source["container_mode"];
	        this.auth_mode = source["auth_mode"];
	        this.telegram_enabled = source["telegram_enabled"];
	        this.last_error = source["last_error"];
	        this.last_file_path = source["last_file_path"];
	        this.recent_logs = source["recent_logs"];
	        this.current_status = source["current_status"];
	        this.last_message = source["last_message"];
	    }
	}

}

export namespace metadata {

	export class StreamerMetadata {
	    screen_id: string;
	    nickname: string;
	    avatar: string;
	    is_live: boolean;
	    live_title: string;

	    static createFrom(source: any = {}) {
	        return new StreamerMetadata(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.screen_id = source["screen_id"];
	        this.nickname = source["nickname"];
	        this.avatar = source["avatar"];
	        this.is_live = source["is_live"];
	        this.live_title = source["live_title"];
	    }
	}

}

export namespace recorder {

	export class ToolStatus {
	    ffmpeg_path: string;
	    ffmpeg_ok: boolean;
	    ffmpeg_version: string;
	    ffprobe_path: string;
	    ffprobe_ok: boolean;
	    ffprobe_version: string;
	    message: string;

	    static createFrom(source: any = {}) {
	        return new ToolStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ffmpeg_path = source["ffmpeg_path"];
	        this.ffmpeg_ok = source["ffmpeg_ok"];
	        this.ffmpeg_version = source["ffmpeg_version"];
	        this.ffprobe_path = source["ffprobe_path"];
	        this.ffprobe_ok = source["ffprobe_ok"];
	        this.ffprobe_version = source["ffprobe_version"];
	        this.message = source["message"];
	    }
	}

}
