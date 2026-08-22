//go:build !noytdlp

package player

// OnlineAudioEnabled reports whether the online-audio feature (yt-dlp
// search, ranking, download) is compiled in. The feature is disabled with
// -tags noytdlp, which drops audio_online.go from the build.
const OnlineAudioEnabled = true
