use std::io::Stdout;

use crossterm::event::{self, Event, KeyCode, KeyEventKind};
use ratatui::{
    backend::CrosstermBackend,
    layout::{Alignment, Constraint, Layout},
    style::Style,
    text::{Line, Span},
    widgets::{Block, BorderType, Borders, List, ListItem, ListState, Paragraph},
    Terminal,
};

use crate::{discover::DaemonInfo, theme};

pub enum PickerResult {
    Selected(String, u16),
    Quit,
}

/// Show an interactive picker. Only called when 2+ daemons were found.
/// The terminal is already in alternate-screen/raw mode (TerminalGuard owns it).
pub fn run(
    terminal: &mut Terminal<CrosstermBackend<Stdout>>,
    daemons: &[DaemonInfo],
) -> PickerResult {
    let mut cursor = 0usize;

    loop {
        terminal
            .draw(|f| {
                let area = f.area();

                // Outer background
                f.render_widget(
                    Block::default().style(Style::default().bg(theme::BG)),
                    area,
                );

                // Centre a box: width 52, height = items + 6
                let box_h = (daemons.len() as u16 + 6).min(area.height);
                let box_w = 54u16.min(area.width);
                let vert = Layout::vertical([
                    Constraint::Fill(1),
                    Constraint::Length(box_h),
                    Constraint::Fill(1),
                ])
                .split(area);
                let horiz = Layout::horizontal([
                    Constraint::Fill(1),
                    Constraint::Length(box_w),
                    Constraint::Fill(1),
                ])
                .split(vert[1]);
                let inner = horiz[1];

                let block = Block::default()
                    .title(" sonotui ")
                    .title_alignment(Alignment::Center)
                    .borders(Borders::ALL)
                    .border_type(BorderType::Rounded)
                    .border_style(Style::default().fg(theme::PRIMARY))
                    .style(Style::default().bg(theme::SURFACE));
                let inner_area = block.inner(inner);
                f.render_widget(block, inner);

                // Split inner: title row, list, hint row
                let chunks = Layout::vertical([
                    Constraint::Length(1),
                    Constraint::Fill(1),
                    Constraint::Length(1),
                ])
                .split(inner_area);

                // Title
                f.render_widget(
                    Paragraph::new(Line::from(vec![Span::styled(
                        "Multiple daemons found — select one",
                        Style::default().fg(theme::TEXT_SOFT),
                    )]))
                    .alignment(Alignment::Center),
                    chunks[0],
                );

                // List
                let items: Vec<ListItem> = daemons
                    .iter()
                    .enumerate()
                    .map(|(i, d)| {
                        let style = if i == cursor {
                            theme::selected_row()
                        } else {
                            Style::default()
                                .fg(theme::TEXT)
                                .bg(theme::SURFACE)
                        };
                        ListItem::new(Line::from(vec![
                            Span::styled(format!("  {:<24}", d.name), style),
                            Span::styled(
                                format!("{}:{}", d.host, d.port),
                                style.fg(theme::MUTED),
                            ),
                        ]))
                    })
                    .collect();

                let mut list_state = ListState::default();
                list_state.select(Some(cursor));
                f.render_stateful_widget(
                    List::new(items).highlight_style(theme::selected_row()),
                    chunks[1],
                    &mut list_state,
                );

                // Hint
                f.render_widget(
                    Paragraph::new(Line::from(vec![Span::styled(
                        "↑↓/jk  select    Enter  connect    q  quit",
                        Style::default().fg(theme::MUTED),
                    )]))
                    .alignment(Alignment::Center),
                    chunks[2],
                );
            })
            .ok();

        if let Ok(Event::Key(key)) = event::read() {
            if key.kind != KeyEventKind::Press {
                continue;
            }
            match key.code {
                KeyCode::Up | KeyCode::Char('k') => {
                    cursor = cursor.saturating_sub(1);
                }
                KeyCode::Down | KeyCode::Char('j') => {
                    cursor = (cursor + 1).min(daemons.len() - 1);
                }
                KeyCode::Enter => {
                    let d = &daemons[cursor];
                    return PickerResult::Selected(d.host.clone(), d.port);
                }
                KeyCode::Char('q') | KeyCode::Esc => return PickerResult::Quit,
                _ => {}
            }
        }
    }
}
