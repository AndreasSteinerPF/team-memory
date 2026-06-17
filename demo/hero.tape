# demo/hero.tape — README hero GIF for TeamMemory.
#
# Renders demo/hero.gif: a real Claude Code session getting blocked by an
# active TeamMemory `requirement` that another agent established earlier.
#
# Re-record with:  vhs demo/hero.tape
# Requires:        vhs, claude (Claude Code CLI), tm.
# Cost per take:   roughly $0.25 of Claude API.

Output demo/hero.gif

Set Theme "Dracula"
Set FontSize 18
Set Width 1280
Set Height 720
Set Padding 30
Set TypingSpeed 35ms

# Off-camera: seed the demo repo with an active `requirement` memory scoped
# to billing/migrations/**, on a fresh `feature/revenue-reporting` branch so
# the on-camera agent is genuinely on a different branch + session than the
# one that proposed the lesson.
Hide
Type "bash demo/hero-setup.sh > /dev/null 2>&1 && cd ~/.tm-hero-repo && clear"
Enter
Sleep 2s
Show

# Context — explain the cross-agent story in three typed comment lines.
Type "# Earlier, an agent on feature/invoice-state proposed a lesson:"
Enter
Type "#   'Billing migrations require downgrade-path tests.'"
Enter
Type "# A teammate confirmed it on a different branch — TeamMemory activated it"
Enter
Type "# as a requirement. Now a fresh agent (different session, different branch)"
Enter
Type "# tries to edit a billing migration without knowing the rule."
Enter
Sleep 1500ms
Enter

# The on-camera moment: a real Claude Code session.
Type `claude --allowedTools "Edit,Read,Write,Bash" -p "Add a 'state' column to billing/migrations/2026_06_create_invoices.sql"`
Sleep 500ms
Enter

# Claude reads the file, tries to Edit, the PreToolUse hook fires and denies,
# Claude reports back. Wall-clock for a -p run is roughly 25-40s; give it
# enough buffer that the response always finishes inside the recording.
Sleep 45s

# Hold the final frame so a viewer can read the response.
Sleep 3s
