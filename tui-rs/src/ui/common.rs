use ratatui::{
    buffer::Buffer,
    layout::{Alignment, Constraint, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Clear, Paragraph},
    Frame,
};
use unicode_width::UnicodeWidthStr;

use crate::app::{App, InputMode, Tab, ALL_TABS};
use crate::theme;

pub fn render_header(f: &mut Frame, area: Rect, app: &App) {
    let boxes = Layout::horizontal([
        Constraint::Length(26),
        Constraint::Min(24),
        Constraint::Length(20),
    ])
    .spacing(1)
    .split(area);

    render_header_left(f, boxes[0], app);
    render_header_center(f, boxes[1], app);
    render_header_right(f, boxes[2], app);
}

pub fn render_tab_bar(f: &mut Frame, area: Rect, app: &App) {
    if area.height == 0 {
        return;
    }

    let tabs_area = Rect::new(area.x, area.y, area.width, 1);
    let labels: Vec<String> = ALL_TABS
        .iter()
        .enumerate()
        .map(|(idx, tab)| format!(" {} {} ", idx + 1, tab.label()))
        .collect();
    let total_width: usize = labels.iter().map(|label| label.width()).sum::<usize>() + 3;
    let left_pad = tabs_area.width.saturating_sub(total_width as u16) / 2;

    let mut spans = Vec::new();
    if left_pad > 0 {
        spans.push(Span::raw(" ".repeat(left_pad as usize)));
    }
    for (idx, tab) in ALL_TABS.iter().enumerate() {
        let style = if *tab == app.active_tab {
            theme::tab_active()
        } else {
            theme::tab_inactive()
        };
        spans.push(Span::styled(labels[idx].clone(), style));
        if idx < ALL_TABS.len() - 1 {
            spans.push(Span::raw(" "));
        }
    }

    f.render_widget(
        Paragraph::new(Line::from(spans)).alignment(Alignment::Left),
        tabs_area,
    );
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
                Paragraph::new(Line::from(Span::styled(text, theme::help_text()))).style(base),
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
        line("1-4 switch tabs"),
        line("gt / gT cycle tabs"),
        line("space play/pause"),
        line("s stop"),
        line("</> previous/next"),
        line("j/k volume on Now Playing"),
        line("J/K fine volume on Now Playing"),
        line("tab cycle speaker"),
        line("l switch to line-in"),
        line(": command line"),
        line("? help"),
        line("q quit"),
        Line::from(""),
    ];

    match app.active_tab {
        Tab::NowPlaying => {
            lines.push(Line::from(Span::styled(
                "Now Playing",
                theme::title_style(),
            )));
            lines.push(line("Album art is rendered directly to the terminal"));
        }
        Tab::Queue => {
            lines.push(Line::from(Span::styled("Queue", theme::title_style())));
            lines.push(line("p play from cursor"));
            lines.push(line("dd delete item"));
            lines.push(line("D clear queue"));
            lines.push(line("J/K reorder"));
        }
        Tab::Library => {
            lines.push(Line::from(Span::styled("Library", theme::title_style())));
            lines.push(line("left/right move between columns"));
            lines.push(line("enter add track or enter directory column"));
            lines.push(line("a add selection"));
            lines.push(line("A add all files in current column"));
            lines.push(line("/ search"));
        }
        Tab::Albums => {
            lines.push(Line::from(Span::styled("Albums", theme::title_style())));
            lines.push(line("hovered album previews tracks"));
            lines.push(line("enter add previewed album"));
            lines.push(line("a add album"));
            lines.push(line("/ search"));
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

fn render_header_left(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border())
        .title(" Sonotui ");
    let inner = block.inner(area);
    f.render_widget(block, area);

    f.render_widget(
        Paragraph::new(Line::from(Span::styled(
            truncate(app.active_speaker_name(), inner.width as usize),
            theme::title_style(),
        )))
        .alignment(Alignment::Center),
        inner,
    );
}

fn render_header_center(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border_focus())
        .title(" Now Playing ");
    let inner = block.inner(area);
    f.render_widget(block, area);

    let title = app.current_track_title();
    let artist = app.current_track_artist();
    let album = app.current_album_name();
    let line = if inner.width >= 44 {
        Line::from(vec![
            Span::styled(truncate(&album, 24), theme::dim_style()),
            Span::styled("  /  ", theme::dim_style()),
            Span::styled(truncate(&title, 26), theme::title_style()),
            Span::styled("  •  ", theme::dim_style()),
            Span::styled(truncate(&artist, 20), theme::secondary_text()),
        ])
    } else if inner.width >= 28 {
        Line::from(vec![
            Span::styled(truncate(&title, 24), theme::title_style()),
            Span::styled("  •  ", theme::dim_style()),
            Span::styled(truncate(&artist, 16), theme::secondary_text()),
        ])
    } else {
        Line::from(Span::styled(
            truncate(&title, inner.width as usize),
            theme::title_style(),
        ))
    };
    f.render_widget(Paragraph::new(line).alignment(Alignment::Center), inner);
}

fn render_header_right(f: &mut Frame, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::pane_border())
        .title(" Status ");
    let inner = block.inner(area);
    f.render_widget(block, area);

    let line = if app.is_line_in {
        Line::from(vec![
            Span::styled("● ", Style::default().fg(theme::SUCCESS)),
            Span::styled("Line-In", theme::transport_style("PLAYING")),
            Span::styled("  ", theme::dim_style()),
            Span::styled(format!("vol {}%", app.volume), theme::volume_style()),
        ])
    } else {
        Line::from(vec![
            Span::styled(
                "● ",
                Style::default().fg(theme::transport_color(&app.transport)),
            ),
            Span::styled(
                app.transport_label(),
                theme::transport_style(&app.transport),
            ),
            Span::styled("  ", theme::dim_style()),
            Span::styled(format!("vol {}%", app.volume), theme::volume_style()),
        ])
    };
    f.render_widget(Paragraph::new(line).alignment(Alignment::Center), inner);
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

fn line(text: &str) -> Line<'static> {
    Line::from(Span::styled(text.to_string(), theme::help_text()))
}
