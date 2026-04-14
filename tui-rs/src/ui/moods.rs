use ratatui::{
    layout::{Constraint, Layout, Rect},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame,
};

use crate::{app::App, theme};

pub fn render(f: &mut Frame, area: Rect, app: &App) {
    if !app.connected {
        let (title, subtitle) = if let Some(ref err) = app.connect_error {
            (err.clone(), "Will retry automatically.")
        } else {
            ("Connecting to daemon\u{2026}".to_string(), "")
        };
        let lines = vec![
            Line::from(""),
            Line::from(Span::styled(
                title,
                Style::default()
                    .fg(theme::TEXT)
                    .add_modifier(Modifier::BOLD),
            )),
            Line::from(""),
            Line::from(Span::styled(subtitle, theme::secondary_text())),
        ];
        f.render_widget(
            Paragraph::new(lines).alignment(ratatui::layout::Alignment::Center),
            area,
        );
        return;
    }

    if app.moods.moods.is_empty() {
        let lines = vec![
            Line::from(""),
            Line::from(Span::styled(
                "No moods found",
                Style::default()
                    .fg(theme::TEXT)
                    .add_modifier(Modifier::BOLD),
            )),
            Line::from(""),
            Line::from(Span::styled(
                "Add .m3u files or moods.json to ~/.config/sonotuid/moods/",
                theme::secondary_text(),
            )),
            Line::from(""),
            Line::from(Span::styled("r to reload", theme::help_text())),
        ];
        f.render_widget(
            Paragraph::new(lines).alignment(ratatui::layout::Alignment::Center),
            area,
        );
        return;
    }

    let chunks =
        Layout::horizontal([Constraint::Percentage(44), Constraint::Percentage(56)]).split(area);
    render_mood_list(f, chunks[0], app);
    render_mood_preview(f, chunks[1], app);
}

fn render_mood_list(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border_focus())
        .title(" Moods ");
    let inner = block.inner(area);
    f.render_widget(block, area);

    let moods = &app.moods.moods;
    let list_height = inner.height.saturating_sub(1) as usize;
    let start = visible_start(app.moods.cursor, list_height, moods.len());
    let mut lines = Vec::new();

    let w = inner.width as usize;
    let content_w = w.saturating_sub(2);
    let name_w = ((content_w * 60) / 100).max(10);
    let count_w = content_w.saturating_sub(name_w + 2).max(6);

    for idx in start..(start + list_height).min(moods.len()) {
        let mood = &moods[idx];
        let is_selected = idx == app.moods.cursor;
        let marker = if is_selected { "\u{276f} " } else { "  " };
        let (name_style, count_style) = if is_selected {
            (theme::selected_row(), theme::selected_row())
        } else {
            (theme::secondary_text(), theme::dim_style())
        };
        let count_text = format!("{} tracks", mood.track_count);
        lines.push(Line::from(vec![
            Span::styled(marker, name_style),
            Span::styled(pad(truncate(&mood.name, name_w), name_w), name_style),
            Span::raw("  "),
            Span::styled(truncate(&count_text, count_w), count_style),
        ]));
    }

    while lines.len() < inner.height.saturating_sub(1) as usize {
        lines.push(Line::from(""));
    }
    lines.push(Line::from(Span::styled(
        "enter play mood   r reload",
        theme::help_text(),
    )));
    f.render_widget(Paragraph::new(lines), inner);
}

fn render_mood_preview(f: &mut Frame, area: Rect, app: &App) {
    let title = app
        .moods
        .moods
        .get(app.moods.cursor)
        .map(|m| m.name.clone())
        .unwrap_or_else(|| "Preview".to_string());
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border())
        .title(format!(" {} ", title));
    let inner = block.inner(area);
    f.render_widget(block, area);

    let mut lines = Vec::new();
    if let Some(mood) = app.moods.moods.get(app.moods.cursor) {
        if !mood.description.is_empty() {
            lines.push(Line::from(Span::styled(
                truncate(&mood.description, inner.width as usize),
                theme::artist_style(),
            )));
        }
        let shuffle_label = if mood.shuffle { "shuffle" } else { "in order" };
        lines.push(Line::from(Span::styled(
            truncate(
                &format!("{} tracks  {}", mood.track_count, shuffle_label),
                inner.width as usize,
            ),
            theme::dim_style(),
        )));
        lines.push(Line::from(""));
    }

    let header_lines = lines.len();
    let body_rows = (inner.height as usize).saturating_sub(header_lines + 1);
    for (i, track) in app.moods.preview_tracks.iter().take(body_rows).enumerate() {
        let duration = if track.duration > 0 {
            crate::app::format_duration(track.duration)
        } else {
            "--:--".to_string()
        };
        let text = truncate(
            &format!("{:>2}  {}  {}", i + 1, track.title, duration),
            inner.width as usize,
        );
        lines.push(Line::from(Span::styled(text, theme::secondary_text())));
    }
    while lines.len() < (inner.height as usize).saturating_sub(1) {
        lines.push(Line::from(""));
    }
    lines.push(Line::from(Span::styled(
        "tracks update with hovered mood",
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
        out.push('\u{2026}');
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
