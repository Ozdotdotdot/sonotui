use ratatui::style::{Color, Modifier, Style};

// Transport state colors (matching Go TUI ANSI 256 palette)
pub fn transport_color(state: &str) -> Color {
    match state {
        "PLAYING" => Color::Indexed(82),          // Green
        "PAUSED_PLAYBACK" => Color::Indexed(226), // Yellow
        "STOPPED" => Color::Indexed(240),         // Gray
        "TRANSITIONING" => Color::Indexed(39),    // Cyan
        _ => Color::Indexed(240),
    }
}

pub fn transport_style(state: &str) -> Style {
    Style::default()
        .fg(transport_color(state))
        .add_modifier(Modifier::BOLD)
}

// Component colors
pub const WHITE: Color = Color::Indexed(15);
pub const ORANGE: Color = Color::Indexed(214);
pub const GRAY: Color = Color::Indexed(240);
pub const DARK_GRAY: Color = Color::Indexed(238);
pub const CYAN: Color = Color::Indexed(39);
pub const PURPLE: Color = Color::Indexed(62);
pub const DIM: Color = Color::Indexed(240);

// Styles
pub fn title_style() -> Style {
    Style::default().fg(WHITE).add_modifier(Modifier::BOLD)
}

pub fn artist_style() -> Style {
    Style::default().fg(ORANGE)
}

pub fn album_style() -> Style {
    Style::default().fg(GRAY).add_modifier(Modifier::ITALIC)
}

pub fn tab_active() -> Style {
    Style::default()
        .fg(WHITE)
        .add_modifier(Modifier::BOLD | Modifier::UNDERLINED)
}

pub fn tab_inactive() -> Style {
    Style::default().fg(GRAY)
}

pub fn cursor_style() -> Style {
    Style::default().fg(PURPLE).add_modifier(Modifier::BOLD)
}

pub fn search_style() -> Style {
    Style::default().fg(ORANGE)
}

pub fn status_style() -> Style {
    Style::default().fg(ORANGE)
}

pub fn dim_style() -> Style {
    Style::default().fg(DIM)
}

pub fn help_border() -> Style {
    Style::default().fg(PURPLE)
}
