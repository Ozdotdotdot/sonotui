use ratatui::{
    layout::Rect,
    style::Style,
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame,
};

use crate::{app::App, theme};

pub fn render(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border_focus())
        .title(format!(" Queue [{} tracks] ", app.queue.items.len()));
    let inner = block.inner(area);
    f.render_widget(block, area);

    let list_height = inner.height.saturating_sub(2) as usize;
    let start = visible_start(app.queue.cursor, list_height, app.queue.items.len());
    let mut lines = Vec::new();

    for idx in start..(start + list_height).min(app.queue.items.len()) {
        let item = &app.queue.items[idx];
        let is_selected = idx == app.queue.cursor;
        let is_playing = !app.track.uri.is_empty() && app.track.uri == item.uri;
        let marker = if is_playing { "▶" } else { " " };
        let duration = if item.duration > 0 {
            crate::app::format_duration(item.duration)
        } else {
            "--:--".to_string()
        };
        let text = truncate(
            &format!(
                "{} {} {:>2}  {}  {}  {}",
                if is_selected { "❯" } else { " " },
                marker,
                item.position,
                item.title,
                item.artist,
                duration
            ),
            inner.width as usize,
        );
        let style = if is_selected {
            theme::selected_row()
        } else if is_playing {
            Style::default().fg(theme::PRIMARY)
        } else {
            theme::secondary_text()
        };
        lines.push(Line::from(Span::styled(text, style)));
    }

    while lines.len() < list_height {
        lines.push(Line::from(""));
    }

    lines.push(Line::from(""));
    let footer = if app.queue.confirm_clear {
        "Clear entire queue? [y]es / [n]o".to_string()
    } else if app.status_msg.is_empty() {
        "p play   dd delete   D clear   J/K reorder   gg/G top/bottom".to_string()
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
