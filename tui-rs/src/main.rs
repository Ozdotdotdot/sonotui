mod app;
mod client;
mod kitty;
mod theme;
mod ui;

use std::{
    io::{self, Stdout, Write},
    sync::mpsc::{self, Receiver, Sender},
    time::{Duration, Instant},
};
use tokio::runtime::Handle;

use anyhow::{Context, Result};
use app::{App, ArtImageData, InputMode, Tab};
use clap::{Parser, ValueEnum};
use client::{DaemonClient, DaemonEvent};
use crossterm::{
    cursor::MoveTo,
    event::{self, Event as CEvent, KeyCode, KeyEvent, KeyEventKind, KeyModifiers},
    execute,
    terminal::{
        self, disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen,
    },
};
use image::ImageReader;
use ratatui::{
    backend::CrosstermBackend,
    layout::{Constraint, Layout, Rect},
    Terminal,
};

#[derive(Parser, Debug)]
#[command(name = "sonotui")]
struct Args {
    #[arg(long, default_value = "127.0.0.1")]
    host: String,
    #[arg(long, default_value_t = 8989)]
    port: u16,
    #[arg(long, default_value = "auto")]
    art: ArtCliMode,
}

#[derive(Clone, Copy, Debug, ValueEnum)]
enum ArtCliMode {
    Auto,
    Kitty,
    Halfblock,
    None,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum ArtMode {
    Kitty,
    Halfblock,
    None,
}

#[derive(Debug)]
enum AppEvent {
    Tick,
    Key(KeyEvent),
    Resize(u16, u16),
    Connected(client::StatusResponse),
    StatusRefresh(client::StatusResponse),
    QueueLoaded(Vec<client::QueueItem>),
    QueueCleared,
    SpeakersLoaded(Vec<client::SpeakerInfo>),
    LibraryColumnLoaded {
        depth: usize,
        path: String,
        title: String,
        entries: Vec<client::LibraryEntry>,
    },
    LibrarySearchLoaded(Vec<client::LibraryEntry>),
    AlbumsLoaded(Vec<client::Album>),
    AlbumsSearchLoaded(Vec<client::Album>),
    AlbumDetailLoaded {
        id: String,
        detail: client::AlbumDetail,
    },
    ArtLoaded {
        url: String,
        image: Option<ArtImageData>,
    },
    ArtEncoded {
        signature: String,
        data: kitty::KittyImageData,
    },
    Sse(DaemonEvent),
    Status(String),
    Error(String),
}

struct TerminalGuard {
    terminal: Terminal<CrosstermBackend<Stdout>>,
}

impl TerminalGuard {
    fn new() -> Result<Self> {
        enable_raw_mode().context("enable raw mode")?;
        let mut stdout = io::stdout();
        execute!(stdout, EnterAlternateScreen).context("enter alternate screen")?;
        let backend = CrosstermBackend::new(stdout);
        let terminal = Terminal::new(backend).context("create terminal")?;
        Ok(Self { terminal })
    }
}

impl Drop for TerminalGuard {
    fn drop(&mut self) {
        let _ = disable_raw_mode();
        let _ = execute!(self.terminal.backend_mut(), LeaveAlternateScreen);
        let _ = self.terminal.show_cursor();
    }
}

#[derive(Default)]
struct DirectArtState {
    active: bool,
    area: Option<Rect>,
    signature: String,
    /// Pre-encoded Kitty payload, keyed by signature. Populated by background task.
    cached_kitty: Option<(String, kitty::KittyImageData)>,
    /// Signature of an encode currently in-flight (prevents duplicate spawns).
    encode_pending: Option<String>,
}

const STATUS_POLL_INTERVAL: u32 = 150; // ~30s at 200ms tick

fn main() -> Result<()> {
    let args = Args::parse();
    let client = DaemonClient::new(&args.host, args.port);
    let art_mode = detect_art_mode(args.art);
    let mut app = App::new(client.clone());
    let (tx, rx) = mpsc::channel();

    let runtime = tokio::runtime::Builder::new_multi_thread()
        .worker_threads(2)
        .enable_all()
        .build()
        .context("create tokio runtime")?;
    let handle = runtime.handle().clone();

    spawn_tick_thread(tx.clone(), Duration::from_millis(200));
    spawn_input_thread(tx.clone());
    spawn_status_load(&handle, client.clone(), tx.clone());

    let mut term = TerminalGuard::new()?;
    let mut art_state = DirectArtState::default();
    run_loop(
        &mut term.terminal,
        &mut app,
        rx,
        tx,
        client,
        art_mode,
        &mut art_state,
        &handle,
    )
}

fn run_loop(
    terminal: &mut Terminal<CrosstermBackend<Stdout>>,
    app: &mut App,
    rx: Receiver<AppEvent>,
    tx: Sender<AppEvent>,
    client: DaemonClient,
    art_mode: ArtMode,
    art_state: &mut DirectArtState,
    handle: &Handle,
) -> Result<()> {
    let mut render_wanted = true;
    let frame_duration = Duration::from_secs_f64(1.0 / 30.0);
    let mut last_render = Instant::now() - Duration::from_secs(1);
    let mut resize_pending: Option<Instant> = None;
    let mut tick_count: u32 = 0;
    const RESIZE_DEBOUNCE: Duration = Duration::from_millis(150);
    // STATUS_POLL_INTERVAL defined at module level

    loop {
        // ── 1. Wait for / receive next event ─────────────────────────────────
        // Smart blocking: short timeout when a render is pending so we don't
        // overshoot the frame budget; block indefinitely when idle (zero CPU).
        let event = if render_wanted || resize_pending.is_some() {
            let timeout = if render_wanted {
                frame_duration.saturating_sub(last_render.elapsed())
            } else {
                RESIZE_DEBOUNCE.saturating_sub(
                    resize_pending.map(|w| w.elapsed()).unwrap_or_default(),
                )
            };
            match rx.recv_timeout(timeout) {
                Ok(evt) => Some(evt),
                Err(mpsc::RecvTimeoutError::Timeout) => None,
                Err(mpsc::RecvTimeoutError::Disconnected) => {
                    return Err(anyhow::anyhow!("event channel closed"));
                }
            }
        } else {
            Some(rx.recv().context("event channel closed")?)
        };

        // ── 2. Process event + drain the queue ───────────────────────────────
        // Draining here — BEFORE rendering — ensures that a tab-switch keypress
        // already queued updates `active_tab` before we call sync_direct_art,
        // so the Kitty write is skipped when the user has already left the tab.
        if let Some(event) = event {
            process_event(
                app, event, &tx, &client, handle, art_mode,
                &mut render_wanted, &mut resize_pending, &mut tick_count,
                art_state,
            );
        }
        while let Ok(event) = rx.try_recv() {
            process_event(
                app, event, &tx, &client, handle, art_mode,
                &mut render_wanted, &mut resize_pending, &mut tick_count,
                art_state,
            );
        }

        if app.should_quit {
            clear_direct_art(terminal.backend_mut(), art_state)?;
            break;
        }

        // ── 3. Handle debounced resize ────────────────────────────────────────
        if let Some(when) = resize_pending {
            if when.elapsed() >= RESIZE_DEBOUNCE {
                resize_pending = None;
                clear_direct_art(terminal.backend_mut(), art_state)?;
                terminal.clear()?;
                render_wanted = true;
            }
        }

        // ── 4. Render when needed and frame budget allows ─────────────────────
        if render_wanted && last_render.elapsed() >= frame_duration {
            let should_clear_direct_art = art_state.active
                && (art_mode == ArtMode::None
                    || app.active_tab != Tab::NowPlaying
                    || app.help_active
                    || app.art_image_data.is_none());
            if should_clear_direct_art {
                clear_direct_art(terminal.backend_mut(), art_state)?;
            }

            let mut art_area = None;
            terminal.draw(|f| {
                let layout = Layout::vertical([
                    Constraint::Length(3),
                    Constraint::Length(2),
                    Constraint::Min(1),
                    Constraint::Length(1),
                ])
                .split(f.area());

                ui::common::render_header(f, layout[0], app);
                ui::render_tab_bar(f, layout[1], app);

                if !app.connected {
                    ui::common::render_connecting(f, layout[2], app);
                } else {
                    match app.active_tab {
                        Tab::NowPlaying => {
                            ui::now_playing::render(f, layout[2], app, art_mode == ArtMode::None);
                            art_area = Some(ui::now_playing::art_area(layout[2], app));
                        }
                        Tab::Queue => ui::queue::render(f, layout[2], app),
                        Tab::Library => ui::library::render(f, layout[2], app),
                        Tab::Albums => ui::albums::render(f, layout[2], app),
                    }
                }

                ui::render_command_line(f, layout[3], app);
                ui::render_help_overlay(f, f.area(), app);
            })?;

            // Kick off background Kitty encode if the signature has changed.
            if art_mode == ArtMode::Kitty
                && app.active_tab == Tab::NowPlaying
                && !app.help_active
            {
                if let (Some(area), Some(image)) = (art_area, app.art_image_data.as_ref()) {
                    let placement =
                        kitty::align_image_to_area(area, image.width, image.height);
                    let sig = format!(
                        "{}:{}:{}:{}:{}",
                        app.art_url, area.x, area.y, area.width, area.height
                    );
                    let cache_hit = art_state
                        .cached_kitty
                        .as_ref()
                        .map(|(s, _)| s == &sig)
                        .unwrap_or(false);
                    let already_pending = art_state
                        .encode_pending
                        .as_ref()
                        .map(|s| s == &sig)
                        .unwrap_or(false);
                    if !cache_hit && !already_pending {
                        art_state.encode_pending = Some(sig.clone());
                        let rgba = image.rgba.clone();
                        let w = image.width;
                        let h = image.height;
                        let tx2 = tx.clone();
                        handle.spawn(async move {
                            let result = tokio::task::spawn_blocking(move || {
                                let (resized, rw, rh) = kitty::resize_image_exact(
                                    &rgba, w, h,
                                    placement.pixel_width,
                                    placement.pixel_height,
                                );
                                kitty::encode_kitty_payload(&resized, rw, rh)
                            })
                            .await;
                            if let Ok(data) = result {
                                let _ = tx2.send(AppEvent::ArtEncoded {
                                    signature: sig,
                                    data,
                                });
                            }
                        });
                    }
                }
            }

            sync_direct_art(terminal.backend_mut(), app, art_mode, art_area, art_state)?;
            render_wanted = false;
            last_render = Instant::now();
        }
    }

    Ok(())
}

#[allow(clippy::too_many_arguments)]
fn process_event(
    app: &mut App,
    event: AppEvent,
    tx: &Sender<AppEvent>,
    client: &DaemonClient,
    handle: &Handle,
    _art_mode: ArtMode,
    render_wanted: &mut bool,
    resize_pending: &mut Option<Instant>,
    tick_count: &mut u32,
    art_state: &mut DirectArtState,
) {
    match event {
        AppEvent::Tick => {
            let had_status = app.status_expiry.is_some();
            app.clear_expired_status();
            if had_status && app.status_expiry.is_none() {
                *render_wanted = true;
            }
            if app.transport == "PLAYING" {
                *render_wanted = true;
            }
            *tick_count += 1;
            if app.connected && *tick_count % STATUS_POLL_INTERVAL == 0 {
                let c = client.clone();
                let t = tx.clone();
                handle.spawn(async move {
                    if let Ok(status) = c.status().await {
                        let _ = t.send(AppEvent::StatusRefresh(status));
                    }
                });
            }
        }
        AppEvent::Key(key) if key.kind == KeyEventKind::Press => {
            handle_key(app, key, tx, client, handle);
            *render_wanted = true;
        }
        AppEvent::Key(_) => {}
        AppEvent::Resize(_w, _h) => {
            *resize_pending = Some(Instant::now());
        }
        AppEvent::Connected(status) => {
            app.apply_status(status);
            app.albums.scan_status = if app.library_ready {
                "done".to_string()
            } else {
                "scanning".to_string()
            };
            spawn_sse(handle, client.clone(), tx.clone());
            spawn_queue_load(handle, client.clone(), tx.clone());
            spawn_speakers_load(handle, client.clone(), tx.clone());
            spawn_library_column_load(
                handle,
                client.clone(),
                tx.clone(),
                0,
                "/".to_string(),
                "Library".to_string(),
            );
            if app.library_ready {
                spawn_albums_load(handle, client.clone(), tx.clone());
            }
            if !app.art_url.is_empty() {
                spawn_art_load(handle, client.clone(), tx.clone(), app.art_url.clone());
            }
            *render_wanted = true;
        }
        AppEvent::StatusRefresh(status) => {
            if status.track.uri != app.track.uri || status.transport != app.transport {
                let art_changed = status.track.art_url != app.art_url;
                app.apply_status(status);
                if art_changed && !app.art_url.is_empty() {
                    app.art_image_data = None;
                    spawn_art_load(handle, client.clone(), tx.clone(), app.art_url.clone());
                }
                *render_wanted = true;
            }
        }
        AppEvent::QueueLoaded(items) => {
            app.queue.items = items;
            if app.queue.cursor >= app.queue.items.len() {
                app.queue.cursor = app.queue.items.len().saturating_sub(1);
            }
            *render_wanted = true;
        }
        AppEvent::QueueCleared => {
            app.queue.items.clear();
            app.queue.cursor = 0;
            app.queue.dd_pending = false;
            app.queue.confirm_clear = false;
            app.set_status("Queue cleared");
            *render_wanted = true;
        }
        AppEvent::SpeakersLoaded(speakers) => {
            app.speakers = speakers;
            *render_wanted = true;
        }
        AppEvent::LibraryColumnLoaded {
            depth,
            path,
            title,
            entries,
        } => {
            if depth == 0 {
                app.library.current_path = path.clone();
                app.library.entries = entries.clone();
                app.library.cursor = 0;
                app.library.columns.clear();
                app.library.columns.push(app::LibraryColumn {
                    title,
                    entries,
                    cursor: 0,
                });
                app.library.active_column = 0;
                queue_library_preview(tx, client, app, 0, handle);
            } else if depth <= 2 && library_preview_matches(app, depth, &path) {
                app.library.columns.truncate(depth);
                app.library.columns.push(app::LibraryColumn {
                    title,
                    entries,
                    cursor: 0,
                });
                if app.library.active_column > app.library.columns.len().saturating_sub(1) {
                    app.library.active_column = app.library.columns.len().saturating_sub(1);
                }
                if depth < 2 {
                    queue_library_preview(tx, client, app, depth, handle);
                }
            }
            *render_wanted = true;
        }
        AppEvent::LibrarySearchLoaded(results) => {
            app.library.search_results = results;
            app.library.search_cursor = 0;
            *render_wanted = true;
        }
        AppEvent::AlbumsLoaded(albums) => {
            let mut albums = albums;
            albums.sort_by(|a, b| {
                a.title
                    .to_lowercase()
                    .cmp(&b.title.to_lowercase())
                    .then(a.artist.to_lowercase().cmp(&b.artist.to_lowercase()))
            });
            app.albums.albums = albums;
            if app.albums.cursor >= app.albums.albums.len() {
                app.albums.cursor = app.albums.albums.len().saturating_sub(1);
            }
            queue_album_preview(tx, client, app, handle);
            *render_wanted = true;
        }
        AppEvent::AlbumsSearchLoaded(albums) => {
            let mut albums = albums;
            albums.sort_by(|a, b| {
                a.title
                    .to_lowercase()
                    .cmp(&b.title.to_lowercase())
                    .then(a.artist.to_lowercase().cmp(&b.artist.to_lowercase()))
            });
            app.albums.search_results = albums;
            app.albums.cursor = 0;
            queue_album_preview(tx, client, app, handle);
            *render_wanted = true;
        }
        AppEvent::AlbumDetailLoaded { id, detail } => {
            if app.albums.preview_id == id {
                app.albums.expand_tracks = detail.tracks;
                *render_wanted = true;
            }
        }
        AppEvent::ArtLoaded { url, image } => {
            if url == app.art_url {
                // Invalidate any cached Kitty encode — image bytes have changed.
                art_state.cached_kitty = None;
                art_state.encode_pending = None;
                app.art_image_data = image;
                *render_wanted = true;
            }
        }
        AppEvent::ArtEncoded { signature, data } => {
            // Background encode finished. Store result and trigger a render.
            if art_state.encode_pending.as_deref() == Some(&signature) {
                art_state.encode_pending = None;
            }
            art_state.cached_kitty = Some((signature, data));
            *render_wanted = true;
        }
        AppEvent::Sse(evt) => {
            handle_sse(app, evt, tx, client, handle);
            *render_wanted = true;
        }
        AppEvent::Status(msg) => {
            app.set_status(msg);
            *render_wanted = true;
        }
        AppEvent::Error(err) => {
            app.set_status(err);
            *render_wanted = true;
        }
    }
}

fn handle_sse(app: &mut App, evt: DaemonEvent, tx: &Sender<AppEvent>, client: &DaemonClient, handle: &Handle) {
    match evt.kind.as_str() {
        "transport" => {
            if let Some(state) = evt.payload.get("state").and_then(|v| v.as_str()) {
                app.transport = state.to_string();
            }
        }
        "track" => {
            if let Some(title) = evt.payload.get("title").and_then(|v| v.as_str()) {
                app.track.title = title.to_string();
            }
            if let Some(artist) = evt.payload.get("artist").and_then(|v| v.as_str()) {
                app.track.artist = artist.to_string();
            }
            if let Some(album) = evt.payload.get("album").and_then(|v| v.as_str()) {
                app.track.album = album.to_string();
            }
            if let Some(duration) = evt.payload.get("duration").and_then(|v| v.as_i64()) {
                app.duration = duration as i32;
                app.track.duration = duration as i32;
            }
            if let Some(uri) = evt.payload.get("uri").and_then(|v| v.as_str()) {
                app.track.uri = uri.to_string();
            }
            if let Some(url) = evt.payload.get("art_url").and_then(|v| v.as_str()) {
                if url != app.art_url {
                    app.art_url = url.to_string();
                    app.track.art_url = url.to_string();
                    app.art_image_data = None;
                    if !url.is_empty() {
                        spawn_art_load(handle, client.clone(), tx.clone(), url.to_string());
                    }
                }
            }
        }
        "position" => {
            if let Some(elapsed) = evt.payload.get("elapsed").and_then(|v| v.as_i64()) {
                app.elapsed = elapsed as i32;
            }
            if let Some(duration) = evt.payload.get("duration").and_then(|v| v.as_i64()) {
                app.duration = duration as i32;
            }
        }
        "volume" => {
            if let Some(volume) = evt.payload.get("value").and_then(|v| v.as_i64()) {
                app.volume = volume as i32;
            }
        }
        "linein" => {
            if let Some(active) = evt.payload.get("active").and_then(|v| v.as_bool()) {
                app.is_line_in = active;
            }
        }
        "queue_changed" => spawn_queue_load(handle, client.clone(), tx.clone()),
        "speaker" => {
            let name = evt
                .payload
                .get("name")
                .and_then(|v| v.as_str())
                .unwrap_or_default();
            let uuid = evt
                .payload
                .get("uuid")
                .and_then(|v| v.as_str())
                .unwrap_or_default();
            app.speaker = Some(client::SpeakerInfo {
                name: name.to_string(),
                uuid: uuid.to_string(),
                ip: String::new(),
            });
            spawn_speakers_load(handle, client.clone(), tx.clone());
        }
        "library_scan" => {
            if let Some(status) = evt.payload.get("status").and_then(|v| v.as_str()) {
                app.albums.scan_status = status.to_string();
                app.library_ready = status == "done";
            }
            if let Some(progress) = evt.payload.get("progress").and_then(|v| v.as_f64()) {
                app.albums.scan_progress = progress;
            }
            if app.library_ready && app.albums.albums.is_empty() {
                spawn_albums_load(handle, client.clone(), tx.clone());
            }
        }
        "error" => {
            if let Some(msg) = evt.payload.get("message").and_then(|v| v.as_str()) {
                app.set_status(format!("Daemon: {msg}"));
            }
        }
        _ => {}
    }
}

fn handle_key(app: &mut App, key: KeyEvent, tx: &Sender<AppEvent>, client: &DaemonClient, handle: &Handle) {
    if app.help_active {
        if matches!(
            key.code,
            KeyCode::Esc | KeyCode::Char('?') | KeyCode::Char('q')
        ) {
            app.help_active = false;
        }
        return;
    }

    if app.input_mode == InputMode::Command {
        handle_command_input(app, key, tx, client, handle);
        return;
    }

    if app.input_mode == InputMode::Search {
        match app.active_tab {
            Tab::Library => handle_library_search(app, key, tx, client, handle),
            Tab::Albums => handle_album_search(app, key, tx, client, handle),
            _ => {}
        }
        return;
    }

    if key.modifiers.contains(KeyModifiers::CONTROL) && key.code == KeyCode::Char('c') {
        app.should_quit = true;
        return;
    }

    match key.code {
        KeyCode::Char('q') => {
            app.should_quit = true;
            return;
        }
        KeyCode::Char('?') => {
            app.help_active = true;
            app.g_pending = false;
            return;
        }
        KeyCode::Char(':') => {
            app.input_mode = InputMode::Command;
            app.cmd_input.clear();
            app.g_pending = false;
            return;
        }
        KeyCode::Char('1') => {
            app.active_tab = Tab::NowPlaying;
            app.g_pending = false;
            return;
        }
        KeyCode::Char('2') => {
            app.active_tab = Tab::Queue;
            app.g_pending = false;
            return;
        }
        KeyCode::Char('3') => {
            app.active_tab = Tab::Library;
            app.g_pending = false;
            return;
        }
        KeyCode::Char('4') => {
            app.active_tab = Tab::Albums;
            app.g_pending = false;
            if app.library_ready && app.albums.albums.is_empty() {
                spawn_albums_load(handle, client.clone(), tx.clone());
            }
            return;
        }
        KeyCode::Char('g') => {
            if app.g_pending {
                match app.active_tab {
                    Tab::Queue => app.queue.cursor = 0,
                    Tab::Library => {
                        if app.library.searching {
                            app.library.search_cursor = 0;
                        } else {
                            app.library.cursor = 0;
                        }
                    }
                    Tab::Albums => app.albums.cursor = 0,
                    Tab::NowPlaying => {}
                }
                app.g_pending = false;
            } else {
                app.g_pending = true;
            }
            return;
        }
        KeyCode::Char('t') if app.g_pending => {
            app.active_tab = app.active_tab.next();
            app.g_pending = false;
            return;
        }
        KeyCode::Char('T') if app.g_pending => {
            app.active_tab = app.active_tab.prev();
            app.g_pending = false;
            return;
        }
        _ => {}
    }

    app.g_pending = false;

    match key.code {
        KeyCode::Char(' ') => {
            let transport = app.transport.clone();
            let line_in = app.is_line_in;
            let queue_len = app.queue.items.len();
            let client = client.clone();
            spawn_task(handle, tx.clone(), async move {
                if transport == "PLAYING" && !line_in {
                    client.pause().await?;
                } else if line_in && queue_len > 0 {
                    client.queue_play(1).await?;
                } else {
                    client.play().await?;
                }
                Ok(AppEvent::Status(String::new()))
            });
        }
        KeyCode::Char('s') => spawn_simple(handle, client.clone(), tx.clone(), SimpleAction::Stop),
        KeyCode::Char('<') | KeyCode::Char(',') => {
            spawn_simple(handle, client.clone(), tx.clone(), SimpleAction::Prev)
        }
        KeyCode::Char('>') | KeyCode::Char('.') => {
            spawn_simple(handle, client.clone(), tx.clone(), SimpleAction::Next)
        }
        KeyCode::Char('l') => spawn_simple(handle, client.clone(), tx.clone(), SimpleAction::LineIn),
        KeyCode::Tab => {
            if let Some(uuid) = app.cycle_speaker() {
                spawn_set_speaker(handle, client.clone(), tx.clone(), uuid);
            }
        }
        _ => match app.active_tab {
            Tab::NowPlaying => handle_now_playing_key(key, tx, client, handle),
            Tab::Queue => handle_queue_key(app, key, tx, client, handle),
            Tab::Library => handle_library_key(app, key, tx, client, handle),
            Tab::Albums => handle_album_key(app, key, tx, client, handle),
        },
    }
}

fn handle_now_playing_key(key: KeyEvent, tx: &Sender<AppEvent>, client: &DaemonClient, handle: &Handle) {
    match key.code {
        KeyCode::Char('k') | KeyCode::Up => spawn_volume(handle, client.clone(), tx.clone(), 5),
        KeyCode::Char('j') | KeyCode::Down => spawn_volume(handle, client.clone(), tx.clone(), -5),
        KeyCode::Char('K') => spawn_volume(handle, client.clone(), tx.clone(), 1),
        KeyCode::Char('J') => spawn_volume(handle, client.clone(), tx.clone(), -1),
        _ => {}
    }
}

fn handle_queue_key(app: &mut App, key: KeyEvent, tx: &Sender<AppEvent>, client: &DaemonClient, handle: &Handle) {
    if app.queue.confirm_clear {
        match key.code {
            KeyCode::Char('y') | KeyCode::Enter => {
                app.queue.confirm_clear = false;
                spawn_queue_clear(handle, client.clone(), tx.clone());
            }
            KeyCode::Char('n') | KeyCode::Esc | KeyCode::Char('q') => {
                app.queue.confirm_clear = false;
            }
            _ => {}
        }
        return;
    }

    match key.code {
        KeyCode::Char('k') | KeyCode::Up => {
            app.queue.cursor = app.queue.cursor.saturating_sub(1);
            app.queue.dd_pending = false;
        }
        KeyCode::Char('j') | KeyCode::Down => {
            if app.queue.cursor + 1 < app.queue.items.len() {
                app.queue.cursor += 1;
            }
            app.queue.dd_pending = false;
        }
        KeyCode::Char('u') if key.modifiers.contains(KeyModifiers::CONTROL) => {
            let delta = page_size();
            app.queue.cursor = app.queue.cursor.saturating_sub(delta / 2);
            app.queue.dd_pending = false;
        }
        KeyCode::Char('d') if key.modifiers.contains(KeyModifiers::CONTROL) => {
            let delta = page_size();
            app.queue.cursor =
                (app.queue.cursor + delta / 2).min(app.queue.items.len().saturating_sub(1));
            app.queue.dd_pending = false;
        }
        KeyCode::Char('G') => {
            app.queue.cursor = app.queue.items.len().saturating_sub(1);
            app.queue.dd_pending = false;
        }
        KeyCode::Char('p') => {
            if let Some(pos) = queue_position(app) {
                spawn_queue_play(handle, client.clone(), tx.clone(), pos);
            }
        }
        KeyCode::Char('d') => {
            if app.queue.dd_pending {
                app.queue.dd_pending = false;
                if let Some(pos) = queue_position(app) {
                    spawn_queue_delete(handle, client.clone(), tx.clone(), pos);
                }
            } else {
                app.queue.dd_pending = true;
            }
        }
        KeyCode::Char('D') => app.queue.confirm_clear = true,
        KeyCode::Char('J') => {
            if let Some(pos) = queue_position(app) {
                if (pos as usize) < app.queue.items.len() {
                    if app.queue.cursor + 1 < app.queue.items.len() {
                        app.queue.cursor += 1;
                    }
                    spawn_queue_reorder(handle, client.clone(), tx.clone(), pos, pos + 1);
                }
            }
        }
        KeyCode::Char('K') => {
            if let Some(pos) = queue_position(app) {
                if pos > 1 {
                    app.queue.cursor = app.queue.cursor.saturating_sub(1);
                    spawn_queue_reorder(handle, client.clone(), tx.clone(), pos, pos - 1);
                }
            }
        }
        _ => {}
    }
}

fn handle_library_key(app: &mut App, key: KeyEvent, tx: &Sender<AppEvent>, client: &DaemonClient, handle: &Handle) {
    if app.library.searching {
        match key.code {
            KeyCode::Esc => {
                app.library.searching = false;
                app.library.search_query.clear();
                app.library.search_results.clear();
                app.library.search_cursor = 0;
            }
            KeyCode::Char('/') => {
                app.library.search_query.clear();
                app.library.search_results.clear();
                app.library.search_cursor = 0;
                app.input_mode = InputMode::Search;
            }
            KeyCode::Char('k') | KeyCode::Up => {
                app.library.search_cursor = app.library.search_cursor.saturating_sub(1);
            }
            KeyCode::Char('j') | KeyCode::Down => {
                if app.library.search_cursor + 1 < app.library.search_results.len() {
                    app.library.search_cursor += 1;
                }
            }
            KeyCode::Enter => {
                if let Some(entry) = app
                    .library
                    .search_results
                    .get(app.library.search_cursor)
                    .cloned()
                {
                    app.library.searching = false;
                    app.library.search_query.clear();
                    app.library.search_results.clear();
                    app.input_mode = InputMode::Normal;
                    if entry.entry_type == "dir" {
                        spawn_library_column_load(
                            handle,
                            client.clone(),
                            tx.clone(),
                            0,
                            entry.path.clone(),
                            title_for_library_path(&entry.path, &entry.name),
                        );
                    } else {
                        let dir = entry
                            .path
                            .rsplit_once('/')
                            .map(|(parent, _)| format!("/{parent}"))
                            .unwrap_or_else(|| "/".to_string());
                        spawn_library_column_load(
                            handle,
                            client.clone(),
                            tx.clone(),
                            0,
                            dir.clone(),
                            title_for_library_path(&dir, ""),
                        );
                    }
                }
            }
            _ => {}
        }
        return;
    }

    match key.code {
        KeyCode::Char('k') | KeyCode::Up => move_library_cursor(app, tx, client, -1, handle),
        KeyCode::Char('j') | KeyCode::Down => {
            move_library_cursor(app, tx, client, 1, handle);
        }
        KeyCode::Char('u') if key.modifiers.contains(KeyModifiers::CONTROL) => {
            move_library_cursor(app, tx, client, -(page_size() as isize / 2), handle);
        }
        KeyCode::Char('d') if key.modifiers.contains(KeyModifiers::CONTROL) => {
            move_library_cursor(app, tx, client, page_size() as isize / 2, handle);
        }
        KeyCode::Char('G') => {
            set_library_cursor(
                app,
                tx,
                client,
                app.library.active_column,
                library_column_len(app, app.library.active_column).saturating_sub(1),
                handle,
            );
        }
        KeyCode::Left | KeyCode::Backspace => {
            if app.library.active_column > 0 {
                app.library.active_column -= 1;
                app.library.columns.truncate(app.library.active_column + 1);
            }
        }
        KeyCode::Right => {
            library_enter_column(app, tx, client, handle);
        }
        KeyCode::Enter => {
            if let Some(entry) = library_current_entry(app).cloned() {
                if entry.entry_type == "dir" {
                    library_enter_column(app, tx, client, handle);
                } else {
                    spawn_queue_batch(
                        handle,
                        client.clone(),
                        tx.clone(),
                        vec![entry.path],
                        if app.is_line_in {
                            "Added 1 item to queue. Press space to switch from line-in.".to_string()
                        } else {
                            "Added 1 item to queue".to_string()
                        },
                    );
                }
            }
        }
        KeyCode::Char('a') => {
            if let Some(entry) = library_current_entry(app).cloned() {
                let msg = if app.is_line_in {
                    "Added selection to queue. Press space to switch from line-in."
                } else {
                    "Added selection to queue"
                };
                spawn_queue_batch(
                    handle,
                    client.clone(),
                    tx.clone(),
                    vec![entry.path],
                    msg.to_string(),
                );
            }
        }
        KeyCode::Char('A') => {
            let paths: Vec<String> = library_current_entries(app)
                .iter()
                .filter(|entry| entry.entry_type == "file")
                .map(|entry| entry.path.clone())
                .collect();
            if !paths.is_empty() {
                let count = paths.len();
                let msg = if app.is_line_in {
                    format!("Added {count} items to queue. Press space to switch from line-in.")
                } else {
                    format!("Added {count} items to queue")
                };
                spawn_queue_batch(handle, client.clone(), tx.clone(), paths, msg);
            }
        }
        KeyCode::Char('/') => {
            app.library.searching = true;
            app.library.search_query.clear();
            app.library.search_results.clear();
            app.library.search_cursor = 0;
            app.input_mode = InputMode::Search;
        }
        _ => {}
    }
}

fn handle_album_key(app: &mut App, key: KeyEvent, tx: &Sender<AppEvent>, client: &DaemonClient, handle: &Handle) {
    if !app.library_ready {
        return;
    }

    if app.albums.searching {
        match key.code {
            KeyCode::Esc => {
                app.albums.searching = false;
                app.albums.search_query.clear();
                app.albums.search_results.clear();
                app.albums.cursor = 0;
            }
            KeyCode::Char('/') => {
                app.albums.search_query.clear();
                app.albums.search_results.clear();
                app.albums.cursor = 0;
                app.input_mode = InputMode::Search;
            }
            KeyCode::Char('k') | KeyCode::Up => {
                app.albums.cursor = app.albums.cursor.saturating_sub(1);
            }
            KeyCode::Char('j') | KeyCode::Down => {
                if app.albums.cursor + 1 < app.albums.search_results.len() {
                    app.albums.cursor += 1;
                }
            }
            KeyCode::Enter => {
                if let Some(selected) = app.albums.search_results.get(app.albums.cursor).cloned() {
                    if let Some(idx) = app
                        .albums
                        .albums
                        .iter()
                        .position(|album| album.id == selected.id)
                    {
                        app.albums.cursor = idx;
                    }
                    app.albums.searching = false;
                    app.albums.search_query.clear();
                    app.albums.search_results.clear();
                    app.input_mode = InputMode::Normal;
                    queue_album_preview(tx, client, app, handle);
                }
            }
            _ => {}
        }
        return;
    }

    match key.code {
        KeyCode::Char('k') | KeyCode::Up => move_album_cursor(app, tx, client, -1, handle),
        KeyCode::Char('j') | KeyCode::Down => {
            move_album_cursor(app, tx, client, 1, handle);
        }
        KeyCode::Char('K') => move_album_cursor(app, tx, client, -1, handle),
        KeyCode::Char('J') => move_album_cursor(app, tx, client, 1, handle),
        KeyCode::Char('G') => {
            app.albums.cursor = visible_albums(app).len().saturating_sub(1);
            queue_album_preview(tx, client, app, handle);
        }
        KeyCode::Esc => {}
        KeyCode::Enter => {
            let paths: Vec<String> = app
                .albums
                .expand_tracks
                .iter()
                .map(|t| t.path.clone())
                .collect();
            if !paths.is_empty() {
                let count = paths.len();
                let msg = if app.is_line_in {
                    format!("Added {count} items to queue. Press space to switch from line-in.")
                } else {
                    format!("Added {count} items to queue")
                };
                spawn_queue_batch(handle, client.clone(), tx.clone(), paths, msg);
            }
        }
        KeyCode::Char('a') => {
            if let Some(album) = current_album(app).cloned() {
                spawn_album_add(handle, client.clone(), tx.clone(), album.id, app.is_line_in);
            }
        }
        KeyCode::Char('/') => {
            app.albums.searching = true;
            app.albums.search_query.clear();
            app.albums.search_results.clear();
            app.input_mode = InputMode::Search;
        }
        KeyCode::Char('r') => app.set_status("Rescanning library…"),
        _ => {}
    }
}

fn handle_library_search(
    app: &mut App,
    key: KeyEvent,
    tx: &Sender<AppEvent>,
    client: &DaemonClient,
    handle: &Handle,
) {
    match key.code {
        KeyCode::Esc => {
            app.input_mode = InputMode::Normal;
        }
        KeyCode::Backspace => {
            app.library.search_query.pop();
            if app.library.search_query.is_empty() {
                app.library.search_results.clear();
            } else {
                spawn_library_search(handle, client.clone(), tx.clone(), app.library.search_query.clone());
            }
        }
        KeyCode::Char('n') if key.modifiers.contains(KeyModifiers::CONTROL) => {
            if app.library.search_cursor + 1 < app.library.search_results.len() {
                app.library.search_cursor += 1;
            }
        }
        KeyCode::Char('p') if key.modifiers.contains(KeyModifiers::CONTROL) => {
            app.library.search_cursor = app.library.search_cursor.saturating_sub(1);
        }
        KeyCode::Up => {
            app.library.search_cursor = app.library.search_cursor.saturating_sub(1);
        }
        KeyCode::Down => {
            if app.library.search_cursor + 1 < app.library.search_results.len() {
                app.library.search_cursor += 1;
            }
        }
        KeyCode::Enter => {
            app.input_mode = InputMode::Normal;
        }
        KeyCode::Char(ch) if !key.modifiers.contains(KeyModifiers::CONTROL) => {
            app.library.search_query.push(ch);
            spawn_library_search(handle, client.clone(), tx.clone(), app.library.search_query.clone());
        }
        _ => {}
    }
}

fn handle_album_search(app: &mut App, key: KeyEvent, tx: &Sender<AppEvent>, client: &DaemonClient, handle: &Handle) {
    match key.code {
        KeyCode::Esc => {
            app.input_mode = InputMode::Normal;
        }
        KeyCode::Backspace => {
            app.albums.search_query.pop();
            if app.albums.search_query.is_empty() {
                app.albums.search_results.clear();
            } else {
                spawn_albums_search(handle, client.clone(), tx.clone(), app.albums.search_query.clone());
            }
        }
        KeyCode::Char('n') if key.modifiers.contains(KeyModifiers::CONTROL) => {
            if app.albums.cursor + 1 < app.albums.search_results.len() {
                app.albums.cursor += 1;
            }
        }
        KeyCode::Char('p') if key.modifiers.contains(KeyModifiers::CONTROL) => {
            app.albums.cursor = app.albums.cursor.saturating_sub(1);
        }
        KeyCode::Up => app.albums.cursor = app.albums.cursor.saturating_sub(1),
        KeyCode::Down => {
            if app.albums.cursor + 1 < app.albums.search_results.len() {
                app.albums.cursor += 1;
            }
        }
        KeyCode::Enter => {
            app.input_mode = InputMode::Normal;
        }
        KeyCode::Char(ch) if !key.modifiers.contains(KeyModifiers::CONTROL) => {
            app.albums.search_query.push(ch);
            spawn_albums_search(handle, client.clone(), tx.clone(), app.albums.search_query.clone());
        }
        _ => {}
    }
}

fn handle_command_input(
    app: &mut App,
    key: KeyEvent,
    tx: &Sender<AppEvent>,
    client: &DaemonClient,
    handle: &Handle,
) {
    match key.code {
        KeyCode::Esc => {
            app.input_mode = InputMode::Normal;
            app.cmd_input.clear();
        }
        KeyCode::Backspace => {
            app.cmd_input.pop();
        }
        KeyCode::Enter => {
            let cmd = app.cmd_input.trim().to_string();
            app.cmd_input.clear();
            app.input_mode = InputMode::Normal;
            exec_command(app, &cmd, tx, client, handle);
        }
        KeyCode::Char(ch) if !key.modifiers.contains(KeyModifiers::CONTROL) => {
            app.cmd_input.push(ch);
        }
        _ => {}
    }
}

fn exec_command(app: &mut App, cmd: &str, tx: &Sender<AppEvent>, client: &DaemonClient, handle: &Handle) {
    let parts: Vec<&str> = cmd.split_whitespace().collect();
    if parts.is_empty() {
        return;
    }
    match parts[0] {
        "q" | "quit" => app.should_quit = true,
        "help" => app.help_active = true,
        "rescan" => {
            app.set_status("Rescanning library…");
            spawn_task(handle, tx.clone(), {
                let c = client.clone();
                async move {
                    c.rescan().await?;
                    Ok(AppEvent::Status("Rescan started".to_string()))
                }
            });
        }
        "reconnect" => {
            app.set_status("Reconnecting…");
            spawn_task(handle, tx.clone(), {
                let c = client.clone();
                async move {
                    c.reconnect().await?;
                    Ok(AppEvent::Status("Reconnected".to_string()))
                }
            });
        }
        "speaker" | "room" => {
            if parts.len() > 1 {
                let needle = parts[1..].join(" ").to_lowercase();
                if let Some(speaker) = app
                    .speakers
                    .iter()
                    .find(|sp| sp.name.to_lowercase().contains(&needle))
                {
                    spawn_set_speaker(handle, client.clone(), tx.clone(), speaker.uuid.clone());
                } else {
                    app.set_status(format!("Speaker not found: {}", parts[1..].join(" ")));
                }
            }
        }
        other => app.set_status(format!("Unknown command: {other}")),
    }
}

fn sync_direct_art(
    backend: &mut CrosstermBackend<Stdout>,
    app: &App,
    art_mode: ArtMode,
    art_area: Option<Rect>,
    state: &mut DirectArtState,
) -> Result<()> {
    if art_mode == ArtMode::None || app.active_tab != Tab::NowPlaying || app.help_active {
        return clear_direct_art(backend, state);
    }

    let Some(area) = art_area.filter(|area| area.width > 0 && area.height > 0) else {
        return clear_direct_art(backend, state);
    };
    let Some(image) = app.art_image_data.as_ref() else {
        return clear_direct_art(backend, state);
    };

    let signature = format!(
        "{}:{}:{}:{}:{}",
        app.art_url, area.x, area.y, area.width, area.height
    );
    if state.active && state.signature == signature {
        return Ok(());
    }

    let mut stdout = io::stdout();
    if state.active {
        if let Some(old) = state.area {
            clear_art_area(&mut stdout, old, art_mode)?;
        }
    }

    match art_mode {
        ArtMode::Kitty => {
            // Only display if the background encode for this exact signature is ready.
            // The encode is kicked off in run_loop; we never block here.
            if let Some((cached_sig, ref encoded)) = &state.cached_kitty {
                if *cached_sig == signature {
                    let placement =
                        kitty::align_image_to_area(area, image.width, image.height);
                    kitty::display_kitty_image(&mut stdout, encoded, placement.area)?;
                } else {
                    // Encode not ready yet — skip this frame, run_loop will re-render
                    // when ArtEncoded arrives.
                    return Ok(());
                }
            } else {
                return Ok(());
            }
        }
        ArtMode::Halfblock => render_halfblock(&mut stdout, area, image)?,
        ArtMode::None => {}
    }

    state.active = true;
    state.area = Some(area);
    state.signature = signature;
    Ok(())
}

fn clear_direct_art(
    _backend: &mut CrosstermBackend<Stdout>,
    state: &mut DirectArtState,
) -> Result<()> {
    if !state.active {
        return Ok(());
    }
    if let Some(area) = state.area {
        let mut stdout = io::stdout();
        clear_art_area(&mut stdout, area, ArtMode::Kitty)?;
    }
    state.active = false;
    state.area = None;
    state.signature.clear();
    Ok(())
}

fn clear_art_area(stdout: &mut Stdout, area: Rect, mode: ArtMode) -> Result<()> {
    for y in 0..area.height {
        execute!(stdout, MoveTo(area.x, area.y + y))?;
        write!(stdout, "{}", " ".repeat(area.width as usize))?;
    }
    if mode == ArtMode::Kitty {
        kitty::hide_kitty_image(stdout, area)?;
    } else {
        stdout.flush()?;
    }
    Ok(())
}

fn render_halfblock(stdout: &mut Stdout, area: Rect, image: &ArtImageData) -> Result<()> {
    clear_art_area(stdout, area, ArtMode::Halfblock)?;
    let rendered = kitty::render_halfblock(
        &image.rgba,
        image.width,
        image.height,
        area.width,
        area.height,
    );
    let lines: Vec<&str> = rendered.lines().collect();
    let top = area.y + area.height.saturating_sub(lines.len() as u16) / 2;
    for (idx, line) in lines.iter().enumerate() {
        execute!(stdout, MoveTo(area.x, top + idx as u16))?;
        write!(stdout, "{line}")?;
    }
    stdout.flush()?;
    Ok(())
}

#[derive(Clone, Copy)]
enum SimpleAction {
    Stop,
    Prev,
    Next,
    LineIn,
}

fn spawn_simple(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, action: SimpleAction) {
    spawn_task(handle, tx, async move {
        match action {
            SimpleAction::Stop => client.stop().await?,
            SimpleAction::Prev => client.prev().await?,
            SimpleAction::Next => client.next().await?,
            SimpleAction::LineIn => client.linein().await?,
        }
        Ok(AppEvent::Status(String::new()))
    });
}

fn spawn_volume(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, delta: i32) {
    spawn_task(handle, tx, async move {
        client.volume_relative(delta).await?;
        Ok(AppEvent::Status(String::new()))
    });
}

fn spawn_set_speaker(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, uuid: String) {
    spawn_task(handle, tx, async move {
        client.set_active_speaker(&uuid).await?;
        Ok(AppEvent::Status(String::new()))
    });
}

fn spawn_queue_play(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, pos: i32) {
    spawn_task(handle, tx, async move {
        client.queue_play(pos).await?;
        Ok(AppEvent::Status(String::new()))
    });
}

fn spawn_queue_delete(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, pos: i32) {
    spawn_task(handle, tx, async move {
        client.queue_delete(pos).await?;
        Ok(AppEvent::Status(String::new()))
    });
}

fn spawn_queue_clear(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>) {
    spawn_task(handle, tx, async move {
        client.queue_clear().await?;
        Ok(AppEvent::QueueCleared)
    });
}

fn spawn_queue_reorder(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, from: i32, to: i32) {
    spawn_task(handle, tx, async move {
        client.queue_reorder(from, to).await?;
        Ok(AppEvent::Status(String::new()))
    });
}

fn spawn_queue_batch(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, paths: Vec<String>, msg: String) {
    spawn_task(handle, tx, async move {
        client.queue_batch(paths).await?;
        Ok(AppEvent::Status(msg))
    });
}

fn spawn_album_add(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, id: String, line_in: bool) {
    spawn_task(handle, tx, async move {
        let detail = client.album_detail(&id).await?;
        let count = detail.tracks.len();
        let paths: Vec<String> = detail.tracks.into_iter().map(|t| t.path).collect();
        client.queue_batch(paths).await?;
        let msg = if line_in {
            format!("Added {count} items to queue. Press space to switch from line-in.")
        } else {
            format!("Added {count} items to queue")
        };
        Ok(AppEvent::Status(msg))
    });
}

fn spawn_status_load(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>) {
    spawn_task(handle, tx, async move {
        Ok(AppEvent::Connected(client.status().await?))
    });
}

fn spawn_queue_load(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>) {
    spawn_task(handle, tx, async move {
        Ok(AppEvent::QueueLoaded(client.queue().await?))
    });
}

fn spawn_speakers_load(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>) {
    spawn_task(handle, tx, async move {
        Ok(AppEvent::SpeakersLoaded(client.speakers().await?))
    });
}

fn spawn_library_column_load(
    handle: &Handle,
    client: DaemonClient,
    tx: Sender<AppEvent>,
    depth: usize,
    path: String,
    title: String,
) {
    spawn_task(handle, tx, async move {
        let entries = client.library(&path).await?;
        Ok(AppEvent::LibraryColumnLoaded {
            depth,
            path,
            title,
            entries,
        })
    });
}

fn spawn_library_search(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, query: String) {
    spawn_task(handle, tx, async move {
        let results = client.library_search(&query).await?;
        Ok(AppEvent::LibrarySearchLoaded(results))
    });
}

fn spawn_albums_load(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>) {
    spawn_task(handle, tx, async move {
        Ok(AppEvent::AlbumsLoaded(client.albums().await?))
    });
}

fn spawn_albums_search(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, query: String) {
    spawn_task(handle, tx, async move {
        let albums = client.albums_search(&query).await?;
        Ok(AppEvent::AlbumsSearchLoaded(albums))
    });
}

fn spawn_album_detail_load(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, id: String) {
    spawn_task(handle, tx, async move {
        let detail = client.album_detail(&id).await?;
        Ok(AppEvent::AlbumDetailLoaded { id, detail })
    });
}

fn spawn_art_load(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>, url: String) {
    spawn_task(handle, tx, async move {
        let bytes = client.fetch_art_bytes(&url).await?;
        // Decode on a blocking thread pool so we don't stall tokio workers.
        let art_url = url.clone();
        let image_data = tokio::task::spawn_blocking(move || -> anyhow::Result<ArtImageData> {
            let reader = ImageReader::new(std::io::Cursor::new(bytes)).with_guessed_format()?;
            let image = reader.decode()?.to_rgba8();
            Ok(ArtImageData {
                rgba: image.clone().into_raw(),
                width: image.width(),
                height: image.height(),
            })
        })
        .await??;
        Ok(AppEvent::ArtLoaded {
            url: art_url,
            image: Some(image_data),
        })
    });
}

fn spawn_sse(handle: &Handle, client: DaemonClient, tx: Sender<AppEvent>) {
    let handle = handle.clone();
    std::thread::spawn(move || {
        let mut backoff = Duration::from_secs(1);
        let mut first = true;

        loop {
            if !first {
                std::thread::sleep(backoff);
                backoff = (backoff * 2).min(Duration::from_secs(30));

                // Re-fetch status on reconnect so the TUI state is accurate.
                // (First connection is handled by AppEvent::Connected.)
                let c = client.clone();
                let t = tx.clone();
                let h = handle.clone();
                std::thread::spawn(move || {
                    if let Ok(status) = h.block_on(c.status()) {
                        let _ = t.send(AppEvent::StatusRefresh(status));
                    }
                });
            }
            first = false;

            let (evt_tx, evt_rx) = mpsc::channel();
            let bridge_tx = tx.clone();
            std::thread::spawn(move || {
                while let Ok(evt) = evt_rx.recv() {
                    if bridge_tx.send(AppEvent::Sse(evt)).is_err() {
                        break;
                    }
                }
            });

            let connected_at = Instant::now();
            match handle.block_on(client.stream_events(evt_tx)) {
                Ok(()) => {
                    // Clean disconnect (daemon restarted). Reset backoff if we
                    // were connected long enough to consider it a real session.
                    if connected_at.elapsed() > Duration::from_secs(5) {
                        backoff = Duration::from_secs(1);
                    }
                }
                Err(err) => {
                    if tx
                        .send(AppEvent::Error(format!("SSE lost ({err}), reconnecting…")))
                        .is_err()
                    {
                        break; // main channel closed, app is quitting
                    }
                }
            }
        }
    });
}

fn spawn_task<F>(handle: &Handle, tx: Sender<AppEvent>, future: F)
where
    F: std::future::Future<Output = Result<AppEvent>> + Send + 'static,
{
    let tx = tx.clone();
    handle.spawn(async move {
        let event = match future.await {
            Ok(event) => event,
            Err(err) => AppEvent::Error(err.to_string()),
        };
        let _ = tx.send(event);
    });
}

fn spawn_tick_thread(tx: Sender<AppEvent>, delay: Duration) {
    std::thread::spawn(move || loop {
        std::thread::sleep(delay);
        if tx.send(AppEvent::Tick).is_err() {
            break;
        }
    });
}

fn spawn_input_thread(tx: Sender<AppEvent>) {
    std::thread::spawn(move || loop {
        match event::poll(Duration::from_millis(100)) {
            Ok(true) => match event::read() {
                Ok(CEvent::Key(key)) => {
                    if tx.send(AppEvent::Key(key)).is_err() {
                        break;
                    }
                }
                Ok(CEvent::Resize(w, h)) => {
                    if tx.send(AppEvent::Resize(w, h)).is_err() {
                        break;
                    }
                }
                Ok(_) => {}
                Err(err) => {
                    let _ = tx.send(AppEvent::Error(format!("input error: {err}")));
                    break;
                }
            },
            Ok(false) => {}
            Err(err) => {
                let _ = tx.send(AppEvent::Error(format!("poll error: {err}")));
                break;
            }
        }
    });
}

fn detect_art_mode(mode: ArtCliMode) -> ArtMode {
    match mode {
        ArtCliMode::Kitty => ArtMode::Kitty,
        ArtCliMode::Halfblock => ArtMode::Halfblock,
        ArtCliMode::None => ArtMode::None,
        ArtCliMode::Auto => {
            let term = std::env::var("TERM").unwrap_or_default();
            if std::env::var_os("KITTY_WINDOW_ID").is_some()
                || std::env::var_os("GHOSTTY_RESOURCES_DIR").is_some()
                || term.contains("kitty")
            {
                ArtMode::Kitty
            } else {
                ArtMode::Halfblock
            }
        }
    }
}

fn page_size() -> usize {
    let (_, rows) = terminal::size().unwrap_or((120, 40));
    rows.saturating_sub(8) as usize
}

fn queue_position(app: &App) -> Option<i32> {
    app.queue
        .items
        .get(app.queue.cursor)
        .map(|item| item.position)
}

fn visible_albums(app: &App) -> &[client::Album] {
    if app.albums.searching && !app.albums.search_results.is_empty() {
        &app.albums.search_results
    } else {
        &app.albums.albums
    }
}

fn current_album(app: &App) -> Option<&client::Album> {
    visible_albums(app).get(app.albums.cursor)
}

fn library_preview_matches(app: &App, depth: usize, path: &str) -> bool {
    if depth == 0 {
        return true;
    }
    let parent_depth = depth.saturating_sub(1);
    app.library
        .columns
        .get(parent_depth)
        .and_then(|column| column.entries.get(column.cursor))
        .map(|entry| entry.path == path)
        .unwrap_or(false)
}

fn queue_library_preview(tx: &Sender<AppEvent>, client: &DaemonClient, app: &App, depth: usize, handle: &Handle) {
    if depth >= 2 {
        return;
    }
    let Some(column) = app.library.columns.get(depth) else {
        return;
    };
    let Some(entry) = column.entries.get(column.cursor) else {
        return;
    };
    if entry.entry_type != "dir" {
        return;
    }
    spawn_library_column_load(
        handle,
        client.clone(),
        tx.clone(),
        depth + 1,
        entry.path.clone(),
        title_for_library_entry(entry),
    );
}

fn move_library_cursor(app: &mut App, tx: &Sender<AppEvent>, client: &DaemonClient, delta: isize, handle: &Handle) {
    let depth = app.library.active_column;
    let len = library_column_len(app, depth);
    if len == 0 {
        return;
    }
    let current = library_column_cursor(app, depth);
    let next = if delta.is_negative() {
        current.saturating_sub(delta.unsigned_abs())
    } else {
        (current + delta as usize).min(len.saturating_sub(1))
    };
    set_library_cursor(app, tx, client, depth, next, handle);
}

fn set_library_cursor(
    app: &mut App,
    tx: &Sender<AppEvent>,
    client: &DaemonClient,
    depth: usize,
    cursor: usize,
    handle: &Handle,
) {
    if let Some(column) = app.library.columns.get_mut(depth) {
        column.cursor = cursor.min(column.entries.len().saturating_sub(1));
    }
    if depth == 0 {
        app.library.cursor = cursor.min(app.library.entries.len().saturating_sub(1));
    }
    app.library.columns.truncate(depth + 1);
    queue_library_preview(tx, client, app, depth, handle);
}

fn library_enter_column(app: &mut App, tx: &Sender<AppEvent>, client: &DaemonClient, handle: &Handle) {
    let depth = app.library.active_column;
    let Some(entry) = app
        .library
        .columns
        .get(depth)
        .and_then(|column| column.entries.get(column.cursor))
        .cloned()
    else {
        return;
    };
    if entry.entry_type != "dir" {
        return;
    }
    if app.library.columns.get(depth + 1).is_none()
        || !library_preview_matches(app, depth + 1, &entry.path)
    {
        spawn_library_column_load(
            handle,
            client.clone(),
            tx.clone(),
            depth + 1,
            entry.path.clone(),
            title_for_library_entry(&entry),
        );
    }
    if depth + 1 <= 2 {
        app.library.active_column = (depth + 1).min(2);
    }
}

fn library_column_len(app: &App, depth: usize) -> usize {
    app.library
        .columns
        .get(depth)
        .map(|column| column.entries.len())
        .unwrap_or(0)
}

fn library_column_cursor(app: &App, depth: usize) -> usize {
    app.library
        .columns
        .get(depth)
        .map(|column| column.cursor)
        .unwrap_or(0)
}

fn library_current_entry(app: &App) -> Option<&client::LibraryEntry> {
    app.library
        .columns
        .get(app.library.active_column)
        .and_then(|column| column.entries.get(column.cursor))
}

fn library_current_entries(app: &App) -> &[client::LibraryEntry] {
    app.library
        .columns
        .get(app.library.active_column)
        .map(|column| column.entries.as_slice())
        .unwrap_or(&[])
}

fn title_for_library_entry(entry: &client::LibraryEntry) -> String {
    if entry.name.is_empty() {
        title_for_library_path(&entry.path, "")
    } else {
        entry.name.clone()
    }
}

fn title_for_library_path(path: &str, fallback: &str) -> String {
    if !fallback.is_empty() {
        return fallback.to_string();
    }
    let trimmed = path.trim_matches('/');
    if trimmed.is_empty() {
        "Library".to_string()
    } else {
        trimmed.rsplit('/').next().unwrap_or("Library").to_string()
    }
}

fn queue_album_preview(tx: &Sender<AppEvent>, client: &DaemonClient, app: &mut App, handle: &Handle) {
    if let Some(album) = current_album(app).cloned() {
        if app.albums.preview_id != album.id {
            app.albums.preview_id = album.id.clone();
            app.albums.expand_tracks.clear();
            spawn_album_detail_load(handle, client.clone(), tx.clone(), album.id);
        }
    }
}

fn move_album_cursor(app: &mut App, tx: &Sender<AppEvent>, client: &DaemonClient, delta: isize, handle: &Handle) {
    let len = visible_albums(app).len();
    if len == 0 {
        return;
    }
    let next = if delta.is_negative() {
        app.albums.cursor.saturating_sub(delta.unsigned_abs())
    } else {
        (app.albums.cursor + delta as usize).min(len.saturating_sub(1))
    };
    app.albums.cursor = next;
    queue_album_preview(tx, client, app, handle);
}
