use ratatui::{
    layout::{Constraint, Layout, Rect},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame,
};

use crate::{app::App, theme};

pub fn render(f: &mut Frame, area: Rect, app: &App) {
    if app.library.searching {
        render_search(f, area, app);
    } else {
        render_columns(f, area, app);
    }
}

fn render_columns(f: &mut Frame, area: Rect, app: &App) {
    let chunks = Layout::horizontal([
        Constraint::Percentage(33),
        Constraint::Percentage(34),
        Constraint::Percentage(33),
    ])
    .split(area);

    for (idx, chunk) in chunks.iter().enumerate() {
        render_column(f, *chunk, app, idx);
    }
}

fn render_column(f: &mut Frame, area: Rect, app: &App, idx: usize) {
    let title = app
        .library
        .columns
        .get(idx)
        .map(|c| c.title.as_str())
        .unwrap_or("");
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(if idx == app.library.active_column {
            theme::pane_border_focus()
        } else {
            theme::pane_border()
        })
        .title(format!(" {} ", title));
    let inner = block.inner(area);
    f.render_widget(block, area);

    let mut lines = Vec::new();
    if let Some(column) = app.library.columns.get(idx) {
        let list_height = inner.height.saturating_sub(1) as usize;
        let start = visible_start(column.cursor, list_height, column.entries.len());
        for row in start..(start + list_height).min(column.entries.len()) {
            let entry = &column.entries[row];
            let icon = if entry.entry_type == "dir" {
                "▸"
            } else {
                "♪"
            };
            let label = if entry.title.is_empty() {
                &entry.name
            } else {
                &entry.title
            };
            let text = truncate(
                &format!(
                    "{} {} {}",
                    if row == column.cursor { "❯" } else { " " },
                    icon,
                    label
                ),
                inner.width as usize,
            );
            let style = if row == column.cursor && idx == app.library.active_column {
                theme::selected_row()
            } else if row == column.cursor {
                Style::default()
                    .fg(theme::TEXT)
                    .add_modifier(Modifier::BOLD)
            } else {
                theme::secondary_text()
            };
            lines.push(Line::from(Span::styled(text, style)));
        }
    }

    while lines.len() < inner.height.saturating_sub(1) as usize {
        lines.push(Line::from(""));
    }
    let footer = if idx == 2 {
        "enter add song   left/right move".to_string()
    } else {
        "left/right move".to_string()
    };
    lines.push(Line::from(Span::styled(footer, theme::help_text())));
    f.render_widget(Paragraph::new(lines), inner);
}

fn render_search(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border_focus())
        .title(" Search Results ");
    let inner = block.inner(area);
    f.render_widget(block, area);

    let list_height = inner.height.saturating_sub(1) as usize;
    let start = visible_start(
        app.library.search_cursor,
        list_height,
        app.library.search_results.len(),
    );
    let mut lines = Vec::new();
    for idx in start..(start + list_height).min(app.library.search_results.len()) {
        let entry = &app.library.search_results[idx];
        let label = if entry.title.is_empty() {
            &entry.name
        } else {
            &entry.title
        };
        let text = truncate(
            &format!(
                "{} {}  {}",
                if idx == app.library.search_cursor {
                    "❯"
                } else {
                    " "
                },
                label,
                entry.path
            ),
            inner.width as usize,
        );
        let style = if idx == app.library.search_cursor {
            theme::selected_row()
        } else {
            theme::secondary_text()
        };
        lines.push(Line::from(Span::styled(text, style)));
    }
    while lines.len() < inner.height.saturating_sub(1) as usize {
        lines.push(Line::from(""));
    }
    lines.push(Line::from(Span::styled(
        "enter jump to location   esc close search",
        theme::help_text(),
    )));
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
