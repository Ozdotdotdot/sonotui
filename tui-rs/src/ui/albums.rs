use ratatui::{
    layout::{Constraint, Layout, Rect},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame,
};

use crate::{app::App, client::Album, theme};

pub fn render(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(Style::default().fg(theme::ORANGE))
        .title(" Albums ");
    let inner = block.inner(area);
    f.render_widget(block, area);

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
                    .fg(theme::WHITE)
                    .add_modifier(Modifier::BOLD),
            )),
            Line::from(""),
            Line::from(Span::styled(
                "Albums become available when the daemon finishes scanning.",
                theme::dim_style(),
            )),
        ];
        f.render_widget(
            Paragraph::new(lines).alignment(ratatui::layout::Alignment::Center),
            inner,
        );
        return;
    }

    if app.albums.expanded {
        render_expanded(f, inner, app);
    } else {
        render_list(f, inner, app);
    }
}

fn render_list(f: &mut Frame, area: Rect, app: &App) {
    let list_height = area.height.saturating_sub(2) as usize;
    let albums = visible_albums(app);
    let start = visible_start(app.albums.cursor, list_height, albums.len());
    let mut lines = Vec::new();

    for idx in start..(start + list_height).min(albums.len()) {
        let album = &albums[idx];
        let text = truncate(
            &format!(
                "{}{}  {}  {} tracks",
                if idx == app.albums.cursor { "> " } else { "  " },
                album.title,
                album.artist,
                album.track_count
            ),
            area.width as usize,
        );
        let style = if idx == app.albums.cursor {
            Style::default()
                .fg(theme::WHITE)
                .add_modifier(Modifier::BOLD)
        } else {
            Style::default().fg(theme::GRAY)
        };
        lines.push(Line::from(Span::styled(text, style)));
    }

    while lines.len() < list_height {
        lines.push(Line::from(""));
    }

    lines.push(Line::from(""));
    let footer = if app.input_mode == crate::app::InputMode::Search {
        format!("/{}", app.albums.search_query)
    } else if app.status_msg.is_empty() {
        "enter expand   a add album   / search   esc close   r rescan".to_string()
    } else {
        app.status_msg.clone()
    };
    lines.push(Line::from(Span::styled(footer, theme::dim_style())));
    f.render_widget(Paragraph::new(lines), area);
}

fn render_expanded(f: &mut Frame, area: Rect, app: &App) {
    let columns =
        Layout::horizontal([Constraint::Percentage(38), Constraint::Percentage(62)]).split(area);
    render_list(f, columns[0], app);

    let mut lines = Vec::new();
    if let Some(album) = current_album(app) {
        lines.push(Line::from(Span::styled(
            truncate(&album.title, columns[1].width as usize),
            Style::default()
                .fg(theme::WHITE)
                .add_modifier(Modifier::BOLD),
        )));
        lines.push(Line::from(Span::styled(
            truncate(
                &format!(
                    "{}  {}  {} tracks",
                    album.artist, album.year, album.track_count
                ),
                columns[1].width as usize,
            ),
            theme::dim_style(),
        )));
        lines.push(Line::from(""));
    }

    for (idx, track) in app.albums.expand_tracks.iter().enumerate() {
        let duration = if track.duration > 0 {
            crate::app::format_duration(track.duration)
        } else {
            String::new()
        };
        let text = truncate(
            &format!(
                "{}{:>2}  {}  {}",
                if idx == app.albums.track_cursor {
                    "> "
                } else {
                    "  "
                },
                idx + 1,
                track.title,
                duration
            ),
            columns[1].width as usize,
        );
        let style = if idx == app.albums.track_cursor {
            Style::default()
                .fg(theme::WHITE)
                .add_modifier(Modifier::BOLD)
        } else {
            Style::default().fg(theme::GRAY)
        };
        lines.push(Line::from(Span::styled(text, style)));
    }

    f.render_widget(Paragraph::new(lines), columns[1]);
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
