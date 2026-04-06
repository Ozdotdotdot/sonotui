use ratatui::{
    layout::Rect,
    style::Style,
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame,
};
use unicode_width::{UnicodeWidthChar, UnicodeWidthStr};

use crate::{app::App, theme};

pub fn render(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border_focus())
        .title(format!(" Queue [{} tracks] ", app.queue.items.len()));
    let inner = block.inner(area);
    f.render_widget(block, area);

    let artist_w = ((inner.width as usize * 21) / 100).clamp(12, 26);
    let title_w = ((inner.width as usize * 33) / 100).clamp(16, 34);
    let album_w = ((inner.width as usize * 30) / 100).clamp(14, 32);
    let duration_w = 8usize.min(inner.width as usize);
    let list_height = inner.height.saturating_sub(3) as usize;
    let start = visible_start(app.queue.cursor, list_height, app.queue.items.len());
    let mut lines = Vec::new();

    lines.push(Line::from(vec![
        Span::styled(pad("Artist", artist_w), theme::dim_style()),
        Span::raw("  "),
        Span::styled(pad("Title", title_w), theme::dim_style()),
        Span::raw("  "),
        Span::styled(pad("Album", album_w), theme::dim_style()),
        Span::raw("  "),
        Span::styled(pad("Duration", duration_w), theme::dim_style()),
    ]));

    for idx in start..(start + list_height).min(app.queue.items.len()) {
        let item = &app.queue.items[idx];
        let is_selected = idx == app.queue.cursor;
        let is_playing = app
            .current_queue_item()
            .map(|current| current.position == item.position)
            .unwrap_or(false);
        let marker = if is_playing { "▶" } else { " " };
        let duration = if item.duration > 0 {
            crate::app::format_duration(item.duration)
        } else {
            "--:--".to_string()
        };
        let artist = truncate(
            &format!(
                "{}{} {}",
                if is_selected { "❯" } else { " " },
                marker,
                if item.artist.is_empty() {
                    "Unknown artist"
                } else {
                    &item.artist
                }
            ),
            artist_w,
        );
        let title = truncate(
            if item.title.is_empty() {
                "Unknown title"
            } else {
                &item.title
            },
            title_w,
        );
        let album = truncate(
            if item.album.is_empty() {
                "Unknown album"
            } else {
                &item.album
            },
            album_w,
        );
        let style = if is_selected {
            theme::selected_row()
        } else if is_playing {
            Style::default().fg(theme::PRIMARY)
        } else {
            theme::secondary_text()
        };
        lines.push(Line::from(vec![
            Span::styled(pad(&artist, artist_w), style),
            Span::raw("  "),
            Span::styled(pad(&title, title_w), style),
            Span::raw("  "),
            Span::styled(pad(&album, album_w), style),
            Span::raw("  "),
            Span::styled(pad_left(&duration, duration_w), style),
        ]));
    }

    while lines.len() < list_height {
        lines.push(Line::from(""));
    }

    lines.push(Line::from(""));
    let footer = if app.queue.confirm_clear {
        "Clear entire queue? [enter]/[y]es / [n]o".to_string()
    } else if app.queue.dd_pending {
        "Press d again to confirm delete, or any other key to cancel".to_string()
    } else if app.status_msg.is_empty() {
        "space play/pause   p play from here   dd delete   D clear   J/K reorder   gg/G top/bottom".to_string()
    } else {
        app.status_msg.clone()
    };
    lines.push(Line::from(Span::styled(footer, theme::help_text())));
    f.render_widget(Paragraph::new(lines), inner);
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
    if input.width() <= width {
        input.to_string()
    } else if width <= 1 {
        String::new()
    } else {
        let mut out = String::new();
        for ch in input.chars() {
            if out.width() + ch.width().unwrap_or(0) >= width {
                break;
            }
            out.push(ch);
        }
        out.push('…');
        out
    }
}

fn pad(input: &str, width: usize) -> String {
    let visible = input.width();
    if visible >= width {
        input.to_string()
    } else {
        format!("{input}{}", " ".repeat(width - visible))
    }
}

fn pad_left(input: &str, width: usize) -> String {
    let visible = input.width();
    if visible >= width {
        input.to_string()
    } else {
        format!("{}{input}", " ".repeat(width - visible))
    }
}
