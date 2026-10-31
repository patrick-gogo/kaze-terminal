package claudesetup

import "testing"

func TestMalformedSubagentHookListsCannotBeFixed(t *testing.T) {
	for _, event := range []string{"SubagentStart", "SubagentStop", "PreToolUse"} {
		for _, value := range []string{`{}`, `null`, `"invalid"`, `true`} {
			t.Run(event+"/"+value, func(t *testing.T) {
				data := []byte(`{"hooks":{"` + event + `":` + value + `}}`)
				report := Inspect(data, func(string) bool { return true })
				defer func() {
					if p := recover(); p != nil {
						t.Fatalf("Fix panicked: %v", p)
					}
				}()
				if _, _, err := Fix(data, report, "/demo/kaze"); err == nil {
					t.Fatal("Fix accepted an invalid hook list")
				}
				for _, entry := range []Entry{report.AgentStart, report.AgentStop, report.Tools} {
					if entry.State != Unreadable {
						t.Fatalf("invalid hook source reported as %+v", entry)
					}
				}
			})
		}
	}
}
