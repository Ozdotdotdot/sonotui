use ratatui::style::{Color, Modifier, Style};

pub const BG: Color = Color::Rgb(28, 31, 38);
pub const SURFACE: Color = Color::Rgb(35, 39, 48);
pub const SURFACE_ALT: Color = Color::Rgb(43, 48, 58);
pub const SURFACE_HI: Color = Color::Rgb(58, 66, 82);
pub const TEXT: Color = Color::Rgb(236, 239, 244);
pub const TEXT_SOFT: Color = Color::Rgb(205, 211, 222);
pub const MUTED: Color = Color::Rgb(144, 152, 168);
pub const BORDER: Color = Color::Rgb(96, 109, 140);
pub const BORDER_STRONG: Color = Color::Rgb(158, 182, 214);
pub const PRIMARY: Color = Color::Rgb(133, 176, 255);
pub const PRIMARY_DEEP: Color = Color::Rgb(77, 113, 201);
pub const WARM: Color = Color::Rgb(255, 191, 92);
pub const CYAN: Color = Color::Rgb(82, 208, 255);
pub const SUCCESS: Color = Color::Rgb(158, 230, 147);
pub const WARNING: Color = Color::Rgb(255, 219, 97);
pub const DANGER: Color = Color::Rgb(255, 123, 123);
pub const PAUSED_BG: Color = Color::Rgb(86, 63, 13);
pub const PLAYING_BG: Color = Color::Rgb(39, 78, 57);
pub const STOPPED_BG: Color = Color::Rgb(76, 80, 91);

pub fn transport_color(state: &str) -> Color {
    match state {
        "PLAYING" => SUCCESS,
        "PAUSED_PLAYBACK" => WARNING,
        "STOPPED" => TEXT_SOFT,
        "TRANSITIONING" => CYAN,
        _ => TEXT_SOFT,
    }
}

pub fn transport_badge_style(state: &str) -> Style {
    let bg = match state {
        "PLAYING" => PLAYING_BG,
        "PAUSED_PLAYBACK" => PAUSED_BG,
        "STOPPED" => STOPPED_BG,
        "TRANSITIONING" => PRIMARY_DEEP,
        _ => SURFACE_HI,
    };
    Style::default()
        .fg(TEXT)
        .bg(bg)
        .add_modifier(Modifier::BOLD)
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

pub fn art_border() -> Style {
    Style::default().fg(WARM)
}

pub fn title_style() -> Style {
    Style::default().fg(TEXT).add_modifier(Modifier::BOLD)
}

pub fn title_emphasis() -> Style {
    Style::default().fg(TEXT).add_modifier(Modifier::BOLD)
}

pub fn artist_style() -> Style {
    Style::default().fg(WARM).add_modifier(Modifier::BOLD)
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

pub fn tab_row_bg() -> Style {
    Style::default().bg(SURFACE).fg(TEXT_SOFT)
}

pub fn cursor_style() -> Style {
    Style::default()
        .fg(BG)
        .bg(PRIMARY)
        .add_modifier(Modifier::BOLD)
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

pub fn status_style() -> Style {
    Style::default().fg(WARM).add_modifier(Modifier::BOLD)
}

pub fn volume_style() -> Style {
    Style::default().fg(CYAN).add_modifier(Modifier::BOLD)
}

pub fn help_border() -> Style {
    Style::default().fg(PRIMARY)
}

pub fn help_text() -> Style {
    Style::default().fg(TEXT_SOFT)
}
