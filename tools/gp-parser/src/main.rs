use guitarpro::model::legacy::enums::NoteType;
use guitarpro::model::legacy::song::Song;
use guitarpro::model::legacy::track::Track;
use serde::Serialize;
use std::collections::HashMap;
use std::env;
use std::path::Path;

const TICKS_PER_QUARTER: i32 = 480;

#[derive(Serialize)]
struct TabOut {
    title: String,
    artist: String,
    tuning: Vec<i32>,
    bars: Vec<BarOut>,
    metadata: HashMap<String, String>,
}

#[derive(Serialize)]
struct BarOut {
    number: i32,
    strings: Vec<StringLineOut>,
    column_ticks: Vec<i32>,
}

#[derive(Serialize)]
struct StringLineOut {
    segments: Vec<SegmentOut>,
}

#[derive(Serialize)]
struct SegmentOut {
    #[serde(rename = "char")]
    ch: String,
    value: i32,
    position: i32,
    width: i32,
}

fn main() {
    let path = env::args().nth(1).expect("usage: gp-parser <file.gp[345x]>");
    let data = std::fs::read(&path).expect("read file");
    let ext = Path::new(&path)
        .extension()
        .and_then(|e| e.to_str())
        .unwrap_or("")
        .to_ascii_lowercase();

    let mut song = Song::default();
    match ext.as_str() {
        "gp3" => song.read_gp3(&data).expect("parse gp3"),
        "gp4" => song.read_gp4(&data).expect("parse gp4"),
        "gp5" => song.read_gp5(&data).expect("parse gp5"),
        "gpx" => song.read_gpx(&data).expect("parse gpx"),
        "gp" => song.read_gp(&data).expect("parse gp"),
        _ => song.read_gp5(&data).expect("parse gp5 fallback"),
    }

    let track_idx = pick_guitar_track(&song);
    let track = &song.tracks[track_idx];
    let tab = track_to_tab(&song, track);
    println!("{}", serde_json::to_string(&tab).expect("json"));
}

fn pick_guitar_track(song: &Song) -> usize {
    for (i, t) in song.tracks.iter().enumerate() {
        if t.percussion_track || t.strings.is_empty() {
            continue;
        }
        let name = t.name.to_ascii_lowercase();
        if name.contains("guitar") || name.contains("gtr") {
            return i;
        }
    }
    for (i, t) in song.tracks.iter().enumerate() {
        if !t.percussion_track && t.strings.len() >= 4 {
            return i;
        }
    }
    0
}

fn track_to_tab(song: &Song, track: &Track) -> TabOut {
    let mut tuning: Vec<i32> = track
        .strings
        .iter()
        .map(|(_, midi)| *midi as i32)
        .collect();
    tuning.reverse();

    let mut bars = Vec::new();
    for (i, measure) in track.measures.iter().enumerate() {
        let mut col_ticks: Vec<i32> = Vec::new();
        let string_count = track.strings.len();
        let mut grids: Vec<Vec<char>> = vec![Vec::new(); string_count];
        let mut col = 0usize;

        for voice in &measure.voices {
            for beat in &voice.beats {
                let ticks = duration_ticks(&beat.duration);
                let units = (ticks / (TICKS_PER_QUARTER / 4)).max(1) as usize;
                while col_ticks.len() <= col {
                    col_ticks.push(0);
                }
                col_ticks[col] = ticks;

                for note in &beat.notes {
                    if matches!(note.kind, NoteType::Rest) || note.string <= 0 {
                        continue;
                    }
                    let s = (note.string as usize).saturating_sub(1);
                    if s >= string_count {
                        continue;
                    }
                    let fret = note.value.max(0) as i32;
                    let text = fret.to_string();
                    ensure_width(&mut grids[s], col + text.len());
                    for (j, ch) in text.chars().enumerate() {
                        if col + j < grids[s].len() {
                            grids[s][col + j] = ch;
                        }
                    }
                }
                col += units;
            }
        }

        let strings: Vec<StringLineOut> = grids
            .iter()
            .rev()
            .map(|grid| string_line_from_grid(grid))
            .collect();

        bars.push(BarOut {
            number: (i + 1) as i32,
            strings,
            column_ticks: col_ticks,
        });
    }

    let mut metadata = HashMap::new();
    metadata.insert("source".into(), "guitar-pro".into());
    metadata.insert("track".into(), track.name.clone());
    if song.tempo > 0 {
        metadata.insert("tempo".into(), song.tempo.to_string());
    }

    TabOut {
        title: if song.name.is_empty() {
            track.name.clone()
        } else {
            song.name.clone()
        },
        artist: song.artist.clone(),
        tuning,
        bars,
        metadata,
    }
}

fn duration_ticks(d: &guitarpro::model::legacy::key_signature::Duration) -> i32 {
    let mut ticks = TICKS_PER_QUARTER * 4 / d.value.max(1) as i32;
    if d.dotted {
        ticks = ticks * 3 / 2;
    }
    if d.double_dotted {
        ticks = ticks * 7 / 4;
    }
    if d.tuplet_enters > 0 && d.tuplet_times > 0 {
        ticks = ticks * d.tuplet_times as i32 / d.tuplet_enters as i32;
    }
    ticks.max(1)
}

fn ensure_width(grid: &mut Vec<char>, width: usize) {
    while grid.len() < width {
        grid.push('-');
    }
}

fn string_line_from_grid(grid: &[char]) -> StringLineOut {
    let mut segments = Vec::new();
    let mut pos = 0i32;
    let mut i = 0usize;
    while i < grid.len() {
        let ch = grid[i];
        if ch >= '0' && ch <= '9' {
            let start = i;
            i += 1;
            while i < grid.len() && grid[i] >= '0' && grid[i] <= '9' {
                i += 1;
            }
            let num: i32 = grid[start..i].iter().collect::<String>().parse().unwrap_or(0);
            let width = (i - start) as i32;
            segments.push(SegmentOut {
                ch: grid[start].to_string(),
                value: num,
                position: pos,
                width,
            });
            pos += width;
        } else {
            if ch != '-' {
                segments.push(SegmentOut {
                    ch: ch.to_string(),
                    value: 0,
                    position: pos,
                    width: 1,
                });
            }
            pos += 1;
            i += 1;
        }
    }
    StringLineOut { segments }
}
