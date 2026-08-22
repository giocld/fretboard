use guitarpro::audio::midi::CHANNEL_DEFAULT_NAMES;
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

/// --all envelope: one TrackOut per track of the file, plus song-level
/// title/artist so consumers can build a full Tab from the first track.
#[derive(Serialize)]
struct AllOut {
    title: String,
    artist: String,
    tracks: Vec<TrackOut>,
}

#[derive(Serialize)]
struct TrackOut {
    name: String,
    instrument: String,
    strings: usize,
    tuning: Vec<i32>,
    key: String,
    bars: Vec<BarOut>,
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
    // Accept flags in any position so both `gp-parser <file> --all` and
    // `gp-parser --all <file>` work.
    let mut args: Vec<String> = env::args().skip(1).collect();
    let mut all_tracks = false;
    args.retain(|a| match a.as_str() {
        "--all" => {
            all_tracks = true;
            false
        }
        "--version" => {
            println!("{}", env!("CARGO_PKG_VERSION"));
            std::process::exit(0);
        }
        _ => true,
    });

    let path = args
        .first()
        .expect("usage: gp-parser [--all] <file.gp[345x]>");
    let data = std::fs::read(path).expect("read file");
    let ext = Path::new(path)
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

    println!("{}", serialize(&song, all_tracks));
}

/// Serializes a parsed song: every track when `all_tracks` (--all envelope),
/// otherwise the single best guitar track (byte-compatible with the original
/// default output).
fn serialize(song: &Song, all_tracks: bool) -> String {
    if all_tracks {
        let tracks: Vec<TrackOut> = song
            .tracks
            .iter()
            .map(|t| track_to_track_out(song, t))
            .collect();
        serde_json::to_string(&AllOut {
            title: song.name.clone(),
            artist: song.artist.clone(),
            tracks,
        })
        .expect("json")
    } else {
        let track_idx = pick_guitar_track(song);
        let track = &song.tracks[track_idx];
        serde_json::to_string(&track_to_tab(song, track)).expect("json")
    }
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

/// MIDI program name for a track: GP6/7 files carry it on the track itself,
/// legacy GP3-5 files carry it on the track's channel.
fn instrument_name(song: &Song, track: &Track) -> String {
    let program = track
        .midi_program_gpif
        .or_else(|| song.channels.get(track.channel_index).map(|c| c.instrument));
    match program {
        Some(p) if p >= 0 => CHANNEL_DEFAULT_NAMES
            .get(p as usize)
            .copied()
            .unwrap_or("Unknown")
            .to_string(),
        _ => "Unknown".to_string(),
    }
}

/// Song key rendered as a display name (e.g. "C major", "A minor").
fn song_key(song: &Song) -> String {
    song.key.to_string()
}

fn track_to_track_out(song: &Song, track: &Track) -> TrackOut {
    let tab = track_to_tab(song, track);
    TrackOut {
        name: track.name.clone(),
        instrument: instrument_name(song, track),
        strings: track.strings.len(),
        tuning: tab.tuning,
        key: song_key(song),
        bars: tab.bars,
    }
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

#[cfg(test)]
mod tests {
    use super::*;
    use guitarpro::model::legacy::beat::{Beat, Voice};
    use guitarpro::model::legacy::enums::NoteType;
    use guitarpro::model::legacy::key_signature::{Duration, KeySignature};
    use guitarpro::model::legacy::measure::Measure;
    use guitarpro::model::legacy::note::Note;
    use guitarpro::model::legacy::track::{Track, TrackSettings};

    fn song_with_tracks() -> Song {
        let mut song = Song::default();
        song.name = "Test Song".into();
        song.artist = "Test Artist".into();
        song.tempo = 120;
        song.key = KeySignature { key: 0, is_minor: false };
        song.lyrics.lines = vec![(0, 0, String::new()); 5];

        let mut track1 = Track::default();
        track1.name = "Guitar 1".into();
        track1.strings = vec![(0, 40), (0, 45), (0, 50), (0, 55), (0, 59), (0, 64)];
        track1.settings = TrackSettings::default();
        track1.measures.push(measure_with_note());

        let mut track2 = Track::default();
        track2.name = "Bass".into();
        track2.strings = vec![(0, 28), (0, 33), (0, 38), (0, 43)];
        track2.settings = TrackSettings::default();
        track2.measures.push(measure_with_note());

        song.tracks.push(track1);
        song.tracks.push(track2);
        song
    }

    fn measure_with_note() -> Measure {
        let mut measure = Measure::default();
        measure.number = 1;
        let mut beat = Beat::default();
        beat.duration = Duration {
            value: 4,
            ..Default::default()
        };
        beat.notes.push(Note {
            value: 3,
            velocity: 100,
            string: 1,
            effect: Default::default(),
            duration_percent: 1.0,
            swap_accidentals: false,
            kind: NoteType::Normal,
            duration: None,
            tuplet: None,
        });
        let mut voice = Voice::default();
        voice.beats.push(beat);
        measure.voices.push(voice);
        measure
    }

    // --all must emit every track with name/instrument/strings/tuning/key/bars,
    // and the first track must serialize identically to the single-track path.
    #[test]
    fn all_tracks_emits_every_track() {
        let song = song_with_tracks();
        let all: serde_json::Value = serde_json::from_str(&serialize(&song, true)).unwrap();
        assert_eq!(all["title"], "Test Song");
        let tracks = all["tracks"].as_array().unwrap();
        assert_eq!(tracks.len(), 2);
        let t = &tracks[0];
        assert_eq!(t["name"], "Guitar 1");
        assert_eq!(t["strings"], 6);
        // track_to_tab emits tuning high-to-low (existing single-track order).
        assert_eq!(t["tuning"], serde_json::json!([64, 59, 55, 50, 45, 40]));
        assert_eq!(t["key"], "C major");
        assert!(!t["bars"].as_array().unwrap().is_empty());
        assert_eq!(tracks[1]["name"], "Bass");
        assert_eq!(tracks[1]["strings"], 4);
    }

    // Default output must remain the single best guitar track.
    #[test]
    fn default_picks_single_guitar_track() {
        let song = song_with_tracks();
        let tab: serde_json::Value = serde_json::from_str(&serialize(&song, false)).unwrap();
        assert_eq!(tab["title"], "Test Song");
        assert_eq!(tab["metadata"]["track"], "Guitar 1");
        // track_to_tab emits tuning high-to-low (existing single-track order).
        assert_eq!(tab["tuning"], serde_json::json!([64, 59, 55, 50, 45, 40]));
    }
}
