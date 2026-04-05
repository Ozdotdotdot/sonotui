use std::io::Write;

use base64::Engine;
use crossterm::terminal;
use flate2::write::ZlibEncoder;
use flate2::Compression;
use ratatui::layout::Rect;

const DELIM: &str = "\u{10EEEE}";

#[rustfmt::skip]
const GRID: &[&str] = &[
    "\u{0305}","\u{030D}","\u{030E}","\u{0310}","\u{0312}",
    "\u{033D}","\u{033E}","\u{033F}","\u{0346}","\u{034A}",
    "\u{034B}","\u{034C}","\u{0350}","\u{0351}","\u{0352}",
    "\u{0357}","\u{035B}","\u{0363}","\u{0364}","\u{0365}",
    "\u{0366}","\u{0367}","\u{0368}","\u{0369}","\u{036A}",
    "\u{036B}","\u{036C}","\u{036D}","\u{036E}","\u{036F}",
    "\u{0483}","\u{0484}","\u{0485}","\u{0486}","\u{0487}",
    "\u{0592}","\u{0593}","\u{0594}","\u{0595}","\u{0597}",
    "\u{0598}","\u{0599}","\u{059C}","\u{059D}","\u{059E}",
    "\u{059F}","\u{05A0}","\u{05A1}","\u{05A8}","\u{05A9}",
    "\u{05AB}","\u{05AC}","\u{05AF}","\u{05C4}","\u{0610}",
    "\u{0611}","\u{0612}","\u{0613}","\u{0614}","\u{0615}",
    "\u{0616}","\u{0617}","\u{0657}","\u{0658}","\u{0659}",
    "\u{065A}","\u{065B}","\u{065D}","\u{065E}","\u{06D6}",
    "\u{06D7}","\u{06D8}","\u{06D9}","\u{06DA}","\u{06DB}",
    "\u{06DC}","\u{06DF}","\u{06E0}","\u{06E1}","\u{06E2}",
    "\u{06E4}","\u{06E7}","\u{06E8}","\u{06EB}","\u{06EC}",
    "\u{0730}","\u{0732}","\u{0733}","\u{0735}","\u{0736}",
    "\u{073A}","\u{073D}","\u{073F}","\u{0740}","\u{0741}",
    "\u{0743}","\u{0745}","\u{0747}","\u{0749}","\u{074A}",
    "\u{07EB}","\u{07EC}","\u{07ED}","\u{07EE}","\u{07EF}",
    "\u{07F0}","\u{07F1}","\u{07F3}","\u{0816}","\u{0817}",
    "\u{0818}","\u{0819}","\u{081B}","\u{081C}","\u{081D}",
    "\u{081E}","\u{081F}","\u{0820}","\u{0821}","\u{0822}",
    "\u{0823}","\u{0825}","\u{0826}","\u{0827}","\u{0829}",
    "\u{082A}","\u{082B}","\u{082C}","\u{082D}","\u{0951}",
    "\u{0953}","\u{0954}","\u{0F82}","\u{0F83}","\u{0F86}",
    "\u{0F87}","\u{135D}","\u{135E}","\u{135F}","\u{17DD}",
    "\u{193A}","\u{1A17}","\u{1A75}","\u{1A76}","\u{1A77}",
    "\u{1A78}","\u{1A79}","\u{1A7A}","\u{1A7B}","\u{1A7C}",
    "\u{1B6B}","\u{1B6D}","\u{1B6E}","\u{1B6F}","\u{1B70}",
    "\u{1B71}","\u{1B72}","\u{1B73}","\u{1CD0}","\u{1CD1}",
    "\u{1CD2}","\u{1CDA}","\u{1CDB}","\u{1CE0}","\u{1DC0}",
    "\u{1DC1}","\u{1DC3}","\u{1DC4}","\u{1DC5}","\u{1DC6}",
    "\u{1DC7}","\u{1DC8}","\u{1DC9}","\u{1DCB}","\u{1DCC}",
    "\u{1DD1}","\u{1DD2}","\u{1DD3}","\u{1DD4}","\u{1DD5}",
    "\u{1DD6}","\u{1DD7}","\u{1DD8}","\u{1DD9}","\u{1DDA}",
    "\u{1DDB}","\u{1DDC}","\u{1DDD}","\u{1DDE}","\u{1DDF}",
    "\u{1DE0}","\u{1DE1}","\u{1DE2}","\u{1DE3}","\u{1DE4}",
    "\u{1DE5}","\u{1DE6}","\u{1DFE}","\u{20D0}","\u{20D1}",
    "\u{20D4}","\u{20D5}","\u{20D6}","\u{20D7}","\u{20DB}",
    "\u{20DC}","\u{20E1}","\u{20E7}","\u{20E9}","\u{20F0}",
    "\u{2CEF}","\u{2CF0}","\u{2CF1}","\u{2DE0}","\u{2DE1}",
    "\u{2DE2}","\u{2DE3}","\u{2DE4}","\u{2DE5}","\u{2DE6}",
    "\u{2DE7}","\u{2DE8}","\u{2DE9}","\u{2DEA}","\u{2DEB}",
    "\u{2DEC}","\u{2DED}","\u{2DEE}","\u{2DEF}","\u{2DF0}",
    "\u{2DF1}","\u{2DF2}","\u{2DF3}","\u{2DF4}","\u{2DF5}",
    "\u{2DF6}","\u{2DF7}","\u{2DF8}","\u{2DF9}","\u{2DFA}",
    "\u{2DFB}","\u{2DFC}","\u{2DFD}","\u{2DFE}","\u{2DFF}",
    "\u{A66F}","\u{A67C}","\u{A67D}","\u{A6F0}","\u{A6F1}",
    "\u{A8E0}","\u{A8E1}","\u{A8E2}","\u{A8E3}","\u{A8E4}",
    "\u{A8E5}","\u{A8E6}","\u{A8E7}","\u{A8E8}","\u{A8E9}",
    "\u{A8EA}","\u{A8EB}","\u{A8EC}","\u{A8ED}","\u{A8EE}",
    "\u{A8EF}","\u{A8F0}","\u{A8F1}","\u{AAB0}","\u{AAB2}",
    "\u{AAB3}","\u{AAB7}","\u{AAB8}","\u{AABE}","\u{AABF}",
    "\u{AAC1}","\u{FE20}","\u{FE21}","\u{FE22}","\u{FE23}",
    "\u{FE24}","\u{FE25}","\u{FE26}","\u{10A0F}","\u{10A38}",
    "\u{1D185}","\u{1D186}","\u{1D187}","\u{1D188}","\u{1D189}",
    "\u{1D1AA}","\u{1D1AB}","\u{1D1AC}","\u{1D1AD}","\u{1D242}",
    "\u{1D243}","\u{1D244}",
];

