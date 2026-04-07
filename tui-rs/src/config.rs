use std::{fs, path::PathBuf};

use serde::{Deserialize, Serialize};

#[derive(Debug, Deserialize, Serialize)]
#[serde(default)]
pub struct Config {
    pub host: Option<String>,
    pub port: Option<u16>,
}

impl Default for Config {
    fn default() -> Self {
        Self { host: None, port: None }
    }
}

impl Config {
    /// Returns (host, port) only if a host was explicitly saved.
    pub fn saved_server(&self) -> Option<(String, u16)> {
        self.host
            .as_deref()
            .map(|h| (h.to_string(), self.port.unwrap_or(8989)))
    }
}

pub fn config_path() -> PathBuf {
    let base = std::env::var("XDG_CONFIG_HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|_| dirs::home_dir().unwrap_or_default().join(".config"));
    base.join("sonotui").join("config.toml")
}

pub fn load() -> Config {
    let path = config_path();
    let Ok(text) = fs::read_to_string(&path) else {
        return Config::default();
    };
    toml::from_str(&text).unwrap_or_default()
}

pub fn save_server(host: &str, port: u16) {
    let cfg = Config {
        host: Some(host.to_string()),
        port: Some(port),
    };
    let Ok(text) = toml::to_string(&cfg) else { return };
    let path = config_path();
    if let Some(parent) = path.parent() {
        let _ = fs::create_dir_all(parent);
    }
    let _ = fs::write(path, text);
}
