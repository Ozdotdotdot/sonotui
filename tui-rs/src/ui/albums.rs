use ratatui::{
    layout::{Constraint, Layout, Rect},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame,
};

use crate::{app::App, client::Album, theme};

pub fn render(f: &mut Frame, area: Rect, app: &App) {
    if !app.library_ready {
        let percent = if app.albums.scan_progress > 0.0 {
            format!(" {:.0}%", app.albums.scan_progress * 100.0)
        } else {
            String::new()
        };
        let lines = vec![
            Line::from(""),
            Line::from(Span::styled(
                format!("Scanning library...{percent}"),
                Style::default()
                    .fg(theme::TEXT)
                    .add_modifier(Modifier::BOLD),
            )),
            Line::from(""),
            Line::from(Span::styled(
                "Albums become available when the daemon finishes scanning.",
                theme::secondary_text(),
            )),
        ];
        f.render_widget(
            Paragraph::new(lines).alignment(ratatui::layout::Alignment::Center),
            area,
        );
        return;
    }

    if app.albums.searching {
        render_album_search(f, area, app);
    } else {
        render_album_columns(f, area, app);
    }
}

fn render_album_columns(f: &mut Frame, area: Rect, app: &App) {
    let chunks =
        Layout::horizontal([Constraint::Percentage(44), Constraint::Percentage(56)]).split(area);
    render_album_list(f, chunks[0], app);
    render_album_preview(f, chunks[1], app);
}

fn render_album_list(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border_focus())
        .title(" Albums ");
    let inner = block.inner(area);
    f.render_widget(block, area);

    let albums = visible_albums(app);
    let list_height = inner.height.saturating_sub(1) as usize;
    let start = visible_start(app.albums.cursor, list_height, albums.len());
    let mut lines = Vec::new();

    let w = inner.width as usize;
    // Reserve 2 chars for marker + space, then split remaining between title (60%) and artist (40%)
    let content_w = w.saturating_sub(2);
    let title_w = ((content_w * 60) / 100).max(10);
    let artist_w = content_w.saturating_sub(title_w + 2).max(6);

    for idx in start..(start + list_height).min(albums.len()) {
        let album = &albums[idx];
        let is_selected = idx == app.albums.cursor;
        let marker = if is_selected { "❯ " } else { "  " };
        let (title_style, artist_style) = if is_selected {
            (theme::selected_row(), theme::selected_row())
        } else {
            (theme::secondary_text(), theme::dim_style())
        };
        lines.push(Line::from(vec![
            Span::styled(marker, title_style),
            Span::styled(pad(truncate(&album.title, title_w), title_w), title_style),
            Span::raw("  "),
            Span::styled(truncate(&album.artist, artist_w), artist_style),
        ]));
    }

    while lines.len() < inner.height.saturating_sub(1) as usize {
        lines.push(Line::from(""));
    }
    lines.push(Line::from(Span::styled(
        "hover previews tracks   enter/a add album",
        theme::help_text(),
    )));
    f.render_widget(Paragraph::new(lines), inner);
}

fn render_album_preview(f: &mut Frame, area: Rect, app: &App) {
    let title = current_album(app)
        .map(|album| album.title.clone())
        .unwrap_or_else(|| "Preview".to_string());
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border())
        .title(format!(" {} ", title));
    let inner = block.inner(area);
    f.render_widget(block, area);

    let mut lines = Vec::new();
    if let Some(album) = current_album(app) {
        lines.push(Line::from(Span::styled(
            truncate(&album.artist, inner.width as usize),
            theme::artist_style(),
        )));
        lines.push(Line::from(Span::styled(
            truncate(
                &format!("{}  {} tracks", album.year, album.track_count),
                inner.width as usize,
            ),
            theme::dim_style(),
        )));
        lines.push(Line::from(""));
    }

    let body_rows = inner.height.saturating_sub(1) as usize;
    let start = visible_start(0, body_rows, app.albums.expand_tracks.len());
    for (offset, track) in app
        .albums
        .expand_tracks
        .iter()
        .skip(start)
        .take(body_rows)
        .enumerate()
    {
        let duration = if track.duration > 0 {
            crate::app::format_duration(track.duration)
        } else {
            "--:--".to_string()
        };
        let text = truncate(
            &format!("{:>2}  {}  {}", start + offset + 1, track.title, duration),
            inner.width as usize,
        );
        lines.push(Line::from(Span::styled(text, theme::secondary_text())));
    }
    while lines.len() < inner.height.saturating_sub(1) as usize {
        lines.push(Line::from(""));
    }
    lines.push(Line::from(Span::styled(
        "tracks update with hovered album",
        theme::help_text(),
    )));
    f.render_widget(Paragraph::new(lines), inner);
}

fn render_album_search(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border_focus())
        .title(" Search Results ");
    let inner = block.inner(area);
    f.render_widget(block, area);

    let albums = &app.albums.search_results;
    let list_height = inner.height.saturating_sub(1) as usize;
    let start = visible_start(app.albums.cursor, list_height, albums.len());
    let mut lines = Vec::new();
    let w = inner.width as usize;
    let content_w = w.saturating_sub(2);
    let title_w = ((content_w * 60) / 100).max(10);
    let artist_w = content_w.saturating_sub(title_w + 2).max(6);

    for idx in start..(start + list_height).min(albums.len()) {
        let album = &albums[idx];
        let is_selected = idx == app.albums.cursor;
        let marker = if is_selected { "❯ " } else { "  " };
        let (title_style, artist_style) = if is_selected {
            (theme::selected_row(), theme::selected_row())
        } else {
            (theme::secondary_text(), theme::dim_style())
        };
        lines.push(Line::from(vec![
            Span::styled(marker, title_style),
            Span::styled(pad(truncate(&album.title, title_w), title_w), title_style),
            Span::raw("  "),
            Span::styled(truncate(&album.artist, artist_w), artist_style),
        ]));
    }
    while lines.len() < inner.height.saturating_sub(1) as usize {
        lines.push(Line::from(""));
    }
    lines.push(Line::from(Span::styled(
        "enter use results   esc close search",
        theme::help_text(),
    )));
    f.render_widget(Paragraph::new(lines), inner);
}

fn visible_albums(app: &App) -> &[Album] {
    if app.albums.searching && !app.albums.search_results.is_empty() {
        &app.albums.search_results
    } else {
        &app.albums.albums
    }
}

fn current_album(app: &App) -> Option<&Album> {
    visible_albums(app).get(app.albums.cursor)
}

fn visible_start(cursor: usize, height: usize, total: usize) -> usize {
    if total <= height {
        0
    } else {
        let mut start = cursor.saturating_sub(height / 2);
        if start + height > total {
            start = total - height;
        }
        start
    }
}

fn truncate(input: &str, width: usize) -> String {
    if input.chars().count() <= width {
        input.to_string()
    } else if width <= 1 {
        String::new()
    } else {
        let mut out: String = input.chars().take(width - 1).collect();
        out.push('…');
        out
    }
}

fn pad(input: String, width: usize) -> String {
    let len = input.chars().count();
    if len >= width {
        input
    } else {
        format!("{input}{}", " ".repeat(width - len))
    }
}
