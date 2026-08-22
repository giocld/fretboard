package msgs

// EditPersistMsg is emitted by the viewer after a quick-edit (E/$) session
// re-parses the edited tab text. The app router persists the new content
// (library ImportOverwrite or equivalent) and stamps the edited metadata.
type EditPersistMsg struct {
	TabID   int64
	Path    string // tab file path the edit was written to
	Content string // the re-parsed, edited raw text
	Title   string // title from the re-parsed tab
	Artist  string // artist from the re-parsed tab
}

// PracticeSessionMsg is emitted when a practice session (P-at-desk loop
// mode) ends: the app router records the practice rep via
// library.RecordPractice. The viewer has no store handle, so the record
// travels as a message.
type PracticeSessionMsg struct {
	TabID       int64
	DurationSec int64 // wall-clock session length
	TempoBPM    int   // tempo the session reached (final BPM)
	Loops       int   // completed loop repetitions
}
