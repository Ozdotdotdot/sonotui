use ratatui::{
    layout::{Alignment, Constraint, Layout, Rect},
    style::{Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Paragraph},
    Frame,
};

use crate::{
    app::{format_duration, App},
    theme,
    ui::common::render_progress_bar,
};

pub fn art_area(area: Rect, _app: &App) -> Rect {
    let layout = compute_layout(area);
    Block::default()
        .borders(Borders::ALL)
        .inner(layout.art_panel)
}

struct NowPlayingLayout {
    art_panel: Rect,
    info_panel: Rect,
}

pub fn render(f: &mut Frame, area: Rect, app: &App, placeholder_only: bool) -> Option<Rect> {
    let layout = compute_layout(area);

    let art_block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::now_playing_border())
        .title(" Album Art ");
    let info_block = Block::default()
        .borders(Borders::ALL)
        .border_style(theme::now_playing_border())
        .title(" Now Playing ");

    let art_inner = art_block.inner(layout.art_panel);
    let info_inner = info_block.inner(layout.info_panel);
    f.render_widget(art_block, layout.art_panel);
    f.render_widget(info_block, layout.info_panel);

    if placeholder_only || app.art_image_data.is_none() {
        render_placeholder(f, art_inner, app.art_image_data.is_some());
    }

    render_info(f, info_inner, app)
}

fn compute_layout(area: Rect) -> NowPlayingLayout {
    if area.width >= 90 {
        let chunks = Layout::horizontal([
            Constraint::Percentage(48),
            Constraint::Length(2),
            Constraint::Percentage(52),
        ])
        .split(area);
        NowPlayingLayout {
            art_panel: chunks[0],
            info_panel: chunks[2],
        }
    } else {
        let chunks = Layout::vertical([
            Constraint::Percentage(52),
            Constraint::Length(1),
            Constraint::Percentage(48),
        ])
        .split(area);
        NowPlayingLayout {
            art_panel: chunks[0],
            info_panel: chunks[2],
        }
    }
}

fn render_placeholder(f: &mut Frame, area: Rect, has_direct_art: bool) {
    let lines = if has_direct_art {
        vec![
            Line::from(""),
            Line::from(Span::styled(
                "album art is rendered directly",
                theme::secondary_text(),
            )),
            Line::from(Span::styled(
                "kitty graphics bypass the ratatui buffer",
                theme::dim_style(),
            )),
        ]
    } else {
        crate::kitty::art_placeholder()
            .lines()
            .map(|line| Line::raw(line.to_string()))
            .collect()
    };
    let top = area.y + area.height.saturating_sub(lines.len() as u16) / 2;
    let rect = Rect::new(area.x, top, area.width, lines.len() as u16);
    f.render_widget(
        Paragraph::new(lines)
            .alignment(Alignment::Center)
            .style(Style::default().bg(theme::BG)),
        rect,
    );
}

fn render_info(f: &mut Frame, area: Rect, app: &App) -> Option<Rect> {
    if area.width < 12 || area.height < 10 {
        return None;
    }

    let chunks = Layout::vertical([
        Constraint::Length(1),
        Constraint::Length(2),
        Constraint::Length(2),
        Constraint::Length(2),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Min(1),
    ])
    .split(area);

    let transport_line = if app.is_line_in {
        Line::from(vec![
            Span::styled("● ", Style::default().fg(theme::SUCCESS)),
            Span::styled("Line-In", theme::transport_style("PLAYING")),
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
        ])
    };
    f.render_widget(Paragraph::new(transport_line), chunks[0]);

    let title = app.current_track_title();
    f.render_widget(
        Paragraph::new(Line::from(Span::styled(title, theme::title_style()))),
        chunks[1],
    );
    let artist = app.current_track_artist();
    f.render_widget(
        Paragraph::new(Line::from(Span::styled(artist, theme::artist_style()))),
        chunks[2],
    );
    let album = app.current_track_album();
    f.render_widget(
        Paragraph::new(Line::from(Span::styled(album, theme::album_style()))),
        chunks[3],
    );

    let duration = app.effective_duration();
    let progress = if duration > 0 {
        app.elapsed as f64 / duration as f64
    } else {
        0.0
    };
    let progress_line = Line::from(vec![
        Span::styled(
            format_duration(app.elapsed),
            Style::default()
                .fg(theme::PRIMARY)
                .add_modifier(Modifier::BOLD),
        ),
        Span::styled(" / ", theme::dim_style()),
        Span::styled(format_duration(duration), theme::secondary_text()),
    ]);
    f.render_widget(Paragraph::new(progress_line), chunks[4]);
    render_progress_bar(f.buffer_mut(), chunks[5], progress, theme::PRIMARY);

    let volume_line = Line::from(vec![
        Span::styled("Volume ", theme::volume_style()),
        Span::styled(
            format!("{}%", app.volume),
            Style::default()
                .fg(theme::TEXT)
                .add_modifier(Modifier::BOLD),
        ),
    ]);
    f.render_widget(Paragraph::new(volume_line), chunks[6]);
    render_progress_bar(
        f.buffer_mut(),
        chunks[7],
        (app.volume as f64 / 100.0).clamp(0.0, 1.0),
        theme::CYAN,
    );

    let help = if area.width >= 72 {
        "space play/pause   </> prev/next   f/b seek   [] vol   tab spk   ? help"
    } else if area.width >= 48 {
        "spc pause   </> skip   f/b seek   [] vol   ?"
    } else {
        "spc  <>  f/b  []  ?"
    };
    f.render_widget(
        Paragraph::new(Line::from(Span::styled(help, theme::help_text()))),
        chunks[8],
    );

    Some(chunks[5])
}
