# Agent instructions (fretboard)

## Required: end every implementation turn with "What changed"

After **any** code change, bug fix, or feature work, the final message **must**
include a section titled exactly:

## What changed

That section is **not optional**. Do not end the turn with only "try it" steps,
build output, or a short recap — include the full structured summary below.

### Template

```markdown
## What changed

### <short headline for the work>
- bullet list of concrete changes (files, behavior, keys, config)
- one line per fix or feature; name files when helpful

### Verified
- `go build …` / `go test …` results (or what you could not run)

### Try it
- rebuild command
- 2–4 steps to confirm the fix in the TUI
```

### Rules

1. **Always** use the `## What changed` heading (user searches for this).
2. Cover **what** changed, **why** (if non-obvious), and **how to verify**.
3. If the user only asked for a summary of prior work, still use this heading.
4. Never skip the summary because the turn was "small" or "just a fix".
5. If work was blocked or incomplete, say so under **What changed** with **Next steps**.
