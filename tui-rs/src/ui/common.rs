use ratatui::{
    buffer::Buffer,
    layout::{Alignment, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Clear, Paragraph},
    Frame,
};
use unicode_width::UnicodeWidthStr;

use crate::app::{App, InputMode, Tab, ALL_TABS};
use crate::theme;

pub fn render_header(f: &mut Frame, area: Rect, app: &App) {
    let bg = Style::default().bg(theme::SURFACE).fg(theme::TEXT);
    let transport_style = if app.is_line_in {
        Style::default()
            .fg(theme::TEXT)
            .bg(theme::PLAYING_BG)
            .add_modifier(Modifier::BOLD)
    } else {
        theme::transport_badge_style(&app.transport)
    };

    let transport = if app.is_line_in {
        " LIVE "
    } else {
        match app.transport.as_str() {
            "PLAYING" => " PLAYING ",
            "PAUSED_PLAYBACK" => " PAUSED ",
            "STOPPED" => " STOPPED ",
            "TRANSITIONING" => " LOADING ",
            _ => " STATUS ",
        }
    };

    let left = vec![
        Span::styled(transport, transport_style),
        Span::raw(" "),
        Span::styled(
            format!("{}  ", app.active_speaker_name()),
            Style::default()
                .fg(theme::TEXT)
                .add_modifier(Modifier::BOLD),
        ),
        Span::styled(
            format!("vol:{}  ", app.volume),
            theme::volume_style().bg(theme::SURFACE),
        ),
    ];

    let summary = app.now_playing_summary();
    let left_width: usize = left.iter().map(span_width).sum();
    let summary_width = area.width as usize;
    let available = summary_width.saturating_sub(left_width);
    let summary = truncate(&summary, available.max(8));

    let mut spans = left;
    if !summary.is_empty() {
        spans.push(Span::styled(summary, Style::default().fg(theme::TEXT_SOFT)));
    }

    f.render_widget(Paragraph::new(Line::from(spans)).style(bg), area);
}

pub fn render_tab_bar(f: &mut Frame, area: Rect, app: &App) {
    let bg = theme::tab_row_bg();
    let labels: Vec<String> = ALL_TABS
        .iter()
        .enumerate()
        .map(|(idx, tab)| format!("  {} {}  ", idx + 1, tab.label()))
        .collect();
    let total_width: usize = labels.iter().map(|label| label.width()).sum::<usize>() + 3 * 2;
    let left_pad = area.width.saturating_sub(total_width as u16) / 2;

    let mut spans = Vec::new();
    if left_pad > 0 {
        spans.push(Span::styled(" ".repeat(left_pad as usize), bg));
    }

    for (idx, tab) in ALL_TABS.iter().enumerate() {
        let style = if *tab == app.active_tab {
            theme::tab_active()
        } else {
            theme::tab_inactive().bg(theme::SURFACE)
        };
        spans.push(Span::styled(labels[idx].clone(), style));
        if idx < ALL_TABS.len() - 1 {
            spans.push(Span::styled("   ", bg));
        }
    }

    let line = Line::from(spans);
    let paragraph = Paragraph::new(line).style(bg).alignment(Alignment::Left);
    f.render_widget(paragraph, area);
}

pub fn render_command_line(f: &mut Frame, area: Rect, app: &App) {
    let base = Style::default().bg(theme::SURFACE).fg(theme::TEXT_SOFT);
    match app.input_mode {
        InputMode::Command => {
            let line = Line::from(vec![
                Span::styled(":", theme::search_style()),
                Span::styled(&app.cmd_input, Style::default().fg(theme::TEXT)),
                Span::styled("█", Style::default().fg(theme::TEXT)),
            ]);
            f.render_widget(Paragraph::new(line).style(base), area);
        }
        InputMode::Search => {
            let query = if app.active_tab == Tab::Library {
                &app.library.search_query
            } else {
                &app.albums.search_query
            };
            let line = Line::from(vec![
                Span::styled("/", theme::search_style()),
                Span::styled(query.as_str(), Style::default().fg(theme::TEXT)),
                Span::styled("█", Style::default().fg(theme::TEXT)),
            ]);
            f.render_widget(Paragraph::new(line).style(base), area);
        }
        InputMode::Normal => {
            let text = if app.status_msg.is_empty() {
                "Ready".to_string()
            } else {
                app.status_msg.clone()
            };
            f.render_widget(
                Paragraph::new(Line::from(Span::styled(text, theme::status_style()))).style(base),
                area,
            );
        }
    }
}

pub fn render_connecting(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::shell_block())
        .title(" sonotui ");
    let inner = block.inner(area);
    f.render_widget(block, area);

    let msg = if app.status_msg.is_empty() {
        "Connecting to sonotuid..."
    } else {
        &app.status_msg
    };
    let paragraph = Paragraph::new(Line::from(Span::styled(
        msg,
        Style::default()
            .fg(theme::TEXT)
            .add_modifier(Modifier::BOLD),
    )))
    .alignment(Alignment::Center)
    .style(Style::default().bg(theme::BG));
    f.render_widget(paragraph, inner);
}

