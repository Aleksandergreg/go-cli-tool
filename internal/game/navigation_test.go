package game

import (
	"strings"
	"testing"
)

func TestMissionNavigationFieldsParsesQuotedAndEscapedArguments(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{name: "double quoted campaign", line: `list --campaign "First Day"`, want: []string{"list", "--campaign", "First Day"}},
		{name: "single quoted campaign", line: `opsquest list --campaign 'The Logpocalypse'`, want: []string{"list", "--campaign", "The Logpocalypse"}},
		{name: "escaped whitespace", line: `play linux\ orientation`, want: []string{"play", "linux orientation"}},
		{name: "escaped double quote", line: `list --campaign "First \"Day\""`, want: []string{"list", "--campaign", `First "Day"`}},
		{name: "empty quoted argument", line: `list --campaign ''`, want: []string{"list", "--campaign", ""}},
		{name: "prefix alone", line: `opsquest`, want: []string{"missions"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, navigation, err := missionNavigationFields(test.line)
			if err != nil {
				t.Fatal(err)
			}
			if !navigation {
				t.Fatal("input was not recognized as mission navigation")
			}
			if strings.Join(got, "\x00") != strings.Join(test.want, "\x00") {
				t.Fatalf("fields = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestMissionNavigationFieldsReportsMalformedNavigationOnly(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr string
	}{
		{name: "unterminated double quote", line: `list --campaign "First Day`, wantErr: "unterminated double quote"},
		{name: "unterminated single quote", line: `opsquest play 'linux-orientation`, wantErr: "unterminated single quote"},
		{name: "malformed prefixed input", line: `opsquest "list`, wantErr: "unterminated double quote"},
		{name: "unfinished escape", line: `next\`, wantErr: "unfinished escape"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, navigation, err := missionNavigationFields(test.line)
			if !navigation {
				t.Fatal("malformed navigation was not recognized")
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want text %q", err, test.wantErr)
			}
		})
	}

	fields, navigation, err := missionNavigationFields(`echo "environment handles this`)
	if err != nil || navigation || fields != nil {
		t.Fatalf("non-navigation parse result = %#v, %v, %v", fields, navigation, err)
	}
}
