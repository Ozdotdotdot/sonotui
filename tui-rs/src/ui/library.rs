use ratatui::{
    layout::Rect,
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame,
};

use crate::{app::App, theme};

pub fn render(f: &mut Frame, area: Rect, app: &App) {
    let title = if app.library.current_path.is_empty() {
        "/"
    } else {
        &app.library.current_path
    };
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(Style::default().fg(theme::CYAN))
        .title(format!(" Library [{}] ", title));
    let inner = block.inner(area);
    f.render_widget(block, area);

    let list_height = inner.height.saturating_sub(2) as usize;
    let (entries, selected) = if app.library.searching {
        (&app.library.search_results, app.library.search_cursor)
    } else {
        (&app.library.entries, app.library.cursor)
    };
    let start = visible_start(selected, list_height, entries.len());
    let mut lines = Vec::new();

    for idx in start..(start + list_height).min(entries.len()) {
        let entry = &entries[idx];
        let prefix = if entry.entry_type == "dir" {
            "[D]"
        } else {
            "[F]"
        };
        let cursor = if idx == selected { "> " } else { "  " };
        let detail = if app.library.searching {
            format!("{} {}", entry.artist, entry.path)
        } else if entry.duration > 0 {
            crate::app::format_duration(entry.duration)
        } else {
            String::new()
        };
        let label = if entry.title.is_empty() {
            &entry.name
        } else {
            &entry.title
        };
        let text = truncate(
            &format!("{cursor}{prefix} {label}  {detail}"),
            inner.width as usize,
        );
        let style = if idx == selected {
            Style::default()
                .fg(theme::WHITE)
                .add_modifier(Modifier::BOLD)
        } else if entry.entry_type == "dir" {
            Style::default().fg(theme::ORANGE)
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
        format!("/{}", app.library.search_query)
    } else if app.status_msg.is_empty() {
        "enter open/add   a add   A add all   / search   backspace up".to_string()
    } else {
        app.status_msg.clone()
    };
    lines.push(Line::from(Span::styled(footer, theme::dim_style())));
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
