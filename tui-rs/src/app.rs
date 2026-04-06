use std::time::Instant;

use crate::client::{Album, DaemonClient, LibraryEntry, QueueItem, SpeakerInfo, TrackInfo};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Tab {
    NowPlaying = 0,
    Queue = 1,
    Library = 2,
    Albums = 3,
}

impl Tab {
    pub fn from_index(i: usize) -> Self {
        match i {
            0 => Tab::NowPlaying,
            1 => Tab::Queue,
            2 => Tab::Library,
            3 => Tab::Albums,
            _ => Tab::NowPlaying,
        }
    }

    pub fn next(self) -> Self {
        Tab::from_index(((self as usize) + 1) % 4)
    }

    pub fn prev(self) -> Self {
        Tab::from_index(((self as usize) + 3) % 4)
    }

    pub fn label(self) -> &'static str {
        match self {
            Tab::NowPlaying => "Now Playing",
            Tab::Queue => "Queue",
            Tab::Library => "Library",
            Tab::Albums => "Albums",
        }
    }
}

pub const ALL_TABS: [Tab; 4] = [Tab::NowPlaying, Tab::Queue, Tab::Library, Tab::Albums];

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum InputMode {
    Normal,
    Command,
    Search,
}

pub struct App {
    pub connected: bool,

    // Playback state
    pub transport: String,
    pub track: TrackInfo,
    pub volume: i32,
    pub elapsed: i32,
    pub duration: i32,
    pub is_line_in: bool,
    pub speakers: Vec<SpeakerInfo>,
    pub speaker: Option<SpeakerInfo>,
    pub library_ready: bool,

    // UI state
    pub active_tab: Tab,
    pub input_mode: InputMode,
    pub help_active: bool,
    pub status_msg: String,
    pub status_expiry: Option<Instant>,
    pub g_pending: bool,
    pub g_pending_expiry: Option<Instant>,
    pub should_quit: bool,

    // Command line
    pub cmd_input: String,

    // Art state
    pub art_url: String,
    pub art_image_data: Option<ArtImageData>,

    // Queue tab
    pub queue: QueueState,

    // Library tab
    pub library: LibraryState,

    // Albums tab
    pub albums: AlbumState,
}

#[derive(Debug)]
pub struct ArtImageData {
    pub rgba: Vec<u8>,
    pub width: u32,
    pub height: u32,
}

pub struct QueueState {
    pub items: Vec<QueueItem>,
    pub cursor: usize,
    pub dd_pending: bool,
    pub dd_pending_expiry: Option<Instant>,
    pub confirm_clear: bool,
}

pub struct LibraryState {
    pub current_path: String,
    pub entries: Vec<LibraryEntry>,
    pub cursor: usize,
    pub columns: Vec<LibraryColumn>,
    pub active_column: usize,
    pub searching: bool,
    pub search_query: String,
    pub search_results: Vec<LibraryEntry>,
    pub search_cursor: usize,
}

pub struct LibraryColumn {
    pub title: String,
    pub entries: Vec<LibraryEntry>,
    pub cursor: usize,
}

pub struct AlbumState {
    pub albums: Vec<Album>,
    pub cursor: usize,
    pub expand_tracks: Vec<LibraryEntry>,
    pub preview_id: String,
    pub searching: bool,
    pub search_query: String,
    pub search_results: Vec<Album>,
    pub scan_progress: f64,
    pub scan_status: String,
}

impl App {
    pub fn new(client: DaemonClient) -> Self {
        let _ = client;
        Self {
            connected: false,
            transport: "STOPPED".to_string(),
            track: TrackInfo::default(),
            volume: 0,
            elapsed: 0,
            duration: 0,
            is_line_in: false,
            speakers: Vec::new(),
            speaker: None,
            library_ready: false,
            active_tab: Tab::NowPlaying,
            input_mode: InputMode::Normal,
            help_active: false,
            status_msg: String::new(),
            status_expiry: None,
            g_pending: false,
            g_pending_expiry: None,
            should_quit: false,
            cmd_input: String::new(),
            art_url: String::new(),
            art_image_data: None,
            queue: QueueState {
                items: Vec::new(),
                cursor: 0,
                dd_pending: false,
                dd_pending_expiry: None,
                confirm_clear: false,
            },
            library: LibraryState {
                current_path: String::new(),
                entries: Vec::new(),
                cursor: 0,
                columns: Vec::new(),
                active_column: 0,
                searching: false,
                search_query: String::new(),
                search_results: Vec::new(),
                search_cursor: 0,
            },
            albums: AlbumState {
                albums: Vec::new(),
                cursor: 0,
                expand_tracks: Vec::new(),
                preview_id: String::new(),
                searching: false,
                search_query: String::new(),
                search_results: Vec::new(),
                scan_progress: 0.0,
                scan_status: String::new(),
            },
        }
    }

    pub fn set_status(&mut self, msg: impl Into<String>) {
        self.status_msg = msg.into();
        self.status_expiry = Some(Instant::now() + std::time::Duration::from_secs(4));
    }