#[derive(Clone, Debug)]
pub struct KittyImageData {
    pub b64_payload: String,
    pub img_width: u32,
    pub img_height: u32,
}

#[derive(Debug, Clone, Copy)]
pub struct AlignedArea {
    pub area: Rect,
    pub pixel_width: u32,
    pub pixel_height: u32,
}

pub fn encode_kitty_payload(rgba: &[u8], width: u32, height: u32) -> KittyImageData {
    let mut encoder = ZlibEncoder::new(Vec::new(), Compression::new(6));
    encoder.write_all(rgba).expect("zlib write");
    let compressed = encoder.finish().expect("zlib finish");
    let b64 = base64::engine::general_purpose::STANDARD.encode(&compressed);
    KittyImageData {
        b64_payload: b64,
        img_width: width,
        img_height: height,
    }
}

pub fn display_kitty_image(
    stdout: &mut impl Write,
    data: &KittyImageData,
    area: Rect,
) -> std::io::Result<()> {
    write!(stdout, "\x1b_Ga=d,d=A,q=2\x1b\\")?;

    let mut chars = data.b64_payload.chars().peekable();
    let first: String = chars.by_ref().take(4096).collect();
    let more = i32::from(chars.peek().is_some());

    write!(
        stdout,
        "\x1b_Gi=1,f=32,U=1,t=d,a=T,m={more},q=2,o=z,s={w},v={h};{first}\x1b\\",
        w = data.img_width,
        h = data.img_height,
    )?;

    while chars.peek().is_some() {
        let chunk: String = chars.by_ref().take(4096).collect();
        let m = i32::from(chars.peek().is_some());
        write!(stdout, "\x1b_Gm={m};{chunk}\x1b\\")?;
    }

    write_placeholder_grid(stdout, area)?;
    stdout.flush()?;
    Ok(())
}

pub fn hide_kitty_image(stdout: &mut impl Write, area: Rect) -> std::io::Result<()> {
    for y in 0..area.height {
        write!(
            stdout,
            "\x1b[{};{}H{}",
            area.y + y + 1,
            area.x + 1,
            " ".repeat(area.width as usize)
        )?;
    }
    write!(stdout, "\x1b_Ga=d,d=A,q=2\x1b\\")?;
    stdout.flush()?;
    Ok(())
}

fn write_placeholder_grid(w: &mut impl Write, area: Rect) -> std::io::Result<()> {
    for y in 0..area.height {
        write!(w, "\x1b[{};{}H", area.y + y + 1, area.x + 1)?;
        write!(w, "\x1b[38;5;1m")?;
        for x in 0..area.width {
            let row = GRID.get(y as usize).unwrap_or(&GRID[0]);
            let col = GRID.get(x as usize).unwrap_or(&GRID[0]);
            write!(w, "{DELIM}{row}{col}")?;
        }
        write!(w, "\x1b[39m")?;
    }
    Ok(())
}

