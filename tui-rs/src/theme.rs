use ratatui::style::{Color, Modifier, Style};

pub const BG: Color = Color::Rgb(28, 31, 38);
pub const SURFACE: Color = Color::Rgb(35, 39, 48);
pub const SURFACE_HI: Color = Color::Rgb(58, 66, 82);
pub const TEXT: Color = Color::Rgb(236, 239, 244);
pub const TEXT_SOFT: Color = Color::Rgb(205, 211, 222);
pub const MUTED: Color = Color::Rgb(144, 152, 168);
pub const BORDER: Color = Color::Rgb(96, 109, 140);
pub const BORDER_STRONG: Color = Color::Rgb(158, 182, 214);
pub const PRIMARY: Color = Color::Rgb(133, 176, 255);
pub const WARM: Color = Color::Rgb(226, 232, 244);
pub const CYAN: Color = Color::Rgb(82, 208, 255);
pub const SUCCESS: Color = Color::Rgb(158, 230, 147);
pub const WARNING: Color = Color::Rgb(255, 219, 97);

pub fn transport_color(state: &str) -> Color {
    match state {
        "PLAYING" => SUCCESS,
        "PAUSED_PLAYBACK" => WARNING,
        "STOPPED" => TEXT_SOFT,
        "TRANSITIONING" => CYAN,
        _ => TEXT_SOFT,
    }
}

pub fn transport_style(state: &str) -> Style {
    Style::default()
        .fg(transport_color(state))
        .add_modifier(Modifier::BOLD)
}

pub fn shell_block() -> Style {
    Style::default().fg(BORDER_STRONG)
}

pub fn pane_border() -> Style {
    Style::default().fg(BORDER)
}

pub fn pane_border_focus() -> Style {
    Style::default().fg(BORDER_STRONG)
}

pub fn now_playing_border() -> Style {
    Style::default().fg(PRIMARY)
}

pub fn title_style() -> Style {
    Style::default().fg(TEXT).add_modifier(Modifier::BOLD)
}

pub fn artist_style() -> Style {
    Style::default().fg(TEXT).add_modifier(Modifier::BOLD)
}

pub fn album_style() -> Style {
    Style::default()
        .fg(TEXT_SOFT)
        .add_modifier(Modifier::ITALIC)
}

pub fn tab_active() -> Style {
    Style::default()
        .fg(BG)
        .bg(PRIMARY)
        .add_modifier(Modifier::BOLD)
}

pub fn tab_inactive() -> Style {
    Style::default().fg(TEXT_SOFT)
}

pub fn selected_row() -> Style {
    Style::default()
        .fg(TEXT)
        .bg(SURFACE_HI)
        .add_modifier(Modifier::BOLD)
}

pub fn dim_style() -> Style {
    Style::default().fg(MUTED)
}

pub fn secondary_text() -> Style {
    Style::default().fg(TEXT_SOFT)
}

pub fn search_style() -> Style {
    Style::default().fg(WARM).add_modifier(Modifier::BOLD)
}

pub fn volume_style() -> Style {
    Style::default().fg(CYAN).add_modifier(Modifier::BOLD)
}

pub fn help_border() -> Style {
    Style::default().fg(PRIMARY)
}

pub fn help_text() -> Style {
    Style::default().fg(MUTED)
}
