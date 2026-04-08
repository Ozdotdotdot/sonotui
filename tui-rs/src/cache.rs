use std::{fs, path::PathBuf};

use serde::{Deserialize, Serialize};

use crate::client::TrackInfo;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CachedState {
    pub transport: String,
    pub track: TrackInfo,
    pub volume: i32,
    pub elapsed: i32,
}

fn cache_path() -> PathBuf {
    dirs::cache_dir()
        .unwrap_or_else(|| std::path::PathBuf::from("."))
        .join("sonotui")
        .join("state.json")
}

pub fn load() -> Option<CachedState> {
    let path = cache_path();
    let text = fs::read_to_string(&path).ok()?;
    serde_json::from_str(&text).ok()
}

pub fn save(state: &CachedState) {
    let path = cache_path();
    if let Some(parent) = path.parent() {
        let _ = fs::create_dir_all(parent);
    }
    if let Ok(text) = serde_json::to_string(state) {
        let _ = fs::write(path, text);
    }
}
