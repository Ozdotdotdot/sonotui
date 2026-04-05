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
    let art_inner = Block::default()
        .borders(Borders::ALL)
        .inner(layout.art_panel);
    centered_art_rect(art_inner)
}

struct NowPlayingLayout {
    art_panel: Rect,
    info_panel: Rect,
}

pub fn render(f: &mut Frame, area: Rect, app: &App, placeholder_only: bool) {
    let layout = compute_layout(area);

    let art_block = Block::default()
        .borders(Borders::ALL)
        .border_style(Style::default().fg(theme::PURPLE))
        .title(" Album Art ");
    let info_block = Block::default()
        .borders(Borders::ALL)
        .border_style(Style::default().fg(theme::DARK_GRAY))
        .title(" Now Playing ");

    let art_inner = art_block.inner(layout.art_panel);
    let info_inner = info_block.inner(layout.info_panel);
    f.render_widget(art_block, layout.art_panel);
    f.render_widget(info_block, layout.info_panel);

    if placeholder_only || app.art_image_data.is_none() {
        render_placeholder(
            f,
            centered_art_rect(art_inner),
            app.art_image_data.is_some(),
        );
    }

    render_info(f, info_inner, app);
}

fn compute_layout(area: Rect) -> NowPlayingLayout {
    if area.width >= 80 {
        let chunks = Layout::horizontal([
            Constraint::Percentage(46),
            Constraint::Length(2),
            Constraint::Percentage(54),
        ])
        .split(area);
        NowPlayingLayout {
            art_panel: chunks[0],
            info_panel: chunks[2],
        }
    } else {
        let chunks = Layout::vertical([
            Constraint::Percentage(46),
            Constraint::Length(1),
            Constraint::Percentage(54),
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
                "album art is drawn directly",
                theme::dim_style(),
            )),
            Line::from(Span::styled(
                "outside ratatui's text buffer",
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
    f.render_widget(Paragraph::new(lines).alignment(Alignment::Center), rect);
}

fn render_info(f: &mut Frame, area: Rect, app: &App) {
    if area.width < 8 || area.height < 8 {
        return;
    }

    let chunks = Layout::vertical([
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(2),
        Constraint::Length(2),
        Constraint::Length(2),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Min(1),
    ])
    .split(area);

    let transport_line = if app.is_line_in {
        Line::from(Span::styled(
            "LIVE  line-in",
            Style::default()
                .fg(theme::transport_color("PLAYING"))
                .add_modifier(Modifier::BOLD),
        ))
    } else {
        Line::from(Span::styled(
            app.transport_label(),
            theme::transport_style(&app.transport),
        ))
    };
    f.render_widget(Paragraph::new(transport_line), chunks[0]);

    if let Some(speaker) = app.speaker.as_ref() {
        f.render_widget(
            Paragraph::new(Line::from(vec![
                Span::styled("Speaker ", theme::dim_style()),
                Span::styled(&speaker.name, Style::default().fg(theme::WHITE)),
            ])),
            chunks[1],
        );
    }

    let title = if app.track.title.is_empty() {
        "No track"
    } else {
        &app.track.title
    };
    f.render_widget(
        Paragraph::new(Line::from(Span::styled(title, theme::title_style()))),
        chunks[2],
    );
    f.render_widget(
        Paragraph::new(Line::from(Span::styled(
            if app.track.artist.is_empty() {
                "Unknown artist"
            } else {
                &app.track.artist
            },
            theme::artist_style(),
        ))),
        chunks[3],
    );
    f.render_widget(
        Paragraph::new(Line::from(Span::styled(
            if app.track.album.is_empty() {
                "Unknown album"
            } else {
                &app.track.album
            },
            theme::album_style(),
        ))),
        chunks[4],
    );

    let progress = if app.duration > 0 {
        app.elapsed as f64 / app.duration as f64
    } else {
        0.0
    };
    let progress_line = Line::from(vec![
        Span::styled(
            format_duration(app.elapsed),
            Style::default().fg(theme::PURPLE),
        ),
        Span::styled(" / ", theme::dim_style()),
        Span::styled(format_duration(app.duration), theme::dim_style()),
    ]);
    f.render_widget(Paragraph::new(progress_line), chunks[5]);
    render_progress_bar(f.buffer_mut(), chunks[6], progress, theme::PURPLE);

    let volume_line = Line::from(vec![
        Span::styled("Volume ", Style::default().fg(theme::CYAN)),
        Span::styled(
            format!("{}%", app.volume),
            Style::default()
                .fg(theme::WHITE)
                .add_modifier(Modifier::BOLD),
        ),
    ]);
    f.render_widget(Paragraph::new(volume_line), chunks[7]);
    render_progress_bar(
        f.buffer_mut(),
        chunks[8],
        (app.volume as f64 / 100.0).clamp(0.0, 1.0),
        theme::CYAN,
    );

    let help = "space play/pause   </> prev/next   j/k volume   tab speaker   ? help";
    f.render_widget(
        Paragraph::new(Line::from(Span::styled(help, theme::dim_style()))),
        chunks[10],
    );
}

fn centered_art_rect(area: Rect) -> Rect {
    if area.width <= 2 || area.height <= 2 {
        return area;
    }
    let mut width = area.width.saturating_sub(2);
    let mut height = area.height.saturating_sub(2);
    let target_width = height.saturating_mul(2);
    if target_width < width {
        width = target_width.max(1);
    } else {
        height = (width / 2).max(1);
    }
    Rect::new(
        area.x + (area.width.saturating_sub(width)) / 2,
        area.y + (area.height.saturating_sub(height)) / 2,
        width.max(1),
        height.max(1),
    )
}
