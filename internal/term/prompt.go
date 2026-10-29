package term

// PromptSeq counts OSC 133;A marks, which shells with shell integration send
// as each prompt starts; a move means the shell is ready for a command.
func (a *Applier) PromptSeq() uint64 { return a.promptSeq }

// isPromptStart matches "A" alone or followed by ";params" (e.g. "A;cl=m").
func isPromptStart(text []byte) bool {
	return len(text) > 0 && text[0] == 'A' && (len(text) == 1 || text[1] == ';')
}