    pub fn clear_expired_status(&mut self) {
        if let Some(expiry) = self.status_expiry {
            if Instant::now() >= expiry {
                self.status_msg.clear();
                self.status_expiry = None;
            }
        }
    }

    pub fn set_g_pending(&mut self) {
        self.g_pending = true;
        self.g_pending_expiry = Some(Instant::now() + std::time::Duration::from_secs(2));
    }

    pub fn clear_g_pending(&mut self) {
        self.g_pending = false;
        self.g_pending_expiry = None;
    }

    pub fn set_dd_pending(&mut self) {
        self.queue.dd_pending = true;
        self.queue.dd_pending_expiry = Some(Instant::now() + std::time::Duration::from_secs(3));
    }

    pub fn clear_dd_pending(&mut self) {
        self.queue.dd_pending = false;
        self.queue.dd_pending_expiry = None;
    }

    pub fn clear_expired_pending(&mut self) {
        if let Some(expiry) = self.g_pending_expiry {
            if Instant::now() >= expiry {
                self.g_pending = false;
                self.g_pending_expiry = None;
            }
        }
        if let Some(expiry) = self.queue.dd_pending_expiry {
            if Instant::now() >= expiry {
                self.queue.dd_pending = false;
                self.queue.dd_pending_expiry = None;
            }
        }
    }

    pub fn cycle_speaker(&mut self) -> Option<String> {
        if self.speakers.len() <= 1 {
            return None;
        }
        let current_idx = self
            .speaker
            .as_ref()
            .and_then(|s| self.speakers.iter().position(|sp| sp.uuid == s.uuid))
            .unwrap_or(0);
        let next_idx = (current_idx + 1) % self.speakers.len();
        let next = &self.speakers[next_idx];
        let uuid = next.uuid.clone();
        Some(uuid)
    }

    pub fn apply_status(&mut self, status: crate::client::StatusResponse) {
        self.connected = true;
        self.transport = status.transport;
        self.track = status.track;
        self.art_url = self.track.art_url.clone();
        self.volume = status.volume;
        self.elapsed = status.elapsed;
        self.duration = status.duration;
        self.is_line_in = status.is_line_in;
        self.speaker = status.speaker;
        self.library_ready = status.library_ready;
    }

    pub fn transport_label(&self) -> &str {
        match self.transport.as_str() {
            "PLAYING" => "Playing",
            "PAUSED_PLAYBACK" => "Paused",
            "STOPPED" => "Stopped",
            "TRANSITIONING" => "Loading…",
            _ => &self.transport,
        }
    }

    pub fn active_speaker_name(&self) -> &str {
        self.speaker
            .as_ref()
            .map(|s| s.name.as_str())
            .filter(|name| !name.is_empty())
            .unwrap_or("No speaker")
    }

    pub fn current_album_name(&self) -> String {
        self.current_track_album()
    }

    pub fn current_queue_item(&self) -> Option<&QueueItem> {
        if !self.track.uri.is_empty() {
            if let Some(item) = self
                .queue
                .items
                .iter()
                .find(|item| item.uri == self.track.uri)
            {
                return Some(item);
            }
        }

        self.queue.items.iter().find(|item| {
            let title_matches = !self.track.title.is_empty()
                && normalize_text(&item.title) == normalize_text(&self.track.title);
            let artist_matches = self.track.artist.is_empty()
                || normalize_text(&item.artist) == normalize_text(&self.track.artist);
            let album_matches = self.track.album.is_empty()
                || normalize_text(&item.album) == normalize_text(&self.track.album);
            title_matches && artist_matches && album_matches
        })
    }

    pub fn current_track_title(&self) -> String {
        if !self.track.title.is_empty() {
            self.track.title.clone()
        } else if let Some(item) = self.current_queue_item() {
            item.title.clone()
        } else {
            "Nothing playing".to_string()
        }
    }

    pub fn current_track_artist(&self) -> String {
        if !self.track.artist.is_empty() {
            self.track.artist.clone()
        } else if let Some(item) = self.current_queue_item() {
            if item.artist.is_empty() {
                "Unknown artist".to_string()
            } else {
                item.artist.clone()
            }
        } else {
            "Unknown artist".to_string()
        }
    }

    pub fn current_track_album(&self) -> String {
        if !self.track.album.is_empty() {
            self.track.album.clone()
        } else if let Some(item) = self.current_queue_item() {
            if item.album.is_empty() {
                "Unknown album".to_string()
            } else {
                item.album.clone()
            }
        } else {
            "Unknown album".to_string()
        }
    }

    pub fn effective_duration(&self) -> i32 {
        if self.duration > 0 {
            self.duration
        } else if self.track.duration > 0 {
            self.track.duration
        } else {
            self.current_queue_item()
                .map(|item| item.duration)
                .unwrap_or(0)
        }
    }
}

pub fn format_duration(secs: i32) -> String {
    let m = secs / 60;
    let s = secs % 60;
    format!("{m}:{s:02}")
}

fn normalize_text(input: &str) -> String {
    input
        .chars()
        .filter(|ch| ch.is_alphanumeric())
        .flat_map(|ch| ch.to_lowercase())
        .collect()
}
