// Write logs to JSON lines files with automatic size/time based rotation.
// Each call appends a single ND-JSON line. When the current file exceeds the
// configured size or max age, a new file with a timestamp suffix is created.

use chrono::{DateTime, Duration as ChronoDuration, Local};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs::{File, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};

const DEFAULT_MAX_SIZE_MB: u64 = 10; // 10 MB
const DEFAULT_MAX_AGE_HOURS: i64 = 12; // 12 h

#[derive(Serialize, Deserialize)]
pub struct EnrichedLogEntry {
    pub service_type: String,
    pub log_type: String,
    pub message: String,
    pub level: Option<LogLevel>,
    pub timestamp: String,
    pub stream: String,
    pub container_id: String,
    pub service_name: String,
    pub service_group: Option<String>,
    pub fields: HashMap<String, String>,
}

#[derive(Serialize, Deserialize, Clone)]
pub enum LogLevel {
    Debug,
    Info,
    Warn,
    Error,
    Fatal,
}

/// 内部共有状態（ファイルハンドルと生成時刻）
struct Inner {
    file: File,
    created_at: DateTime<Local>,
}

#[derive(Clone)]
pub struct JsonFileExporter {
    directory: PathBuf,
    base_name: String,
    inner: Arc<Mutex<Inner>>, // ここだけを Mutex で保護すれば済む
    max_size_bytes: u64,
    max_age: ChronoDuration,
}

impl JsonFileExporter {
    /// デフォルト（10 MB または 12 時間）設定で作成
    pub fn new(file_path: &str) -> Self {
        Self::with_rotation(file_path, DEFAULT_MAX_SIZE_MB, DEFAULT_MAX_AGE_HOURS)
    }

    /// 最大サイズ（MB）と最大経過時間（h）を指定して作成
    pub fn with_rotation(file_path: &str, max_size_mb: u64, max_age_hours: i64) -> Self {
        let path = Path::new(file_path);
        let directory = path.parent().unwrap_or(Path::new(".")).to_path_buf();
        let base_name = path.file_stem().unwrap().to_string_lossy().to_string();

        // ディレクトリが無い場合は作る
        std::fs::create_dir_all(&directory).ok();

        let file = Self::open_new_log_file(&directory, &base_name);

        Self {
            directory,
            base_name,
            inner: Arc::new(Mutex::new(Inner {
                file,
                created_at: Local::now(),
            })),
            max_size_bytes: max_size_mb * 1024 * 1024,
            max_age: ChronoDuration::hours(max_age_hours),
        }
    }

    fn open_new_log_file(dir: &Path, base_name: &str) -> File {
        let timestamp = Local::now().format("%Y%m%d_%H%M%S");
        let filename = format!("{}_{}.json", base_name, timestamp);
        let full_path = dir.join(filename);

        OpenOptions::new()
            .create(true)
            .write(true)
            .append(true)
            .open(full_path)
            .expect("Unable to create log file")
    }

    fn rotate_if_needed(&self, inner: &mut Inner) {
        let need_rotate_size = inner
            .file
            .metadata()
            .map(|m| m.len() >= self.max_size_bytes)
            .unwrap_or(false);

        let need_rotate_time = Local::now() - inner.created_at >= self.max_age;

        if need_rotate_size || need_rotate_time {
            // まず現在のファイルを flush/sync
            let _ = inner.file.flush();
            let _ = inner.file.sync_data();

            // 新しいファイルを開いて差し替え
            inner.file = Self::open_new_log_file(&self.directory, &self.base_name);
            inner.created_at = Local::now();
        }
    }

    pub fn export(&self, log: EnrichedLogEntry) {
        if let Ok(json) = serde_json::to_string(&log) {
            self.write_line(&json);
        } else {
            eprintln!("Error serializing log");
        }
    }

    /// 既にシリアライズ済みの 1 行 JSON をそのまま書き込む
    pub fn export_raw(&self, json_line: &str) {
        self.write_line(json_line);
    }

    fn write_line(&self, line: &str) {
        let mut inner = self.inner.lock().unwrap();

        if let Err(e) = writeln!(inner.file, "{}", line) {
            eprintln!("Error writing to file: {}", e);
            return;
        }

        // flush & rotate & fsync
        let _ = inner.file.flush();
        self.rotate_if_needed(&mut inner);
        let _ = inner.file.sync_data();
    }
}