pub fn render_help_overlay(f: &mut Frame, area: Rect, app: &App) {
    if !app.help_active {
        return;
    }

    let width = 60u16.min(area.width.saturating_sub(4));
    let height = 24u16.min(area.height.saturating_sub(4));
    let popup = Rect::new(
        area.x + (area.width.saturating_sub(width)) / 2,
        area.y + (area.height.saturating_sub(height)) / 2,
        width,
        height,
    );

    f.render_widget(Clear, popup);

    let mut lines = vec![
        Line::from(Span::styled(
            "Global",
            Style::default()
                .fg(theme::TEXT)
                .add_modifier(Modifier::BOLD),
        )),
        Line::from(Span::styled("1-4 switch tabs", theme::help_text())),
        Line::from(Span::styled("gt / gT cycle tabs", theme::help_text())),
        Line::from(Span::styled("space play/pause", theme::help_text())),
        Line::from(Span::styled("s stop", theme::help_text())),
        Line::from(Span::styled("</> previous/next", theme::help_text())),
        Line::from(Span::styled(
            "j/k volume on Now Playing",
            theme::help_text(),
        )),
        Line::from(Span::styled(
            "J/K fine volume on Now Playing",
            theme::help_text(),
        )),
        Line::from(Span::styled("tab cycle speaker", theme::help_text())),
        Line::from(Span::styled("l switch to line-in", theme::help_text())),
        Line::from(Span::styled(": command line", theme::help_text())),
        Line::from(Span::styled("? help", theme::help_text())),
        Line::from(Span::styled("q quit", theme::help_text())),
        Line::from(""),
    ];

    match app.active_tab {
        Tab::NowPlaying => {
            lines.push(Line::from(Span::styled(
                "Now Playing",
                theme::title_style(),
            )));
            lines.push(Line::from(Span::styled(
                "Album art is rendered directly to the terminal",
                theme::help_text(),
            )));
        }
        Tab::Queue => {
            lines.push(Line::from(Span::styled("Queue", theme::title_style())));
            lines.push(Line::from(Span::styled(
                "p play from cursor",
                theme::help_text(),
            )));
            lines.push(Line::from(Span::styled(
                "dd delete item",
                theme::help_text(),
            )));
            lines.push(Line::from(Span::styled(
                "D clear queue",
                theme::help_text(),
            )));
            lines.push(Line::from(Span::styled("J/K reorder", theme::help_text())));
        }
        Tab::Library => {
            lines.push(Line::from(Span::styled("Library", theme::title_style())));
            lines.push(Line::from(Span::styled(
                "enter open directory or add file",
                theme::help_text(),
            )));
            lines.push(Line::from(Span::styled(
                "a add selection",
                theme::help_text(),
            )));
            lines.push(Line::from(Span::styled(
                "A add all files in folder",
                theme::help_text(),
            )));
            lines.push(Line::from(Span::styled(
                "backspace go up",
                theme::help_text(),
            )));
            lines.push(Line::from(Span::styled("/ search", theme::help_text())));
            lines.push(Line::from(Span::styled(
                "ctrl+n / ctrl+p search next/prev",
                theme::help_text(),
            )));
        }
        Tab::Albums => {
            lines.push(Line::from(Span::styled("Albums", theme::title_style())));
            lines.push(Line::from(Span::styled(
                "enter expand or add expanded album",
                theme::help_text(),
            )));
            lines.push(Line::from(Span::styled("a add album", theme::help_text())));
            lines.push(Line::from(Span::styled("/ search", theme::help_text())));
            lines.push(Line::from(Span::styled("esc collapse", theme::help_text())));
            lines.push(Line::from(Span::styled(
                "r show rescan status message",
                theme::help_text(),
            )));
        }
    }

    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::help_border())
        .title(" Help ");
    f.render_widget(
        Paragraph::new(lines)
            .block(block)
            .style(Style::default().bg(theme::SURFACE).fg(theme::TEXT_SOFT)),
        popup,
    );
}

pub fn render_progress_bar(buf: &mut Buffer, area: Rect, ratio: f64, fill_color: Color) {
    if area.width == 0 {
        return;
    }
    let filled = ((area.width as f64) * ratio.clamp(0.0, 1.0)) as u16;
    for x in 0..area.width {
        let cell = &mut buf[(area.x + x, area.y)];
        if x < filled {
            cell.set_char('━');
            cell.set_fg(fill_color);
        } else {
            cell.set_char('─');
            cell.set_fg(theme::SURFACE_HI);
        }
    }
}

fn span_width(span: &Span<'_>) -> usize {
    span.content.as_ref().width()
}

fn truncate(input: &str, width: usize) -> String {
    if input.width() <= width {
        input.to_string()
    } else if width <= 1 {
        String::new()
    } else {
        let mut out = String::new();
        for ch in input.chars() {
            let next = out.width() + ch.len_utf8();
            if next >= width {
                break;
            }
            out.push(ch);
        }
        out.push('…');
        out
    }
}
