use ratatui::{
    buffer::Buffer,
    layout::{Alignment, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Clear, Paragraph},
    Frame,
};

use crate::app::{App, InputMode, Tab, ALL_TABS};
use crate::theme;

pub fn render_tab_bar(f: &mut Frame, area: Rect, app: &App) {
    let mut spans = Vec::new();
    for (idx, tab) in ALL_TABS.iter().enumerate() {
        let style = if *tab == app.active_tab {
            theme::tab_active()
        } else {
            theme::tab_inactive()
        };
        spans.push(Span::styled(
            format!(" {} {} ", idx + 1, tab.label()),
            style,
        ));
        if idx < ALL_TABS.len() - 1 {
            spans.push(Span::styled(" | ", theme::dim_style()));
        }
    }

    if let Some(speaker) = app.speaker.as_ref() {
        spans.push(Span::styled(
            format!("   {}", speaker.name),
            theme::dim_style(),
        ));
    }

    f.render_widget(Paragraph::new(Line::from(spans)), area);
}

pub fn render_command_line(f: &mut Frame, area: Rect, app: &App) {
    match app.input_mode {
        InputMode::Command => {
            let line = Line::from(vec![
                Span::styled(":", theme::search_style()),
                Span::styled(&app.cmd_input, Style::default().fg(theme::WHITE)),
                Span::styled("█", Style::default().fg(theme::WHITE)),
            ]);
            f.render_widget(Paragraph::new(line), area);
        }
        InputMode::Search => {
            let query = if app.active_tab == Tab::Library {
                &app.library.search_query
            } else {
                &app.albums.search_query
            };
            let line = Line::from(vec![
                Span::styled("/", theme::search_style()),
                Span::styled(query.as_str(), Style::default().fg(theme::WHITE)),
                Span::styled("█", Style::default().fg(theme::WHITE)),
            ]);
            f.render_widget(Paragraph::new(line), area);
        }
        InputMode::Normal => {
            if !app.status_msg.is_empty() {
                f.render_widget(
                    Paragraph::new(Line::from(Span::styled(
                        &app.status_msg,
                        theme::status_style(),
                    ))),
                    area,
                );
            }
        }
    }
}

pub fn render_connecting(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::help_border())
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
            .fg(theme::WHITE)
            .add_modifier(Modifier::BOLD),
    )))
    .alignment(Alignment::Center);
    f.render_widget(paragraph, inner);
}

pub fn render_help_overlay(f: &mut Frame, area: Rect, app: &App) {
    if !app.help_active {
        return;
    }

    let width = 58u16.min(area.width.saturating_sub(4));
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
                .fg(theme::WHITE)
                .add_modifier(Modifier::BOLD),
        )),
        Line::from("1-4 switch tabs"),
        Line::from("gt / gT cycle tabs"),
        Line::from("space play/pause"),
        Line::from("s stop"),
        Line::from("</> previous/next"),
        Line::from("j/k volume on Now Playing"),
        Line::from("J/K fine volume on Now Playing"),
        Line::from("tab cycle speaker"),
        Line::from("l switch to line-in"),
        Line::from(": command line"),
        Line::from("? help"),
        Line::from("q quit"),
        Line::from(""),
    ];

    match app.active_tab {
        Tab::NowPlaying => {
            lines.push(Line::from("Now Playing"));
            lines.push(Line::from("Large art is rendered directly to the terminal"));
        }
        Tab::Queue => {
            lines.push(Line::from("Queue"));
            lines.push(Line::from("p play from cursor"));
            lines.push(Line::from("dd delete item"));
            lines.push(Line::from("D clear queue"));
            lines.push(Line::from("J/K reorder"));
        }
        Tab::Library => {
            lines.push(Line::from("Library"));
            lines.push(Line::from("enter open directory or add file"));
            lines.push(Line::from("a add selection"));
            lines.push(Line::from("A add all files in folder"));
            lines.push(Line::from("backspace go up"));
            lines.push(Line::from("/ search"));
            lines.push(Line::from("ctrl+n / ctrl+p search next/prev"));
        }
        Tab::Albums => {
            lines.push(Line::from("Albums"));
            lines.push(Line::from("enter expand or add expanded album"));
            lines.push(Line::from("a add album"));
            lines.push(Line::from("/ search"));
            lines.push(Line::from("esc collapse"));
            lines.push(Line::from("r show rescan status message"));
        }
    }

    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::help_border())
        .title(" Help ");
    f.render_widget(Paragraph::new(lines).block(block), popup);
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
            cell.set_fg(theme::DARK_GRAY);
        }
    }
}