pub fn art_placeholder() -> String {
    "╭──────────╮\n│    ♫     │\n╰──────────╯".to_string()
}

pub fn align_image_to_area(area: Rect, image_width: u32, image_height: u32) -> AlignedArea {
    if area.width == 0 || area.height == 0 || image_width == 0 || image_height == 0 {
        return AlignedArea {
            area,
            pixel_width: 0,
            pixel_height: 0,
        };
    }

    let (cell_width, cell_height) = terminal::window_size()
        .ok()
        .and_then(|size| {
            if size.columns == 0 || size.rows == 0 {
                None
            } else {
                Some((
                    size.width as f64 / size.columns as f64,
                    size.height as f64 / size.rows as f64,
                ))
            }
        })
        .unwrap_or((8.0, 16.0));

    let bounds_px_w = area.width as f64 * cell_width;
    let bounds_px_h = area.height as f64 * cell_height;
    let scale = (bounds_px_w / image_width as f64).min(bounds_px_h / image_height as f64);
    let used_px_w = (image_width as f64 * scale).round().max(1.0);
    let used_px_h = (image_height as f64 * scale).round().max(1.0);

    let used_cells_w = ((used_px_w / cell_width).ceil() as u16).clamp(1, area.width);
    let used_cells_h = ((used_px_h / cell_height).ceil() as u16).clamp(1, area.height);
    let x = area.x + area.width.saturating_sub(used_cells_w) / 2;
    let y = area.y + area.height.saturating_sub(used_cells_h) / 2;

    AlignedArea {
        area: Rect::new(x, y, used_cells_w, used_cells_h),
        pixel_width: used_px_w as u32,
        pixel_height: used_px_h as u32,
    }
}

pub fn resize_image_exact(
    rgba: &[u8],
    img_w: u32,
    img_h: u32,
    target_w: u32,
    target_h: u32,
) -> (Vec<u8>, u32, u32) {
    let new_w = target_w.max(1);
    let new_h = target_h.max(1);
    let mut out = vec![0u8; (new_w * new_h * 4) as usize];

    for y in 0..new_h {
        for x in 0..new_w {
            let src_x = (x * img_w / new_w).min(img_w.saturating_sub(1));
            let src_y = (y * img_h / new_h).min(img_h.saturating_sub(1));
            let si = ((src_y * img_w + src_x) * 4) as usize;
            let di = ((y * new_w + x) * 4) as usize;
            if si + 3 < rgba.len() && di + 3 < out.len() {
                out[di..di + 4].copy_from_slice(&rgba[si..si + 4]);
            }
        }
    }

    (out, new_w, new_h)
}

pub fn render_halfblock(rgba: &[u8], img_w: u32, img_h: u32, cols: u16, rows: u16) -> String {
    if cols == 0 || rows == 0 {
        return art_placeholder();
    }

    let target_h = (rows as u32) * 2;
    let scale_x = cols as f64 / img_w as f64;
    let scale_y = target_h as f64 / img_h as f64;
    let scale = scale_x.min(scale_y);

    let dst_w = ((img_w as f64 * scale).round() as u32).max(1);
    let mut dst_h = ((img_h as f64 * scale).round() as u32).max(1);
    if dst_h % 2 != 0 {
        dst_h += 1;
    }

    let pad_left = ((cols as u32).saturating_sub(dst_w) / 2) as usize;
    let mut buf = String::with_capacity((dst_w as usize + 40) * (dst_h as usize / 2));

    for y in (0..dst_h).step_by(2) {
        if pad_left > 0 {
            buf.push_str(&" ".repeat(pad_left));
        }
        for x in 0..dst_w {
            let (tr, tg, tb) = sample_pixel(rgba, img_w, img_h, x, y, dst_w, dst_h);
            let (br, bg, bb) = if y + 1 < dst_h {
                sample_pixel(rgba, img_w, img_h, x, y + 1, dst_w, dst_h)
            } else {
                (0, 0, 0)
            };
            buf.push_str(&format!("\x1b[38;2;{tr};{tg};{tb};48;2;{br};{bg};{bb}m▀"));
        }
        buf.push_str("\x1b[0m");
        if y + 2 < dst_h {
            buf.push('\n');
        }
    }
    buf
}

fn sample_pixel(
    rgba: &[u8],
    img_w: u32,
    img_h: u32,
    x: u32,
    y: u32,
    dst_w: u32,
    dst_h: u32,
) -> (u8, u8, u8) {
    let src_x = (x * img_w / dst_w).min(img_w.saturating_sub(1));
    let src_y = (y * img_h / dst_h).min(img_h.saturating_sub(1));
    let idx = ((src_y * img_w + src_x) * 4) as usize;
    if idx + 2 < rgba.len() {
        (rgba[idx], rgba[idx + 1], rgba[idx + 2])
    } else {
        (0, 0, 0)
    }
}
