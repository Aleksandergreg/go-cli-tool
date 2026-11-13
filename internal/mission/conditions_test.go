package mission

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMissionValidationRejectsFieldsOutsideConditionContract(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	item, _ := catalog.Find("linux-orientation")
	item.Validation.All[0].Path = "/not-used"
	if err := validateMission(item); err == nil || !strings.Contains(err.Error(), `type "output_equals" does not support path`) {
		t.Fatalf("validateMission() error = %v", err)
	}
}

func TestConditionValidationRejectsExplicitZeroValueFieldsOutsideContract(t *testing.T) {
	tests := []struct {
		data    string
		wantErr string
	}{
		{data: `{"type":"output_equals","pid":0}`, wantErr: `does not support pid`},
		{data: `{"type":"file_exists","value":""}`, wantErr: `does not support value`},
		{data: `{"type":"path_missing","values":[]}`, wantErr: `does not support values`},
	}
	for _, test := range tests {
		var condition Condition
		if err := json.Unmarshal([]byte(test.data), &condition); err != nil {
			t.Fatalf("Unmarshal(%s) error = %v", test.data, err)
		}
		if err := validateCondition(condition, EnvironmentSimulated); err == nil || !strings.Contains(err.Error(), test.wantErr) {
			t.Errorf("validateCondition(%s) error = %v, want substring %q", test.data, err, test.wantErr)
		}
	}
}

func TestConditionDecoderPreservesUnknownFieldRejection(t *testing.T) {
	var condition Condition
	err := json.Unmarshal([]byte(`{"type":"output_equals","surprise":0}`), &condition)
	if err == nil || !strings.Contains(err.Error(), `unknown field "surprise"`) {
		t.Fatalf("Unmarshal() error = %v", err)
	}
}
